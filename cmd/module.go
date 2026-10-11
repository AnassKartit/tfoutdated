package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/anasskartit/tfoutdated/internal/resolver"
)

var moduleCmd = &cobra.Command{
	Use:   "module <source>",
	Short: "Tell whether a module is safe to use: deprecated, archived, moved, latest version",
	Long: `Module looks up one module source — before you write it into a configuration.
It reports the latest version and whether the module is deprecated, archived
or moved upstream, with the replacement when one is named.

Exit codes: 0 usable, 1 error, 2 deprecated or archived.`,
	Example: `  tfoutdated module Azure/avm-ptn-virtualwan/azurerm
  tfoutdated module terraform-aws-modules/vpc/aws -format=json
  tfoutdated module "git::https://github.com/org/repo.git?ref=v1.2.0"`,
	Args: cobra.ExactArgs(1),
	RunE: runModule,
}

func init() {
	rootCmd.AddCommand(moduleCmd)
}

func runModule(cmd *cobra.Command, args []string) error {
	res := resolver.New(resolver.Options{SkipLifecycle: flagOffline})
	st, err := res.ModuleStatus(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	if flagJSON || flagOutput == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(st); err != nil {
			return err
		}
	} else {
		printModuleStatus(st)
	}

	if st.Deprecated {
		return &ExitCodeError{Code: ExitFindings, Message: "module is deprecated"}
	}
	return nil
}

func printModuleStatus(st *resolver.ModuleStatus) {
	if flagNoColor {
		lipgloss.SetColorProfile(0)
	}
	bad := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	ok := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	warn := lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))

	fmt.Println()
	switch {
	case st.Deprecated:
		fmt.Printf("  %s %s\n", bad.Render("✗ deprecated"), st.Source)
	case st.MovedTo != "":
		fmt.Printf("  %s %s\n", warn.Render("! moved"), st.Source)
	default:
		fmt.Printf("  %s %s\n", ok.Render("✓ ok"), st.Source)
	}
	row := func(k, v string) {
		if v != "" {
			fmt.Printf("    %-12s %s\n", dim.Render(k), v)
		}
	}
	row("latest", st.LatestVersion)
	if st.ReplacedBy != "" {
		row("replaced by", cyan.Render(st.ReplacedBy))
	}
	row("why", st.Reason)
	row("repository", st.RepoURL)
	fmt.Printf("\n  %s\n\n", st.Advice)
}
