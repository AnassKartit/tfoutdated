package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// GitHubRenderer renders analysis results using GitHub Actions workflow commands.
type GitHubRenderer struct{}

func (r *GitHubRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	basePath, _ := os.Getwd()

	// ── Workflow Annotations ─────────────────────────────────────────────
	for _, dep := range analysis.Dependencies {
		relFile := relPath(dep.FilePath, basePath)

		if dep.Deprecated {
			fmt.Fprintf(w, "::error file=%s,line=%d::DEPRECATED: %s (%s) %s → %s",
				relFile, dep.Line, dep.Name, dep.Source, dep.CurrentVer, dep.LatestVer)
			if dep.ReplacedBy != "" {
				fmt.Fprintf(w, " | replaced by: %s", dep.ReplacedBy)
			}
			fmt.Fprintln(w)
			continue
		}

		level := "warning"
		if dep.UpdateType == resolver.UpdateMajor {
			level = "error"
		}
		fmt.Fprintf(w, "::%s file=%s,line=%d::%s update: %s (%s) %s → %s\n",
			level, relFile, dep.Line, dep.UpdateType.String(), dep.Name, dep.Source, dep.CurrentVer, dep.LatestVer)
	}

	// ── Breaking Changes (grouped, collapsible) ──────────────────────────
	// Only show breaking changes that affect user's actual code (have impacts)
	affectsUser := filterBreakingChangesWithImpact(analysis)

	if len(affectsUser) > 0 {
		// Group by provider
		groupOrder := []string{}
		groupMap := map[string][]breaking.BreakingChange{}
		for _, bc := range affectsUser {
			key := bc.Provider
			if _, ok := groupMap[key]; !ok {
				groupOrder = append(groupOrder, key)
			}
			groupMap[key] = append(groupMap[key], bc)
		}

		for _, provider := range groupOrder {
			changes := groupMap[provider]
			fmt.Fprintf(w, "::group::%s — %d breaking changes affecting your code\n", provider, len(changes))
			for _, bc := range changes {
				level := "warning"
				if bc.Severity >= breaking.SeverityBreaking {
					level = "error"
				}
				autoFix := ""
				if bc.AutoFixable {
					autoFix = " [AUTO-FIXABLE]"
				}
				fmt.Fprintf(w, "::%s::Breaking: %s %s — %s%s\n",
					level, bc.ResourceType, bc.Attribute, truncateDescription(bc.Description, 200), autoFix)
			}
			fmt.Fprintln(w, "::endgroup::")
		}
	}

	// ── GITHUB_STEP_SUMMARY ──────────────────────────────────────────────
	var major, minor, patch int
	for _, dep := range analysis.Dependencies {
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			major++
		case resolver.UpdateMinor:
			minor++
		case resolver.UpdatePatch:
			patch++
		}
	}

	autoFixCount := 0
	for _, bc := range analysis.BreakingChanges {
		if bc.AutoFixable {
			autoFixCount++
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## tfoutdated Report")
	fmt.Fprintln(w)

	// Status badges
	if len(analysis.BreakingChanges) > 0 {
		fmt.Fprintf(w, "> **%d** outdated dependencies with **%d** breaking changes\n\n", len(analysis.Dependencies), len(analysis.BreakingChanges))
	} else if len(analysis.Dependencies) > 0 {
		fmt.Fprintf(w, "> **%d** outdated dependencies (no breaking changes)\n\n", len(analysis.Dependencies))
	} else {
		fmt.Fprintln(w, "> All dependencies are up to date!")
		return nil
	}

	// Summary table
	fmt.Fprintln(w, "### Summary")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Metric | Count |")
	fmt.Fprintln(w, "|--------|------:|")
	fmt.Fprintf(w, "| Total dependencies | %d |\n", len(analysis.Dependencies))
	if major > 0 {
		fmt.Fprintf(w, "| Major updates | %d |\n", major)
	}
	if minor > 0 {
		fmt.Fprintf(w, "| Minor updates | %d |\n", minor)
	}
	if patch > 0 {
		fmt.Fprintf(w, "| Patch updates | %d |\n", patch)
	}
	if len(analysis.BreakingChanges) > 0 {
		fmt.Fprintf(w, "| Breaking changes | %d |\n", len(analysis.BreakingChanges))
	}
	if autoFixCount > 0 {
		fmt.Fprintf(w, "| Auto-fixable | %d |\n", autoFixCount)
	}
	fmt.Fprintln(w)

	// Dependencies table
	if len(analysis.Dependencies) > 0 {
		fmt.Fprintln(w, "### Outdated Dependencies")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| Dependency | Current | Latest | Type | Breaking |")
		fmt.Fprintln(w, "|------------|---------|--------|------|----------|")
		for _, dep := range analysis.Dependencies {
			breakCount := countBreakingForDep(analysis.BreakingChanges, dep)
			breakStr := "-"
			if breakCount > 0 {
				breakStr = fmt.Sprintf("%d", breakCount)
			}
			typeEmoji := ""
			switch dep.UpdateType {
			case resolver.UpdateMajor:
				typeEmoji = "MAJOR"
			case resolver.UpdateMinor:
				typeEmoji = "MINOR"
			case resolver.UpdatePatch:
				typeEmoji = "PATCH"
			}
			fmt.Fprintf(w, "| `%s` | %s | %s | %s | %s |\n",
				dep.Source, dep.CurrentVer, dep.LatestVer, typeEmoji, breakStr)
		}
		fmt.Fprintln(w)
	}

	// Breaking changes table (only those affecting user's code)
	if len(affectsUser) > 0 {
		fmt.Fprintln(w, "### Breaking Changes (affecting your code)")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| Provider | Resource | Change | Severity | Auto-fixable |")
		fmt.Fprintln(w, "|----------|----------|--------|----------|:------------:|")
		for _, bc := range affectsUser {
			autoFix := "-"
			if bc.AutoFixable {
				autoFix = "Yes"
			}
			desc := truncateDescription(bc.Description, 80)
			fmt.Fprintf(w, "| %s | `%s` | %s | %s | %s |\n",
				bc.Provider, bc.ResourceType, desc, bc.Severity.String(), autoFix)
		}
		fmt.Fprintln(w)
	}

	// Quick fix suggestion
	if autoFixCount > 0 || len(analysis.Dependencies) > 0 {
		fmt.Fprintln(w, "### Quick Fix")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "```bash")
		fmt.Fprintln(w, "# Auto-fix all safe changes")
		fmt.Fprintln(w, "tfoutdated fix")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "# Preview changes first")
		fmt.Fprintln(w, "tfoutdated fix --dry-run")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "# Only non-breaking upgrades")
		fmt.Fprintln(w, "tfoutdated fix --safe")
		fmt.Fprintln(w, "```")
		fmt.Fprintln(w)
	}

	return nil
}

// filterBreakingChangesWithImpact returns only breaking changes that have
// matching impact items (i.e., they affect the user's actual code).
// Falls back to all breaking changes if no impact analysis was performed.
func filterBreakingChangesWithImpact(analysis *analyzer.Analysis) []breaking.BreakingChange {
	if len(analysis.Impacts) == 0 {
		// No impact analysis was performed — return all
		return analysis.BreakingChanges
	}

	// Build a set of (resourceType, attribute) pairs from impacts
	type key struct{ rt, attr string }
	affected := make(map[key]bool)
	for _, imp := range analysis.Impacts {
		affected[key{imp.BreakingChange.ResourceType, imp.BreakingChange.Attribute}] = true
	}

	var filtered []breaking.BreakingChange
	for _, bc := range analysis.BreakingChanges {
		if affected[key{bc.ResourceType, bc.Attribute}] {
			filtered = append(filtered, bc)
		}
	}
	return filtered
}

func relPath(file, base string) string {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		return file
	}
	return rel
}

