package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	searchExampleCategory string
	searchExampleLimit    int
	searchExampleJSON     bool
	searchRunbookTag      string
	searchRunbookLimit    int
	searchRunbookJSON     bool
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search examples and runbooks",
	Long: `Semantic search over query examples and investigation runbooks.

Examples:
  ep search examples "attestation participation"
  ep search runbooks "finality delay"`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var searchExamplesCmd = &cobra.Command{
	Use:   "examples <query>",
	Short: "Search query examples",
	Args:  cobra.ExactArgs(1),
	RunE:  forwardSearchHelper,
}

var searchRunbooksCmd = &cobra.Command{
	Use:   "runbooks <query>",
	Short: "Search investigation runbooks",
	Args:  cobra.ExactArgs(1),
	RunE:  forwardSearchHelper,
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.AddCommand(searchExamplesCmd)
	searchCmd.AddCommand(searchRunbooksCmd)

	searchExamplesCmd.Flags().StringVar(&searchExampleCategory, "category", "", "Filter by category")
	searchExamplesCmd.Flags().IntVar(&searchExampleLimit, "limit", 3, "Max results (default: 3, max: 10)")
	searchExamplesCmd.Flags().BoolVar(&searchExampleJSON, "json", false, "Output in JSON format")

	searchRunbooksCmd.Flags().StringVar(&searchRunbookTag, "tag", "", "Filter by tag")
	searchRunbooksCmd.Flags().IntVar(&searchRunbookLimit, "limit", 3, "Max results (default: 3, max: 5)")
	searchRunbooksCmd.Flags().BoolVar(&searchRunbookJSON, "json", false, "Output in JSON format")
}

func forwardSearchHelper(_ *cobra.Command, _ []string) error {
	helperPath, workingDir, err := findSearchHelper()
	if err != nil {
		return err
	}

	cmd := exec.Command(helperPath, searchHelperArgs()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Dir = workingDir
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("search helper failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("running search helper: %w", err)
	}

	return nil
}

func searchHelperArgs() []string {
	for idx := 1; idx < len(os.Args); idx++ {
		if os.Args[idx] == "search" {
			return append([]string(nil), os.Args[idx+1:]...)
		}
	}

	return nil
}

func findSearchHelper() (string, string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("determining executable path: %w", err)
	}

	searchHelperName := "ep-search"
	if runtime.GOOS == "windows" {
		searchHelperName += ".exe"
	}

	helperDir := filepath.Dir(exePath)
	helperPath := filepath.Join(helperDir, searchHelperName)
	if info, statErr := os.Stat(helperPath); statErr == nil && !info.IsDir() {
		workingDir, resolveErr := resolveSearchRuntimeDir(helperDir)
		if resolveErr != nil {
			return "", "", resolveErr
		}
		return helperPath, workingDir, nil
	}

	helperPath, err = exec.LookPath(searchHelperName)
	if err != nil {
		return "", "", fmt.Errorf(
			"search support is not installed. install %q alongside %q or rebuild with `make install`",
			searchHelperName,
			filepath.Base(exePath),
		)
	}

	workingDir, err := resolveSearchRuntimeDir(filepath.Dir(helperPath))
	if err != nil {
		return "", "", err
	}

	return helperPath, workingDir, nil
}

func resolveSearchRuntimeDir(helperDir string) (string, error) {
	for _, dir := range []string{mustGetwd(), helperDir} {
		if dir == "" {
			continue
		}
		if hasSearchRuntime(dir) {
			return dir, nil
		}
	}

	return "", fmt.Errorf(
		"search runtime is not installed. run `make download-models` in the repo or install search assets next to ep-search",
	)
}

func hasSearchRuntime(dir string) bool {
	modelPath := filepath.Join(dir, "models", "MiniLM-L6-v2.Q8_0.gguf")
	if _, err := os.Stat(modelPath); err != nil {
		return false
	}

	libName := "libllama_go.so"
	if runtime.GOOS == "darwin" {
		libName = "libllama_go.dylib"
	} else if runtime.GOOS == "windows" {
		libName = "llama_go.dll"
	}

	_, err := os.Stat(filepath.Join(dir, libName))
	return err == nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}
