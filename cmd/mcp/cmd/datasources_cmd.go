package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/defaults"
	"github.com/ethpandaops/mcp/pkg/types"
)

var (
	dsType     string
	dsJSON     bool
	dsProxyURL string
	dsIssuer   string
	dsClientID string
)

var datasourcesCmd = &cobra.Command{
	Use:     "datasources",
	Aliases: []string{"ds"},
	Short:   "List available datasources",
	Long: `List datasources discovered from the credential proxy.

Examples:
  ethpandaops-mcp datasources
  ethpandaops-mcp datasources --type clickhouse
  ethpandaops-mcp datasources --json`,
	RunE: runDatasources,
}

func init() {
	rootCmd.AddCommand(datasourcesCmd)

	datasourcesCmd.Flags().StringVar(&dsType, "type", "", "Filter by type (clickhouse, prometheus, loki)")
	datasourcesCmd.Flags().BoolVar(&dsJSON, "json", false, "Output in JSON format")
	datasourcesCmd.Flags().StringVar(&dsProxyURL, "proxy-url", defaults.ProxyURL, "Proxy server URL")
	datasourcesCmd.Flags().StringVar(&dsIssuer, "issuer", defaults.IssuerURL, "OIDC issuer URL")
	datasourcesCmd.Flags().StringVar(&dsClientID, "client-id", defaults.ClientID, "OAuth client ID")
}

func runDatasources(_ *cobra.Command, _ []string) error {
	suppressLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	proxyClient, err := buildProxyClient(ctx, dsProxyURL, dsIssuer, dsClientID)
	if err != nil {
		return err
	}

	defer func() { _ = proxyClient.Stop(ctx) }()

	// Collect all datasource info.
	var all []types.DatasourceInfo

	if dsType == "" || dsType == "clickhouse" {
		all = append(all, proxyClient.ClickHouseDatasourceInfo()...)
	}

	if dsType == "" || dsType == "prometheus" {
		all = append(all, proxyClient.PrometheusDatasourceInfo()...)
	}

	if dsType == "" || dsType == "loki" {
		all = append(all, proxyClient.LokiDatasourceInfo()...)
	}

	if dsJSON || !isTerminal() {
		return outputJSON(all)
	}

	// Human-readable output.
	if len(all) == 0 {
		fmt.Println("No datasources found.")

		return nil
	}

	for _, ds := range all {
		fmt.Printf("%-12s  %-20s", ds.Type, ds.Name)

		if ds.Description != "" {
			fmt.Printf("  %s", ds.Description)
		}

		if len(ds.Metadata) > 0 {
			var meta []string
			for k, v := range ds.Metadata {
				meta = append(meta, fmt.Sprintf("%s=%s", k, v))
			}

			fmt.Printf("  [%s]", strings.Join(meta, " "))
		}

		fmt.Println()
	}

	return nil
}
