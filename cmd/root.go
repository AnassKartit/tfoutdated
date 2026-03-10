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
)

var rootCmd = &cobra.Command{
	Use:   "tfoutdated",
	Short: "Detect outdated Terraform modules and providers",
	Long: `tfoutdated scans Terraform configurations for outdated modules and providers,
performs impact analysis of upgrades, detects breaking changes, and recommends
safe upgrade paths.

It supports Azure providers (azurerm, azuread) and Azure Verified Modules (AVM)
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
}

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

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}
	return nil
}
