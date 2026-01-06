"""Well-known endpoints for OAuth 2.1 discovery (RFC 9728, RFC 8414)."""

from dataclasses import dataclass, field, asdict
from typing import Any


@dataclass
class ProtectedResourceMetadata:
    """OAuth 2.0 Protected Resource Metadata (RFC 9728).

    This metadata document advertises the authorization servers
    that can be used to access this protected resource.
    """

    resource: str  # Canonical URI of the resource
    authorization_servers: list[str]  # Authorization server issuer URLs
    bearer_methods_supported: list[str] = field(
        default_factory=lambda: ["header"]
    )
    scopes_supported: list[str] = field(
        default_factory=lambda: ["execute_python", "get_output_file", "read_resources"]
    )
    resource_documentation: str | None = None

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        result = {
            "resource": self.resource,
            "authorization_servers": self.authorization_servers,
            "bearer_methods_supported": self.bearer_methods_supported,
            "scopes_supported": self.scopes_supported,
        }
        if self.resource_documentation:
            result["resource_documentation"] = self.resource_documentation
        return result


@dataclass
class AuthorizationServerMetadata:
    """OAuth 2.0 Authorization Server Metadata (RFC 8414).

    Advertises the authorization server's capabilities and endpoints.
    """

    issuer: str
    authorization_endpoint: str
    token_endpoint: str
    response_types_supported: list[str] = field(
        default_factory=lambda: ["code"]
    )
    grant_types_supported: list[str] = field(
        default_factory=lambda: ["authorization_code", "refresh_token"]
    )
    code_challenge_methods_supported: list[str] = field(
        default_factory=lambda: ["S256"]
    )
    token_endpoint_auth_methods_supported: list[str] = field(
        default_factory=lambda: ["none"]  # Public clients
    )
    scopes_supported: list[str] = field(
        default_factory=lambda: ["execute_python", "get_output_file", "read_resources"]
    )
    revocation_endpoint: str | None = None
    userinfo_endpoint: str | None = None

    # MCP-specific extensions
    client_id_metadata_document_supported: bool = True

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        result = {
            "issuer": self.issuer,
            "authorization_endpoint": self.authorization_endpoint,
            "token_endpoint": self.token_endpoint,
            "response_types_supported": self.response_types_supported,
            "grant_types_supported": self.grant_types_supported,
            "code_challenge_methods_supported": self.code_challenge_methods_supported,
            "token_endpoint_auth_methods_supported": self.token_endpoint_auth_methods_supported,
            "scopes_supported": self.scopes_supported,
            "client_id_metadata_document_supported": self.client_id_metadata_document_supported,
        }

        if self.revocation_endpoint:
            result["revocation_endpoint"] = self.revocation_endpoint

        if self.userinfo_endpoint:
            result["userinfo_endpoint"] = self.userinfo_endpoint

        return result


def create_protected_resource_metadata(base_url: str) -> ProtectedResourceMetadata:
    """Create protected resource metadata for the MCP server.

    Args:
        base_url: The canonical base URL of the MCP server.

    Returns:
        Protected resource metadata document.
    """
    # Normalize URL (remove trailing slash)
    resource = base_url.rstrip("/")

    return ProtectedResourceMetadata(
        resource=resource,
        authorization_servers=[resource],  # We are our own authorization server
        bearer_methods_supported=["header"],
        scopes_supported=[
            "execute_python",
            "get_output_file",
            "read_resources",
        ],
        resource_documentation=f"{resource}/docs",
    )


def create_authorization_server_metadata(base_url: str) -> AuthorizationServerMetadata:
    """Create authorization server metadata.

    Args:
        base_url: The canonical base URL of the authorization server.

    Returns:
        Authorization server metadata document.
    """
    # Normalize URL (remove trailing slash)
    issuer = base_url.rstrip("/")

    return AuthorizationServerMetadata(
        issuer=issuer,
        authorization_endpoint=f"{issuer}/auth/authorize",
        token_endpoint=f"{issuer}/auth/token",
        revocation_endpoint=f"{issuer}/auth/revoke",
        userinfo_endpoint=f"{issuer}/auth/userinfo",
        response_types_supported=["code"],
        grant_types_supported=["authorization_code", "refresh_token"],
        code_challenge_methods_supported=["S256"],
        token_endpoint_auth_methods_supported=["none"],
        scopes_supported=[
            "execute_python",
            "get_output_file",
            "read_resources",
        ],
        client_id_metadata_document_supported=True,
    )


def format_www_authenticate(
    resource_metadata_url: str,
    scope: str | None = None,
    error: str | None = None,
    error_description: str | None = None,
) -> str:
    """Format WWW-Authenticate header for 401/403 responses.

    Per RFC 9728 and RFC 6750, the WWW-Authenticate header should
    include the resource metadata URL and optionally scope/error info.

    Args:
        resource_metadata_url: URL to the protected resource metadata.
        scope: Required scope(s) for the resource.
        error: OAuth error code (e.g., "invalid_token", "insufficient_scope").
        error_description: Human-readable error description.

    Returns:
        Formatted WWW-Authenticate header value.
    """
    parts = [f'Bearer resource_metadata="{resource_metadata_url}"']

    if scope:
        parts.append(f'scope="{scope}"')

    if error:
        parts.append(f'error="{error}"')

    if error_description:
        # Escape quotes in description
        safe_desc = error_description.replace('"', '\\"')
        parts.append(f'error_description="{safe_desc}"')

    return ", ".join(parts)
