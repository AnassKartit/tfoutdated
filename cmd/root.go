package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Exit codes
const (
	ExitUpToDate        = 0
	ExitUpdatesAvail    = 1
	ExitBreakingChanges = 2
)

var (
	flagPath           string
	flagRecursive      bool
	flagOutput         string
	flagDryRun         bool
	flagFix            bool
	flagProviderFilter []string
	flagSeverity       string
	flagNoColor        bool
	flagJSON           bool
	flagReposFile      string
	flagOutputFile     string
	flagVerbose        bool
)

var rootCmd = &cobra.Command{
	Use:   "tfoutdated",
	Short: "Detect outdated Terraform modules and providers",
	Long: `tfoutdated scans Terraform configurations for outdated modules and providers,
performs impact analysis of upgrades, detects breaking changes, and recommends
safe upgrade paths.

It supports multi-cloud providers (AWS, Azure, GCP) and verified modules
with built-in breaking change knowledge.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		cmd.Help()
	}

	rootCmd.PersistentFlags().StringVarP(&flagPath, "path", "p", ".", "Path to Terraform configuration directory")
	rootCmd.PersistentFlags().BoolVarP(&flagRecursive, "recursive", "r", true, "Recursively scan subdirectories")
	rootCmd.PersistentFlags().StringVarP(&flagOutput, "output", "o", "table", "Output format: table, json, markdown, html, github, azdevops")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "Show what would change without modifying files")
	rootCmd.PersistentFlags().StringSliceVar(&flagProviderFilter, "provider-filter", nil, "Only check specific providers (e.g., azurerm,azuread)")
	rootCmd.PersistentFlags().StringVar(&flagSeverity, "severity", "patch", "Minimum severity to report: patch, minor, major")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Shorthand for --output json")
	rootCmd.PersistentFlags().StringVar(&flagReposFile, "repos", "", "File with repo URLs/paths (one per line)")
	rootCmd.PersistentFlags().StringVar(&flagOutputFile, "output-file", "", "Write report to file (auto-detects format from extension: .html, .md, .json)")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Show all breaking changes (no truncation)")

	// Short names in the terraform style; they set the same values as the
	// longer ones above, which stay for existing scripts.
	rootCmd.PersistentFlags().StringVar(&flagOutput, "format", "table", "Same as --output")
	rootCmd.PersistentFlags().StringVar(&flagOutputFile, "out", "", "Same as --output-file")
	rootCmd.PersistentFlags().BoolVar(&flagOffline, "offline", false, "Skip upstream lifecycle checks (GitHub, AVM index); registry only")

	// Every flag can also come from TFOUTDATED_<FLAG>, e.g. TFOUTDATED_FAIL_ON.
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return applyEnv(cmd)
	}
}

var flagOffline bool

// formatFromFilename detects output format from file extension.
func formatFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".html", ".htm":
		return "html"
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	default:
		return ""
	}
}

// Execute runs the root command. Use ExitCode on the result for the process
// exit status.
func Execute() error {
	rootCmd.SetArgs(normalizeArgs(os.Args[1:], longFlagNames(rootCmd)))
	if err := rootCmd.Execute(); err != nil {
		if ExitCode(err) == ExitFindings {
			// Findings are a result, not a failure of the tool.
			fmt.Fprintf(os.Stderr, "\n%v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		return err
	}
	return nil
}

// resolveTargetPath returns the directory to operate on.
//
// A positional argument wins over --path so that "tfoutdated scan ./infra" behaves
// the way every other CLI does. Without this, cobra's default ArbitraryArgs policy
// accepted the argument and silently discarded it, and the command ran against the
// --path default of "." — scanning, or worse rewriting, the whole working tree
// instead of the directory the user named.
func resolveTargetPath(args []string) (string, error) {
	target := flagPath
	if len(args) > 0 && args[0] != "" {
		if flagPath != "." && flagPath != args[0] {
			return "", fmt.Errorf("conflicting paths: argument %q and --path %q; pass only one", args[0], flagPath)
		}
		target = args[0]
	}

	// Comma-separated multi-path support is handled by the caller; keep it intact.
	if strings.Contains(target, ",") {
		return target, nil
	}

	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolving path %q: %w", target, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("path %q: %w", target, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is a file; pass the directory that contains it", target)
	}
	return abs, nil
}
