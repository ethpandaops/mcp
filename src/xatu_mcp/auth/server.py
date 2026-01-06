"""OAuth 2.1 authorization server endpoints."""

from datetime import datetime, UTC
from typing import Any
from urllib.parse import urlencode, urlparse, parse_qs

import structlog
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, RedirectResponse, HTMLResponse
from starlette.routing import Route

from xatu_mcp.config import AuthConfig
from xatu_mcp.auth.models import (
    AuthorizationCode,
    AuthorizationRequest,
    InMemoryStore,
    PKCEChallenge,
    Session,
    TokenPair,
    TokenRequest,
    User,
)
from xatu_mcp.auth.tokens import (
    TokenManager,
    TokenError,
    TokenExpiredError,
    TokenInvalidError,
    TokenAudienceError,
)
from xatu_mcp.auth.github import (
    GitHubOAuthClient,
    GitHubOAuthError,
    generate_state,
    validate_redirect_uri,
)
from xatu_mcp.auth.discovery import (
    create_protected_resource_metadata,
    create_authorization_server_metadata,
    format_www_authenticate,
)

logger = structlog.get_logger()


class AuthorizationServer:
    """OAuth 2.1 authorization server implementation."""

    def __init__(
        self,
        config: AuthConfig,
        base_url: str,
    ) -> None:
        """Initialize the authorization server.

        Args:
            config: Authentication configuration.
            base_url: Canonical base URL of the server.
        """
        self._config = config
        self._base_url = base_url.rstrip("/")

        # Initialize components
        self._store = InMemoryStore()
        self._token_manager = TokenManager(config.tokens)

        if config.github:
            self._github = GitHubOAuthClient(config.github)
        else:
            self._github = None

        # Pre-compute metadata
        self._resource_metadata = create_protected_resource_metadata(base_url)
        self._server_metadata = create_authorization_server_metadata(base_url)

        logger.info(
            "Authorization server initialized",
            base_url=base_url,
            github_enabled=self._github is not None,
            allowed_orgs=config.allowed_orgs,
        )

    @property
    def store(self) -> InMemoryStore:
        """Get the in-memory store."""
        return self._store

    @property
    def token_manager(self) -> TokenManager:
        """Get the token manager."""
        return self._token_manager

    def get_resource_metadata_url(self) -> str:
        """Get the URL for protected resource metadata."""
        return f"{self._base_url}/.well-known/oauth-protected-resource"

    # -------------------------------------------------------------------------
    # Well-Known Endpoints
    # -------------------------------------------------------------------------

    async def handle_protected_resource_metadata(
        self, request: Request
    ) -> JSONResponse:
        """Handle /.well-known/oauth-protected-resource endpoint (RFC 9728)."""
        return JSONResponse(
            self._resource_metadata.to_dict(),
            headers={"Cache-Control": "max-age=3600"},
        )

    async def handle_authorization_server_metadata(
        self, request: Request
    ) -> JSONResponse:
        """Handle /.well-known/oauth-authorization-server endpoint (RFC 8414)."""
        return JSONResponse(
            self._server_metadata.to_dict(),
            headers={"Cache-Control": "max-age=3600"},
        )

    async def handle_openid_configuration(
        self, request: Request
    ) -> JSONResponse:
        """Handle /.well-known/openid-configuration endpoint.

        For compatibility with clients that use OIDC discovery.
        """
        return JSONResponse(
            self._server_metadata.to_dict(),
            headers={"Cache-Control": "max-age=3600"},
        )

    # -------------------------------------------------------------------------
    # Authorization Endpoints
    # -------------------------------------------------------------------------

    async def handle_authorize(self, request: Request) -> RedirectResponse | JSONResponse:
        """Handle OAuth 2.1 authorization endpoint.

        This initiates the authorization flow by validating the request
        and redirecting to GitHub for authentication.
        """
        if not self._github:
            return JSONResponse(
                {"error": "server_error", "error_description": "GitHub OAuth not configured"},
                status_code=500,
            )

        # Parse authorization request
        params = dict(request.query_params)

        auth_request = AuthorizationRequest(
            client_id=params.get("client_id", ""),
            redirect_uri=params.get("redirect_uri", ""),
            response_type=params.get("response_type", ""),
            scope=params.get("scope", ""),
            state=params.get("state", ""),
            code_challenge=params.get("code_challenge", ""),
            code_challenge_method=params.get("code_challenge_method", ""),
            resource=params.get("resource", ""),
        )

        # Validate request
        errors = auth_request.validate()
        if errors:
            return JSONResponse(
                {
                    "error": "invalid_request",
                    "error_description": "; ".join(errors),
                },
                status_code=400,
            )

        # Validate redirect URI
        if not validate_redirect_uri(auth_request.redirect_uri):
            return JSONResponse(
                {
                    "error": "invalid_request",
                    "error_description": "Invalid redirect_uri",
                },
                status_code=400,
            )

        # Generate state for GitHub OAuth
        github_state = generate_state()

        # Store pending authorization
        self._store.save_pending_authorization(
            github_state,
            {
                "client_id": auth_request.client_id,
                "redirect_uri": auth_request.redirect_uri,
                "scope": auth_request.scope,
                "state": auth_request.state,
                "code_challenge": auth_request.code_challenge,
                "code_challenge_method": auth_request.code_challenge_method,
                "resource": auth_request.resource,
                "created_at": datetime.now(UTC).isoformat(),
            },
        )

        # Redirect to GitHub
        github_callback_uri = f"{self._base_url}/auth/github/callback"
        github_url = self._github.get_authorization_url(
            redirect_uri=github_callback_uri,
            state=github_state,
            scope="read:user read:org",
        )

        logger.info(
            "Starting authorization flow",
            client_id=auth_request.client_id,
            scope=auth_request.scope,
        )

        return RedirectResponse(url=github_url, status_code=302)

    async def handle_github_callback(
        self, request: Request
    ) -> RedirectResponse | HTMLResponse:
        """Handle GitHub OAuth callback.

        This is called after the user authenticates with GitHub.
        """
        if not self._github:
            return HTMLResponse(
                "<h1>Error</h1><p>GitHub OAuth not configured</p>",
                status_code=500,
            )

        # Get callback parameters
        code = request.query_params.get("code")
        state = request.query_params.get("state")
        error = request.query_params.get("error")
        error_description = request.query_params.get("error_description", "")

        if error:
            logger.warning("GitHub OAuth error", error=error, description=error_description)
            return HTMLResponse(
                f"<h1>Authentication Failed</h1><p>{error}: {error_description}</p>",
                status_code=400,
            )

        if not code or not state:
            return HTMLResponse(
                "<h1>Error</h1><p>Missing code or state parameter</p>",
                status_code=400,
            )

        # Retrieve pending authorization
        pending = self._store.get_pending_authorization(state)
        if not pending:
            logger.warning("Invalid state in callback", state=state)
            return HTMLResponse(
                "<h1>Error</h1><p>Invalid or expired state</p>",
                status_code=400,
            )

        # Clean up pending authorization
        self._store.delete_pending_authorization(state)

        try:
            # Exchange code for GitHub token
            github_callback_uri = f"{self._base_url}/auth/github/callback"
            github_token = await self._github.exchange_code(code, github_callback_uri)

            # Get GitHub user profile
            github_user = await self._github.get_user(github_token.access_token)

            # Validate org membership
            if self._config.allowed_orgs:
                is_member = github_user.is_member_of(self._config.allowed_orgs)
                if not is_member:
                    logger.warning(
                        "User not in allowed organizations",
                        github_login=github_user.login,
                        user_orgs=github_user.organizations,
                        allowed_orgs=self._config.allowed_orgs,
                    )
                    # Don't reveal user orgs or allowed orgs to prevent information disclosure
                    return HTMLResponse(
                        "<h1>Access Denied</h1>"
                        "<p>You are not authorized to access this resource.</p>"
                        "<p>Please contact your administrator if you believe this is an error.</p>",
                        status_code=403,
                    )

            # Get or create user
            user = self._store.get_user_by_github_id(github_user.id)
            if user:
                # Update user info
                user.name = github_user.name
                user.email = github_user.email
                user.avatar_url = github_user.avatar_url
                user.organizations = github_user.organizations
                user.updated_at = datetime.now(UTC)
            else:
                user = User.from_github_user(github_user)
                self._store.save_user(user)

            # Create authorization code for the original client
            pkce = PKCEChallenge(
                code_challenge=pending["code_challenge"],
                code_challenge_method=pending["code_challenge_method"],
            )

            auth_code = AuthorizationCode.create(
                client_id=pending["client_id"],
                redirect_uri=pending["redirect_uri"],
                scope=pending["scope"],
                resource=pending["resource"],
                user_id=user.id,
                pkce=pkce,
                state=pending["state"],
            )

            self._store.save_authorization_code(auth_code)

            logger.info(
                "Authorization successful",
                github_login=github_user.login,
                user_id=user.id,
                client_id=pending["client_id"],
            )

            # Redirect back to client with authorization code
            redirect_params = {
                "code": auth_code.code,
            }
            if pending["state"]:
                redirect_params["state"] = pending["state"]

            redirect_url = f"{pending['redirect_uri']}?{urlencode(redirect_params)}"
            return RedirectResponse(url=redirect_url, status_code=302)

        except GitHubOAuthError as e:
            logger.error("GitHub OAuth failed", error=str(e))
            return HTMLResponse(
                f"<h1>Authentication Failed</h1><p>{str(e)}</p>",
                status_code=400,
            )

    async def handle_token(self, request: Request) -> JSONResponse:
        """Handle OAuth 2.1 token endpoint.

        Supports authorization_code and refresh_token grant types.
        """
        # Parse form data
        form = await request.form()
        grant_type = form.get("grant_type", "")

        if grant_type == "authorization_code":
            return await self._handle_authorization_code_grant(form)
        elif grant_type == "refresh_token":
            return await self._handle_refresh_token_grant(form)
        else:
            return JSONResponse(
                {
                    "error": "unsupported_grant_type",
                    "error_description": f"Grant type '{grant_type}' is not supported",
                },
                status_code=400,
            )

    async def _handle_authorization_code_grant(
        self, form: Any
    ) -> JSONResponse:
        """Handle authorization_code grant type."""
        token_request = TokenRequest(
            grant_type="authorization_code",
            code=form.get("code"),
            redirect_uri=form.get("redirect_uri"),
            client_id=form.get("client_id"),
            code_verifier=form.get("code_verifier"),
            resource=form.get("resource"),
        )

        # Validate request
        errors = token_request.validate_authorization_code()
        if errors:
            return JSONResponse(
                {
                    "error": "invalid_request",
                    "error_description": "; ".join(errors),
                },
                status_code=400,
            )

        # Get authorization code
        auth_code = self._store.get_authorization_code(token_request.code)
        if not auth_code:
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": "Invalid authorization code",
                },
                status_code=400,
            )

        # Validate authorization code
        if not auth_code.is_valid():
            self._store.delete_authorization_code(token_request.code)
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": "Authorization code expired or already used",
                },
                status_code=400,
            )

        # Validate client_id
        if auth_code.client_id != token_request.client_id:
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": "Client ID mismatch",
                },
                status_code=400,
            )

        # Validate redirect_uri
        if auth_code.redirect_uri != token_request.redirect_uri:
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": "Redirect URI mismatch",
                },
                status_code=400,
            )

        # Validate resource (RFC 8707)
        if auth_code.resource != token_request.resource:
            return JSONResponse(
                {
                    "error": "invalid_target",
                    "error_description": "Resource mismatch",
                },
                status_code=400,
            )

        # Verify PKCE
        if not auth_code.pkce.verify(token_request.code_verifier):
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": "Invalid code_verifier (PKCE)",
                },
                status_code=400,
            )

        # Mark code as used
        self._store.mark_authorization_code_used(token_request.code)

        # Create tokens
        access_token, access_jti, refresh_token, refresh_jti = (
            self._token_manager.create_token_pair(
                user_id=auth_code.user_id,
                client_id=auth_code.client_id,
                scope=auth_code.scope,
                resource=auth_code.resource,
            )
        )

        # Create session
        session = Session.create(
            user_id=auth_code.user_id,
            access_token_jti=access_jti,
            refresh_token_jti=refresh_jti,
            client_id=auth_code.client_id,
            scope=auth_code.scope,
            resource=auth_code.resource,
        )
        self._store.save_session(session)

        logger.info(
            "Tokens issued",
            user_id=auth_code.user_id,
            client_id=auth_code.client_id,
            scope=auth_code.scope,
        )

        return JSONResponse({
            "access_token": access_token,
            "token_type": "Bearer",
            "expires_in": self._token_manager.get_access_token_ttl(),
            "refresh_token": refresh_token,
            "scope": auth_code.scope,
        })

    async def _handle_refresh_token_grant(self, form: Any) -> JSONResponse:
        """Handle refresh_token grant type."""
        token_request = TokenRequest(
            grant_type="refresh_token",
            refresh_token=form.get("refresh_token"),
        )

        # Validate request
        errors = token_request.validate_refresh_token()
        if errors:
            return JSONResponse(
                {
                    "error": "invalid_request",
                    "error_description": "; ".join(errors),
                },
                status_code=400,
            )

        try:
            # Validate refresh token (audience validation mandatory per RFC 8707)
            claims = self._token_manager.validate_token(
                token_request.refresh_token,
                expected_audience=self._base_url,
                expected_type="refresh",
            )

            # Get session
            session = self._store.get_session_by_refresh_jti(claims.jti)
            if not session or not session.is_valid():
                return JSONResponse(
                    {
                        "error": "invalid_grant",
                        "error_description": "Invalid or revoked refresh token",
                    },
                    status_code=400,
                )

            # Get user
            user = self._store.get_user(session.user_id)
            if not user:
                return JSONResponse(
                    {
                        "error": "invalid_grant",
                        "error_description": "User not found",
                    },
                    status_code=400,
                )

            # Re-validate org membership if configured
            if self._config.allowed_orgs:
                is_member = any(
                    org in self._config.allowed_orgs for org in user.organizations
                )
                if not is_member:
                    # Revoke session
                    self._store.revoke_session(session.id)
                    return JSONResponse(
                        {
                            "error": "invalid_grant",
                            "error_description": "User is no longer a member of allowed organizations",
                        },
                        status_code=400,
                    )

            # Create new tokens (rotate refresh token)
            access_token, access_jti, refresh_token, refresh_jti = (
                self._token_manager.create_token_pair(
                    user_id=user.id,
                    client_id=session.client_id,
                    scope=session.scope,
                    resource=session.resource,
                )
            )

            # Update session with new token JTIs
            self._store.update_session_tokens(session.id, access_jti, refresh_jti)

            logger.info(
                "Tokens refreshed",
                user_id=user.id,
                client_id=session.client_id,
            )

            return JSONResponse({
                "access_token": access_token,
                "token_type": "Bearer",
                "expires_in": self._token_manager.get_access_token_ttl(),
                "refresh_token": refresh_token,
                "scope": session.scope,
            })

        except TokenExpiredError:
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": "Refresh token has expired",
                },
                status_code=400,
            )

        except TokenError as e:
            return JSONResponse(
                {
                    "error": "invalid_grant",
                    "error_description": str(e),
                },
                status_code=400,
            )

    async def handle_revoke(self, request: Request) -> JSONResponse:
        """Handle token revocation endpoint."""
        form = await request.form()
        token = form.get("token")
        token_type_hint = form.get("token_type_hint", "access_token")

        if not token:
            return JSONResponse(
                {
                    "error": "invalid_request",
                    "error_description": "Token is required",
                },
                status_code=400,
            )

        try:
            # Try to decode the token to get its JTI
            claims = self._token_manager.decode_token_unsafe(token)
            jti = claims.get("jti")
            token_type = claims.get("token_type", "access")

            if jti:
                # Find and revoke the session
                if token_type == "refresh":
                    session = self._store.get_session_by_refresh_jti(jti)
                else:
                    session = self._store.get_session_by_access_jti(jti)

                if session:
                    self._store.revoke_session(session.id)
                    logger.info("Session revoked", session_id=session.id)

        except Exception:
            # Per RFC 7009, always return 200 for revocation
            pass

        # Always return 200 per RFC 7009
        return JSONResponse({})

    async def handle_userinfo(self, request: Request) -> JSONResponse:
        """Handle userinfo endpoint (returns current user info)."""
        # Get token from Authorization header
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            return JSONResponse(
                {"error": "invalid_token"},
                status_code=401,
                headers={
                    "WWW-Authenticate": format_www_authenticate(
                        self.get_resource_metadata_url(),
                        error="invalid_token",
                        error_description="Missing or invalid Authorization header",
                    ),
                },
            )

        token = auth_header[7:]  # Remove "Bearer " prefix

        try:
            claims = self._token_manager.validate_token(
                token,
                expected_audience=self._base_url,
                expected_type="access",
            )

            user = self._store.get_user(claims.sub)
            if not user:
                return JSONResponse(
                    {"error": "invalid_token"},
                    status_code=401,
                )

            return JSONResponse({
                "sub": user.id,
                "name": user.name,
                "preferred_username": user.github_login,
                "email": user.email,
                "picture": user.avatar_url,
                "organizations": user.organizations,
            })

        except TokenError as e:
            return JSONResponse(
                {"error": "invalid_token", "error_description": str(e)},
                status_code=401,
                headers={
                    "WWW-Authenticate": format_www_authenticate(
                        self.get_resource_metadata_url(),
                        error="invalid_token",
                        error_description=str(e),
                    ),
                },
            )

    # -------------------------------------------------------------------------
    # Login Page (for browser-based flow)
    # -------------------------------------------------------------------------

    async def handle_login(self, request: Request) -> HTMLResponse:
        """Handle login page for browser-based authentication.

        This page uses JavaScript to generate proper PKCE parameters as required
        by OAuth 2.1. The code_verifier is stored in sessionStorage for use
        in the callback page when exchanging the authorization code for tokens.
        """
        html = """
        <!DOCTYPE html>
        <html>
        <head>
            <title>Xatu MCP - Login</title>
            <style>
                body {
                    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
                    display: flex;
                    justify-content: center;
                    align-items: center;
                    height: 100vh;
                    margin: 0;
                    background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
                }
                .container {
                    background: white;
                    padding: 40px;
                    border-radius: 10px;
                    box-shadow: 0 10px 40px rgba(0,0,0,0.2);
                    text-align: center;
                    max-width: 400px;
                }
                h1 {
                    margin-bottom: 10px;
                    color: #333;
                }
                p {
                    color: #666;
                    margin-bottom: 30px;
                }
                .github-btn {
                    display: inline-flex;
                    align-items: center;
                    padding: 12px 24px;
                    background: #24292e;
                    color: white;
                    text-decoration: none;
                    border-radius: 6px;
                    font-weight: 500;
                    transition: background 0.2s;
                    cursor: pointer;
                    border: none;
                    font-size: 16px;
                }
                .github-btn:hover {
                    background: #2f363d;
                }
                .github-btn svg {
                    margin-right: 10px;
                }
                .note {
                    margin-top: 20px;
                    font-size: 12px;
                    color: #999;
                }
            </style>
        </head>
        <body>
            <div class="container">
                <h1>Xatu MCP</h1>
                <p>Sign in to access Ethereum network analytics</p>
                <button id="login-btn" class="github-btn">
                    <svg height="20" width="20" viewBox="0 0 16 16" fill="currentColor">
                        <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path>
                    </svg>
                    Sign in with GitHub
                </button>
                <p class="note">
                    Access requires membership in an authorized GitHub organization.
                </p>
            </div>
            <script>
                // PKCE implementation for OAuth 2.1
                // Generate cryptographically random code_verifier and derive code_challenge

                function generateCodeVerifier() {
                    // Generate 32 random bytes (256 bits) for the code verifier
                    const array = new Uint8Array(32);
                    crypto.getRandomValues(array);
                    // Base64url encode (RFC 4648)
                    return base64UrlEncode(array);
                }

                function base64UrlEncode(buffer) {
                    const base64 = btoa(String.fromCharCode.apply(null, buffer));
                    return base64.replace(/\\+/g, '-').replace(/\\//g, '_').replace(/=+$/, '');
                }

                async function generateCodeChallenge(codeVerifier) {
                    // SHA-256 hash the code verifier
                    const encoder = new TextEncoder();
                    const data = encoder.encode(codeVerifier);
                    const digest = await crypto.subtle.digest('SHA-256', data);
                    // Base64url encode the hash
                    return base64UrlEncode(new Uint8Array(digest));
                }

                async function startLogin() {
                    // Generate PKCE values
                    const codeVerifier = generateCodeVerifier();
                    const codeChallenge = await generateCodeChallenge(codeVerifier);

                    // Generate state for CSRF protection
                    const stateArray = new Uint8Array(16);
                    crypto.getRandomValues(stateArray);
                    const state = base64UrlEncode(stateArray);

                    // Store code_verifier and state in sessionStorage for the callback page
                    sessionStorage.setItem('pkce_code_verifier', codeVerifier);
                    sessionStorage.setItem('oauth_state', state);

                    // Build authorization URL with proper PKCE parameters
                    const params = new URLSearchParams({
                        response_type: 'code',
                        client_id: 'browser',
                        redirect_uri: window.location.origin + '/auth/callback-page',
                        scope: 'execute_python get_output_file',
                        code_challenge: codeChallenge,
                        code_challenge_method: 'S256',
                        state: state,
                        resource: '{base_url}'
                    });

                    // Redirect to authorization endpoint
                    window.location.href = '/auth/authorize?' + params.toString();
                }

                document.getElementById('login-btn').addEventListener('click', startLogin);
            </script>
        </body>
        </html>
        """.replace("{base_url}", self._base_url)

        return HTMLResponse(html)

    def get_routes(self) -> list[Route]:
        """Get Starlette routes for the authorization server."""
        return [
            # Well-known endpoints
            Route(
                "/.well-known/oauth-protected-resource",
                self.handle_protected_resource_metadata,
                methods=["GET"],
            ),
            Route(
                "/.well-known/oauth-authorization-server",
                self.handle_authorization_server_metadata,
                methods=["GET"],
            ),
            Route(
                "/.well-known/openid-configuration",
                self.handle_openid_configuration,
                methods=["GET"],
            ),
            # OAuth endpoints
            Route("/auth/authorize", self.handle_authorize, methods=["GET"]),
            Route("/auth/github/callback", self.handle_github_callback, methods=["GET"]),
            Route("/auth/token", self.handle_token, methods=["POST"]),
            Route("/auth/revoke", self.handle_revoke, methods=["POST"]),
            Route("/auth/userinfo", self.handle_userinfo, methods=["GET"]),
            # Login page
            Route("/auth/login", self.handle_login, methods=["GET"]),
        ]
