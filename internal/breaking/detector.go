package breaking

import (
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// Detector coordinates breaking change detection from multiple sources.
type Detector struct {
	kb *KnowledgeBase
}

// NewDetector creates a new breaking change detector.
func NewDetector() *Detector {
	return &Detector{
		kb: NewKnowledgeBase(),
	}
}

// Detect finds all breaking changes relevant to the resolved dependencies.
// Deduplicates so the same breaking change is not listed multiple times
// even if the same provider appears at multiple version pins.
func (d *Detector) Detect(resolved *resolver.ResolvedResult) []BreakingChange {
	var allChanges []BreakingChange
	seenChanges := make(map[string]bool) // dedup individual breaking changes

	for _, dep := range resolved.Dependencies {
		// For modules, use Source as the knowledge base key (e.g., "Azure/avm-res-network-bastionhost/azurerm").
		// For providers, use Name.
		provider := dep.Name
		if dep.IsModule {
			provider = dep.Source
		}
		fromVersion := dep.Current.String()
		toVersion := dep.Latest.String()

		kbChanges := d.kb.GetChanges(provider, fromVersion, toVersion)
		for _, bc := range kbChanges {
			// Dedup key: provider + version + resource + attribute + kind
			key := bc.Provider + ":" + bc.Version + ":" + bc.ResourceType + ":" + bc.Attribute + ":" + bc.Kind.String()
			if seenChanges[key] {
				continue
			}
			seenChanges[key] = true
			allChanges = append(allChanges, bc)
		}
	}

	return allChanges
}

// DetectWithChangelogs also parses changelogs for additional breaking changes.
// This is separate because it requires network access.
func (d *Detector) DetectWithChangelogs(resolved *resolver.ResolvedResult) []BreakingChange {
	allChanges := d.Detect(resolved)

	fetcher := NewFetcher()
	seen := make(map[string]bool)

	// Process provider changelogs
	for _, dep := range resolved.Dependencies {
		if dep.IsModule {
			continue
		}

		provider := dep.Name
		if seen[provider] {
			continue
		}
		seen[provider] = true

		changelog, err := fetcher.FetchChangelog(provider)
		if err != nil {
			continue
		}

		parsed := ParseChangelog(provider, changelog)

		// Filter to relevant version range
		for _, bc := range parsed {
			if bc.Version > dep.Current.String() && bc.Version <= dep.Latest.String() {
				if !d.isKnown(bc) {
					allChanges = append(allChanges, bc)
				}
			}
		}
	}

	// Process module changelogs
	seenModules := make(map[string]bool)
	for _, dep := range resolved.Dependencies {
		if !dep.IsModule {
			continue
		}

		if seenModules[dep.Source] {
			continue
		}
		seenModules[dep.Source] = true

		changelog, err := fetcher.FetchModuleChangelog(dep.Source)
		if err != nil {
			continue
		}

		parsed := ParseChangelog(dep.Name, changelog)

		// Filter to relevant version range and mark as module changes
		for _, bc := range parsed {
			if bc.Version > dep.Current.String() && bc.Version <= dep.Latest.String() {
				bc.IsModule = true
				if !d.isKnown(bc) {
					allChanges = append(allChanges, bc)
				}
			}
		}
	}

	return allChanges
}

func (d *Detector) isKnown(bc BreakingChange) bool {
	kbChanges := d.kb.GetChanges(bc.Provider, "", bc.Version)
	for _, known := range kbChanges {
		if known.ResourceType == bc.ResourceType && known.Attribute == bc.Attribute && known.Version == bc.Version {
			return true
		}
	}
	return false
}
