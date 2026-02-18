package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/auth/client"
	"github.com/ethpandaops/mcp/pkg/auth/store"
)

var (
	authIssuerURL string
	authClientID  string
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  `Authenticate with the ethpandaops credential proxy using OAuth PKCE.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to the credential proxy",
	Long: `Log in to the ethpandaops credential proxy using OAuth PKCE.
This opens a browser window for authentication and stores the tokens locally.`,
	RunE: runLogin,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from the credential proxy",
	Long:  `Remove locally stored authentication tokens.`,
	RunE:  runLogout,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long:  `Show the current authentication status and token information.`,
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)

	// Login flags.
	authLoginCmd.Flags().StringVar(&authIssuerURL, "issuer", "", "OIDC issuer URL (required)")
	authLoginCmd.Flags().StringVar(&authClientID, "client-id", "", "OAuth client ID (required)")
	_ = authLoginCmd.MarkFlagRequired("issuer")
	_ = authLoginCmd.MarkFlagRequired("client-id")
}

func runLogin(_ *cobra.Command, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Info("Starting OAuth login flow")

	// Create OAuth client.
	authClient := client.New(log, client.Config{
		IssuerURL: authIssuerURL,
		ClientID:  authClientID,
	})

	// Perform login.
	tokens, err := authClient.Login(ctx)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Create credential store.
	credStore := store.New(log, store.Config{
		AuthClient: authClient,
	})

	// Save tokens.
	if err := credStore.Save(tokens); err != nil {
		return fmt.Errorf("saving tokens: %w", err)
	}

	log.Info("Successfully authenticated!")
	fmt.Printf("\nAuthentication successful. Token expires at: %s\n", tokens.ExpiresAt.Format(time.RFC3339))

	return nil
}

func runLogout(_ *cobra.Command, _ []string) error {
	// Create credential store.
	credStore := store.New(log, store.Config{})

	// Clear tokens.
	if err := credStore.Clear(); err != nil {
		return fmt.Errorf("clearing tokens: %w", err)
	}

	log.Info("Successfully logged out")
	fmt.Println("Logged out successfully.")

	return nil
}

func runStatus(_ *cobra.Command, _ []string) error {
	// Create credential store.
	credStore := store.New(log, store.Config{})

	// Load tokens.
	tokens, err := credStore.Load()
	if err != nil {
		return fmt.Errorf("loading tokens: %w", err)
	}

	if tokens == nil {
		fmt.Println("Not authenticated.")
		fmt.Println("\nTo log in, run:")
		fmt.Println("  ethpandaops-mcp login")

		return nil
	}

	fmt.Println("Authentication Status:")
	fmt.Printf("  Token type: %s\n", tokens.TokenType)

	if tokens.ExpiresAt.After(time.Now()) {
		remaining := time.Until(tokens.ExpiresAt)
		fmt.Printf("  Expires in: %s\n", remaining.Round(time.Second))
		fmt.Printf("  Expires at: %s\n", tokens.ExpiresAt.Format(time.RFC3339))
		fmt.Println("  Status: Valid")
	} else {
		fmt.Printf("  Expired at: %s\n", tokens.ExpiresAt.Format(time.RFC3339))
		fmt.Println("  Status: Expired")

		if tokens.RefreshToken != "" {
			fmt.Println("\n  A refresh token is available. Run 'ethpandaops-mcp login' to re-authenticate.")
		}
	}

	// Show credential path.
	home, _ := os.UserHomeDir()
	credPath := filepath.Join(home, ".config", "ethpandaops-mcp", "credentials.json")
	fmt.Printf("\n  Credentials stored at: %s\n", credPath)

	return nil
}
