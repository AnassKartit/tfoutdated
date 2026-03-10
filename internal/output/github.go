package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// GitHubRenderer renders analysis results using GitHub Actions workflow commands.
type GitHubRenderer struct{}

func (r *GitHubRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	basePath, _ := os.Getwd()

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

	// Breaking changes as errors
	for _, bc := range analysis.BreakingChanges {
		fmt.Fprintf(w, "::error::Breaking change: %s %s — %s\n",
			bc.Provider, bc.ResourceType, bc.Description)
	}

	// Summary for GITHUB_STEP_SUMMARY
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

	fmt.Fprintf(w, "\n## tfoutdated Summary\n")
	fmt.Fprintf(w, "| Metric | Count |\n|--------|-------|\n")
	fmt.Fprintf(w, "| Total deps | %d |\n", len(analysis.Dependencies))
	fmt.Fprintf(w, "| Major | %d |\n", major)
	fmt.Fprintf(w, "| Minor | %d |\n", minor)
	fmt.Fprintf(w, "| Patch | %d |\n", patch)
	fmt.Fprintf(w, "| Breaking changes | %d |\n", len(analysis.BreakingChanges))

	return nil
}

func relPath(file, base string) string {
	rel, err := filepath.Rel(base, file)
	if err != nil {
		return file
	}
	return rel
}
