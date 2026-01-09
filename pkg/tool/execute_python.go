package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethpandaops/xatu-mcp/pkg/auth"
	"github.com/ethpandaops/xatu-mcp/pkg/config"
	"github.com/ethpandaops/xatu-mcp/pkg/sandbox"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
)

// sessionsWithResourceTip tracks sessions that have already seen the resource tip.
var sessionsWithResourceTip sync.Map

// resourceTipMessage is shown after the first execution in a session to guide users to MCP resources.
const resourceTipMessage = `
💡 TIP: Read these MCP resources for available datasources and schemas:
   - datasources://list - available datasource UIDs
   - datasources://clickhouse - ClickHouse datasources only
   - clickhouse-schema://{cluster}/{table} - table schema (if schema discovery enabled)
   - api://xatu - Python library documentation
   - networks://active - available networks`

const (
	// ExecutePythonToolName is the name of the execute_python tool.
	ExecutePythonToolName = "execute_python"
	// DefaultTimeout is the default execution timeout in seconds.
	DefaultTimeout = 60
	// MaxTimeout is the maximum allowed execution timeout in seconds.
	MaxTimeout = 300
	// MinTimeout is the minimum allowed execution timeout in seconds.
	MinTimeout = 1
)

// executePythonDescription is the description of the execute_python tool.
const executePythonDescription = `Execute Python code in a sandboxed environment with the xatu library pre-installed.

Read api://xatu for library documentation. Read datasources://list for available datasource UIDs.

Key modules: clickhouse, prometheus, loki, storage

Files written to /workspace/ persist within a session. Use storage.upload() for public URLs.`

// NewExecutePythonTool creates the execute_python tool definition.
func NewExecutePythonTool(
	log logrus.FieldLogger,
	sandboxSvc sandbox.Service,
	cfg *config.Config,
) Definition {
	return Definition{
		Tool: mcp.Tool{
			Name:        ExecutePythonToolName,
			Description: executePythonDescription,
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"code": map[string]any{
						"type":        "string",
						"description": "Python code to execute",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Execution timeout in seconds (default: from config, max: 300)",
						"minimum":     MinTimeout,
						"maximum":     MaxTimeout,
					},
					"session_id": map[string]any{
						"type":        "string",
						"description": "Optional session ID to reuse a persistent container. If omitted, a new session is created (when sessions are enabled).",
					},
				},
				Required: []string{"code"},
			},
		},
		Handler: newExecutePythonHandler(log, sandboxSvc, cfg),
	}
}

// newExecutePythonHandler creates the handler function for execute_python.
func newExecutePythonHandler(
	log logrus.FieldLogger,
	sandboxSvc sandbox.Service,
	cfg *config.Config,
) Handler {
	handlerLog := log.WithField("tool", ExecutePythonToolName)

	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Extract code from arguments using mcp-go helper methods.
		code, err := request.RequireString("code")
		if err != nil {
			return CallToolError(fmt.Errorf("invalid arguments: %w", err)), nil
		}

		if code == "" {
			return CallToolError(fmt.Errorf("code is required")), nil
		}

		// Extract timeout from arguments using mcp-go helper with default.
		timeout := request.GetInt("timeout", cfg.Sandbox.Timeout)

		// Validate timeout.
		if timeout < MinTimeout || timeout > MaxTimeout {
			return CallToolError(
				fmt.Errorf("timeout must be between %d and %d seconds", MinTimeout, MaxTimeout),
			), nil
		}

		// Extract optional session_id.
		sessionID := request.GetString("session_id", "")

		// Extract owner ID from auth context for session binding.
		var ownerID string
		if user := auth.GetAuthUser(ctx); user != nil {
			ownerID = fmt.Sprintf("%d", user.GitHubID)
		}

		handlerLog.WithFields(logrus.Fields{
			"code_length": len(code),
			"timeout":     timeout,
			"backend":     sandboxSvc.Name(),
			"session_id":  sessionID,
			"owner_id":    ownerID,
		}).Info("Executing Python code")

		// Build environment variables for the sandbox.
		env := buildSandboxEnv(cfg)

		// Execute the code in the sandbox.
		result, err := sandboxSvc.Execute(ctx, sandbox.ExecuteRequest{
			Code:      code,
			Env:       env,
			Timeout:   time.Duration(timeout) * time.Second,
			SessionID: sessionID,
			OwnerID:   ownerID,
		})
		if err != nil {
			handlerLog.WithError(err).Error("Execution failed")

			return CallToolError(fmt.Errorf("execution error: %w", err)), nil
		}

		// Format the response.
		response := formatExecutionResult(result, cfg)

		// Show resource tip after the first execution in a session.
		sessionKey := result.SessionID
		if sessionKey == "" {
			sessionKey = result.ExecutionID // Use execution ID if no session
		}

		if _, alreadyShown := sessionsWithResourceTip.LoadOrStore(sessionKey, true); !alreadyShown {
			response += resourceTipMessage
		}

		handlerLog.WithFields(logrus.Fields{
			"execution_id": result.ExecutionID,
			"exit_code":    result.ExitCode,
			"duration":     result.DurationSeconds,
			"output_files": result.OutputFiles,
			"session_id":   result.SessionID,
		}).Info("Execution completed")

		return CallToolSuccess(response), nil
	}
}

// formatExecutionResult formats the execution result into a string.
func formatExecutionResult(result *sandbox.ExecutionResult, cfg *config.Config) string {
	var parts []string

	if result.Stdout != "" {
		parts = append(parts, fmt.Sprintf("[stdout]\n%s", result.Stdout))
	}

	if result.Stderr != "" {
		parts = append(parts, fmt.Sprintf("[stderr]\n%s", result.Stderr))
	}

	if len(result.OutputFiles) > 0 {
		parts = append(parts, fmt.Sprintf("[files] %s", strings.Join(result.OutputFiles, ", ")))
	}

	// Include session info if available.
	if result.SessionID != "" {
		sessionInfo := fmt.Sprintf("[session] id=%s ttl=%s",
			result.SessionID, result.SessionTTLRemaining.Round(time.Second))

		if len(result.SessionFiles) > 0 {
			workspaceFiles := make([]string, 0, len(result.SessionFiles))
			for _, f := range result.SessionFiles {
				workspaceFiles = append(workspaceFiles, fmt.Sprintf("%s(%s)", f.Name, formatSize(f.Size)))
			}

			sessionInfo += fmt.Sprintf(" workspace=[%s]", strings.Join(workspaceFiles, ", "))
		}

		parts = append(parts, sessionInfo)
	}

	parts = append(parts, fmt.Sprintf("[exit=%d duration=%.2fs id=%s]",
		result.ExitCode, result.DurationSeconds, result.ExecutionID))

	// Add note about localhost URLs if storage is configured with localhost.
	if cfg.Storage != nil && strings.Contains(cfg.Storage.PublicURLPrefix, "localhost") {
		if strings.Contains(result.Stdout, "localhost") {
			parts = append(parts, "[note] Storage URLs use localhost - these are accessible if running the server locally.")
		}
	}

	return strings.Join(parts, "\n")
}

// formatSize formats a byte size into a human-readable string.
func formatSize(bytes int64) string {
	const unit = 1024

	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// buildSandboxEnv creates the environment variables map for the sandbox.
func buildSandboxEnv(cfg *config.Config) map[string]string {
	env := make(map[string]string, 8)

	// Grafana configuration - all datasource queries route through Grafana.
	env["XATU_GRAFANA_URL"] = cfg.Grafana.URL
	env["XATU_GRAFANA_TOKEN"] = cfg.Grafana.ServiceToken
	env["XATU_HTTP_TIMEOUT"] = strconv.Itoa(cfg.Grafana.Timeout)

	// S3 Storage.
	if cfg.Storage != nil {
		env["XATU_S3_ENDPOINT"] = cfg.Storage.Endpoint
		env["XATU_S3_ACCESS_KEY"] = cfg.Storage.AccessKey
		env["XATU_S3_SECRET_KEY"] = cfg.Storage.SecretKey
		env["XATU_S3_BUCKET"] = cfg.Storage.Bucket
		env["XATU_S3_REGION"] = cfg.Storage.Region

		if cfg.Storage.PublicURLPrefix != "" {
			env["XATU_S3_PUBLIC_URL_PREFIX"] = cfg.Storage.PublicURLPrefix
		}
	}

	return env
}
