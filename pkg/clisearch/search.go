package clisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/app"
	"github.com/ethpandaops/mcp/pkg/config"
	"github.com/ethpandaops/mcp/pkg/searchruntime"
	"github.com/ethpandaops/mcp/pkg/searchsvc"
)

var (
	cfgFile  string
	logLevel string
	log      = logrus.New()
)

var rootCmd = &cobra.Command{
	Use:   "ep-search",
	Short: "Search examples and runbooks",
	Long: `Semantic search over query examples and investigation runbooks.

Examples:
  ep-search examples "attestation participation"
  ep-search runbooks "finality delay"`,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return err
		}

		log.SetLevel(level)
		log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
		return nil
	},
	SilenceUsage: true,
}

var (
	searchExCategory string
	searchExLimit    int
	searchExJSON     bool

	searchRbTag   string
	searchRbLimit int
	searchRbJSON  bool
)

var searchExamplesCmd = &cobra.Command{
	Use:   "examples <query>",
	Short: "Search query examples",
	Long: `Semantic search over ClickHouse, Prometheus, Loki, and Dora query examples.
Returns matching examples with SQL/PromQL/LogQL queries and similarity scores.`,
	Args: cobra.ExactArgs(1),
	RunE: runSearchExamples,
}

var searchRunbooksCmd = &cobra.Command{
	Use:   "runbooks <query>",
	Short: "Search investigation runbooks",
	Long: `Semantic search over procedural runbooks for multi-step investigations.
Returns matching runbooks with full content, prerequisites, and tags.`,
	Args: cobra.ExactArgs(1),
	RunE: runSearchRunbooks,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: config.yaml or $CONFIG_PATH)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")

	rootCmd.AddCommand(searchExamplesCmd)
	searchExamplesCmd.Flags().StringVar(&searchExCategory, "category", "", "Filter by category")
	searchExamplesCmd.Flags().IntVar(&searchExLimit, "limit", 3, "Max results (default: 3, max: 10)")
	searchExamplesCmd.Flags().BoolVar(&searchExJSON, "json", false, "Output in JSON format")

	rootCmd.AddCommand(searchRunbooksCmd)
	searchRunbooksCmd.Flags().StringVar(&searchRbTag, "tag", "", "Filter by tag")
	searchRunbooksCmd.Flags().IntVar(&searchRbLimit, "limit", 3, "Max results (default: 3, max: 5)")
	searchRunbooksCmd.Flags().BoolVar(&searchRbJSON, "json", false, "Output in JSON format")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func buildSearchApp(ctx context.Context) (*app.App, *searchruntime.Runtime, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}

	a := app.New(log, cfg)
	if err := a.BuildLight(ctx); err != nil {
		return nil, nil, fmt.Errorf("building app: %w", err)
	}

	runtime, err := searchruntime.Build(log, cfg.SemanticSearch, a.ExtensionRegistry)
	if err != nil {
		_ = a.Stop(ctx)
		return nil, nil, fmt.Errorf("building search runtime: %w", err)
	}

	return a, runtime, nil
}

func runSearchExamples(_ *cobra.Command, args []string) error {
	ctx := context.Background()

	a, runtime, err := buildSearchApp(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.Stop(ctx) }()
	defer func() { _ = runtime.Close() }()

	service := searchsvc.New(runtime.ExampleIndex, a.ExtensionRegistry, runtime.RunbookIndex, runtime.RunbookRegistry)
	response, err := service.SearchExamples(args[0], searchExCategory, searchExLimit)
	if err != nil {
		return err
	}

	if searchExJSON {
		return printJSON(map[string]any{
			"query":   args[0],
			"results": response.Results,
		})
	}

	if len(response.Results) == 0 {
		fmt.Println("No matching examples found.")
		return nil
	}

	for i, r := range response.Results {
		if i > 0 {
			fmt.Println("---")
		}

		fmt.Printf("[%s] %s (score: %.2f)\n", r.CategoryName, r.ExampleName, r.SimilarityScore)
		fmt.Printf("  %s\n", r.Description)

		if r.TargetCluster != "" {
			fmt.Printf("  Cluster: %s\n", r.TargetCluster)
		}

		fmt.Printf("\n%s\n\n", r.Query)
	}

	return nil
}

func runSearchRunbooks(_ *cobra.Command, args []string) error {
	ctx := context.Background()

	a, runtime, err := buildSearchApp(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = a.Stop(ctx) }()
	defer func() { _ = runtime.Close() }()

	service := searchsvc.New(runtime.ExampleIndex, a.ExtensionRegistry, runtime.RunbookIndex, runtime.RunbookRegistry)
	response, err := service.SearchRunbooks(args[0], searchRbTag, searchRbLimit)
	if err != nil {
		return err
	}

	if searchRbJSON {
		return printJSON(map[string]any{
			"query":   args[0],
			"results": response.Results,
		})
	}

	if len(response.Results) == 0 {
		fmt.Println("No matching runbooks found.")
		return nil
	}

	for i, r := range response.Results {
		if i > 0 {
			fmt.Print("\n===\n\n")
		}

		fmt.Printf("%s (score: %.2f)\n", r.Name, r.SimilarityScore)
		fmt.Printf("  %s\n", r.Description)
		fmt.Printf("  Tags: %s\n", strings.Join(r.Tags, ", "))

		if len(r.Prerequisites) > 0 {
			fmt.Printf("  Prerequisites: %s\n", strings.Join(r.Prerequisites, ", "))
		}

		fmt.Printf("\n%s\n", r.Content)
	}

	return nil
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}
