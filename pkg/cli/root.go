// Package cli provides the command-line interface for ethpandaops Ethereum analytics.
package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/ethpandaops/panda/internal/github"
	"github.com/ethpandaops/panda/internal/version"
)

var (
	cfgFile  string
	logLevel string
	log      = logrus.New()
)

// updateResult carries the latest version from the background check.
// A nil value means the check failed or was skipped.
var updateResult = make(chan *string, 1)

// skipUpdateCheckCommands lists commands that should not trigger
// update checks or display update notifications.
var skipUpdateCheckCommands = map[string]bool{
	"upgrade":    true,
	"version":    true,
	"completion": true,
	"init":       true,
	"help":       true,
}

var rootCmd = &cobra.Command{
	Use:   "panda",
	Short: "Ethereum network analytics CLI",
	Long: `A CLI for Ethereum network analytics with access to ClickHouse blockchain data,
Prometheus metrics, Loki logs, and sandboxed Python execution.

Run 'panda <command> --help' for details on any command.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			return err
		}

		log.SetLevel(level)
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})

		if !skipUpdateCheckCommands[cmd.Name()] {
			go backgroundUpdateCheck()
		}

		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
		if skipUpdateCheckCommands[cmd.Name()] {
			return nil
		}

		printUpdateNotification()

		return nil
	},
	SilenceUsage: true,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default: $PANDA_CONFIG, ~/.config/panda/config.yaml, or ./config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"log level (debug, info, warn, error)")

	_ = rootCmd.RegisterFlagCompletionFunc("log-level", cobra.FixedCompletions(
		[]string{"debug", "info", "warn", "error"}, cobra.ShellCompDirectiveNoFileComp,
	))
	_ = rootCmd.RegisterFlagCompletionFunc("config",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"yaml", "yml"}, cobra.ShellCompDirectiveFilterFileExt
		})
}

// backgroundUpdateCheck queries GitHub for the latest release and
// sends the result through the updateResult channel.
func backgroundUpdateCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	checker := github.NewReleaseChecker(github.RepoOwner, github.RepoName)

	release, err := checker.LatestRelease(ctx)
	if err != nil {
		updateResult <- nil
		return
	}

	updateResult <- &release.TagName
}

// printUpdateNotification waits briefly for the background check and
// prints a one-line update notice to stderr if a newer version exists.
func printUpdateNotification() {
	var latestVersion *string

	select {
	case latestVersion = <-updateResult:
	case <-time.After(2 * time.Second):
		return
	}

	if latestVersion == nil {
		return
	}

	if version.IsNewer(version.Version, *latestVersion) {
		fmt.Fprintf(os.Stderr,
			"\nUpdate available: %s -> %s. Run 'panda upgrade' to update.\n",
			version.Version, *latestVersion,
		)
	}
}
