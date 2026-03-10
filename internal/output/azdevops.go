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

// AzDevOpsRenderer renders analysis results using Azure DevOps ##vso logging commands.
type AzDevOpsRenderer struct{}

func (r *AzDevOpsRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	basePath, _ := os.Getwd()

	// ── Dependency Annotations ───────────────────────────────────────────
	for _, dep := range analysis.Dependencies {
		relFile := azRelPath(dep.FilePath, basePath)

		if dep.Deprecated {
			fmt.Fprintf(w, "##vso[task.logissue type=error;sourcepath=%s;linenumber=%d]DEPRECATED: %s (%s) — %s → %s",
				relFile, dep.Line, dep.Name, dep.Source, dep.CurrentVer, dep.LatestVer)
			if dep.ReplacedBy != "" {
				fmt.Fprintf(w, " | replaced by: %s", dep.ReplacedBy)
			}
			fmt.Fprintln(w)
			continue
		}

		severity := "warning"
		if dep.UpdateType == resolver.UpdateMajor {
			severity = "error"
		}
		fmt.Fprintf(w, "##vso[task.logissue type=%s;sourcepath=%s;linenumber=%d]%s: %s (%s) %s → %s\n",
			severity, relFile, dep.Line, dep.UpdateType.String(), dep.Name, dep.Source, dep.CurrentVer, dep.LatestVer)
	}

	// ── Breaking Changes (grouped, collapsible) ──────────────────────────
	if len(analysis.BreakingChanges) > 0 {
		// Group by provider
		groupOrder := []string{}
		groupMap := map[string][]breaking.BreakingChange{}
		for _, bc := range analysis.BreakingChanges {
			key := bc.Provider
			if _, ok := groupMap[key]; !ok {
				groupOrder = append(groupOrder, key)
			}
			groupMap[key] = append(groupMap[key], bc)
		}

		for _, provider := range groupOrder {
			changes := groupMap[provider]
			fmt.Fprintf(w, "##[group]%s — %d breaking changes\n", provider, len(changes))
			for _, bc := range changes {
				severity := "warning"
				if bc.Severity >= breaking.SeverityBreaking {
					severity = "error"
				}
				autoFix := ""
				if bc.AutoFixable {
					autoFix = " [AUTO-FIXABLE]"
				}
				fmt.Fprintf(w, "##vso[task.logissue type=%s]Breaking: %s.%s — %s%s\n",
					severity, bc.ResourceType, bc.Attribute,
					truncateDescription(bc.Description, 200), autoFix)
			}
			fmt.Fprintln(w, "##[endgroup]")
		}
	}

	// ── Summary ──────────────────────────────────────────────────────────
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
	fmt.Fprintf(w, "##[section]tfoutdated Summary\n")
	fmt.Fprintf(w, "  Dependencies scanned: %d\n", analysis.TotalDeps)
	fmt.Fprintf(w, "  Outdated:             %d (%d major, %d minor, %d patch)\n", len(analysis.Dependencies), major, minor, patch)
	fmt.Fprintf(w, "  Up-to-date:           %d\n", analysis.UpToDate)
	fmt.Fprintf(w, "  Breaking changes:     %d\n", len(analysis.BreakingChanges))
	if autoFixCount > 0 {
		fmt.Fprintf(w, "  Auto-fixable:         %d\n", autoFixCount)
	}
	fmt.Fprintln(w)

	// Quick fix suggestion
	if len(analysis.Dependencies) > 0 {
		fmt.Fprintln(w, "##[section]Quick Fix")
		fmt.Fprintln(w, "  Run: tfoutdated fix --dry-run    # preview changes")
		fmt.Fprintln(w, "  Run: tfoutdated fix              # apply all fixes")
		fmt.Fprintln(w, "  Run: tfoutdated fix --safe       # only non-breaking upgrades")
		fmt.Fprintln(w)
	}

	// Task completion status
	if len(analysis.Dependencies) > 0 || len(analysis.BreakingChanges) > 0 {
		fmt.Fprintf(w, "##vso[task.complete result=SucceededWithIssues;]%d outdated, %d breaking\n",
			len(analysis.Dependencies), len(analysis.BreakingChanges))
	}

	return nil
}

func azRelPath(file, base string) string {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		return file
	}
	return rel
}
