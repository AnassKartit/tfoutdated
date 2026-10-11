package analyzer

import (
	"fmt"
	"strings"

	goversion "github.com/hashicorp/go-version"

	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// NeedsReview reports a provider major upgrade for which no breaking change is
// known. Modules are diffed against the registry, so an empty result there is
// a real answer; for providers it only means the knowledge base and the
// changelog had nothing, which is not the same as nothing breaking.
func NeedsReview(dep DependencyAnalysis, changes []breaking.BreakingChange) bool {
	if dep.IsModule || dep.UpdateType != resolver.UpdateMajor {
		return false
	}
	for _, bc := range changes {
		if bc.Provider == dep.Name || bc.Provider == dep.Source {
			return false
		}
	}
	return true
}

// upgradeGuides lists provider upgrade guides whose URL pattern is published
// for every major version. Others fall back to their release notes.
var upgradeGuides = map[string]string{
	"azurerm": "https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/guides/%d.0-upgrade-guide",
}

// UpgradeGuideURL returns the upgrade guide for a provider's target major
// version, or "" when none is known.
func UpgradeGuideURL(dep DependencyAnalysis) string {
	name := dep.Name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	pattern, ok := upgradeGuides[strings.ToLower(name)]
	if !ok {
		return ""
	}
	latest, err := goversion.NewVersion(dep.LatestVer)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(pattern, latest.Segments()[0])
}
