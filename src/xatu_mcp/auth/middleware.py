"""Authentication middleware for protecting MCP endpoints."""

from dataclasses import dataclass
from typing import Callable, Awaitable, Any

import structlog
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response, JSONResponse

from xatu_mcp.config import AuthConfig
from xatu_mcp.auth.models import InMemoryStore, User, Session
from xatu_mcp.auth.tokens import (
    TokenManager,
    TokenClaims,
    TokenError,
    TokenExpiredError,
    TokenInvalidError,
    TokenAudienceError,
)
from xatu_mcp.auth.discovery import format_www_authenticate

logger = structlog.get_logger()


@dataclass
class AuthenticatedUser:
    """Authenticated user context attached to requests."""

    user: User
    session: Session
    claims: TokenClaims
    scopes: list[str]

    def has_scope(self, scope: str) -> bool:
        """Check if user has the specified scope."""
        return scope in self.scopes


class AuthenticationMiddleware(BaseHTTPMiddleware):
    """Middleware for authenticating requests via Bearer tokens."""

    # Paths that don't require authentication
    PUBLIC_PATHS = frozenset([
        "/",
        "/health",
        "/ready",
        "/.well-known/oauth-protected-resource",
        "/.well-known/oauth-authorization-server",
        "/.well-known/openid-configuration",
        "/auth/authorize",
        "/auth/github/callback",
        "/auth/token",
        "/auth/revoke",
        "/auth/login",
    ])

    # Path prefixes that don't require authentication
    PUBLIC_PREFIXES = (
        "/auth/",
        "/.well-known/",
    )

    def __init__(
        self,
        app: Any,
        config: AuthConfig,
        token_manager: TokenManager,
        store: InMemoryStore,
        base_url: str,
    ) -> None:
        """Initialize the authentication middleware.

        Args:
            app: The ASGI application.
            config: Authentication configuration.
            token_manager: Token manager for validating tokens.
            store: In-memory store for sessions and users.
            base_url: Base URL of the server (for audience validation).
        """
        super().__init__(app)
        self._config = config
        self._token_manager = token_manager
        self._store = store
        self._base_url = base_url.rstrip("/")
        self._resource_metadata_url = f"{self._base_url}/.well-known/oauth-protected-resource"

    def _is_public_path(self, path: str) -> bool:
        """Check if the path is public (doesn't require auth)."""
        if path in self.PUBLIC_PATHS:
            return True
        return any(path.startswith(prefix) for prefix in self.PUBLIC_PREFIXES)

    async def dispatch(
        self,
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        """Process the request, validating authentication if required.

        Args:
            request: The incoming request.
            call_next: The next middleware/handler to call.

        Returns:
            The response from downstream handlers or an error response.
        """
        # Skip auth for public paths
        if self._is_public_path(request.url.path):
            return await call_next(request)

        # Skip auth if disabled
        if not self._config.enabled:
            return await call_next(request)

        # Get Authorization header
        auth_header = request.headers.get("Authorization", "")

        if not auth_header:
            return self._unauthorized_response(
                "Missing Authorization header",
            )

        if not auth_header.startswith("Bearer "):
            return self._unauthorized_response(
                "Authorization header must use Bearer scheme",
            )

        token = auth_header[7:]  # Remove "Bearer " prefix

        if not token:
            return self._unauthorized_response(
                "Empty Bearer token",
            )

        # Validate token
        try:
            claims = self._token_manager.validate_token(
                token,
                expected_audience=self._base_url,
                expected_type="access",
            )
        except TokenExpiredError:
            return self._unauthorized_response(
                "Token has expired",
                error="invalid_token",
            )
        except TokenAudienceError as e:
            return self._unauthorized_response(
                str(e),
                error="invalid_token",
            )
        except TokenInvalidError as e:
            return self._unauthorized_response(
                str(e),
                error="invalid_token",
            )
        except TokenError as e:
            return self._unauthorized_response(
                str(e),
                error="invalid_token",
            )

        # Validate session
        session = self._store.get_session_by_access_jti(claims.jti)
        if not session:
            return self._unauthorized_response(
                "Session not found",
                error="invalid_token",
            )

        if not session.is_valid():
            return self._unauthorized_response(
                "Session has been revoked or expired",
                error="invalid_token",
            )

        # Get user
        user = self._store.get_user(claims.sub)
        if not user:
            return self._unauthorized_response(
                "User not found",
                error="invalid_token",
            )

        # Create authenticated user context
        scopes = claims.scope.split() if claims.scope else []
        auth_user = AuthenticatedUser(
            user=user,
            session=session,
            claims=claims,
            scopes=scopes,
        )

        # Attach to request state
        request.state.auth_user = auth_user

        logger.debug(
            "Request authenticated",
            user_id=user.id,
            github_login=user.github_login,
            scopes=scopes,
            path=request.url.path,
        )

        return await call_next(request)

    def _unauthorized_response(
        self,
        description: str,
        error: str = "invalid_token",
        status_code: int = 401,
    ) -> JSONResponse:
        """Create an unauthorized response with proper WWW-Authenticate header.

        Args:
            description: Human-readable error description.
            error: OAuth error code.
            status_code: HTTP status code.

        Returns:
            JSON error response with WWW-Authenticate header.
        """
        logger.warning(
            "Authentication failed",
            error=error,
            description=description,
        )

        return JSONResponse(
            {
                "error": error,
                "error_description": description,
            },
            status_code=status_code,
            headers={
                "WWW-Authenticate": format_www_authenticate(
                    self._resource_metadata_url,
                    error=error,
                    error_description=description,
                ),
            },
        )


def require_scope(scope: str) -> Callable:
    """Decorator to require a specific scope for a handler.

    Usage:
        @require_scope("execute_python")
        async def my_handler(request: Request) -> Response:
            ...

    Args:
        scope: The required scope.

    Returns:
        Decorator function.
    """
    def decorator(
        func: Callable[[Request], Awaitable[Response]]
    ) -> Callable[[Request], Awaitable[Response]]:
        async def wrapper(request: Request) -> Response:
            auth_user = getattr(request.state, "auth_user", None)

            if not auth_user:
                return JSONResponse(
                    {
                        "error": "unauthorized",
                        "error_description": "Authentication required",
                    },
                    status_code=401,
                )

            if not auth_user.has_scope(scope):
                return JSONResponse(
                    {
                        "error": "insufficient_scope",
                        "error_description": f"Required scope: {scope}",
                    },
                    status_code=403,
                    headers={
                        "WWW-Authenticate": f'Bearer error="insufficient_scope", scope="{scope}"',
                    },
                )

            return await func(request)

        return wrapper
    return decorator


def get_current_user(request: Request) -> AuthenticatedUser | None:
    """Get the current authenticated user from a request.

    Args:
        request: The Starlette request.

    Returns:
        The authenticated user, or None if not authenticated.
    """
    return getattr(request.state, "auth_user", None)


def require_authenticated(
    func: Callable[[Request], Awaitable[Response]]
) -> Callable[[Request], Awaitable[Response]]:
    """Decorator to require authentication for a handler.

    Usage:
        @require_authenticated
        async def my_handler(request: Request) -> Response:
            user = get_current_user(request)
            ...

    Args:
        func: The handler function.

    Returns:
        Wrapped handler that checks authentication.
    """
    async def wrapper(request: Request) -> Response:
        auth_user = getattr(request.state, "auth_user", None)

        if not auth_user:
            return JSONResponse(
                {
                    "error": "unauthorized",
                    "error_description": "Authentication required",
                },
                status_code=401,
            )

        return await func(request)

    return wrapper
