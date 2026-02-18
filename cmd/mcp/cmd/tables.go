package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/defaults"
	clickhouseplugin "github.com/ethpandaops/mcp/plugins/clickhouse"
)

var (
	tablesJSON     bool
	tablesProxyURL string
	tablesIssuer   string
	tablesClientID string
)

var tablesCmd = &cobra.Command{
	Use:   "tables [table-name]",
	Short: "List ClickHouse tables or show table schema",
	Long: `List available ClickHouse tables or show detailed schema for a specific table.

Requires the proxy to have ClickHouse datasources with schema discovery.

Examples:
  ethpandaops-mcp tables
  ethpandaops-mcp tables fct_block_canonical
  ethpandaops-mcp tables --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTables,
}

func init() {
	rootCmd.AddCommand(tablesCmd)

	tablesCmd.Flags().BoolVar(&tablesJSON, "json", false, "Output in JSON format")
	tablesCmd.Flags().StringVar(&tablesProxyURL, "proxy-url", defaults.ProxyURL, "Proxy server URL")
	tablesCmd.Flags().StringVar(&tablesIssuer, "issuer", defaults.IssuerURL, "OIDC issuer URL")
	tablesCmd.Flags().StringVar(&tablesClientID, "client-id", defaults.ClientID, "OAuth client ID")
}

func runTables(_ *cobra.Command, args []string) error {
	suppressLogs()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Load config for ClickHouse plugin setup.
	cfg, err := loadConfigOrDefaults(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Build proxy client.
	proxyClient, err := buildProxyClient(ctx, tablesProxyURL, tablesIssuer, tablesClientID)
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

	// Start plugins to trigger schema discovery.
	if err := pluginReg.StartAll(ctx); err != nil {
		return fmt.Errorf("starting plugins: %w", err)
	}

	defer pluginReg.StopAll(ctx)

	// Find the ClickHouse plugin's schema client.
	chPlugin := pluginReg.Get("clickhouse")
	if chPlugin == nil {
		return fmt.Errorf("clickhouse plugin not available")
	}

	chp, ok := chPlugin.(*clickhouseplugin.Plugin)
	if !ok {
		return fmt.Errorf("unexpected plugin type for clickhouse")
	}

	schemaClient := chp.SchemaClient()
	if schemaClient == nil {
		return fmt.Errorf("schema discovery not enabled (configure schema_discovery in config)")
	}

	// Poll for schema data (discovery is async).
	var allTables map[string]*clickhouseplugin.ClusterTables

	deadline := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)

	defer ticker.Stop()

	for {
		allTables = schemaClient.GetAllTables()
		if len(allTables) > 0 {
			break
		}

		select {
		case <-deadline:
			return fmt.Errorf("timeout waiting for schema discovery")
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Handle specific table lookup.
	if len(args) > 0 {
		return showTable(schemaClient, args[0])
	}

	// List all tables.
	return listTables(allTables)
}

func showTable(client clickhouseplugin.ClickHouseSchemaClient, tableName string) error {
	schema, cluster, found := client.GetTable(tableName)
	if !found {
		return fmt.Errorf("table %q not found", tableName)
	}

	type tableDetail struct {
		Name    string                        `json:"name"`
		Cluster string                        `json:"cluster"`
		Schema  *clickhouseplugin.TableSchema `json:"schema"`
	}

	detail := tableDetail{
		Name:    tableName,
		Cluster: cluster,
		Schema:  schema,
	}

	if tablesJSON || !isTerminal() {
		return outputJSON(detail)
	}

	// Human-readable output.
	fmt.Printf("Table: %s (cluster: %s)\n", tableName, cluster)

	if schema.Engine != "" {
		fmt.Printf("Engine: %s\n", schema.Engine)
	}

	if schema.Comment != "" {
		fmt.Printf("Comment: %s\n", schema.Comment)
	}

	if len(schema.Networks) > 0 {
		fmt.Printf("Networks: %s\n", strings.Join(schema.Networks, ", "))
	}

	fmt.Printf("\nColumns (%d):\n", len(schema.Columns))

	for _, col := range schema.Columns {
		comment := ""
		if col.Comment != "" {
			comment = fmt.Sprintf("  -- %s", col.Comment)
		}

		fmt.Printf("  %-40s %s%s\n", col.Name, col.Type, comment)
	}

	return nil
}

func listTables(allTables map[string]*clickhouseplugin.ClusterTables) error {
	type tableEntry struct {
		Name    string `json:"name"`
		Cluster string `json:"cluster"`
		Engine  string `json:"engine,omitempty"`
	}

	var entries []tableEntry

	for clusterName, ct := range allTables {
		for tableName, schema := range ct.Tables {
			entries = append(entries, tableEntry{
				Name:    tableName,
				Cluster: clusterName,
				Engine:  schema.Engine,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Cluster != entries[j].Cluster {
			return entries[i].Cluster < entries[j].Cluster
		}

		return entries[i].Name < entries[j].Name
	})

	if tablesJSON || !isTerminal() {
		return outputJSON(entries)
	}

	if len(entries) == 0 {
		fmt.Println("No tables found.")

		return nil
	}

	currentCluster := ""

	for _, e := range entries {
		if e.Cluster != currentCluster {
			if currentCluster != "" {
				fmt.Println()
			}

			fmt.Printf("## %s\n", e.Cluster)

			currentCluster = e.Cluster
		}

		fmt.Printf("  %s\n", e.Name)
	}

	return nil
}
