package analyzer

import (
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

// AnalyzeImpact maps breaking changes to actual resources in scanned files.
func AnalyzeImpact(analysis *Analysis, scan *scanner.ScanResult) []ImpactItem {
	var impacts []ImpactItem

	for _, bc := range analysis.BreakingChanges {
		// For provider-level breaking changes, find matching dependencies
		matched := false

		// If the breaking change targets a specific resource type, match against scanned resource blocks
		if bc.ResourceType != "" && scan != nil {
			for _, res := range scan.Resources {
				if res.Type != bc.ResourceType {
					continue
				}

				item := ImpactItem{
					FilePath:          res.FilePath,
					Line:              res.StartLine,
					ResourceType:      bc.ResourceType,
					Attribute:         bc.Attribute,
					BreakingChange:    bc,
					ChangeDescription: bc.Description,
					AffectedFile:      res.FilePath,
					AffectedLine:      res.StartLine,
					ResourceName:      res.Type + "." + res.Name,
					ActualBefore:      res.RawHCL,
				}

				// Apply deterministic transform if available
				if bc.Transform != nil {
					after, linesChanged := breaking.ApplyTransform(res.RawHCL, bc.Transform)
					item.ActualAfter = after
					item.LinesChanged = linesChanged
				}

				impacts = append(impacts, item)
				matched = true
			}
		}

		// Also check for provider config blocks (e.g., features block changes)
		if bc.Kind == breaking.ProviderConfigChanged && scan != nil {
			for _, res := range scan.Resources {
				if res.Type == "provider" && res.Name == bc.Provider {
					item := ImpactItem{
						FilePath:          res.FilePath,
						Line:              res.StartLine,
						ResourceType:      "provider",
						Attribute:         bc.Attribute,
						BreakingChange:    bc,
						ChangeDescription: bc.Description,
						AffectedFile:      res.FilePath,
						AffectedLine:      res.StartLine,
						ResourceName:      "provider." + res.Name,
						ActualBefore:      res.RawHCL,
					}

					if bc.Transform != nil {
						after, linesChanged := breaking.ApplyTransform(res.RawHCL, bc.Transform)
						item.ActualAfter = after
						item.LinesChanged = linesChanged
					}

					impacts = append(impacts, item)
					matched = true
				}
			}
		}

		// Fall back to dependency-level matching (existing behavior)
		if !matched {
			for _, dep := range analysis.Dependencies {
				if !matchesProvider(dep, bc) {
					continue
				}

				impacts = append(impacts, ImpactItem{
					FilePath:          dep.FilePath,
					Line:              dep.Line,
					ResourceType:      bc.ResourceType,
					Attribute:         bc.Attribute,
					BreakingChange:    bc,
					ChangeDescription: bc.Description,
				})
			}
		}
	}

	return impacts
}

func matchesProvider(dep DependencyAnalysis, bc breaking.BreakingChange) bool {
	if dep.IsModule {
		return dep.Source == bc.Provider
	}
	return dep.Name == bc.Provider || dep.Source == bc.Provider
}
