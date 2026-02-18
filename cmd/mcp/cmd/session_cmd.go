package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/defaults"
)

var (
	sessionJSON     bool
	sessionProxyURL string
	sessionIssuer   string
	sessionClientID string
)

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage sandbox sessions",
	Long:  `Create, list, and destroy persistent sandbox sessions.`,
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active sessions",
	RunE:  runSessionList,
}

var sessionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new session",
	RunE:  runSessionCreate,
}

var sessionDestroyCmd = &cobra.Command{
	Use:   "destroy <session-id>",
	Short: "Destroy a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runSessionDestroy,
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionCreateCmd)
	sessionCmd.AddCommand(sessionDestroyCmd)

	sessionCmd.PersistentFlags().BoolVar(&sessionJSON, "json", false, "Output in JSON format")
	sessionCmd.PersistentFlags().StringVar(&sessionProxyURL, "proxy-url", defaults.ProxyURL, "Proxy server URL")
	sessionCmd.PersistentFlags().StringVar(&sessionIssuer, "issuer", defaults.IssuerURL, "OIDC issuer URL")
	sessionCmd.PersistentFlags().StringVar(&sessionClientID, "client-id", defaults.ClientID, "OAuth client ID")
}

func runSessionList(_ *cobra.Command, _ []string) error {
	suppressLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := loadConfigOrDefaults(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sandboxSvc, err := buildSandboxService(ctx, cfg)
	if err != nil {
		return err
	}

	defer func() { _ = sandboxSvc.Stop(ctx) }()

	sessions, err := sandboxSvc.ListSessions(ctx, "")
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	if sessionJSON || !isTerminal() {
		return outputJSON(sessions)
	}

	if len(sessions) == 0 {
		fmt.Println("No active sessions.")

		return nil
	}

	for _, s := range sessions {
		fmt.Printf("%-36s  created=%-20s  last_used=%-20s  ttl=%s  files=%d\n",
			s.ID,
			s.CreatedAt.Format(time.RFC3339),
			s.LastUsed.Format(time.RFC3339),
			s.TTLRemaining.Round(time.Second),
			len(s.WorkspaceFiles),
		)
	}

	return nil
}

func runSessionCreate(_ *cobra.Command, _ []string) error {
	suppressLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := loadConfigOrDefaults(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Session creation needs env vars from proxy.
	proxyClient, err := buildProxyClient(ctx, sessionProxyURL, sessionIssuer, sessionClientID)
	if err != nil {
		return err
	}

	defer func() { _ = proxyClient.Stop(ctx) }()

	pluginReg, err := buildPluginRegistry(cfg)
	if err != nil {
		return fmt.Errorf("building plugin registry: %w", err)
	}

	injectProxyClient(pluginReg, proxyClient)

	sandboxSvc, err := buildSandboxService(ctx, cfg)
	if err != nil {
		return err
	}

	defer func() { _ = sandboxSvc.Stop(ctx) }()

	env, err := buildSandboxEnv(pluginReg, proxyClient)
	if err != nil {
		return fmt.Errorf("building sandbox environment: %w", err)
	}

	sessionID, err := sandboxSvc.CreateSession(ctx, "", env)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	if sessionJSON || !isTerminal() {
		return outputJSON(map[string]string{"session_id": sessionID})
	}

	fmt.Println(sessionID)

	return nil
}

func runSessionDestroy(_ *cobra.Command, args []string) error {
	suppressLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := loadConfigOrDefaults(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	sandboxSvc, err := buildSandboxService(ctx, cfg)
	if err != nil {
		return err
	}

	defer func() { _ = sandboxSvc.Stop(ctx) }()

	if err := sandboxSvc.DestroySession(ctx, args[0], ""); err != nil {
		return fmt.Errorf("destroying session: %w", err)
	}

	if sessionJSON || !isTerminal() {
		return outputJSON(map[string]string{"status": "destroyed", "session_id": args[0]})
	}

	fmt.Printf("Session %s destroyed.\n", args[0])

	return nil
}
