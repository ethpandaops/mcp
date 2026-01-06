"""GitHub OAuth integration with PKCE support."""

import secrets
from dataclasses import dataclass
from urllib.parse import urlencode, urlparse

import httpx
import structlog

from xatu_mcp.config import AuthGitHubConfig
from xatu_mcp.auth.models import GitHubUser

logger = structlog.get_logger()


GITHUB_AUTHORIZE_URL = "https://github.com/login/oauth/authorize"
GITHUB_TOKEN_URL = "https://github.com/login/oauth/access_token"
GITHUB_API_URL = "https://api.github.com"


class GitHubOAuthError(Exception):
    """Error during GitHub OAuth flow."""

    pass


@dataclass
class GitHubTokenResponse:
    """GitHub OAuth token response."""

    access_token: str
    token_type: str
    scope: str


class GitHubOAuthClient:
    """GitHub OAuth client with PKCE support."""

    def __init__(self, config: AuthGitHubConfig) -> None:
        """Initialize GitHub OAuth client.

        Args:
            config: GitHub OAuth configuration.
        """
        self._client_id = config.client_id
        self._client_secret = config.client_secret

    def get_authorization_url(
        self,
        redirect_uri: str,
        state: str,
        scope: str = "read:user read:org",
    ) -> str:
        """Generate GitHub OAuth authorization URL.

        Note: GitHub OAuth does not support PKCE. We implement PKCE at our
        authorization server level for MCP compliance, but GitHub's OAuth
        flow uses client_secret for security.

        Args:
            redirect_uri: The callback URL.
            state: CSRF protection state parameter.
            scope: GitHub OAuth scopes.

        Returns:
            The authorization URL to redirect the user to.
        """
        params = {
            "client_id": self._client_id,
            "redirect_uri": redirect_uri,
            "scope": scope,
            "state": state,
            "allow_signup": "false",
        }

        url = f"{GITHUB_AUTHORIZE_URL}?{urlencode(params)}"

        logger.debug(
            "Generated GitHub authorization URL",
            redirect_uri=redirect_uri,
            scope=scope,
        )

        return url

    async def exchange_code(
        self,
        code: str,
        redirect_uri: str,
    ) -> GitHubTokenResponse:
        """Exchange authorization code for access token.

        Args:
            code: The authorization code from GitHub callback.
            redirect_uri: The callback URL (must match original).

        Returns:
            GitHub token response.

        Raises:
            GitHubOAuthError: If the token exchange fails.
        """
        async with httpx.AsyncClient() as client:
            response = await client.post(
                GITHUB_TOKEN_URL,
                data={
                    "client_id": self._client_id,
                    "client_secret": self._client_secret,
                    "code": code,
                    "redirect_uri": redirect_uri,
                },
                headers={
                    "Accept": "application/json",
                },
                timeout=30.0,
            )

            if response.status_code != 200:
                logger.error(
                    "GitHub token exchange failed",
                    status_code=response.status_code,
                    response=response.text,
                )
                raise GitHubOAuthError(
                    f"Token exchange failed: {response.status_code}"
                )

            data = response.json()

            if "error" in data:
                error = data.get("error")
                error_description = data.get("error_description", "")
                logger.error(
                    "GitHub OAuth error",
                    error=error,
                    description=error_description,
                )
                raise GitHubOAuthError(f"{error}: {error_description}")

            return GitHubTokenResponse(
                access_token=data["access_token"],
                token_type=data.get("token_type", "bearer"),
                scope=data.get("scope", ""),
            )

    async def get_user(self, access_token: str) -> GitHubUser:
        """Fetch user profile from GitHub API.

        Args:
            access_token: GitHub access token.

        Returns:
            GitHub user profile.

        Raises:
            GitHubOAuthError: If the API request fails.
        """
        async with httpx.AsyncClient() as client:
            # Get user profile
            response = await client.get(
                f"{GITHUB_API_URL}/user",
                headers={
                    "Authorization": f"Bearer {access_token}",
                    "Accept": "application/vnd.github+json",
                    "X-GitHub-Api-Version": "2022-11-28",
                },
                timeout=30.0,
            )

            if response.status_code != 200:
                logger.error(
                    "GitHub user API failed",
                    status_code=response.status_code,
                    response=response.text,
                )
                raise GitHubOAuthError(
                    f"Failed to fetch user profile: {response.status_code}"
                )

            user_data = response.json()

            # Get user organizations
            orgs = await self._get_user_organizations(client, access_token)

            logger.info(
                "Fetched GitHub user profile",
                github_id=user_data["id"],
                login=user_data["login"],
                orgs=orgs,
            )

            return GitHubUser(
                id=user_data["id"],
                login=user_data["login"],
                name=user_data.get("name"),
                email=user_data.get("email"),
                avatar_url=user_data.get("avatar_url"),
                organizations=orgs,
            )

    async def _get_user_organizations(
        self,
        client: httpx.AsyncClient,
        access_token: str,
    ) -> list[str]:
        """Fetch user's organization memberships.

        Args:
            client: HTTP client.
            access_token: GitHub access token.

        Returns:
            List of organization logins the user is a member of.
        """
        response = await client.get(
            f"{GITHUB_API_URL}/user/orgs",
            headers={
                "Authorization": f"Bearer {access_token}",
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
            },
            timeout=30.0,
        )

        if response.status_code != 200:
            logger.warning(
                "Failed to fetch user organizations",
                status_code=response.status_code,
            )
            return []

        orgs_data = response.json()
        return [org["login"] for org in orgs_data]

    async def validate_org_membership(
        self,
        access_token: str,
        allowed_orgs: list[str],
    ) -> tuple[bool, list[str]]:
        """Validate user is a member of allowed organizations.

        Args:
            access_token: GitHub access token.
            allowed_orgs: List of allowed organization logins.

        Returns:
            Tuple of (is_member, list of matching orgs).
        """
        if not allowed_orgs:
            return True, []

        async with httpx.AsyncClient() as client:
            user_orgs = await self._get_user_organizations(client, access_token)

        matching_orgs = [org for org in user_orgs if org in allowed_orgs]
        is_member = len(matching_orgs) > 0

        logger.info(
            "Validated org membership",
            allowed_orgs=allowed_orgs,
            user_orgs=user_orgs,
            matching_orgs=matching_orgs,
            is_member=is_member,
        )

        return is_member, matching_orgs

    async def refresh_user_orgs(self, access_token: str) -> list[str]:
        """Refresh user's organization memberships.

        This should be called during token refresh to ensure
        org membership is still valid.

        Args:
            access_token: GitHub access token.

        Returns:
            List of organization logins.
        """
        async with httpx.AsyncClient() as client:
            return await self._get_user_organizations(client, access_token)


def generate_state() -> str:
    """Generate a cryptographically secure state parameter.

    Returns:
        Random state string.
    """
    return secrets.token_urlsafe(32)


def validate_redirect_uri(uri: str, allowed_patterns: list[str] | None = None) -> bool:
    """Validate a redirect URI for security.

    Per OAuth 2.1 and MCP spec, redirect URIs must be either:
    - localhost (http://localhost:*, http://127.0.0.1:*, http://[::1]:*)
    - HTTPS URLs

    Args:
        uri: The redirect URI to validate.
        allowed_patterns: Optional list of additional allowed patterns.

    Returns:
        True if the URI is valid.
    """
    try:
        parsed = urlparse(uri)

        # Check for localhost (allowed with HTTP)
        if parsed.hostname in ("localhost", "127.0.0.1", "::1"):
            return parsed.scheme in ("http", "https")

        # Non-localhost must be HTTPS
        if parsed.scheme != "https":
            return False

        # Must have a valid host
        if not parsed.hostname:
            return False

        # Check against allowed patterns if provided
        if allowed_patterns:
            return any(
                uri.startswith(pattern) for pattern in allowed_patterns
            )

        return True

    except Exception:
        return False
