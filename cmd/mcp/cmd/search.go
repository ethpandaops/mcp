package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/embedding"
	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/ethpandaops/mcp/pkg/resource"
	"github.com/ethpandaops/mcp/runbooks"

	clickhouseplugin "github.com/ethpandaops/mcp/plugins/clickhouse"
	lokiplugin "github.com/ethpandaops/mcp/plugins/loki"
	prometheusplugin "github.com/ethpandaops/mcp/plugins/prometheus"
)

const defaultSearchLimit = 5

var (
	searchQuery    string
	searchCategory string
	searchTag      string
	searchLimit    int
	searchJSON     bool
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search examples and runbooks",
	Long:  `Search for query examples and investigation runbooks using semantic search.`,
}

var searchExamplesCmd = &cobra.Command{
	Use:   "examples",
	Short: "Search query examples",
	Long: `Search for query examples using semantic similarity.

Examples:
  ethpandaops-mcp search examples --query "block arrival time"
  ethpandaops-mcp search examples --query "attestation" --category attestations --limit 3`,
	RunE: runSearchExamples,
}

var searchRunbooksCmd = &cobra.Command{
	Use:   "runbooks",
	Short: "Search investigation runbooks",
	Long: `Search for investigation runbooks using semantic similarity.

Examples:
  ethpandaops-mcp search runbooks --query "finality delay"
  ethpandaops-mcp search runbooks --query "slow queries" --tag performance`,
	RunE: runSearchRunbooks,
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.AddCommand(searchExamplesCmd)
	searchCmd.AddCommand(searchRunbooksCmd)

	// Shared flags.
	searchCmd.PersistentFlags().StringVar(&searchQuery, "query", "", "Search query (required)")
	searchCmd.PersistentFlags().IntVar(&searchLimit, "limit", defaultSearchLimit, "Maximum number of results")
	searchCmd.PersistentFlags().BoolVar(&searchJSON, "json", false, "Output in JSON format")

	_ = searchCmd.MarkPersistentFlagRequired("query")

	// Example-specific flags.
	searchExamplesCmd.Flags().StringVar(&searchCategory, "category", "", "Filter by category")

	// Runbook-specific flags.
	searchRunbooksCmd.Flags().StringVar(&searchTag, "tag", "", "Filter by tag")
}

// resolveModelPath finds the embedding model file, checking multiple locations.
func resolveModelPath() (string, error) {
	// If config has a path and it exists, use it.
	if cfgFile != "" {
		cfg, err := loadConfigOrDefaults(cfgFile)
		if err == nil && cfg.SemanticSearch.ModelPath != "" {
			if _, err := os.Stat(cfg.SemanticSearch.ModelPath); err == nil {
				return cfg.SemanticSearch.ModelPath, nil
			}
		}
	}

	// Check common paths.
	paths := []string{
		"models/MiniLM-L6-v2.Q8_0.gguf",
		"/usr/share/mcp/MiniLM-L6-v2.Q8_0.gguf",
	}

	// Check user cache dir.
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, home+"/.cache/ethpandaops-mcp/models/MiniLM-L6-v2.Q8_0.gguf")
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf(
		"embedding model not found. Run 'make download-models' or download MiniLM-L6-v2.Q8_0.gguf to models/",
	)
}

// buildExampleSearchIndex creates a plugin registry (for examples) and search index.
func buildExampleSearchIndex() (*resource.ExampleIndex, error) {
	modelPath, err := resolveModelPath()
	if err != nil {
		return nil, err
	}

	embedder, err := embedding.New(modelPath, 0)
	if err != nil {
		return nil, fmt.Errorf("loading embedding model: %w", err)
	}

	// Create plugin registry just for examples (no init needed).
	reg := plugin.NewRegistry(log)
	reg.Add(clickhouseplugin.New())
	reg.Add(prometheusplugin.New())
	reg.Add(lokiplugin.New())

	categories := resource.GetQueryExamples(reg)

	index, err := resource.NewExampleIndex(log, embedder, categories)
	if err != nil {
		_ = embedder.Close()
		return nil, fmt.Errorf("building search index: %w", err)
	}

	return index, nil
}

func runSearchExamples(_ *cobra.Command, _ []string) error {
	suppressLogs()

	index, err := buildExampleSearchIndex()
	if err != nil {
		return err
	}

	defer func() { _ = index.Close() }()

	results, err := index.Search(searchQuery, searchLimit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by category if specified.
	if searchCategory != "" {
		filtered := make([]resource.SearchResult, 0, len(results))

		for _, r := range results {
			if r.CategoryKey == searchCategory {
				filtered = append(filtered, r)
			}
		}

		results = filtered
	}

	if searchJSON || !isTerminal() {
		return outputJSON(results)
	}

	// Human-readable output.
	if len(results) == 0 {
		fmt.Println("No examples found.")

		return nil
	}

	for i, r := range results {
		if i > 0 {
			fmt.Println()
		}

		fmt.Printf("## %s (%.0f%% match)\n", r.Example.Name, r.Score*100)
		fmt.Printf("Category: %s | Cluster: %s\n", r.CategoryName, r.Example.Cluster)

		if r.Example.Description != "" {
			fmt.Printf("Description: %s\n", r.Example.Description)
		}

		fmt.Printf("\n%s\n", strings.TrimSpace(r.Example.Query))
	}

	return nil
}

func runSearchRunbooks(_ *cobra.Command, _ []string) error {
	suppressLogs()

	modelPath, err := resolveModelPath()
	if err != nil {
		return err
	}

	embedder, err := embedding.New(modelPath, 0)
	if err != nil {
		return fmt.Errorf("loading embedding model: %w", err)
	}

	runbookReg, err := runbooks.NewRegistry(log)
	if err != nil {
		_ = embedder.Close()
		return fmt.Errorf("loading runbooks: %w", err)
	}

	index, err := resource.NewRunbookIndex(log, embedder, runbookReg.All())
	if err != nil {
		_ = embedder.Close()
		return fmt.Errorf("building search index: %w", err)
	}

	results, err := index.Search(searchQuery, searchLimit)
	if err != nil {
		_ = embedder.Close()
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by tag if specified.
	if searchTag != "" {
		filtered := make([]resource.RunbookSearchResult, 0, len(results))

		for _, r := range results {
			for _, t := range r.Runbook.Tags {
				if t == searchTag {
					filtered = append(filtered, r)

					break
				}
			}
		}

		results = filtered
	}

	if searchJSON || !isTerminal() {
		return outputJSON(results)
	}

	// Human-readable output.
	if len(results) == 0 {
		fmt.Println("No runbooks found.")

		return nil
	}

	for i, r := range results {
		if i > 0 {
			fmt.Println()
		}

		fmt.Printf("## %s (%.0f%% match)\n", r.Runbook.Name, r.Score*100)
		fmt.Printf("Description: %s\n", r.Runbook.Description)

		if len(r.Runbook.Tags) > 0 {
			fmt.Printf("Tags: %s\n", strings.Join(r.Runbook.Tags, ", "))
		}

		fmt.Printf("\n%s\n", strings.TrimSpace(r.Runbook.Content))
	}

	_ = embedder.Close()

	return nil
}
