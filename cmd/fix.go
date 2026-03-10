package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/config"
	"github.com/anasskartit/tfoutdated/internal/fixer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

var (
	flagFixSafe bool
)

var fixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Automatically fix outdated dependencies with breaking change transforms",
	Long: `Fix updates version constraints in .tf files, applies variable renames,
value transforms, and provider constraint updates.

By default, upgrades all dependencies (including major bumps) and applies
auto-fixable transforms. Use --safe to only upgrade to non-breaking versions.
Use --dry-run to preview changes without modifying files.`,
	RunE: runFix,
}

func init() {
	fixCmd.Flags().BoolVar(&flagFixSafe, "safe", false, "Upgrade to safest non-breaking version (uses upgrade path targets)")
	rootCmd.AddCommand(fixCmd)
}

func runFix(cmd *cobra.Command, args []string) error {
	path, err := filepath.Abs(flagPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	cfg := config.Load(path)

	s := scanner.New(scanner.Options{
		Path:      path,
		Recursive: flagRecursive,
		Ignores:   cfg.Ignore,
	})
	result, err := s.Scan()
	if err != nil {
		return fmt.Errorf("scanning: %w", err)
	}

	if len(result.Modules) == 0 && len(result.Providers) == 0 {
		fmt.Println("No Terraform dependencies found.")
		return nil
	}

	res := resolver.New(resolver.Options{
		ProviderFilter: flagProviderFilter,
	})
	resolved, err := res.Resolve(result)
	if err != nil {
		return fmt.Errorf("resolving versions: %w", err)
	}

	a := analyzer.New()
	analysis := a.Analyze(resolved)

	analysis.BreakingChanges = detectAllBreakingChanges(resolved)

	analysis.UpgradePaths = analyzer.ComputeUpgradePaths(resolved, analysis.BreakingChanges)

	f := fixer.New(fixer.Options{
		DryRun:  flagDryRun,
		SafeOnly: flagFixSafe,
	})
	changes, err := f.Fix(analysis)
	if err != nil {
		return fmt.Errorf("applying fixes: %w", err)
	}

	// Apply module call transforms (variable renames/removals from breaking changes)
	// Only for modules that were actually version-bumped
	moduleChanges, mcErr := f.FixModuleCalls(analysis, result, changes)
	if mcErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: error applying module transforms: %v\n", mcErr)
	}

	// Fix provider constraints that need widening due to module upgrades
	var providerChanges []fixer.ProviderChange
	if len(changes) > 0 {
		pc, pcErr := f.FixProviderConstraints(changes, result, res.GetRegistry())
		if pcErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: error fixing provider constraints: %v\n", pcErr)
		} else {
			providerChanges = pc
		}
	}

	if len(changes) == 0 && len(moduleChanges) == 0 && len(providerChanges) == 0 {
		fmt.Println("No safe upgrades available.")
		return nil
	}

	for _, c := range changes {
		if flagDryRun {
			fmt.Printf("[dry-run] %s: %s %s → %s\n", c.FilePath, c.Name, c.OldVersion, c.NewVersion)
		} else {
			fmt.Printf("Updated %s: %s %s → %s\n", c.FilePath, c.Name, c.OldVersion, c.NewVersion)
		}
	}

	for _, mc := range moduleChanges {
		if flagDryRun {
			fmt.Printf("[dry-run] %s: module %q %s %s → %s\n", mc.FilePath, mc.Module, mc.Type, mc.Old, mc.New)
		} else {
			fmt.Printf("Transformed %s: module %q %s %s → %s\n", mc.FilePath, mc.Module, mc.Type, mc.Old, mc.New)
		}
	}

	for _, pc := range providerChanges {
		if flagDryRun {
			fmt.Printf("[dry-run] %s: provider %s %s → %s (%s)\n", pc.FilePath, pc.ProviderName, pc.OldConstraint, pc.NewConstraint, pc.Reason)
		} else {
			fmt.Printf("Updated %s: provider %s %s → %s (%s)\n", pc.FilePath, pc.ProviderName, pc.OldConstraint, pc.NewConstraint, pc.Reason)
		}
	}

	totalUpdates := len(changes) + len(providerChanges)
	fmt.Printf("\n%d dependencies %s.\n", totalUpdates, map[bool]string{true: "would be updated", false: "updated"}[flagDryRun])
	return nil
}
