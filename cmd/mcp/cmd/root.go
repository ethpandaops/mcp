package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// ExitCodeError is returned by commands that need to exit with a specific code.
type ExitCodeError struct {
	Code int
}

func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

var (
	cfgFile  string
	logLevel string
	log      = logrus.New()
)

var rootCmd = &cobra.Command{
	Use:   "ethpandaops-mcp",
	Short: "ethpandaops MCP server for Ethereum network analytics",
	Long: `An MCP (Model Context Protocol) server that provides AI assistants with
Ethereum network analytics capabilities including ClickHouse blockchain data,
Prometheus metrics, Loki logs, and sandboxed Python execution.`,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return err
		}

		log.SetLevel(level)
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})

		return nil
	},
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var exitErr *ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}

		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: config.yaml or $CONFIG_PATH)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
}
