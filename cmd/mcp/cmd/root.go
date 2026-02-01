package cmd

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/ethpandaops/mcp/pkg/config"
	"github.com/ethpandaops/mcp/pkg/observability"
)

var (
	cfgFile  string
	logLevel string
	log      = logrus.New()
)

var rootCmd = &cobra.Command{
	Use:   "mcp",
	Short: "ethpandaops MCP server for Ethereum network analytics",
	Long: `An MCP (Model Context Protocol) server that provides AI assistants with
Ethereum network analytics capabilities including ClickHouse blockchain data,
Prometheus metrics, Loki logs, and sandboxed Python execution.`,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		// Load config to get logging settings if available.
		cfg, err := config.Load(cfgFile)
		if err != nil {
			// Fall back to CLI flag if config fails to load.
			level, err := logrus.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			log.SetLevel(level)
			log.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
			})
			return nil
		}

		// Configure logging based on config file.
		loggerCfg := observability.LoggerConfig{
			Level:      observability.LogLevel(cfg.Observability.Logging.Level),
			Format:     observability.LogFormat(cfg.Observability.Logging.Format),
			OutputPath: cfg.Observability.Logging.OutputPath,
		}

		// CLI flag overrides config file.
		if logLevel != "" && logLevel != "info" {
			loggerCfg.Level = observability.LogLevel(logLevel)
		}

		configuredLog, err := observability.ConfigureLogger(loggerCfg)
		if err != nil {
			// Fall back to default logging.
			level, _ := logrus.ParseLevel(logLevel)
			log.SetLevel(level)
			log.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
			})
			return nil
		}

		// Copy settings to the global log.
		log.SetLevel(configuredLog.Level)
		log.SetFormatter(configuredLog.Formatter)
		log.SetOutput(configuredLog.Out)

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: config.yaml or $CONFIG_PATH)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
}
