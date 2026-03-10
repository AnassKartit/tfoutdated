package fixer

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

// ProviderChange describes a provider constraint update applied to a file.
type ProviderChange struct {
	FilePath      string
	ProviderName  string
	OldConstraint string
	NewConstraint string
	Line          int
	Reason        string
}

// FixProviderConstraints checks whether module upgrades require wider provider
// constraints and, if so, updates them. It returns the list of changes made (or
// that would be made in dry-run mode).
func (f *Fixer) FixProviderConstraints(
	moduleChanges []Change,
	scan *scanner.ScanResult,
	registryClient *resolver.RegistryClient,
) ([]ProviderChange, error) {
	if len(moduleChanges) == 0 || scan == nil || registryClient == nil {
		return nil, nil
	}

	// Build a lookup from (module name, filepath) -> ModuleDependency so we can
	// find the Source for each changed module.
	modLookup := make(map[string]scanner.ModuleDependency)
	for _, m := range scan.Modules {
		key := m.Name + "\x00" + m.FilePath
		modLookup[key] = m
	}

	// Collect provider requirements from all upgraded modules.
	// Map: "namespace/name" -> list of constraint strings from modules.
	providerReqs := make(map[string][]string)
	providerReasons := make(map[string][]string)

	for _, change := range moduleChanges {
		key := change.Name + "\x00" + change.FilePath
		mod, ok := modLookup[key]
		if !ok {
			continue
		}

		deps, err := registryClient.GetModuleProviderDeps(mod.Source, change.NewVersion)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch provider deps for %s@%s: %v\n", mod.Source, change.NewVersion, err)
			continue
		}

		for _, dep := range deps {
			if dep.Version == "" {
				continue
			}
			ns := dep.Namespace
			if ns == "" {
				ns = "hashicorp"
			}
			provKey := ns + "/" + dep.Name
			providerReqs[provKey] = append(providerReqs[provKey], dep.Version)
			providerReasons[provKey] = append(providerReasons[provKey],
				fmt.Sprintf("%s@%s", mod.Source, change.NewVersion))
		}
	}

	if len(providerReqs) == 0 {
		return nil, nil
	}

	// Build a lookup for current provider constraints from the scan.
	// Multiple entries for the same provider (e.g., cdktf.json + package.json) are kept.
	providersByKey := make(map[string][]*scanner.ProviderDependency)
	for i := range scan.Providers {
		p := &scan.Providers[i]
		ns := p.Namespace
		if ns == "" {
			ns = "hashicorp"
		}
		provKey := ns + "/" + p.Name
		providersByKey[provKey] = append(providersByKey[provKey], p)
	}

	var changes []ProviderChange
	byFile := make(map[string][]Change)

	for provKey, constraints := range providerReqs {
		// Find the highest minimum version required across all module constraints.
		var highestMin string
		for _, c := range constraints {
			minVer := extractMinVersion(c)
			if minVer == "" {
				continue
			}
			if highestMin == "" || compareVersions(minVer, highestMin) > 0 {
				highestMin = minVer
			}
		}
		if highestMin == "" {
			continue
		}

		provs, ok := providersByKey[provKey]
		if !ok || len(provs) == 0 {
			// Provider not declared in the user's config; skip (they may rely on
			// implicit provider requirements).
			continue
		}

		reason := fmt.Sprintf("required by %s", strings.Join(unique(providerReasons[provKey]), ", "))

		for _, prov := range provs {
			if isConstraintCompatible(prov.Version, highestMin) {
				continue
			}

			newConstraint := buildNewConstraint(prov.Version, highestMin)
			if newConstraint == prov.Version {
				continue
			}

			pc := ProviderChange{
				FilePath:      prov.FilePath,
				ProviderName:  prov.Name,
				OldConstraint: prov.Version,
				NewConstraint: newConstraint,
				Line:          prov.Line,
				Reason:        reason,
			}
			changes = append(changes, pc)

			// Reuse the existing rewriteVersions infrastructure via Change structs.
			byFile[prov.FilePath] = append(byFile[prov.FilePath], Change{
				FilePath:   prov.FilePath,
				Name:       prov.Name,
				OldVersion: prov.Version,
				NewVersion: newConstraint,
				Line:       prov.Line,
			})
		}
	}

	if f.opts.DryRun || len(changes) == 0 {
		return changes, nil
	}

	for filePath, fileChanges := range byFile {
		if IsCdktfFile(filePath) {
			if strings.HasSuffix(filePath, "cdktf.json") {
				if err := applyCdktfChanges(filePath, fileChanges); err != nil {
					return changes, fmt.Errorf("applying cdktf provider changes to %s: %w", filePath, err)
				}
			} else if strings.HasSuffix(filePath, "package.json") {
				if err := applyPackageJSONChanges(filePath, fileChanges); err != nil {
					return changes, fmt.Errorf("applying package.json provider changes to %s: %w", filePath, err)
				}
			}
			continue
		}
		if err := applyChanges(filePath, fileChanges); err != nil {
			return changes, fmt.Errorf("applying provider constraint changes to %s: %w", filePath, err)
		}
	}

	return changes, nil
}

// extractMinVersion parses the minimum version from a constraint string.
// Examples:
//
//	"~> 3.75.0"   -> "3.75.0"
//	">= 4.0"      -> "4.0"
//	">= 3.0, < 5" -> "3.0"
//	"3.75.0"       -> "3.75.0"
func extractMinVersion(constraint string) string {
	// Handle compound constraints — take the first part which is typically the
	// lower bound.
	parts := strings.Split(constraint, ",")
	first := strings.TrimSpace(parts[0])

	// Strip constraint operators.
	for _, prefix := range []string{"~>", ">=", ">", "="} {
		if strings.HasPrefix(first, prefix) {
			first = strings.TrimSpace(strings.TrimPrefix(first, prefix))
			break
		}
	}

	// Validate it looks like a version.
	first = strings.TrimSpace(first)
	if first == "" {
		return ""
	}
	// Quick sanity: must start with a digit.
	if first[0] < '0' || first[0] > '9' {
		return ""
	}
	return first
}

// parseVersionParts splits "X.Y.Z" into major, minor, patch integers.
// Missing parts default to 0.
func parseVersionParts(v string) (major, minor, patch int) {
	parts := strings.SplitN(v, ".", 4)
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return
}

// compareVersions returns >0 if a > b, <0 if a < b, 0 if equal.
func compareVersions(a, b string) int {
	aMaj, aMin, aPat := parseVersionParts(a)
	bMaj, bMin, bPat := parseVersionParts(b)
	if aMaj != bMaj {
		return aMaj - bMaj
	}
	if aMin != bMin {
		return aMin - bMin
	}
	return aPat - bPat
}

// isConstraintCompatible returns true if the existing constraint already allows
// the requiredMin version (i.e., no change is needed).
func isConstraintCompatible(constraint string, requiredMin string) bool {
	reqMaj, reqMin, reqPat := parseVersionParts(requiredMin)

	// Parse the existing constraint to understand what it allows.
	trimmed := strings.TrimSpace(constraint)

	// Handle compound constraints (e.g., ">= 3.0, < 5.0").
	parts := strings.Split(trimmed, ",")
	first := strings.TrimSpace(parts[0])

	if strings.HasPrefix(first, "~>") {
		// Pessimistic constraint: ~> X.Y.Z allows >= X.Y.Z and < X.(Y+1).0
		// ~> X.Y allows >= X.Y.0 and < (X+1).0.0
		ver := strings.TrimSpace(strings.TrimPrefix(first, "~>"))
		maj, min, pat := parseVersionParts(ver)
		verParts := strings.Split(ver, ".")

		if len(verParts) >= 3 {
			// ~> X.Y.Z: allows [X.Y.Z, X.(Y+1).0)
			if reqMaj != maj || reqMin != min {
				return false
			}
			return reqPat >= pat
		}
		// ~> X.Y: allows [X.Y.0, (X+1).0.0)
		if reqMaj != maj {
			return false
		}
		return reqMin >= min
	}

	if strings.HasPrefix(first, ">=") {
		// >= X.Y.Z: allows anything >= X.Y.Z
		ver := strings.TrimSpace(strings.TrimPrefix(first, ">="))
		// The constraint allows requiredMin if the lower bound <= requiredMin.
		if compareVersions(ver, requiredMin) <= 0 {
			// The lower bound is satisfied. Check upper bound if present.
			if len(parts) > 1 {
				upper := strings.TrimSpace(parts[1])
				if strings.HasPrefix(upper, "<=") {
					upperVer := strings.TrimSpace(strings.TrimPrefix(upper, "<="))
					return compareVersions(requiredMin, upperVer) <= 0
				}
				if strings.HasPrefix(upper, "<") {
					upperVer := strings.TrimSpace(strings.TrimPrefix(upper, "<"))
					return compareVersions(requiredMin, upperVer) < 0
				}
			}
			return true
		}
		return false
	}

	// Bare version or "= X.Y.Z" — exact match only.
	ver := strings.TrimSpace(strings.TrimPrefix(first, "="))
	ver = strings.TrimSpace(ver)
	return compareVersions(ver, requiredMin) >= 0 && reqMaj == parseVersionMajor(ver)
}

func parseVersionMajor(v string) int {
	maj, _, _ := parseVersionParts(v)
	return maj
}

// buildNewConstraint creates a new constraint string that allows requiredMin
// while preserving the user's constraint style.
func buildNewConstraint(oldConstraint string, requiredMin string) string {
	trimmed := strings.TrimSpace(oldConstraint)
	reqMaj, reqMin, _ := parseVersionParts(requiredMin)

	// Detect cross-major bump
	oldMin := extractMinVersion(trimmed)
	oldMaj, _, _ := parseVersionParts(oldMin)
	isCrossMajor := oldMin != "" && reqMaj > oldMaj

	if strings.HasPrefix(trimmed, "~>") {
		if isCrossMajor {
			// Cross-major: always use two-part ~> X.Y to allow all patches in the new major
			// e.g., ~> 3.75.0 → ~> 4.10 (allows 4.10.x through 4.999.x)
			return fmt.Sprintf("~> %d.%d", reqMaj, reqMin)
		}
		oldVer := strings.TrimSpace(strings.TrimPrefix(trimmed, "~>"))
		oldParts := strings.Split(oldVer, ".")

		if len(oldParts) >= 3 {
			// Same major, three-part style -> ~> reqMaj.reqMin.0
			return fmt.Sprintf("~> %d.%d.0", reqMaj, reqMin)
		}
		// ~> X.Y style -> ~> reqMaj.reqMin
		return fmt.Sprintf("~> %d.%d", reqMaj, reqMin)
	}

	if strings.HasPrefix(trimmed, ">=") {
		// Check for compound constraint like ">= X.Y, < Z.0"
		parts := strings.SplitN(trimmed, ",", 2)
		if len(parts) == 2 {
			// Preserve the upper bound if it's still valid.
			upper := strings.TrimSpace(parts[1])
			// Extract version from "<" or "<=" prefix
			upperVer := upper
			upperVer = strings.TrimPrefix(upperVer, "<=")
			upperVer = strings.TrimPrefix(upperVer, "<")
			upperVer = strings.TrimSpace(upperVer)
			if upperVer != "" && compareVersions(requiredMin, upperVer) < 0 {
				return fmt.Sprintf(">= %s, %s", requiredMin, upper)
			}
			// Upper bound is no longer valid; just use >= requiredMin
		}
		return fmt.Sprintf(">= %s", requiredMin)
	}

	// Bare version or unknown style — use pessimistic constraint.
	return fmt.Sprintf("~> %d.%d.0", reqMaj, reqMin)
}

// unique deduplicates a string slice, preserving order.
func unique(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
