"""Data models for authentication and sessions."""

from dataclasses import dataclass, field
from datetime import datetime, UTC
from enum import Enum
from typing import Any
import secrets
import uuid


class TokenType(Enum):
    """Token types for OAuth 2.1."""

    ACCESS = "access"
    REFRESH = "refresh"


@dataclass
class GitHubUser:
    """GitHub user profile information."""

    id: int
    login: str
    name: str | None
    email: str | None
    avatar_url: str | None
    organizations: list[str] = field(default_factory=list)

    def is_member_of(self, allowed_orgs: list[str]) -> bool:
        """Check if user is a member of any allowed organization."""
        if not allowed_orgs:
            return True
        return any(org in allowed_orgs for org in self.organizations)


@dataclass
class User:
    """Authenticated user."""

    id: str
    github_id: int
    github_login: str
    name: str | None
    email: str | None
    avatar_url: str | None
    organizations: list[str]
    created_at: datetime
    updated_at: datetime

    @classmethod
    def from_github_user(cls, github_user: GitHubUser) -> "User":
        """Create a User from GitHub user profile."""
        now = datetime.now(UTC)
        return cls(
            id=str(uuid.uuid4()),
            github_id=github_user.id,
            github_login=github_user.login,
            name=github_user.name,
            email=github_user.email,
            avatar_url=github_user.avatar_url,
            organizations=github_user.organizations,
            created_at=now,
            updated_at=now,
        )


@dataclass
class PKCEChallenge:
    """PKCE challenge data for authorization flow."""

    code_challenge: str
    code_challenge_method: str = "S256"

    def verify(self, code_verifier: str) -> bool:
        """Verify the code verifier against the challenge."""
        import hashlib
        import base64

        if self.code_challenge_method != "S256":
            return False

        # Generate expected challenge from verifier
        digest = hashlib.sha256(code_verifier.encode("ascii")).digest()
        expected = base64.urlsafe_b64encode(digest).rstrip(b"=").decode("ascii")

        return secrets.compare_digest(expected, self.code_challenge)


@dataclass
class AuthorizationCode:
    """OAuth 2.1 authorization code."""

    code: str
    client_id: str
    redirect_uri: str
    scope: str
    resource: str  # RFC 8707 resource indicator
    user_id: str
    pkce: PKCEChallenge
    state: str | None
    created_at: datetime
    expires_at: datetime
    used: bool = False

    @classmethod
    def create(
        cls,
        client_id: str,
        redirect_uri: str,
        scope: str,
        resource: str,
        user_id: str,
        pkce: PKCEChallenge,
        state: str | None = None,
        ttl_seconds: int = 600,  # 10 minutes
    ) -> "AuthorizationCode":
        """Create a new authorization code."""
        now = datetime.now(UTC)
        from datetime import timedelta

        return cls(
            code=secrets.token_urlsafe(32),
            client_id=client_id,
            redirect_uri=redirect_uri,
            scope=scope,
            resource=resource,
            user_id=user_id,
            pkce=pkce,
            state=state,
            created_at=now,
            expires_at=now + timedelta(seconds=ttl_seconds),
        )

    def is_expired(self) -> bool:
        """Check if the authorization code is expired."""
        return datetime.now(UTC) > self.expires_at

    def is_valid(self) -> bool:
        """Check if the authorization code is valid (not used and not expired)."""
        return not self.used and not self.is_expired()


@dataclass
class Session:
    """User session with tokens."""

    id: str
    user_id: str
    access_token_jti: str
    refresh_token_jti: str
    client_id: str
    scope: str
    resource: str  # RFC 8707 audience binding
    created_at: datetime
    expires_at: datetime
    last_used_at: datetime
    revoked: bool = False

    @classmethod
    def create(
        cls,
        user_id: str,
        access_token_jti: str,
        refresh_token_jti: str,
        client_id: str,
        scope: str,
        resource: str,
        ttl_seconds: int = 2592000,  # 30 days
    ) -> "Session":
        """Create a new session."""
        now = datetime.now(UTC)
        from datetime import timedelta

        return cls(
            id=str(uuid.uuid4()),
            user_id=user_id,
            access_token_jti=access_token_jti,
            refresh_token_jti=refresh_token_jti,
            client_id=client_id,
            scope=scope,
            resource=resource,
            created_at=now,
            expires_at=now + timedelta(seconds=ttl_seconds),
            last_used_at=now,
        )

    def is_valid(self) -> bool:
        """Check if the session is valid."""
        return not self.revoked and datetime.now(UTC) < self.expires_at


@dataclass
class TokenPair:
    """Access and refresh token pair."""

    access_token: str
    refresh_token: str
    token_type: str = "Bearer"
    expires_in: int = 3600  # seconds
    scope: str = ""


@dataclass
class AuthorizationRequest:
    """OAuth 2.1 authorization request parameters."""

    client_id: str
    redirect_uri: str
    response_type: str
    scope: str
    state: str
    code_challenge: str
    code_challenge_method: str
    resource: str  # RFC 8707

    def validate(self) -> list[str]:
        """Validate the authorization request, returning list of errors."""
        errors = []

        if self.response_type != "code":
            errors.append(f"unsupported_response_type: {self.response_type}")

        if self.code_challenge_method != "S256":
            errors.append(f"invalid_request: code_challenge_method must be S256")

        if not self.code_challenge:
            errors.append("invalid_request: code_challenge is required")

        if not self.resource:
            errors.append("invalid_request: resource parameter is required (RFC 8707)")

        if not self.redirect_uri:
            errors.append("invalid_request: redirect_uri is required")

        # Validate redirect_uri is localhost or HTTPS
        if self.redirect_uri:
            from urllib.parse import urlparse

            parsed = urlparse(self.redirect_uri)
            is_localhost = parsed.hostname in ("localhost", "127.0.0.1", "::1")
            is_https = parsed.scheme == "https"

            if not (is_localhost or is_https):
                errors.append("invalid_request: redirect_uri must be localhost or HTTPS")

        return errors


@dataclass
class TokenRequest:
    """OAuth 2.1 token request parameters."""

    grant_type: str
    code: str | None = None
    redirect_uri: str | None = None
    client_id: str | None = None
    code_verifier: str | None = None
    refresh_token: str | None = None
    resource: str | None = None  # RFC 8707

    def validate_authorization_code(self) -> list[str]:
        """Validate authorization code grant request."""
        errors = []

        if self.grant_type != "authorization_code":
            errors.append(f"unsupported_grant_type: {self.grant_type}")

        if not self.code:
            errors.append("invalid_request: code is required")

        if not self.redirect_uri:
            errors.append("invalid_request: redirect_uri is required")

        if not self.client_id:
            errors.append("invalid_request: client_id is required")

        if not self.code_verifier:
            errors.append("invalid_request: code_verifier is required (PKCE)")

        if not self.resource:
            errors.append("invalid_request: resource is required (RFC 8707)")

        return errors

    def validate_refresh_token(self) -> list[str]:
        """Validate refresh token grant request."""
        errors = []

        if self.grant_type != "refresh_token":
            errors.append(f"unsupported_grant_type: {self.grant_type}")

        if not self.refresh_token:
            errors.append("invalid_request: refresh_token is required")

        return errors


class InMemoryStore:
    """In-memory storage for auth data. Replace with PostgreSQL later."""

    def __init__(self) -> None:
        self._users: dict[str, User] = {}
        self._users_by_github_id: dict[int, str] = {}
        self._sessions: dict[str, Session] = {}
        self._sessions_by_access_jti: dict[str, str] = {}
        self._sessions_by_refresh_jti: dict[str, str] = {}
        self._authorization_codes: dict[str, AuthorizationCode] = {}
        self._pending_authorizations: dict[str, dict[str, Any]] = {}

    # User methods
    def get_user(self, user_id: str) -> User | None:
        """Get user by ID."""
        return self._users.get(user_id)

    def get_user_by_github_id(self, github_id: int) -> User | None:
        """Get user by GitHub ID."""
        user_id = self._users_by_github_id.get(github_id)
        if user_id:
            return self._users.get(user_id)
        return None

    def save_user(self, user: User) -> None:
        """Save or update a user."""
        self._users[user.id] = user
        self._users_by_github_id[user.github_id] = user.id

    def update_user_orgs(self, user_id: str, organizations: list[str]) -> None:
        """Update user's organization memberships."""
        user = self._users.get(user_id)
        if user:
            user.organizations = organizations
            user.updated_at = datetime.now(UTC)

    # Session methods
    def get_session(self, session_id: str) -> Session | None:
        """Get session by ID."""
        return self._sessions.get(session_id)

    def get_session_by_access_jti(self, jti: str) -> Session | None:
        """Get session by access token JTI."""
        session_id = self._sessions_by_access_jti.get(jti)
        if session_id:
            return self._sessions.get(session_id)
        return None

    def get_session_by_refresh_jti(self, jti: str) -> Session | None:
        """Get session by refresh token JTI."""
        session_id = self._sessions_by_refresh_jti.get(jti)
        if session_id:
            return self._sessions.get(session_id)
        return None

    def save_session(self, session: Session) -> None:
        """Save or update a session."""
        self._sessions[session.id] = session
        self._sessions_by_access_jti[session.access_token_jti] = session.id
        self._sessions_by_refresh_jti[session.refresh_token_jti] = session.id

    def revoke_session(self, session_id: str) -> None:
        """Revoke a session."""
        session = self._sessions.get(session_id)
        if session:
            session.revoked = True

    def update_session_tokens(
        self, session_id: str, access_jti: str, refresh_jti: str
    ) -> None:
        """Update session with new token JTIs (for refresh)."""
        session = self._sessions.get(session_id)
        if session:
            # Remove old mappings
            self._sessions_by_access_jti.pop(session.access_token_jti, None)
            self._sessions_by_refresh_jti.pop(session.refresh_token_jti, None)

            # Update session
            session.access_token_jti = access_jti
            session.refresh_token_jti = refresh_jti
            session.last_used_at = datetime.now(UTC)

            # Add new mappings
            self._sessions_by_access_jti[access_jti] = session.id
            self._sessions_by_refresh_jti[refresh_jti] = session.id

    # Authorization code methods
    def get_authorization_code(self, code: str) -> AuthorizationCode | None:
        """Get authorization code."""
        return self._authorization_codes.get(code)

    def save_authorization_code(self, auth_code: AuthorizationCode) -> None:
        """Save authorization code."""
        self._authorization_codes[auth_code.code] = auth_code

    def mark_authorization_code_used(self, code: str) -> None:
        """Mark authorization code as used."""
        auth_code = self._authorization_codes.get(code)
        if auth_code:
            auth_code.used = True

    def delete_authorization_code(self, code: str) -> None:
        """Delete authorization code."""
        self._authorization_codes.pop(code, None)

    # Pending authorization methods (for OAuth flow state)
    def save_pending_authorization(self, state: str, data: dict[str, Any]) -> None:
        """Save pending authorization state."""
        self._pending_authorizations[state] = data

    def get_pending_authorization(self, state: str) -> dict[str, Any] | None:
        """Get pending authorization by state."""
        return self._pending_authorizations.get(state)

    def delete_pending_authorization(self, state: str) -> None:
        """Delete pending authorization."""
        self._pending_authorizations.pop(state, None)

    # Cleanup methods
    def cleanup_expired(self) -> None:
        """Remove expired codes and sessions."""
        now = datetime.now(UTC)

        # Clean expired authorization codes
        expired_codes = [
            code
            for code, auth_code in self._authorization_codes.items()
            if auth_code.is_expired()
        ]
        for code in expired_codes:
            del self._authorization_codes[code]

        # Clean expired sessions
        expired_sessions = [
            session_id
            for session_id, session in self._sessions.items()
            if now > session.expires_at
        ]
        for session_id in expired_sessions:
            session = self._sessions[session_id]
            self._sessions_by_access_jti.pop(session.access_token_jti, None)
            self._sessions_by_refresh_jti.pop(session.refresh_token_jti, None)
            del self._sessions[session_id]
