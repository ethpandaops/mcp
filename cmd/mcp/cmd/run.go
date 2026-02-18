package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/defaults"
	"github.com/ethpandaops/mcp/pkg/sandbox"
)

// RunResult is the JSON output format for the run command.
type RunResult struct {
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	ExitCode        int      `json:"exit_code"`
	OutputFiles     []string `json:"output_files,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	DurationSeconds float64  `json:"duration_seconds"`
}

var (
	runCode      string
	runFile      string
	runSessionID string
	runTimeout   int
	runJSON      bool
	runProxyURL  string
	runIssuerURL string
	runClientID  string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute Python code in a sandboxed container",
	Long: `Execute Python code with the ethpandaops library for Ethereum data analysis.

The code runs in a Docker container with access to ClickHouse, Prometheus,
Loki, and S3 storage via the credential proxy.

Examples:
  ethpandaops-mcp run --code 'print("hello")'
  ethpandaops-mcp run --file query.py --session-id abc123
  ethpandaops-mcp run --code 'from ethpandaops import clickhouse; print(clickhouse.list_datasources())' --timeout 120`,
	RunE: runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&runCode, "code", "", "Python code to execute")
	runCmd.Flags().StringVar(&runFile, "file", "", "Path to Python file to execute")
	runCmd.Flags().StringVar(&runSessionID, "session-id", "", "Reuse a persistent session")
	runCmd.Flags().IntVar(&runTimeout, "timeout", defaults.SandboxTimeout, "Execution timeout in seconds")
	runCmd.Flags().BoolVar(&runJSON, "json", false, "Force JSON output")
	runCmd.Flags().StringVar(&runProxyURL, "proxy-url", defaults.ProxyURL, "Proxy server URL")
	runCmd.Flags().StringVar(&runIssuerURL, "issuer", defaults.IssuerURL, "OIDC issuer URL")
	runCmd.Flags().StringVar(&runClientID, "client-id", defaults.ClientID, "OAuth client ID")
}

func runRun(_ *cobra.Command, _ []string) error {
	suppressLogs()

	// Validate flags: exactly one of --code or --file must be provided.
	if runCode == "" && runFile == "" {
		return fmt.Errorf("either --code or --file is required")
	}

	if runCode != "" && runFile != "" {
		return fmt.Errorf("--code and --file are mutually exclusive")
	}

	// Read code from file if --file was provided.
	code := runCode
	if runFile != "" {
		data, err := os.ReadFile(runFile)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", runFile, err)
		}

		code = string(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(runTimeout+30)*time.Second)
	defer cancel()

	// Load config or use defaults.
	cfg, err := loadConfigOrDefaults(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Build proxy client (authenticates + discovers datasources).
	proxyClient, err := buildProxyClient(ctx, runProxyURL, runIssuerURL, runClientID)
	if err != nil {
		return err
	}

	defer func() { _ = proxyClient.Stop(ctx) }()

	// Build plugin registry and inject proxy.
	pluginReg, err := buildPluginRegistry(cfg)
	if err != nil {
		return fmt.Errorf("building plugin registry: %w", err)
	}

	injectProxyClient(pluginReg, proxyClient)

	// Build sandbox service.
	sandboxSvc, err := buildSandboxService(ctx, cfg)
	if err != nil {
		return err
	}

	defer func() { _ = sandboxSvc.Stop(ctx) }()

	// Build environment variables.
	env, err := buildSandboxEnv(pluginReg, proxyClient)
	if err != nil {
		return fmt.Errorf("building sandbox environment: %w", err)
	}

	// Register proxy token for this execution.
	executionID := uuid.New().String()
	proxyToken := proxyClient.RegisterToken(executionID)
	env["ETHPANDAOPS_PROXY_TOKEN"] = proxyToken

	defer proxyClient.RevokeToken(executionID)

	// Execute the code.
	result, err := sandboxSvc.Execute(ctx, sandbox.ExecuteRequest{
		Code:      code,
		Env:       env,
		Timeout:   time.Duration(runTimeout) * time.Second,
		SessionID: runSessionID,
	})
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	// Output results.
	if runJSON || !isTerminal() {
		return outputJSON(RunResult{
			Stdout:          result.Stdout,
			Stderr:          result.Stderr,
			ExitCode:        result.ExitCode,
			OutputFiles:     result.OutputFiles,
			SessionID:       result.SessionID,
			DurationSeconds: result.DurationSeconds,
		})
	}

	// Human-readable output.
	if result.Stdout != "" {
		fmt.Print(result.Stdout)

		if result.Stdout[len(result.Stdout)-1] != '\n' {
			fmt.Println()
		}
	}

	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)

		if result.Stderr[len(result.Stderr)-1] != '\n' {
			fmt.Fprintln(os.Stderr)
		}
	}

	if len(result.OutputFiles) > 0 {
		for _, f := range result.OutputFiles {
			fmt.Fprintf(os.Stderr, "[file] %s\n", f)
		}
	}

	if result.SessionID != "" {
		fmt.Fprintf(os.Stderr, "[session] %s\n", result.SessionID)
	}

	// Exit with the sandbox exit code.
	if result.ExitCode != 0 {
		os.Exit(result.ExitCode)
	}

	return nil
}
