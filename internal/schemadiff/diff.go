package schemadiff

import (
	"fmt"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	"github.com/anasskartit/tfoutdated/internal/breaking"
)

// SchemaChange represents a detected change between two module versions.
type SchemaChange struct {
	Kind        breaking.ChangeKind
	Severity    breaking.Severity
	Name        string // variable/output/provider name
	Description string
	OldValue    string
	NewValue    string
	ValueHint   breaking.ValueHint
}

// DiffModules compares two parsed module schemas and returns detected changes.
func DiffModules(oldMod, newMod *tfconfig.Module, source string) []SchemaChange {
	var changes []SchemaChange

	changes = append(changes, diffVariables(oldMod, newMod, source)...)
	changes = append(changes, diffOutputs(oldMod, newMod, source)...)
	changes = append(changes, diffProviders(oldMod, newMod, source)...)
	changes = append(changes, diffResources(oldMod, newMod, source)...)

	return changes
}

func diffVariables(oldMod, newMod *tfconfig.Module, source string) []SchemaChange {
	var changes []SchemaChange

	// Step 1: Build removed and added sets
	removed := make(map[string]*tfconfig.Variable)
	added := make(map[string]*tfconfig.Variable)

	for name, oldVar := range oldMod.Variables {
		if _, exists := newMod.Variables[name]; !exists {
			removed[name] = oldVar
		}
	}
	for name, newVar := range newMod.Variables {
		if _, exists := oldMod.Variables[name]; !exists {
			added[name] = newVar
		}
	}

	// Step 2: Detect renames via multi-signal matching
	renames := detectRenames(removed, added, oldMod, newMod)

	// Track rename targets so we exclude them from RequiredAdded detection
	renameTargets := make(map[string]bool)
	for _, newName := range renames {
		renameTargets[newName] = true
	}

	// Step 3: Emit changes for removed variables
	for name, oldVar := range oldMod.Variables {
		newVar, exists := newMod.Variables[name]
		if !exists {
			if rename, ok := renames[name]; ok {
				renamedVar := newMod.Variables[rename]
				hint := InferValueHint(name, oldVar.Description, rename, renamedVar.Description)
				changes = append(changes, SchemaChange{
					Kind:        breaking.VariableRenamed,
					Severity:    breaking.SeverityBreaking,
					Name:        name,
					Description: fmt.Sprintf("Variable '%s' renamed to '%s'", name, rename),
					OldValue:    name,
					NewValue:    rename,
					ValueHint:   hint,
				})
			} else {
				changes = append(changes, SchemaChange{
					Kind:        breaking.VariableRemoved,
					Severity:    breaking.SeverityBreaking,
					Name:        name,
					Description: fmt.Sprintf("Variable '%s' has been removed", name),
					OldValue:    name,
				})
			}
			continue
		}

		// Variable type changed
		if oldVar.Type != newVar.Type && oldVar.Type != "" && newVar.Type != "" {
			changes = append(changes, SchemaChange{
				Kind:        breaking.TypeChanged,
				Severity:    breaking.SeverityBreaking,
				Name:        name,
				Description: fmt.Sprintf("Variable '%s' type changed from '%s' to '%s'", name, oldVar.Type, newVar.Type),
				OldValue:    oldVar.Type,
				NewValue:    newVar.Type,
			})
		}

		// Variable changed from optional to required
		if !oldVar.Required && newVar.Required {
			changes = append(changes, SchemaChange{
				Kind:        breaking.RequiredAdded,
				Severity:    breaking.SeverityBreaking,
				Name:        name,
				Description: fmt.Sprintf("Variable '%s' changed from optional to required", name),
			})
		}

		// Variable changed from required to optional (non-breaking but noteworthy)
		if oldVar.Required && !newVar.Required {
			changes = append(changes, SchemaChange{
				Kind:        breaking.DefaultChanged,
				Severity:    breaking.SeverityInfo,
				Name:        name,
				Description: fmt.Sprintf("Variable '%s' changed from required to optional (default added)", name),
			})
		}
	}

	// Step 4: Check for new required variables (breaking), excluding rename targets
	for name, newVar := range newMod.Variables {
		if _, exists := oldMod.Variables[name]; !exists {
			if newVar.Required && !renameTargets[name] {
				changes = append(changes, SchemaChange{
					Kind:        breaking.RequiredAdded,
					Severity:    breaking.SeverityBreaking,
					Name:        name,
					Description: fmt.Sprintf("New required variable '%s' added", name),
					NewValue:    name,
				})
			}
		}
	}

	return changes
}

func diffOutputs(oldMod, newMod *tfconfig.Module, source string) []SchemaChange {
	var changes []SchemaChange

	// Build removed and added sets
	removed := make(map[string]*tfconfig.Output)
	added := make(map[string]*tfconfig.Output)

	for name, oldOut := range oldMod.Outputs {
		if _, exists := newMod.Outputs[name]; !exists {
			removed[name] = oldOut
		}
	}
	for name, newOut := range newMod.Outputs {
		if _, exists := oldMod.Outputs[name]; !exists {
			added[name] = newOut
		}
	}

	// Detect renames
	renames := detectOutputRenames(removed, added)

	for name, oldOut := range oldMod.Outputs {
		newOut, exists := newMod.Outputs[name]
		if !exists {
			if rename, ok := renames[name]; ok {
				changes = append(changes, SchemaChange{
					Kind:        breaking.OutputRenamed,
					Severity:    breaking.SeverityBreaking,
					Name:        name,
					Description: fmt.Sprintf("Output '%s' renamed to '%s'", name, rename),
					OldValue:    name,
					NewValue:    rename,
				})
			} else {
				changes = append(changes, SchemaChange{
					Kind:        breaking.OutputRemoved,
					Severity:    breaking.SeverityBreaking,
					Name:        name,
					Description: fmt.Sprintf("Output '%s' has been removed", name),
					OldValue:    name,
				})
			}
			continue
		}

		// Output type changed
		if oldOut.Type != newOut.Type && oldOut.Type != "" && newOut.Type != "" {
			changes = append(changes, SchemaChange{
				Kind:        breaking.TypeChanged,
				Severity:    breaking.SeverityWarning,
				Name:        name,
				Description: fmt.Sprintf("Output '%s' type changed from '%s' to '%s'", name, oldOut.Type, newOut.Type),
				OldValue:    oldOut.Type,
				NewValue:    newOut.Type,
			})
		}
	}

	return changes
}

func diffProviders(oldMod, newMod *tfconfig.Module, source string) []SchemaChange {
	var changes []SchemaChange

	// Check for new required providers (potentially breaking)
	for name, newProv := range newMod.RequiredProviders {
		if _, exists := oldMod.RequiredProviders[name]; !exists {
			changes = append(changes, SchemaChange{
				Kind:        breaking.ProviderMigrated,
				Severity:    breaking.SeverityBreaking,
				Name:        name,
				Description: fmt.Sprintf("New provider '%s' (%s) is now required", name, newProv.Source),
				NewValue:    newProv.Source,
			})
		}
	}

	// Check for removed providers
	for name, oldProv := range oldMod.RequiredProviders {
		if _, exists := newMod.RequiredProviders[name]; !exists {
			changes = append(changes, SchemaChange{
				Kind:        breaking.BehaviorChanged,
				Severity:    breaking.SeverityInfo,
				Name:        name,
				Description: fmt.Sprintf("Provider '%s' (%s) is no longer required", name, oldProv.Source),
				OldValue:    oldProv.Source,
			})
		}
	}

	return changes
}

func diffResources(oldMod, newMod *tfconfig.Module, source string) []SchemaChange {
	var changes []SchemaChange

	// Detect provider migration (e.g., azurerm → azapi)
	oldProviders := countResourceProviders(oldMod)
	newProviders := countResourceProviders(newMod)

	for prov, newCount := range newProviders {
		oldCount := oldProviders[prov]
		if oldCount == 0 && newCount > 0 {
			// New provider used in resources that wasn't before
			// Check if another provider decreased — suggests migration
			for oldProv, oldC := range oldProviders {
				newC := newProviders[oldProv]
				if newC < oldC && oldProv != prov {
					changes = append(changes, SchemaChange{
						Kind:     breaking.ProviderMigrated,
						Severity: breaking.SeverityWarning,
						Name:     "internal_resources",
						Description: fmt.Sprintf("Internal resources migrated from %s to %s provider (%d → %d resources)",
							oldProv, prov, oldC, newC),
						OldValue: oldProv,
						NewValue: prov,
					})
				}
			}
		}
	}

	return changes
}

func countResourceProviders(mod *tfconfig.Module) map[string]int {
	counts := make(map[string]int)
	for _, r := range mod.ManagedResources {
		counts[r.Provider.Name]++
	}
	return counts
}

