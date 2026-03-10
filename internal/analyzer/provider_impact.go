package analyzer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

// AnalyzeProviderImpact checks each module's provider_dependencies against a target provider version.
func AnalyzeProviderImpact(scan *scanner.ScanResult, registry *resolver.RegistryClient, targetProvider, targetVersion string) (*ProviderImpactSummary, error) {
	start := time.Now()

	var modules []scanner.ModuleDependency
	for _, m := range scan.Modules {
		if m.SourceType == "registry" {
			modules = append(modules, m)
		}
	}

	var results []ProviderImpactResult
	for _, mod := range modules {
		result := analyzeModuleProviderImpact(mod, registry, targetProvider, targetVersion)
		results = append(results, result)
	}

	summary := &ProviderImpactSummary{
		TargetProvider: targetProvider,
		TargetVersion:  targetVersion,
		TotalModules:   len(results),
		Results:        results,
		DurationMs:     time.Since(start).Milliseconds(),
	}

	for _, r := range results {
		if r.Compatible && !r.RequiresModuleUpgrade {
			summary.Compatible++
		} else if r.RequiresModuleUpgrade && r.MinCompatibleVer != "" {
			summary.NeedUpgrade++
		} else {
			summary.Incompatible++
		}
	}

	return summary, nil
}

func analyzeModuleProviderImpact(mod scanner.ModuleDependency, registry *resolver.RegistryClient, targetProvider, targetVersion string) ProviderImpactResult {
	result := ProviderImpactResult{
		Module:           mod.Name,
		ModuleSource:     mod.Source,
		CurrentModuleVer: mod.Version,
		File:             mod.FilePath,
		Line:             mod.Line,
	}

	// Get all module versions
	versions, err := registry.GetModuleVersions(mod.Source)
	if err != nil {
		return result
	}
	if len(versions) > 0 {
		result.LatestModuleVer = versions[len(versions)-1].String()
	}

	// Get provider deps for current module version
	if mod.Version != "latest" && mod.Version != "" {
		cleanVer := cleanVersionConstraint(mod.Version)
		currentDeps, err := registry.GetModuleProviderDeps(mod.Source, cleanVer)
		if err == nil {
			result.CurrentProviderDeps = currentDeps
			result.Compatible = isCompatible(currentDeps, targetProvider, targetVersion)
		}
	}

	// Get provider deps for latest module version
	if result.LatestModuleVer != "" {
		latestDeps, err := registry.GetModuleProviderDeps(mod.Source, result.LatestModuleVer)
		if err == nil {
			result.LatestProviderDeps = latestDeps
		}
	}

	// If current is not compatible, find minimum compatible version
	if !result.Compatible {
		result.RequiresModuleUpgrade = true
		var allVerStrings []string
		for _, v := range versions {
			allVerStrings = append(allVerStrings, v.String())
		}
		result.MinCompatibleVer = findMinCompatibleVersion(mod.Source, allVerStrings, registry, targetProvider, targetVersion)
	}

	return result
}

func cleanVersionConstraint(v string) string {
	for _, prefix := range []string{"~>", ">=", "<=", "!=", ">", "<", "="} {
		v = strings.TrimPrefix(strings.TrimSpace(v), prefix)
	}
	v = strings.TrimSpace(v)
	if idx := strings.Index(v, ","); idx >= 0 {
		v = strings.TrimSpace(v[:idx])
	}
	return v
}

// isCompatible checks if a set of provider deps is satisfied by targetProvider@targetVersion.
func isCompatible(deps []resolver.ProviderDep, targetProvider, targetVersion string) bool {
	for _, dep := range deps {
		if !matchesProviderDep(dep, targetProvider) {
			continue
		}
		if satisfiesConstraint(targetVersion, dep.Version) {
			return true
		}
		return false
	}
	return true
}

func matchesProviderDep(dep resolver.ProviderDep, targetProvider string) bool {
	return strings.EqualFold(dep.Source, targetProvider) ||
		strings.EqualFold(fmt.Sprintf("%s/%s", dep.Namespace, dep.Name), targetProvider)
}

func satisfiesConstraint(version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	parts := strings.Split(constraint, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if !satisfiesSingle(version, part) {
			return false
		}
	}
	return true
}

func satisfiesSingle(version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)

	if strings.HasPrefix(constraint, "~>") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, "~>"))
		return satisfiesPessimistic(version, base)
	}
	if strings.HasPrefix(constraint, ">=") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, ">="))
		return compareVersionNums(version, base) >= 0
	}
	if strings.HasPrefix(constraint, "<=") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, "<="))
		return compareVersionNums(version, base) <= 0
	}
	if strings.HasPrefix(constraint, "!=") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, "!="))
		return compareVersionNums(version, base) != 0
	}
	if strings.HasPrefix(constraint, ">") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, ">"))
		return compareVersionNums(version, base) > 0
	}
	if strings.HasPrefix(constraint, "<") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, "<"))
		return compareVersionNums(version, base) < 0
	}
	if strings.HasPrefix(constraint, "=") {
		base := strings.TrimSpace(strings.TrimPrefix(constraint, "="))
		return compareVersionNums(version, base) == 0
	}
	return compareVersionNums(version, constraint) == 0
}

func satisfiesPessimistic(version, base string) bool {
	baseParts := piParseVersion(base)
	verParts := piParseVersion(version)

	if len(baseParts) == 0 || len(verParts) == 0 {
		return false
	}

	if compareVersionNums(version, base) < 0 {
		return false
	}

	lockCount := len(baseParts) - 1
	if lockCount < 1 {
		lockCount = 1
	}

	for i := 0; i < lockCount && i < len(verParts) && i < len(baseParts); i++ {
		if verParts[i] != baseParts[i] {
			return false
		}
	}

	return true
}

func piParseVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, "-"); idx >= 0 {
		v = v[:idx]
	}
	if idx := strings.Index(v, "+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums = append(nums, n)
	}
	return nums
}

func compareVersionNums(a, b string) int {
	pa := piParseVersion(a)
	pb := piParseVersion(b)
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := 0; i < maxLen; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

func findMinCompatibleVersion(source string, allVersions []string, registry *resolver.RegistryClient, targetProvider, targetVersion string) string {
	sorted := make([]string, len(allVersions))
	copy(sorted, allVersions)
	sort.Slice(sorted, func(i, j int) bool {
		return compareVersionNums(sorted[i], sorted[j]) > 0
	})

	var lastCompatible string
	for _, ver := range sorted {
		if strings.Contains(ver, "-") {
			continue
		}
		deps, err := registry.GetModuleProviderDeps(source, ver)
		if err != nil {
			continue
		}
		if isCompatible(deps, targetProvider, targetVersion) {
			lastCompatible = ver
		} else if lastCompatible != "" {
			break
		}
	}
	return lastCompatible
}
