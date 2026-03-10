package cmd

import (
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/schemadiff"
)

// detectAllBreakingChanges runs both the knowledge base and dynamic schema diff
// detection, returning a deduplicated list of breaking changes.
func detectAllBreakingChanges(resolved *resolver.ResolvedResult) []breaking.BreakingChange {
	bd := breaking.NewDetector()
	breakingChanges := bd.Detect(resolved)

	// Dynamic schema diffing for modules
	var inputs []schemadiff.DetectInput
	for _, dep := range resolved.Dependencies {
		if dep.IsModule {
			inputs = append(inputs, schemadiff.DetectInput{
				Source:         dep.Source,
				CurrentVersion: dep.Current.String(),
				LatestVersion:  dep.Latest.String(),
			})
		}
	}
	if len(inputs) == 0 {
		return breakingChanges
	}

	sd := schemadiff.NewDetector()
	diffResults := sd.Detect(inputs)
	for _, r := range diffResults {
		if r.Error != nil {
			continue
		}
		latestVer := ""
		for _, dep := range resolved.Dependencies {
			if dep.IsModule && dep.Source == r.Source {
				latestVer = dep.Latest.String()
				break
			}
		}
		dynamicChanges := schemadiff.ToBreakingChanges([]schemadiff.DetectResult{r}, latestVer)
		for _, dc := range dynamicChanges {
			isDuplicate := false
			for _, existing := range breakingChanges {
				if existing.Provider == dc.Provider && existing.Attribute == dc.Attribute && existing.Kind == dc.Kind {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				breakingChanges = append(breakingChanges, dc)
			}
		}
	}

	return breakingChanges
}
