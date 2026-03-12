package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ethpandaops/panda/pkg/types"
)

var gettingStartedCmd = &cobra.Command{
	GroupID: groupDiscovery,
	Use:     "getting-started",
	Short:   "Show the getting started guide",
	Long: `Display the getting started guide with workflow guidance, available commands,
and critical syntax rules for querying Ethereum data.

Examples:
  panda getting-started
  panda getting-started -o json`,
	RunE: runGettingStarted,
}

func init() {
	rootCmd.AddCommand(gettingStartedCmd)
}

func runGettingStarted(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	response, err := readResourceWithClientContext(ctx, "panda://getting-started", types.ClientContextCLIParam)
	if err != nil {
		return fmt.Errorf("reading getting-started guide: %w", err)
	}

	if isJSON() {
		return printJSON(response)
	}

	fmt.Print(response.Content)

	return nil
}
