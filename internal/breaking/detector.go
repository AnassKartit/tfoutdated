package breaking

import (
	goversion "github.com/hashicorp/go-version"

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
	allChanges = append(allChanges, d.ProviderChangelogChanges(resolved)...)

	fetcher := NewFetcher()

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
			if inVersionRange(bc.Version, dep.Current.String(), dep.Latest.String()) {
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

// ProviderChangelogChanges reads each provider's changelog and returns the
// breaking changes released after the current version, up to the latest, that
// the knowledge base does not already cover. Needs network access.
func (d *Detector) ProviderChangelogChanges(resolved *resolver.ResolvedResult) []BreakingChange {
	var changes []BreakingChange
	fetcher := NewFetcher()
	seen := make(map[string]bool)

	for _, dep := range resolved.Dependencies {
		if dep.IsModule || dep.Current == nil || dep.Latest == nil {
			continue
		}
		key := dep.Name + "@" + dep.Current.String() + "->" + dep.Latest.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		changelog, err := fetcher.FetchChangelog(dep.Name)
		if err != nil {
			continue
		}
		for _, bc := range ParseChangelog(dep.Name, changelog) {
			if inVersionRange(bc.Version, dep.Current.String(), dep.Latest.String()) && !d.isKnown(bc) {
				changes = append(changes, bc)
			}
		}
	}
	return changes
}

// inVersionRange reports from < v <= to, comparing as versions: as strings,
// "10.0.0" sorts before "9.0.0".
func inVersionRange(v, from, to string) bool {
	ver, err := goversion.NewVersion(v)
	if err != nil {
		return false
	}
	lo, err := goversion.NewVersion(from)
	if err != nil {
		return false
	}
	hi, err := goversion.NewVersion(to)
	if err != nil {
		return false
	}
	return ver.GreaterThan(lo) && ver.LessThanOrEqual(hi)
}
