package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	if flagNoColor {
		lipgloss.SetColorProfile(0)
	}
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	cyan := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	bold := lipgloss.NewStyle().Bold(true)
	red := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)

	path, err := filepath.Abs(flagPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if flagDryRun {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, yellow.Render("  ▸ DRY RUN — no files will be modified"))
		fmt.Fprintln(os.Stdout)
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
		fmt.Println(dim.Render("  No Terraform dependencies found."))
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
		DryRun:   flagDryRun,
		SafeOnly: flagFixSafe,
	})
	changes, err := f.Fix(analysis)
	if err != nil {
		return fmt.Errorf("applying fixes: %w", err)
	}

	moduleChanges, mcErr := f.FixModuleCalls(analysis, result, changes)
	if mcErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: error applying module transforms: %v\n", mcErr)
	}

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
		fmt.Println(dim.Render("  All dependencies are up to date."))
		return nil
	}

	// Group changes by file
	type entry struct {
		icon, text string
	}
	fileGroups := map[string][]entry{}
	fileOrder := []string{}
	addToGroup := func(file string, e entry) {
		if _, ok := fileGroups[file]; !ok {
			fileOrder = append(fileOrder, file)
		}
		fileGroups[file] = append(fileGroups[file], e)
	}

	versionCount, renameCount, providerCount := 0, 0, 0

	for _, c := range changes {
		versionCount++
		addToGroup(c.FilePath, entry{
			icon: green.Render("✓"),
			text: fmt.Sprintf("%s  %s → %s",
				bold.Render(c.Name),
				dim.Render(c.OldVersion),
				green.Render(c.NewVersion)),
		})
	}

	for _, mc := range moduleChanges {
		renameCount++
		addToGroup(mc.FilePath, entry{
			icon: yellow.Render("↻"),
			text: fmt.Sprintf("%s  %s %s → %s",
				bold.Render(mc.Module),
				dim.Render(mc.Type),
				dim.Render(mc.Old),
				yellow.Render(mc.New)),
		})
	}

	for _, pc := range providerChanges {
		providerCount++
		addToGroup(pc.FilePath, entry{
			icon: cyan.Render("⚡"),
			text: fmt.Sprintf("%s  %s → %s  %s",
				bold.Render(pc.ProviderName),
				dim.Render(pc.OldConstraint),
				cyan.Render(pc.NewConstraint),
				dim.Render(pc.Reason)),
		})
	}

	fmt.Fprintln(os.Stdout)
	for _, file := range fileOrder {
		relFile, err := filepath.Rel(path, file)
		if err != nil {
			relFile = file
		}
		fmt.Fprintf(os.Stdout, "  %s\n", dim.Render(relFile))
		for _, e := range fileGroups[file] {
			fmt.Fprintf(os.Stdout, "    %s %s\n", e.icon, e.text)
		}
		fmt.Fprintln(os.Stdout)
	}

	// Summary line
	total := versionCount + renameCount + providerCount
	parts := []string{}
	if versionCount > 0 {
		parts = append(parts, green.Render(fmt.Sprintf("%d upgraded", versionCount)))
	}
	if renameCount > 0 {
		parts = append(parts, yellow.Render(fmt.Sprintf("%d renamed", renameCount)))
	}
	if providerCount > 0 {
		parts = append(parts, cyan.Render(fmt.Sprintf("%d constraints", providerCount)))
	}

	verb := "applied"
	if flagDryRun {
		verb = "would apply"
	}

	fmt.Fprintf(os.Stdout, "  %s %s  %s\n\n",
		red.Render(fmt.Sprintf("%d", total)),
		bold.Render("changes "+verb+":"),
		strings.Join(parts, dim.Render(" · ")))

	return nil
}
