package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/charmbracelet/lipgloss"
)

// TableRenderer renders analysis results as a styled console table.
type TableRenderer struct {
	NoColor bool
}

func (r *TableRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	if r.NoColor {
		lipgloss.SetColorProfile(0)
	}

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	majorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red
	minorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226")) // yellow
	patchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))  // green
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// Calculate column widths
	depW, curW, latW := 30, 10, 10

	for _, dep := range analysis.Dependencies {
		name := dep.Source
		if dep.IsModule {
			name = dep.Source
		}
		if len(name) > depW {
			depW = len(name)
		}
		if len(dep.CurrentVer) > curW {
			curW = len(dep.CurrentVer)
		}
		if len(dep.LatestVer) > latW {
			latW = len(dep.LatestVer)
		}
	}

	// Cap width
	if depW > 50 {
		depW = 50
	}

	typeW := 7
	impactW := 12
	filesW := 10

	// Print header
	fmt.Fprintf(w, "\n")
	header := fmt.Sprintf(" %-*s  %-*s  %-*s  %-*s  %-*s  %-*s",
		depW, "DEPENDENCY",
		curW, "CURRENT",
		latW, "LATEST",
		typeW, "TYPE",
		impactW, "IMPACT",
		filesW, "FILES",
	)
	fmt.Fprintln(w, headerStyle.Render(header))
	fmt.Fprintln(w, strings.Repeat("─", depW+curW+latW+typeW+impactW+filesW+12))

	// Print rows
	for _, dep := range analysis.Dependencies {
		name := dep.Source
		if len(name) > depW {
			name = name[:depW-1] + "…"
		}

		// Color the update type
		var typeStr string
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			typeStr = majorStyle.Render("MAJOR")
		case resolver.UpdateMinor:
			typeStr = minorStyle.Render("MINOR")
		case resolver.UpdatePatch:
			typeStr = patchStyle.Render("PATCH")
		}

		// Count breaking changes for this dependency
		breakCount := countBreakingForDep(analysis.BreakingChanges, dep)
		var impactStr string
		if breakCount > 0 {
			impactStr = majorStyle.Render(fmt.Sprintf("%d break", breakCount))
		} else {
			impactStr = dimStyle.Render("none")
		}

		// Count files
		fileCount := countFilesForDep(analysis.Dependencies, dep)
		filesStr := fmt.Sprintf("%d files", fileCount)

		fmt.Fprintf(w, " %-*s  %-*s  %-*s  %-*s  %-*s  %-*s\n",
			depW, name,
			curW, dep.CurrentVer,
			latW, dep.LatestVer,
			typeW+len(typeStr)-len(stripAnsi(typeStr)), typeStr,
			impactW+len(impactStr)-len(stripAnsi(impactStr)), impactStr,
			filesW, filesStr,
		)
	}

	fmt.Fprintln(w, strings.Repeat("─", depW+curW+latW+typeW+impactW+filesW+12))

	// Summary
	fmt.Fprintf(w, "\n%s\n", analysis.Summary())

	// Breaking change details
	if len(analysis.BreakingChanges) > 0 {
		fmt.Fprintf(w, "\n")
		fmt.Fprintln(w, headerStyle.Render("Breaking Changes:"))
		fmt.Fprintln(w, "")

		for i, bc := range analysis.BreakingChanges {
			sevStyle := dimStyle
			if bc.Severity >= breaking.SeverityBreaking {
				sevStyle = majorStyle
			} else if bc.Severity >= breaking.SeverityWarning {
				sevStyle = minorStyle
			}

			fmt.Fprintf(w, "  %s %s %s\n",
				dimStyle.Render(fmt.Sprintf("%d.", i+1)),
				sevStyle.Render("["+bc.Severity.String()+"]"),
				bc.Description)

			if bc.EffortLevel != "" {
				fmt.Fprintf(w, "     %s %s\n", dimStyle.Render("Effort:"), bc.EffortEmoji())
			}
			if bc.MigrationGuide != "" {
				fmt.Fprintf(w, "     %s %s\n", dimStyle.Render("Fix:"), bc.MigrationGuide)
			}

			// Show real user code snippets if available from impact analysis
			realSnippetShown := false
			for _, impact := range analysis.Impacts {
				if impact.ActualBefore == "" || impact.BreakingChange.ResourceType != bc.ResourceType {
					continue
				}
				if impact.BreakingChange.Attribute != bc.Attribute {
					continue
				}
				realSnippetShown = true
				fmt.Fprintf(w, "\n     %s %s (%s:%d)\n",
					dimStyle.Render("Your code:"), impact.ResourceName, impact.AffectedFile, impact.AffectedLine)
				if impact.LinesChanged > 0 {
					fmt.Fprintf(w, "     %s %d lines to change\n", dimStyle.Render("Scope:"), impact.LinesChanged)
				}
				fmt.Fprintf(w, "     %s\n", dimStyle.Render("Before:"))
				for _, line := range strings.Split(impact.ActualBefore, "\n") {
					fmt.Fprintf(w, "     %s  %s\n", majorStyle.Render("-"), line)
				}
				if impact.ActualAfter != "" {
					fmt.Fprintf(w, "     %s\n", dimStyle.Render("After:"))
					for _, line := range strings.Split(impact.ActualAfter, "\n") {
						fmt.Fprintf(w, "     %s  %s\n", patchStyle.Render("+"), line)
					}
				}
			}

			// Fall back to generic knowledge base snippets
			if !realSnippetShown && bc.BeforeSnippet != "" && bc.AfterSnippet != "" {
				fmt.Fprintf(w, "\n     %s\n", dimStyle.Render("Before (generic example):"))
				for _, line := range strings.Split(bc.BeforeSnippet, "\n") {
					fmt.Fprintf(w, "     %s  %s\n", majorStyle.Render("-"), line)
				}
				fmt.Fprintf(w, "     %s\n", dimStyle.Render("After (generic example):"))
				for _, line := range strings.Split(bc.AfterSnippet, "\n") {
					fmt.Fprintf(w, "     %s  %s\n", patchStyle.Render("+"), line)
				}
			}
			fmt.Fprintln(w)
		}
	}

	// Upgrade paths
	if len(analysis.UpgradePaths) > 0 {
		fmt.Fprintln(w, headerStyle.Render("Recommended Upgrade Paths:"))
		fmt.Fprintln(w, "")

		for _, path := range analysis.UpgradePaths {
			fmt.Fprintf(w, "  %s:\n", path.Name)
			for i, step := range path.Steps {
				marker := "→"
				style := patchStyle
				if !step.Safe {
					marker = "⚠"
					style = majorStyle
				}
				fmt.Fprintf(w, "    %d. %s %s %s\n",
					i+1,
					style.Render(marker),
					step.From,
					style.Render("→ "+step.To),
				)
			}
			if path.NonBreakingTarget != "" {
				fmt.Fprintf(w, "    %s Safe target (no code changes): %s\n", patchStyle.Render("✓"), path.NonBreakingTarget)
			} else if path.HasBreakingSteps() {
				fmt.Fprintf(w, "    %s Breaking changes — review required before upgrading\n", majorStyle.Render("⚠"))
			}
			fmt.Fprintln(w)
		}
	}

	// Alignment issues
	if len(analysis.Alignments) > 0 {
		fmt.Fprintln(w, headerStyle.Render("Version Alignment Issues:"))
		fmt.Fprintln(w, "")
		for _, issue := range analysis.Alignments {
			fmt.Fprintf(w, "  %s used at multiple versions:\n", issue.Name)
			for ver, files := range issue.Versions {
				fmt.Fprintf(w, "    %s: %s\n", ver, strings.Join(files, ", "))
			}
			fmt.Fprintln(w)
		}
	}

	// Recommendations
	if len(analysis.Recommendations) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, headerStyle.Render(fmt.Sprintf("Recommendations (%d):", len(analysis.Recommendations))))
		fmt.Fprintln(w)

		for i, rec := range analysis.Recommendations {
			sevStyle := dimStyle
			switch rec.Severity {
			case "critical", "high":
				sevStyle = majorStyle
			case "medium":
				sevStyle = minorStyle
			}

			icon := "~"
			switch rec.Severity {
			case "critical":
				icon = "!!!"
			case "high":
				icon = " !!"
			case "medium":
				icon = "  !"
			}

			fmt.Fprintf(w, "  %s %s %s\n",
				sevStyle.Render(icon+" "+strings.ToUpper(rec.Severity)),
				headerStyle.Render(rec.Title), "")

			for _, line := range rec.Details {
				if line == "" {
					fmt.Fprintln(w)
				} else {
					fmt.Fprintf(w, "  %s\n", dimStyle.Render(line))
				}
			}

			if len(rec.Fix) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "  %s\n", headerStyle.Render("Changes to make:"))
				for _, fix := range rec.Fix {
					parts := strings.SplitN(fix, "→", 2)
					if len(parts) != 2 {
						fmt.Fprintf(w, "    %s\n", fix)
						continue
					}
					left := strings.TrimSpace(parts[0])
					right := strings.TrimSpace(parts[1])
					filePart := ""
					oldValue := left
					if spaceIdx := strings.Index(left, "  "); spaceIdx > 0 {
						filePart = left[:spaceIdx]
						oldValue = strings.TrimSpace(left[spaceIdx:])
					}
					if filePart != "" {
						fmt.Fprintf(w, "    %s\n", dimStyle.Render(filePart))
					}
					fmt.Fprintf(w, "    %s\n", majorStyle.Render("- "+oldValue))
					fmt.Fprintf(w, "    %s\n", patchStyle.Render("+ "+right))
				}
			}

			if i < len(analysis.Recommendations)-1 {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "  %s\n", dimStyle.Render(strings.Repeat("·", 80)))
			}
		}
		fmt.Fprintln(w)
	}

	// Provider Impact
	if analysis.ProviderImpact != nil {
		imp := analysis.ProviderImpact
		fmt.Fprintln(w)
		fmt.Fprintln(w, headerStyle.Render(fmt.Sprintf("Provider Impact — %s → %s:", imp.TargetProvider, imp.TargetVersion)))
		fmt.Fprintln(w)

		fmt.Fprintf(w, "  %s  %s  %s\n",
			patchStyle.Render(fmt.Sprintf("✓ %d compatible", imp.Compatible)),
			minorStyle.Render(fmt.Sprintf("⬆ %d need upgrade", imp.NeedUpgrade)),
			majorStyle.Render(fmt.Sprintf("✗ %d incompatible", imp.Incompatible)),
		)
		fmt.Fprintln(w)

		for _, r := range imp.Results {
			var icon, color string
			if r.Compatible && !r.RequiresModuleUpgrade {
				icon = "✓"
				color = "46" // green
			} else if r.RequiresModuleUpgrade && r.MinCompatibleVer != "" {
				icon = fmt.Sprintf("⬆ upgrade to %s", r.MinCompatibleVer)
				color = "226" // yellow
			} else {
				icon = "✗ incompatible"
				color = "196" // red
			}
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
			fmt.Fprintf(w, "  %-30s %-10s %-10s %s\n",
				r.Module, r.CurrentModuleVer, r.LatestModuleVer, statusStyle.Render(icon))
		}
		fmt.Fprintln(w)

		// Verdict
		if imp.NeedUpgrade == 0 && imp.Incompatible == 0 {
			fmt.Fprintf(w, "  %s\n", patchStyle.Render(
				fmt.Sprintf("✓ Safe to upgrade %s to %s — all modules compatible.", imp.TargetProvider, imp.TargetVersion)))
		} else if imp.Incompatible > 0 {
			fmt.Fprintf(w, "  %s\n", majorStyle.Render(
				fmt.Sprintf("✗ Cannot upgrade %s to %s — %d module(s) have no compatible version.", imp.TargetProvider, imp.TargetVersion, imp.Incompatible)))
		} else {
			fmt.Fprintf(w, "  %s\n", minorStyle.Render(
				fmt.Sprintf("⬆ Upgrade possible after updating %d module(s) above.", imp.NeedUpgrade)))
		}
		fmt.Fprintln(w)
	}

	return nil
}

func countBreakingForDep(changes []breaking.BreakingChange, dep analyzer.DependencyAnalysis) int {
	count := 0
	for _, bc := range changes {
		if bc.Provider == dep.Name || bc.Provider == dep.Source {
			count++
		}
	}
	return count
}

func countFilesForDep(deps []analyzer.DependencyAnalysis, target analyzer.DependencyAnalysis) int {
	files := make(map[string]bool)
	for _, dep := range deps {
		if dep.Source == target.Source {
			files[dep.FilePath] = true
		}
	}
	if len(files) == 0 {
		return 1
	}
	return len(files)
}

// stripAnsi removes ANSI escape codes for width calculation.
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
