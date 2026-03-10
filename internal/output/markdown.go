package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
)

// MarkdownRenderer renders analysis results as markdown.
type MarkdownRenderer struct{}

func (r *MarkdownRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	fmt.Fprintln(w, "# Terraform Dependency Report")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "**%s**\n\n", analysis.Summary())

	// Dependencies table
	fmt.Fprintln(w, "## Outdated Dependencies")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| Dependency | Current | Latest | Type | Impact |")
	fmt.Fprintln(w, "|---|---|---|---|---|")

	for _, dep := range analysis.Dependencies {
		impact := "none"
		breakCount := 0
		for _, bc := range analysis.BreakingChanges {
			if bc.Provider == dep.Name || bc.Provider == dep.Source {
				breakCount++
			}
		}
		if breakCount > 0 {
			impact = fmt.Sprintf("%d breaking", breakCount)
		}

		fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
			dep.Source, dep.CurrentVer, dep.LatestVer, dep.UpdateType.String(), impact)
	}
	fmt.Fprintln(w)

	// Breaking changes
	if len(analysis.BreakingChanges) > 0 {
		fmt.Fprintln(w, "## Breaking Changes")
		fmt.Fprintln(w)

		for _, bc := range analysis.BreakingChanges {
			resourceLabel := bc.Provider
			if bc.ResourceType != "" {
				resourceLabel += " / `" + bc.ResourceType + "`"
			}
			if bc.Attribute != "" {
				resourceLabel += " `."+bc.Attribute+"`"
			}
			fmt.Fprintf(w, "### [%s] %s\n\n", bc.Severity.String(), resourceLabel)
			fmt.Fprintf(w, "%s\n\n", bc.Description)
			if bc.EffortLevel != "" {
				fmt.Fprintf(w, "**Effort:** %s\n\n", bc.EffortEmoji())
			}
			if bc.MigrationGuide != "" {
				fmt.Fprintf(w, "> **Migration:** %s\n\n", bc.MigrationGuide)
			}
			// Show real user code snippets if available
			realSnippetShown := false
			for _, impact := range analysis.Impacts {
				if impact.ActualBefore == "" || impact.BreakingChange.ResourceType != bc.ResourceType {
					continue
				}
				if impact.BreakingChange.Attribute != bc.Attribute {
					continue
				}
				realSnippetShown = true
				scopeInfo := ""
				if impact.LinesChanged > 0 {
					scopeInfo = fmt.Sprintf(" (%d lines to change)", impact.LinesChanged)
				}
				fmt.Fprintf(w, "<details><summary>%s in %s:%d%s</summary>\n\n",
					impact.ResourceName, impact.AffectedFile, impact.AffectedLine, scopeInfo)
				fmt.Fprintln(w, "**Before (your code):**")
				fmt.Fprintln(w, "```hcl")
				fmt.Fprintln(w, impact.ActualBefore)
				fmt.Fprintln(w, "```")
				fmt.Fprintln(w)
				if impact.ActualAfter != "" {
					fmt.Fprintln(w, "**After (computed fix):**")
					fmt.Fprintln(w, "```hcl")
					fmt.Fprintln(w, impact.ActualAfter)
					fmt.Fprintln(w, "```")
				}
				fmt.Fprintln(w, "</details>")
				fmt.Fprintln(w)
			}

			// Fall back to generic snippets
			if !realSnippetShown && bc.BeforeSnippet != "" && bc.AfterSnippet != "" {
				fmt.Fprintln(w, "<details><summary>Show code change (generic example)</summary>")
				fmt.Fprintln(w)
				fmt.Fprintln(w, "**Before:**")
				fmt.Fprintln(w, "```hcl")
				fmt.Fprintln(w, bc.BeforeSnippet)
				fmt.Fprintln(w, "```")
				fmt.Fprintln(w)
				fmt.Fprintln(w, "**After:**")
				fmt.Fprintln(w, "```hcl")
				fmt.Fprintln(w, bc.AfterSnippet)
				fmt.Fprintln(w, "```")
				fmt.Fprintln(w, "</details>")
				fmt.Fprintln(w)
			}
		}
	}

	// Upgrade paths
	if len(analysis.UpgradePaths) > 0 {
		fmt.Fprintln(w, "## Recommended Upgrade Paths")
		fmt.Fprintln(w)

		for _, path := range analysis.UpgradePaths {
			fmt.Fprintf(w, "### %s\n\n", path.Name)
			for i, step := range path.Steps {
				safety := "safe"
				if !step.Safe {
					safety = "**breaking**"
				}
				fmt.Fprintf(w, "%d. `%s` → `%s` (%s)\n", i+1, step.From, step.To, safety)
			}
			if path.NonBreakingTarget != "" {
				fmt.Fprintf(w, "\nSafe target (no code changes): `%s`\n", path.NonBreakingTarget)
			} else if path.HasBreakingSteps() {
				fmt.Fprintf(w, "\n> **Warning:** Breaking changes detected — review required before upgrading\n")
			}
			fmt.Fprintln(w)
		}
	}

	// Alignment issues
	if len(analysis.Alignments) > 0 {
		fmt.Fprintln(w, "## Version Alignment Issues")
		fmt.Fprintln(w)
		for _, issue := range analysis.Alignments {
			fmt.Fprintf(w, "**%s** used at multiple versions:\n\n", issue.Name)
			for ver, files := range issue.Versions {
				fmt.Fprintf(w, "- `%s`: %s\n", ver, strings.Join(files, ", "))
			}
			fmt.Fprintln(w)
		}
	}

	// Recommendations
	if len(analysis.Recommendations) > 0 {
		fmt.Fprintln(w, "## Recommendations")
		fmt.Fprintln(w)
		for _, rec := range analysis.Recommendations {
			icon := "i"
			switch rec.Severity {
			case "critical":
				icon = "!!!"
			case "high":
				icon = "!!"
			case "medium":
				icon = "!"
			}
			fmt.Fprintf(w, "### %s %s — %s\n", icon, strings.ToUpper(rec.Severity), rec.Title)
			fmt.Fprintln(w)
			for _, d := range rec.Details {
				if d == "" {
					fmt.Fprintln(w)
				} else {
					fmt.Fprintf(w, "> %s\n", d)
				}
			}
			fmt.Fprintln(w)
			if len(rec.Fix) > 0 {
				fmt.Fprintln(w, "**Changes to make:**")
				fmt.Fprintln(w)
				fmt.Fprintln(w, "```diff")
				for _, fix := range rec.Fix {
					parts := strings.SplitN(fix, "→", 2)
					if len(parts) != 2 {
						fmt.Fprintf(w, "  %s\n", fix)
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
						fmt.Fprintf(w, "# %s\n", filePart)
					}
					fmt.Fprintf(w, "- %s\n", oldValue)
					fmt.Fprintf(w, "+ %s\n", right)
				}
				fmt.Fprintln(w, "```")
				fmt.Fprintln(w)
			}
		}
	}

	// Provider Impact
	if analysis.ProviderImpact != nil {
		imp := analysis.ProviderImpact
		fmt.Fprintf(w, "## Impact Analysis — %s → %s\n\n", imp.TargetProvider, imp.TargetVersion)

		fmt.Fprintln(w, "| Metric | Count |")
		fmt.Fprintln(w, "|--------|------:|")
		fmt.Fprintf(w, "| Compatible | %d |\n", imp.Compatible)
		if imp.NeedUpgrade > 0 {
			fmt.Fprintf(w, "| Need Upgrade | %d |\n", imp.NeedUpgrade)
		}
		if imp.Incompatible > 0 {
			fmt.Fprintf(w, "| Incompatible | %d |\n", imp.Incompatible)
		}
		fmt.Fprintln(w)

		// Action items
		hasActions := false
		for _, r := range imp.Results {
			if r.RequiresModuleUpgrade {
				if !hasActions {
					fmt.Fprintln(w, "### Before upgrading, update these modules:")
					fmt.Fprintln(w)
					hasActions = true
				}
				fmt.Fprintf(w, "- **%s** (%s:%d)\n", r.Module, r.File, r.Line)
				fmt.Fprintln(w, "  ```diff")
				fmt.Fprintf(w, "  - version = \"%s\"\n", r.CurrentModuleVer)
				if r.MinCompatibleVer != "" {
					fmt.Fprintf(w, "  + version = \"%s\"\n", r.MinCompatibleVer)
				}
				fmt.Fprintln(w, "  ```")
			}
		}
		if hasActions {
			fmt.Fprintln(w)
		}

		// Compatibility matrix
		fmt.Fprintln(w, "### Compatibility Matrix")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| Module | Version | Latest | Status |")
		fmt.Fprintln(w, "|--------|---------|--------|--------|")
		for _, r := range imp.Results {
			status := "compatible"
			if r.RequiresModuleUpgrade && r.MinCompatibleVer != "" {
				status = fmt.Sprintf("upgrade to %s", r.MinCompatibleVer)
			} else if !r.Compatible {
				status = "incompatible"
			}
			fmt.Fprintf(w, "| `%s` | `%s` | `%s` | %s |\n", r.Module, r.CurrentModuleVer, r.LatestModuleVer, status)
		}
		fmt.Fprintln(w)

		// Verdict
		if imp.NeedUpgrade == 0 && imp.Incompatible == 0 {
			fmt.Fprintf(w, "> **Safe to upgrade %s to %s** — all modules compatible.\n", imp.TargetProvider, imp.TargetVersion)
		} else if imp.Incompatible > 0 {
			fmt.Fprintf(w, "> **Cannot upgrade %s to %s** — %d module(s) have no compatible version.\n",
				imp.TargetProvider, imp.TargetVersion, imp.Incompatible)
		} else {
			fmt.Fprintf(w, "> **Upgrade possible** after updating %d module(s) above.\n", imp.NeedUpgrade)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "---")
	fmt.Fprintln(w, "*Generated by [tfoutdated](https://github.com/anasskartit/tfoutdated)*")
	return nil
}
