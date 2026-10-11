package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/charmbracelet/lipgloss"
)

// TableRenderer renders analysis results as a styled console table.
type TableRenderer struct {
	NoColor bool
	Verbose bool
}

// maxBreakingDefault is the number of breaking changes shown before truncation.
const maxBreakingDefault = 10

func (r *TableRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	if r.NoColor {
		lipgloss.SetColorProfile(0)
	}

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	majorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // bold red
	minorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))            // yellow
	patchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))             // green
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cyanStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	autoFixStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("236")).Padding(0, 1)

	// ── Summary ─────────────────────────────────────────────────────────
	var major, minor, patch, deprecatedCount int
	for _, dep := range analysis.Dependencies {
		if dep.Deprecated {
			deprecatedCount++
		}
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			major++
		case resolver.UpdateMinor:
			minor++
		case resolver.UpdatePatch:
			patch++
		}
	}
	breakTotal := breaking.CountBreaking(analysis.BreakingChanges)
	autoFixCount := 0
	for _, bc := range analysis.BreakingChanges {
		if bc.AutoFixable {
			autoFixCount++
		}
	}

	fmt.Fprintln(w)

	// One-line summary with colored counts
	summaryParts := []string{fmt.Sprintf("  %s outdated", majorStyle.Render(fmt.Sprintf("%d", major+minor+patch)))}
	if major > 0 || minor > 0 || patch > 0 {
		counts := []string{}
		if major > 0 {
			counts = append(counts, majorStyle.Render(fmt.Sprintf("%d major", major)))
		}
		if minor > 0 {
			counts = append(counts, minorStyle.Render(fmt.Sprintf("%d minor", minor)))
		}
		if patch > 0 {
			counts = append(counts, patchStyle.Render(fmt.Sprintf("%d patch", patch)))
		}
		summaryParts = append(summaryParts, "("+strings.Join(counts, ", ")+")")
	}
	if deprecatedCount > 0 {
		summaryParts = append(summaryParts, dimStyle.Render("·"), majorStyle.Render(fmt.Sprintf("%d deprecated", deprecatedCount)))
	}
	if breakTotal > 0 {
		s := majorStyle.Render(fmt.Sprintf("%d breaking", breakTotal))
		if autoFixCount > 0 {
			s += " " + dimStyle.Render("(") + autoFixStyle.Render(fmt.Sprintf("%d auto-fixable", autoFixCount)) + dimStyle.Render(")")
		}
		summaryParts = append(summaryParts, dimStyle.Render("·"), s)
	}
	if analysis.UpToDate > 0 {
		summaryParts = append(summaryParts, dimStyle.Render("·"), patchStyle.Render(fmt.Sprintf("%d up-to-date", analysis.UpToDate)))
	}
	fmt.Fprintln(w, strings.Join(summaryParts, " "))
	fmt.Fprintln(w)

	// ── Deprecated / archived ────────────────────────────────────────────
	// Shown first: a newer version of a deprecated module does not help.
	var deprecated []analyzer.DependencyAnalysis
	for _, dep := range analysis.Dependencies {
		if dep.Deprecated {
			deprecated = append(deprecated, dep)
		}
	}
	if len(deprecated) > 0 {
		fmt.Fprintln(w, sectionStyle.Render(fmt.Sprintf("Deprecated or archived (%d)", len(deprecated))))
		for _, dep := range deprecated {
			fmt.Fprintf(w, "  %s %s %s\n", majorStyle.Render("✗"), dep.Source, dimStyle.Render(formatLocation(dep.FilePath, dep.Line)))
			if dep.ReplacedBy != "" {
				fmt.Fprintf(w, "    %s %s\n", dimStyle.Render("replaced by"), cyanStyle.Render(dep.ReplacedBy))
			}
			if dep.DeprecationReason != "" {
				fmt.Fprintf(w, "    %s\n", dimStyle.Render(dep.DeprecationReason))
			}
		}
		fmt.Fprintln(w)
	}

	// ── Dependency Table ─────────────────────────────────────────────────
	depW, curW, latW := 30, 10, 10
	locW := 14

	for _, dep := range analysis.Dependencies {
		name := dep.Source
		if len(name) > depW {
			depW = len(name)
		}
		if len(dep.CurrentVer) > curW {
			curW = len(dep.CurrentVer)
		}
		if len(dep.LatestVer) > latW {
			latW = len(dep.LatestVer)
		}
		loc := formatLocation(dep.FilePath, dep.Line)
		if len(loc) > locW {
			locW = len(loc)
		}
	}
	if depW > 50 {
		depW = 50
	}
	if locW > locationWidth {
		locW = locationWidth
	}
	typeW := 12
	impactW := 12

	totalW := depW + locW + curW + latW + typeW + impactW + 12
	header := fmt.Sprintf(" %-*s  %-*s  %-*s  %-*s  %-*s  %-*s",
		depW, "DEPENDENCY",
		locW, "LOCATION",
		curW, "CURRENT",
		latW, "LATEST",
		typeW, "TYPE",
		impactW, "IMPACT",
	)
	fmt.Fprintln(w, headerStyle.Render(header))
	fmt.Fprintln(w, dimStyle.Render(strings.Repeat("─", totalW)))

	var needsReview []analyzer.DependencyAnalysis
	for _, dep := range analysis.Dependencies {
		name := dep.Source
		if len(name) > depW {
			name = name[:depW-1] + "…"
		}

		loc := trimHead(formatLocation(dep.FilePath, dep.Line), locW)

		var typeStr string
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			jump := dep.Distance.MajorsBehind
			if jump <= 0 {
				jump = 1
			}
			typeStr = majorStyle.Render(fmt.Sprintf("MAJOR ↑%d", jump))
		case resolver.UpdateMinor:
			jump := dep.Distance.MinorsBehind
			if jump <= 0 {
				jump = 1
			}
			typeStr = minorStyle.Render(fmt.Sprintf("MINOR ↑%d", jump))
		case resolver.UpdatePatch:
			jump := dep.Distance.PatchesBehind
			if jump <= 0 {
				jump = 1
			}
			typeStr = patchStyle.Render(fmt.Sprintf("PATCH ↑%d", jump))
		}

		breakCount := countBreakingForDep(analysis.BreakingChanges, dep)
		var impactStr string
		switch {
		case breakCount > 0:
			impactStr = majorStyle.Render(fmt.Sprintf("%d break", breakCount))
		case analyzer.NeedsReview(dep, analysis.BreakingChanges):
			impactStr = minorStyle.Render("⚠ review")
			needsReview = append(needsReview, dep)
		default:
			impactStr = patchStyle.Render("✓ none")
		}

		fmt.Fprintf(w, " %-*s  %-*s  %-*s  %-*s  %-*s  %-*s\n",
			depW, name,
			locW+len(dimStyle.Render(loc))-len(loc), dimStyle.Render(loc),
			curW, dimStyle.Render(dep.CurrentVer),
			latW, dep.LatestVer,
			typeW+len(typeStr)-len(stripAnsi(typeStr)), typeStr,
			impactW+len(impactStr)-len(stripAnsi(impactStr)), impactStr,
		)
	}
	fmt.Fprintln(w, dimStyle.Render(strings.Repeat("─", totalW)))
	printReviewNotes(w, minorStyle.Render("⚠ review"), needsReview)

	// ── Breaking Changes (grouped by module/provider) ────────────────────
	if len(analysis.BreakingChanges) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, sectionStyle.Render(fmt.Sprintf(" BREAKING CHANGES (%d) ", breakTotal)))
		fmt.Fprintln(w)

		// Group by provider
		type bcGroup struct {
			provider string
			changes  []breaking.BreakingChange
		}
		groupOrder := []string{}
		groupMap := map[string][]breaking.BreakingChange{}
		for _, bc := range analysis.BreakingChanges {
			key := bc.Provider
			if _, ok := groupMap[key]; !ok {
				groupOrder = append(groupOrder, key)
			}
			groupMap[key] = append(groupMap[key], bc)
		}

		totalShown := 0
		limit := maxBreakingDefault
		if r.Verbose {
			limit = len(analysis.BreakingChanges) + 1
		}

		for _, provider := range groupOrder {
			changes := groupMap[provider]

			verInfo := findVersionForProvider(analysis.Dependencies, provider)
			groupHeader := cyanStyle.Render(provider)
			if verInfo != "" {
				groupHeader += " " + dimStyle.Render(verInfo)
			}
			groupHeader += dimStyle.Render(fmt.Sprintf(" — %d breaking changes", breaking.CountBreaking(changes)))
			fmt.Fprintf(w, "  %s\n", groupHeader)

			// Categorize
			var renames, removals, typeChanges, other []breaking.BreakingChange
			for _, bc := range changes {
				switch bc.Kind {
				case breaking.AttributeRenamed, breaking.ResourceRenamed, breaking.VariableRenamed, breaking.OutputRenamed:
					renames = append(renames, bc)
				case breaking.AttributeRemoved, breaking.ResourceRemoved, breaking.VariableRemoved, breaking.OutputRemoved:
					removals = append(removals, bc)
				case breaking.TypeChanged:
					typeChanges = append(typeChanges, bc)
				default:
					other = append(other, bc)
				}
			}

			printCategory := func(label string, items []breaking.BreakingChange) {
				if len(items) == 0 || totalShown >= limit {
					return
				}
				fmt.Fprintf(w, "\n    %s %s\n", dimStyle.Render("▸"), headerStyle.Render(label))
				for _, bc := range items {
					if totalShown >= limit {
						break
					}
					totalShown++
					printBreakingChange(w, bc, totalShown, analysis, dimStyle, majorStyle, minorStyle, patchStyle, autoFixStyle)
				}
			}

			title := fmt.Sprintf("Renames (%d) — suggested, check before renaming", len(renames))
			if allAutoFixable(renames) {
				title = fmt.Sprintf("Renames (%d) — auto-fixable", len(renames))
			}
			printCategory(title, renames)
			printCategory(fmt.Sprintf("Removals (%d)", len(removals)), removals)
			printCategory(fmt.Sprintf("Type Changes (%d)", len(typeChanges)), typeChanges)
			printCategory(fmt.Sprintf("Other (%d)", len(other)), other)

			fmt.Fprintln(w)
		}

		remaining := len(analysis.BreakingChanges) - totalShown
		if remaining > 0 {
			fmt.Fprintf(w, "  %s\n\n",
				dimStyle.Render(fmt.Sprintf("... and %d more. Use --verbose to see all.", remaining)))
		}
	}

	// ── Upgrade Paths ────────────────────────────────────────────────────
	if len(analysis.UpgradePaths) > 0 {
		fmt.Fprintln(w, sectionStyle.Render(" UPGRADE PATHS "))
		fmt.Fprintln(w)

		for _, path := range analysis.UpgradePaths {
			fmt.Fprintf(w, "  %s\n", headerStyle.Render(path.Name))
			for i, step := range path.Steps {
				if step.Safe {
					fmt.Fprintf(w, "    %s %s %s %s\n",
						dimStyle.Render(fmt.Sprintf("%d.", i+1)),
						patchStyle.Render("→"),
						dimStyle.Render(step.From),
						patchStyle.Render("→ "+step.To),
					)
				} else {
					fmt.Fprintf(w, "    %s %s %s %s\n",
						dimStyle.Render(fmt.Sprintf("%d.", i+1)),
						majorStyle.Render("⚠"),
						dimStyle.Render(step.From),
						majorStyle.Render("→ "+step.To),
					)
				}
			}
			if path.NonBreakingTarget != "" {
				fmt.Fprintf(w, "    %s %s\n", patchStyle.Render("✓"), patchStyle.Render("Safe target: "+path.NonBreakingTarget))
			} else if path.HasBreakingSteps() {
				fmt.Fprintf(w, "    %s %s\n", majorStyle.Render("⚠"), majorStyle.Render("Breaking changes — review required"))
			}
			fmt.Fprintln(w)
		}
	}

	// ── Not checked ──────────────────────────────────────────────────────
	// Shown before anything else so that a scan which could not reach a private
	// module host is never mistaken for a scan that found nothing wrong.
	if len(analysis.Unresolved) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, sectionStyle.Render(fmt.Sprintf(" NOT CHECKED (%d) ", len(analysis.Unresolved))))
		fmt.Fprintln(w)
		for _, u := range analysis.Unresolved {
			kind := "provider"
			if u.IsModule {
				kind = "module"
			}
			pinned := u.Declared
			if pinned == "" {
				pinned = majorStyle.Render("unpinned")
			}
			fmt.Fprintf(w, "  %s %s %s  pinned at %s\n",
				majorStyle.Render("?"), dimStyle.Render(kind), u.Source, pinned)
			fmt.Fprintf(w, "    %s\n", dimStyle.Render(filepath.Base(u.FilePath)+":"+fmt.Sprint(u.Line)))
			if u.Reason != "" {
				fmt.Fprintf(w, "    %s\n", dimStyle.Render(truncate(u.Reason, 100)))
			}
			fmt.Fprintln(w)
		}
	}

	// ── Alignment Issues ─────────────────────────────────────────────────
	if len(analysis.Alignments) > 0 {
		fmt.Fprintln(w, sectionStyle.Render(" ALIGNMENT ISSUES "))
		fmt.Fprintln(w)
		for _, issue := range analysis.Alignments {
			fmt.Fprintf(w, "  %s %s used at multiple versions:\n", minorStyle.Render("⚠"), issue.Name)
			for ver, files := range issue.Versions {
				fmt.Fprintf(w, "    %s: %s\n", ver, strings.Join(files, ", "))
			}
			fmt.Fprintln(w)
		}
	}

	// ── Recommendations ──────────────────────────────────────────────────
	if len(analysis.Recommendations) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, sectionStyle.Render(fmt.Sprintf(" RECOMMENDATIONS (%d) ", len(analysis.Recommendations))))
		fmt.Fprintln(w)

		for i, rec := range analysis.Recommendations {
			sevStyle := dimStyle
			icon := "○"
			switch rec.Severity {
			case "critical":
				sevStyle = majorStyle
				icon = "●"
			case "high":
				sevStyle = majorStyle
				icon = "●"
			case "medium":
				sevStyle = minorStyle
				icon = "◐"
			}

			fmt.Fprintf(w, "  %s %s\n",
				sevStyle.Render(icon+" "+strings.ToUpper(rec.Severity)),
				headerStyle.Render(rec.Title))

			for _, line := range rec.Details {
				if line == "" {
					continue
				}
				fmt.Fprintf(w, "    %s\n", dimStyle.Render(line))
			}

			if len(rec.Fix) > 0 {
				fmt.Fprintln(w)
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
				fmt.Fprintf(w, "  %s\n", dimStyle.Render(strings.Repeat("─", 50)))
			}
		}
		fmt.Fprintln(w)
	}

	// ── Provider Impact ──────────────────────────────────────────────────
	if analysis.ProviderImpact != nil {
		imp := analysis.ProviderImpact
		fmt.Fprintln(w)
		fmt.Fprintln(w, sectionStyle.Render(fmt.Sprintf(" PROVIDER IMPACT — %s → %s ", imp.TargetProvider, imp.TargetVersion)))
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
				color = "46"
			} else if r.RequiresModuleUpgrade && r.MinCompatibleVer != "" {
				icon = fmt.Sprintf("⬆ upgrade to %s", r.MinCompatibleVer)
				color = "226"
			} else {
				icon = "✗ incompatible"
				color = "196"
			}
			statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(color))
			fmt.Fprintf(w, "  %-30s %-10s %-10s %s\n",
				r.Module, r.CurrentModuleVer, r.LatestModuleVer, statusStyle.Render(icon))
		}
		fmt.Fprintln(w)

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

	// ── Quick Fix Hint ──────────────────────────────────────────────────
	if len(analysis.Dependencies) > 0 {
		fmt.Fprintf(w, "  %s %s\n\n", dimStyle.Render("Run"), headerStyle.Render("tfoutdated fix"))
	}

	return nil
}

// printBreakingChange prints a single breaking change entry with truncation.
func printBreakingChange(
	w io.Writer,
	bc breaking.BreakingChange,
	idx int,
	analysis *analyzer.Analysis,
	dimStyle, majorStyle, minorStyle, patchStyle, autoFixStyle lipgloss.Style,
) {
	sevStyle := dimStyle
	if bc.Severity >= breaking.SeverityBreaking {
		sevStyle = majorStyle
	} else if bc.Severity >= breaking.SeverityWarning {
		sevStyle = minorStyle
	}

	autoFixLabel := ""
	if bc.AutoFixable {
		autoFixLabel = " " + autoFixStyle.Render("[AUTO-FIX]")
	}

	desc := truncateDescription(bc.Description, 180)

	fmt.Fprintf(w, "    %s %s %s%s\n",
		dimStyle.Render(fmt.Sprintf("%d.", idx)),
		sevStyle.Render("["+bc.Severity.String()+"]"),
		desc,
		autoFixLabel)

	if bc.MigrationGuide != "" {
		guide := truncateDescription(bc.MigrationGuide, 120)
		fmt.Fprintf(w, "       %s %s\n", dimStyle.Render("Fix:"), guide)
	}

	// Show real user code snippets if available
	for _, impact := range analysis.Impacts {
		if impact.ActualBefore == "" || impact.BreakingChange.ResourceType != bc.ResourceType {
			continue
		}
		if impact.BreakingChange.Attribute != bc.Attribute {
			continue
		}
		fmt.Fprintf(w, "       %s %s (%s:%d)\n",
			dimStyle.Render("Your code:"), impact.ResourceName, filepath.Base(impact.AffectedFile), impact.AffectedLine)
		fmt.Fprintf(w, "       %s\n", dimStyle.Render("Before:"))
		beforeLines := strings.Split(impact.ActualBefore, "\n")
		for i, line := range beforeLines {
			if i >= 3 {
				fmt.Fprintf(w, "       %s  %s\n", majorStyle.Render("-"), dimStyle.Render("..."))
				break
			}
			fmt.Fprintf(w, "       %s  %s\n", majorStyle.Render("-"), line)
		}
		if impact.ActualAfter != "" {
			fmt.Fprintf(w, "       %s\n", dimStyle.Render("After:"))
			afterLines := strings.Split(impact.ActualAfter, "\n")
			for i, line := range afterLines {
				if i >= 3 {
					fmt.Fprintf(w, "       %s  %s\n", patchStyle.Render("+"), dimStyle.Render("..."))
					break
				}
				fmt.Fprintf(w, "       %s  %s\n", patchStyle.Render("+"), line)
			}
		}
		break // only show first match
	}
	fmt.Fprintln(w)
}

// truncateDescription truncates a description string to maxLen characters.
func truncateDescription(desc string, maxLen int) string {
	desc = strings.ReplaceAll(desc, "\n", " ")
	desc = strings.Join(strings.Fields(desc), " ")
	if len(desc) > maxLen {
		return desc[:maxLen-3] + "..."
	}
	return desc
}

// locationWidth caps the Location column. It is deliberately wider than the
// other columns: the path is the actionable part of a finding.
const locationWidth = 44

// formatLocation returns a file:line string that actually identifies the file.
//
// It used to show only the base name, which collapses in any real repository:
// scanning one Azure estate produced twelve distinct files all labelled
// "main.tf" and ten labelled "provider.tf", so the column could not tell you
// what to edit. The directory is what disambiguates, so when the path is too
// long it is trimmed from the left — the tail is the informative end.
func formatLocation(filePath string, line int) string {
	path := filePath
	if abs := filepath.IsAbs(path); abs {
		if cwd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
				path = rel
			}
		}
	}
	path = filepath.ToSlash(path)
	if line > 0 {
		return fmt.Sprintf("%s:%d", path, line)
	}
	return path
}

// trimHead shortens a path to n characters by dropping the front. The tail
// carries the directory and file that identify the finding, so the head is what
// can be given up.
func trimHead(s string, n int) string {
	if n <= 1 || len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n+1:]
}

// findVersionForProvider returns a version transition string for a provider.
func findVersionForProvider(deps []analyzer.DependencyAnalysis, provider string) string {
	for _, dep := range deps {
		if dep.Name == provider || dep.Source == provider {
			return fmt.Sprintf("(%s → %s)", dep.CurrentVer, dep.LatestVer)
		}
	}
	return ""
}

func allAutoFixable(changes []breaking.BreakingChange) bool {
	for _, bc := range changes {
		if !bc.AutoFixable {
			return false
		}
	}
	return true
}

// printReviewNotes explains each "⚠ review" once per provider and target
// version: a major upgrade tfoutdated has no breaking change data for.
func printReviewNotes(w io.Writer, label string, deps []analyzer.DependencyAnalysis) {
	seen := map[string]bool{}
	for _, dep := range deps {
		key := dep.Source + "@" + dep.LatestVer
		if seen[key] {
			continue
		}
		seen[key] = true
		where := "read its release notes before upgrading"
		if guide := analyzer.UpgradeGuideURL(dep); guide != "" {
			where = "read " + guide
		}
		fmt.Fprintf(w, "  %s %s → %s is a major upgrade with no known breaking changes in tfoutdated: %s\n",
			label, dep.Source, dep.LatestVer, where)
	}
}

func countBreakingForDep(changes []breaking.BreakingChange, dep analyzer.DependencyAnalysis) int {
	count := 0
	for _, bc := range changes {
		if (bc.Provider == dep.Name || bc.Provider == dep.Source) && bc.IsBreaking() {
			count++
		}
	}
	return count
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

// truncate shortens a message for single-line display.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
