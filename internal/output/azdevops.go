package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// AzDevOpsRenderer renders analysis results using Azure DevOps ##vso logging commands.
type AzDevOpsRenderer struct{}

func (r *AzDevOpsRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	basePath, _ := os.Getwd()

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

	// Breaking changes as errors
	for _, bc := range analysis.BreakingChanges {
		fmt.Fprintf(w, "##vso[task.logissue type=error]Breaking change: %s %s — %s\n",
			bc.Provider, bc.ResourceType, bc.Description)
	}

	// Summary
	fmt.Fprintf(w, "\n##[section]tfoutdated: %d deps scanned, %d breaking changes (%s)\n",
		len(analysis.Dependencies), len(analysis.BreakingChanges), analysis.Summary())

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
