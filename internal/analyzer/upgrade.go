package analyzer

import (
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// ComputeUpgradePaths generates safe upgrade paths for all dependencies.
// It takes breaking changes into account: if a dependency has known breaking
// changes, NonBreakingTarget is cleared and affected steps are marked unsafe.
func ComputeUpgradePaths(resolved *resolver.ResolvedResult, breakingChanges []breaking.BreakingChange) []UpgradePath {
	var paths []UpgradePath

	for _, dep := range resolved.Dependencies {
		path := computeUpgradePath(dep)
		if len(path.Steps) > 0 {
			// Check if this dependency has any breaking changes
			depBreakingCount := countBreakingChangesForDep(breakingChanges, dep)
			if depBreakingCount > 0 {
				// Clear the safe target — upgrading requires code changes
				path.NonBreakingTarget = ""
				// Mark all steps as unsafe and populate breaking change count
				for i := range path.Steps {
					path.Steps[i].Safe = false
					path.Steps[i].BreakingChanges = depBreakingCount
				}
			}
			paths = append(paths, path)
		}
	}

	return paths
}

// countBreakingChangesForDep counts breaking changes that match a dependency.
// For modules: matches bc.Provider against dep.Source.
// For providers: matches bc.Provider against dep.Name.
func countBreakingChangesForDep(changes []breaking.BreakingChange, dep resolver.ResolvedDependency) int {
	count := 0
	for _, bc := range changes {
		if bc.Severity < breaking.SeverityWarning {
			continue
		}
		if dep.IsModule {
			if bc.Provider == dep.Source {
				count++
			}
		} else {
			if bc.Provider == dep.Name {
				count++
			}
		}
	}
	return count
}

func computeUpgradePath(dep resolver.ResolvedDependency) UpgradePath {
	path := UpgradePath{
		Name: dep.Name,
	}

	if dep.Current == nil || dep.Latest == nil {
		return path
	}

	cs := dep.Current.Segments()
	ls := dep.Latest.Segments()
	for len(cs) < 3 {
		cs = append(cs, 0)
	}
	for len(ls) < 3 {
		ls = append(ls, 0)
	}

	currentMajor := cs[0]
	latestMajor := ls[0]

	if currentMajor == latestMajor {
		// Same major version — single step upgrade
		path.Steps = append(path.Steps, UpgradeStep{
			From: dep.Current.String(),
			To:   dep.Latest.String(),
			Safe: dep.UpdateType <= resolver.UpdateMinor,
		})
		path.NonBreakingTarget = dep.Latest.String()
		return path
	}

	// Different major versions — step through each major version
	// Find the latest version of each major in between
	majorVersions := make(map[int]string) // major → latest version string
	for _, v := range dep.AllVersions {
		segs := v.Segments()
		if len(segs) < 1 {
			continue
		}
		maj := segs[0]
		if maj >= currentMajor && maj <= latestMajor {
			majorVersions[maj] = v.String()
		}
	}

	// Step 1: upgrade to latest of current major
	if latestOfCurrentMajor, ok := majorVersions[currentMajor]; ok && latestOfCurrentMajor != dep.Current.String() {
		path.Steps = append(path.Steps, UpgradeStep{
			From: dep.Current.String(),
			To:   latestOfCurrentMajor,
			Safe: true,
		})
		path.NonBreakingTarget = latestOfCurrentMajor
	}

	// Step 2+: upgrade through each subsequent major version
	for maj := currentMajor + 1; maj <= latestMajor; maj++ {
		latestOfMaj, ok := majorVersions[maj]
		if !ok {
			continue
		}

		prev := dep.Current.String()
		if len(path.Steps) > 0 {
			prev = path.Steps[len(path.Steps)-1].To
		}

		path.Steps = append(path.Steps, UpgradeStep{
			From: prev,
			To:   latestOfMaj,
			Safe: false, // major version bumps are always potentially breaking
		})
	}

	return path
}

// CountBreakingChangesForUpgrade counts breaking changes between two versions.
func CountBreakingChangesForUpgrade(changes []breaking.BreakingChange, provider, from, to string) int {
	count := 0
	for _, bc := range changes {
		if bc.Provider == provider {
			count++
		}
	}
	return count
}
