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

		// Kick off a background update check if the cache is stale.
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
// updates the local cache file. Runs as a fire-and-forget goroutine;
// errors are silently ignored.
func backgroundUpdateCheck() {
	cache, _ := github.LoadCache()
	if cache != nil && !cache.IsStale() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	checker := github.NewReleaseChecker(github.RepoOwner, github.RepoName)

	release, err := checker.LatestRelease(ctx)
	if err != nil {
		return
	}

	_ = github.SaveCache(&github.UpdateCache{
		LatestVersion: release.TagName,
		CheckedAt:     time.Now(),
	})
}

// printUpdateNotification reads the cached version check and prints
// a one-line update notice to stderr if a newer version is available.
func printUpdateNotification() {
	cache, err := github.LoadCache()
	if err != nil || cache == nil {
		return
	}

	if version.IsNewer(version.Version, cache.LatestVersion) {
		fmt.Fprintf(os.Stderr,
			"\nUpdate available: %s -> %s. Run 'panda upgrade' to update.\n",
			version.Version, cache.LatestVersion,
		)
	}
}
