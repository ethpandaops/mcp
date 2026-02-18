// Package defaults provides production default values for the ethpandaops MCP CLI.
package defaults

const (
	// ProxyURL is the default production proxy URL.
	ProxyURL = "https://mcp-proxy.ethpandaops.io"
	// IssuerURL is the default OIDC issuer URL.
	IssuerURL = "https://dex.ethpandaops.io"
	// ClientID is the default OAuth client ID.
	ClientID = "ethpandaops-mcp"

	// SandboxImage is the default sandbox Docker image.
	SandboxImage = "ethpandaops-mcp-sandbox:latest"
	// SandboxTimeout is the default execution timeout in seconds.
	SandboxTimeout = 60
)
