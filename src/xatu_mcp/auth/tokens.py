"""JWT token creation and validation."""

from dataclasses import dataclass
from datetime import datetime, timedelta, UTC
from typing import Any
import secrets
import uuid

import jwt
import structlog

from xatu_mcp.config import AuthTokensConfig

logger = structlog.get_logger()


class TokenError(Exception):
    """Base exception for token errors."""

    pass


class TokenExpiredError(TokenError):
    """Token has expired."""

    pass


class TokenInvalidError(TokenError):
    """Token is invalid."""

    pass


class TokenAudienceError(TokenError):
    """Token audience does not match."""

    pass


@dataclass
class TokenClaims:
    """Decoded token claims."""

    jti: str  # JWT ID
    sub: str  # Subject (user ID)
    aud: str  # Audience (resource)
    iss: str  # Issuer
    iat: datetime  # Issued at
    exp: datetime  # Expiration
    scope: str  # Scopes
    client_id: str  # Client ID
    token_type: str  # "access" or "refresh"

    @classmethod
    def from_dict(cls, claims: dict[str, Any]) -> "TokenClaims":
        """Create TokenClaims from JWT claims dictionary."""
        return cls(
            jti=claims["jti"],
            sub=claims["sub"],
            aud=claims["aud"],
            iss=claims["iss"],
            iat=datetime.fromtimestamp(claims["iat"], tz=UTC),
            exp=datetime.fromtimestamp(claims["exp"], tz=UTC),
            scope=claims.get("scope", ""),
            client_id=claims.get("client_id", ""),
            token_type=claims.get("token_type", "access"),
        )


class TokenManager:
    """Manages JWT token creation and validation."""

    def __init__(self, config: AuthTokensConfig) -> None:
        """Initialize token manager.

        Args:
            config: Token configuration including TTLs and issuer.

        Raises:
            ValueError: If secret_key is not configured (required for security).
        """
        self._config = config
        self._algorithm = "HS256"

        if not config.secret_key:
            raise ValueError(
                "Authentication secret_key is required. "
                "Generate a secure key with: python -c \"import secrets; print(secrets.token_urlsafe(32))\" "
                "and set it in your config.yaml under auth.tokens.secret_key"
            )

        self._secret_key = config.secret_key

        logger.info("Token manager initialized", issuer=config.issuer)

    def create_access_token(
        self,
        user_id: str,
        client_id: str,
        scope: str,
        resource: str,
    ) -> tuple[str, str]:
        """Create an access token.

        Args:
            user_id: The user's ID.
            client_id: The OAuth client ID.
            scope: Space-separated scopes.
            resource: The resource (audience) this token is bound to (RFC 8707).

        Returns:
            Tuple of (token string, JTI).
        """
        jti = str(uuid.uuid4())
        now = datetime.now(UTC)
        exp = now + timedelta(seconds=self._config.access_token_ttl)

        claims = {
            "jti": jti,
            "sub": user_id,
            "aud": resource,
            "iss": self._config.issuer,
            "iat": int(now.timestamp()),
            "exp": int(exp.timestamp()),
            "scope": scope,
            "client_id": client_id,
            "token_type": "access",
        }

        token = jwt.encode(claims, self._secret_key, algorithm=self._algorithm)

        logger.debug(
            "Created access token",
            jti=jti,
            user_id=user_id,
            client_id=client_id,
            scope=scope,
            resource=resource,
            expires_in=self._config.access_token_ttl,
        )

        return token, jti

    def create_refresh_token(
        self,
        user_id: str,
        client_id: str,
        scope: str,
        resource: str,
    ) -> tuple[str, str]:
        """Create a refresh token.

        Args:
            user_id: The user's ID.
            client_id: The OAuth client ID.
            scope: Space-separated scopes.
            resource: The resource (audience) this token is bound to (RFC 8707).

        Returns:
            Tuple of (token string, JTI).
        """
        jti = str(uuid.uuid4())
        now = datetime.now(UTC)
        exp = now + timedelta(seconds=self._config.refresh_token_ttl)

        claims = {
            "jti": jti,
            "sub": user_id,
            "aud": resource,
            "iss": self._config.issuer,
            "iat": int(now.timestamp()),
            "exp": int(exp.timestamp()),
            "scope": scope,
            "client_id": client_id,
            "token_type": "refresh",
        }

        token = jwt.encode(claims, self._secret_key, algorithm=self._algorithm)

        logger.debug(
            "Created refresh token",
            jti=jti,
            user_id=user_id,
            client_id=client_id,
            expires_in=self._config.refresh_token_ttl,
        )

        return token, jti

    def create_token_pair(
        self,
        user_id: str,
        client_id: str,
        scope: str,
        resource: str,
    ) -> tuple[str, str, str, str]:
        """Create both access and refresh tokens.

        Args:
            user_id: The user's ID.
            client_id: The OAuth client ID.
            scope: Space-separated scopes.
            resource: The resource (audience) these tokens are bound to (RFC 8707).

        Returns:
            Tuple of (access_token, access_jti, refresh_token, refresh_jti).
        """
        access_token, access_jti = self.create_access_token(
            user_id, client_id, scope, resource
        )
        refresh_token, refresh_jti = self.create_refresh_token(
            user_id, client_id, scope, resource
        )

        return access_token, access_jti, refresh_token, refresh_jti

    def validate_token(
        self,
        token: str,
        expected_audience: str,
        expected_type: str = "access",
    ) -> TokenClaims:
        """Validate a token and return its claims.

        Args:
            token: The JWT token string.
            expected_audience: Expected audience (resource). REQUIRED per RFC 8707
                for resource-bound token validation.
            expected_type: Expected token type ("access" or "refresh").

        Returns:
            Decoded token claims.

        Raises:
            TokenExpiredError: If the token has expired.
            TokenInvalidError: If the token is invalid.
            TokenAudienceError: If the audience doesn't match.
            ValueError: If expected_audience is not provided.
        """
        # Audience validation is mandatory per RFC 8707
        if not expected_audience:
            raise ValueError(
                "expected_audience is required for token validation (RFC 8707)"
            )

        try:
            # Decode without audience verification first to get claims
            claims = jwt.decode(
                token,
                self._secret_key,
                algorithms=[self._algorithm],
                issuer=self._config.issuer,
                options={
                    "require": ["jti", "sub", "aud", "iss", "iat", "exp"],
                    "verify_aud": False,  # We'll verify manually for better error messages
                },
            )

            # Verify token type
            token_type = claims.get("token_type", "access")
            if token_type != expected_type:
                raise TokenInvalidError(
                    f"Expected {expected_type} token, got {token_type}"
                )

            # Verify audience (RFC 8707 - critical for security)
            token_audience = claims.get("aud")
            if token_audience != expected_audience:
                logger.warning(
                    "Token audience mismatch",
                    expected=expected_audience,
                    actual=token_audience,
                )
                raise TokenAudienceError(
                    f"Token audience '{token_audience}' does not match "
                    f"expected audience '{expected_audience}'"
                )

            return TokenClaims.from_dict(claims)

        except jwt.ExpiredSignatureError as e:
            raise TokenExpiredError("Token has expired") from e

        except jwt.InvalidIssuerError as e:
            raise TokenInvalidError("Invalid token issuer") from e

        except jwt.DecodeError as e:
            raise TokenInvalidError("Invalid token format") from e

        except jwt.InvalidTokenError as e:
            raise TokenInvalidError(f"Invalid token: {e}") from e

    def decode_token_unsafe(self, token: str) -> dict[str, Any]:
        """Decode a token without verification (for debugging only).

        WARNING: This does not verify the token signature or expiration.
        Only use for debugging/logging purposes.

        Args:
            token: The JWT token string.

        Returns:
            Decoded claims dictionary.
        """
        return jwt.decode(
            token,
            options={"verify_signature": False},
        )

    def get_access_token_ttl(self) -> int:
        """Get the access token TTL in seconds."""
        return self._config.access_token_ttl

    def get_refresh_token_ttl(self) -> int:
        """Get the refresh token TTL in seconds."""
        return self._config.refresh_token_ttl
