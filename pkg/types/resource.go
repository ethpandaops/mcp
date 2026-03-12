package types

import (
	"context"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

// ClientContext identifies the calling context for resource handlers.
type ClientContext int

const (
	// ClientContextMCP indicates the caller is an MCP client (LLM tool use).
	ClientContextMCP ClientContext = iota
	// ClientContextCLI indicates the caller is a CLI agent using panda commands.
	ClientContextCLI
)

// ClientContextCLIParam is the wire value for CLI context in API query parameters.
const ClientContextCLIParam = "cli"

type clientContextKeyType struct{}

var clientContextKey = clientContextKeyType{}

// GetClientContext extracts the ClientContext from a context.
// Returns ClientContextMCP as the default.
func GetClientContext(ctx context.Context) ClientContext {
	if v, ok := ctx.Value(clientContextKey).(ClientContext); ok {
		return v
	}

	return ClientContextMCP
}

// WithClientContext returns a new context with the given ClientContext.
func WithClientContext(ctx context.Context, cc ClientContext) context.Context {
	return context.WithValue(ctx, clientContextKey, cc)
}

// ReadHandler handles reading a resource by URI.
type ReadHandler func(ctx context.Context, uri string) (string, error)

// StaticResource is a resource with a fixed URI.
type StaticResource struct {
	Resource mcp.Resource
	Handler  ReadHandler
}

// TemplateResource is a resource with a URI pattern.
type TemplateResource struct {
	Template mcp.ResourceTemplate
	Pattern  *regexp.Regexp
	Handler  ReadHandler
}
