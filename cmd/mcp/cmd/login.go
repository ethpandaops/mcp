package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/auth/client"
	"github.com/ethpandaops/mcp/pkg/auth/store"
	"github.com/ethpandaops/mcp/pkg/defaults"
)

var (
	loginProxyURL  string
	loginIssuerURL string
	loginClientID  string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the ethpandaops credential proxy",
	Long: `Log in to the ethpandaops credential proxy using OAuth PKCE.
This opens a browser window for authentication and stores tokens locally.

Uses production defaults unless overridden with flags.`,
	RunE: runTopLevelLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from the ethpandaops credential proxy",
	Long:  `Remove locally stored authentication tokens.`,
	RunE:  runTopLevelLogout,
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)

	loginCmd.Flags().StringVar(&loginProxyURL, "proxy-url", defaults.ProxyURL, "Proxy server URL")
	loginCmd.Flags().StringVar(&loginIssuerURL, "issuer", defaults.IssuerURL, "OIDC issuer URL")
	loginCmd.Flags().StringVar(&loginClientID, "client-id", defaults.ClientID, "OAuth client ID")
}

func runTopLevelLogin(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("Logging in to %s...\n", loginProxyURL)

	authClient := client.New(log, client.Config{
		IssuerURL: loginIssuerURL,
		ClientID:  loginClientID,
	})

	tokens, err := authClient.Login(ctx)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	credStore := store.New(log, store.Config{
		AuthClient: authClient,
	})

	if err := credStore.Save(tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	fmt.Printf("Successfully authenticated! Token expires at: %s\n",
		tokens.ExpiresAt.Format(time.RFC3339))

	return nil
}

func runTopLevelLogout(_ *cobra.Command, _ []string) error {
	credStore := store.New(log, store.Config{})

	if err := credStore.Clear(); err != nil {
		return fmt.Errorf("clearing tokens: %w", err)
	}

	fmt.Println("Logged out successfully.")

	return nil
}
