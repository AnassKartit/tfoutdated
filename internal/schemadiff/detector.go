package schemadiff

import (
	"fmt"
	"sync"

	"github.com/anasskartit/tfoutdated/internal/breaking"
)

// Detector performs dynamic breaking change detection by diffing module schemas.
type Detector struct {
	fetcher *ModuleFetcher
}

// NewDetector creates a new schema diff detector.
func NewDetector() *Detector {
	return &Detector{
		fetcher: NewModuleFetcher(),
	}
}

// DetectInput describes a module to check for breaking changes.
type DetectInput struct {
	Source         string // registry source, e.g. "Azure/avm-res-network-virtualnetwork/azurerm"
	CurrentVersion string
	LatestVersion  string
}

// DetectResult holds the results for a single module.
type DetectResult struct {
	Source  string
	Changes []SchemaChange
	Error   error
}

// Detect checks multiple modules for breaking changes by diffing their schemas.
func (d *Detector) Detect(inputs []DetectInput) []DetectResult {
	var (
		results []DetectResult
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	// Limit concurrency to avoid rate limiting
	sem := make(chan struct{}, 4)

	for _, input := range inputs {
		wg.Add(1)
		go func(in DetectInput) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := DetectResult{Source: in.Source}

			oldMod, err := d.fetcher.FetchModuleSchema(in.Source, in.CurrentVersion)
			if err != nil {
				result.Error = fmt.Errorf("fetching %s@%s: %w", in.Source, in.CurrentVersion, err)
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				return
			}

			newMod, err := d.fetcher.FetchModuleSchema(in.Source, in.LatestVersion)
			if err != nil {
				result.Error = fmt.Errorf("fetching %s@%s: %w", in.Source, in.LatestVersion, err)
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				return
			}

			result.Changes = DiffModules(oldMod, newMod, in.Source)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(input)
	}

	wg.Wait()
	return results
}

// ToBreakingChanges converts schema diff results to the standard BreakingChange format.
func ToBreakingChanges(results []DetectResult, version string) []breaking.BreakingChange {
	var bcs []breaking.BreakingChange

	for _, r := range results {
		if r.Error != nil {
			continue
		}
		for _, change := range r.Changes {
			bc := breaking.BreakingChange{
				Provider:        r.Source,
				Version:         version,
				Kind:            change.Kind,
				Severity:        change.Severity,
				IsModule:        true,
				Attribute:       change.Name,
				Description:     change.Description,
				OldValue:        change.OldValue,
				NewValue:        change.NewValue,
				EffortLevel:     effortFromKind(change.Kind),
				MigrationGuide:  migrationFromChange(change),
				DynamicDetected: true,
			}

			// Generate Transform rules for auto-fixable changes
			switch change.Kind {
			case breaking.VariableRenamed:
				bc.AutoFixable = true
				transform := &breaking.Transform{
					RenameAttrs: map[string]string{change.OldValue: change.NewValue},
				}
				if change.ValueHint.Confidence != breaking.ValueHintNone {
					transform.ValueHints = map[string]breaking.ValueHint{
						change.OldValue: change.ValueHint,
					}
				}
				bc.Transform = transform
			case breaking.VariableRemoved:
				bc.AutoFixable = false
				bc.Transform = &breaking.Transform{
					RemoveAttrs: []string{change.Name},
				}
			case breaking.RequiredAdded:
				bc.AutoFixable = false
			default:
				bc.AutoFixable = false
			}

			bcs = append(bcs, bc)
		}
	}

	return bcs
}

func effortFromKind(kind breaking.ChangeKind) string {
	switch kind {
	case breaking.VariableRemoved, breaking.ProviderMigrated:
		return "large"
	case breaking.VariableRenamed, breaking.TypeChanged, breaking.RequiredAdded:
		return "medium"
	case breaking.OutputRemoved, breaking.OutputRenamed, breaking.DefaultChanged:
		return "small"
	default:
		return "small"
	}
}

func migrationFromChange(change SchemaChange) string {
	switch change.Kind {
	case breaking.VariableRemoved:
		return fmt.Sprintf("Remove '%s' from your module call. Check module documentation for replacement.", change.Name)
	case breaking.VariableRenamed:
		return fmt.Sprintf("Rename '%s' to '%s' in your module call.", change.OldValue, change.NewValue)
	case breaking.OutputRemoved:
		return fmt.Sprintf("Output '%s' no longer exists. Update any references to this output.", change.Name)
	case breaking.OutputRenamed:
		return fmt.Sprintf("Rename references from module.<name>.%s to module.<name>.%s.", change.OldValue, change.NewValue)
	case breaking.TypeChanged:
		return fmt.Sprintf("Variable '%s' type changed from %s to %s. Update your value accordingly.", change.Name, change.OldValue, change.NewValue)
	case breaking.RequiredAdded:
		return fmt.Sprintf("Add required variable '%s' to your module call.", change.Name)
	case breaking.ProviderMigrated:
		return fmt.Sprintf("Module now requires the %s provider. Add it to your required_providers block.", change.NewValue)
	default:
		return change.Description
	}
}
