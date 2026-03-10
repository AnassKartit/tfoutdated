package recommend

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// Recommend analyzes dependencies and generates actionable recommendations.
func Recommend(analysis *analyzer.Analysis) []analyzer.Recommendation {
	var recs []analyzer.Recommendation

	recs = append(recs, checkProviderFragmentation(analysis)...)
	recs = append(recs, checkUnpinnedVersions(analysis)...)
	recs = append(recs, checkMajorDrift(analysis)...)
	recs = append(recs, checkDeprecated(analysis)...)
	recs = append(recs, checkModuleStaleness(analysis)...)
	recs = append(recs, checkAVMPreOneWarnings(analysis)...)
	recs = append(recs, checkAVMProviderConflicts(analysis)...)

	return recs
}

// checkProviderFragmentation detects multiple versions of the same provider across files.
func checkProviderFragmentation(analysis *analyzer.Analysis) []analyzer.Recommendation {
	type fileVersion struct {
		File    string
		Line    int
		Version string
	}

	providerFiles := make(map[string][]fileVersion)
	providerLatest := make(map[string]string)

	for _, dep := range analysis.Dependencies {
		if dep.IsModule {
			continue
		}
		src := strings.ToLower(dep.Source)
		providerFiles[src] = append(providerFiles[src], fileVersion{
			File:    dep.FilePath,
			Line:    dep.Line,
			Version: dep.CurrentVer,
		})
		if dep.LatestVer != "" {
			providerLatest[src] = dep.LatestVer
		}
	}

	var recs []analyzer.Recommendation
	for source, files := range providerFiles {
		versionSet := make(map[string]bool)
		for _, fv := range files {
			versionSet[fv.Version] = true
		}
		if len(versionSet) <= 1 {
			continue
		}

		var versions []string
		for v := range versionSet {
			versions = append(versions, v)
		}
		sort.Slice(versions, func(i, j int) bool {
			return compareVersionNums(versions[i], versions[j]) > 0
		})
		highestCurrent := versions[0]
		latest := providerLatest[source]

		targetVersion := highestCurrent
		hasMajorSplit := hasMajorVersionSplit(versions)

		severity := "medium"
		title := fmt.Sprintf("Provider %s: %d different versions in use — standardize on %s",
			source, len(versionSet), targetVersion)

		if hasMajorSplit {
			severity = "critical"
			title = fmt.Sprintf("Provider %s: MAJOR version split detected (%s) — unify before upgrading",
				source, strings.Join(versions, " vs "))
		}

		var details []string
		details = append(details,
			fmt.Sprintf("Target version to standardize on: %s (highest currently in use)", targetVersion))
		if latest != "" && latest != targetVersion {
			details = append(details,
				fmt.Sprintf("Latest available: %s (upgrade to this after unifying)", latest))
		}

		byVersion := make(map[string][]fileVersion)
		for _, fv := range files {
			byVersion[fv.Version] = append(byVersion[fv.Version], fv)
		}

		for _, v := range versions {
			fvs := byVersion[v]
			if v == targetVersion {
				details = append(details, fmt.Sprintf("  %s (target) — %d file(s), no change needed", v, len(fvs)))
			} else {
				details = append(details, fmt.Sprintf("  %s — %d file(s) need update to %s", v, len(fvs), targetVersion))
			}
		}

		var fix []string
		for _, fv := range files {
			if fv.Version != targetVersion {
				rel := shortPath(fv.File)
				fix = append(fix,
					fmt.Sprintf("%s:%d  version = \"%s\"  →  version = \"%s\"",
						rel, fv.Line, fv.Version, targetVersion))
			}
		}

		recs = append(recs, analyzer.Recommendation{
			Severity: severity,
			Category: "version-fragmentation",
			Title:    title,
			Details:  details,
			Fix:      fix,
		})
	}

	return recs
}

// checkUnpinnedVersions flags dependencies with loose or missing version constraints.
func checkUnpinnedVersions(analysis *analyzer.Analysis) []analyzer.Recommendation {
	type unpinned struct {
		Name       string
		Source     string
		Latest     string
		Constraint string
		Reason     string
		File       string
		Line       int
	}

	var items []unpinned
	for _, dep := range analysis.Dependencies {
		constraint := strings.TrimSpace(dep.Constraint)
		reason := ""

		switch {
		case constraint == "" || constraint == "latest":
			reason = "no version constraint"
		case constraint == "*":
			reason = "wildcard allows any version"
		case strings.HasPrefix(constraint, ">=") && !strings.Contains(constraint, "<") && !strings.Contains(constraint, ","):
			reason = "no upper bound (>= without <)"
		case strings.HasPrefix(constraint, ">") && !strings.HasPrefix(constraint, ">=") && !strings.Contains(constraint, "<"):
			reason = "no upper bound (> without <)"
		}

		if reason != "" {
			items = append(items, unpinned{
				Name:       dep.Name,
				Source:     dep.Source,
				Latest:     dep.LatestVer,
				Constraint: constraint,
				Reason:     reason,
				File:       dep.FilePath,
				Line:       dep.Line,
			})
		}
	}

	if len(items) == 0 {
		return nil
	}

	details := []string{
		"Unpinned or loosely pinned versions mean different environments can get different versions,",
		"breaking reproducibility and potentially introducing breaking changes silently.",
		"",
		"Use pessimistic constraints like ~> 3.75.0 (allows 3.75.x) or exact pins.",
		"",
		"Issues found:",
	}
	for _, item := range items {
		constraintStr := item.Constraint
		if constraintStr == "" {
			constraintStr = "(none)"
		}
		details = append(details,
			fmt.Sprintf("  %s: version = \"%s\" — %s", item.Name, constraintStr, item.Reason))
	}

	var fix []string
	for _, item := range items {
		rel := shortPath(item.File)
		pinVersion := item.Latest
		if pinVersion == "" {
			pinVersion = "x.y.z"
		}
		fix = append(fix,
			fmt.Sprintf("%s:%d  %s: version = \"%s\"  →  version = \"~> %s\"",
				rel, item.Line, item.Name, item.Constraint, pinVersion))
	}

	return []analyzer.Recommendation{{
		Severity: "high",
		Category: "unpinned",
		Title:    fmt.Sprintf("%d dependency(s) have loose or missing version constraints — pin them", len(items)),
		Details:  details,
		Fix:      fix,
	}}
}

// checkMajorDrift detects providers with major version updates pending.
func checkMajorDrift(analysis *analyzer.Analysis) []analyzer.Recommendation {
	type drift struct {
		Source  string
		Current string
		Latest  string
		File    string
		Line    int
	}

	bySource := make(map[string][]drift)
	for _, dep := range analysis.Dependencies {
		if dep.UpdateType != resolver.UpdateMajor {
			continue
		}
		src := strings.ToLower(dep.Source)
		bySource[src] = append(bySource[src], drift{
			Source:  dep.Source,
			Current: dep.CurrentVer,
			Latest:  dep.LatestVer,
			File:    dep.FilePath,
			Line:    dep.Line,
		})
	}

	var recs []analyzer.Recommendation
	for source, drifts := range bySource {
		latest := drifts[0].Latest

		majorVersions := make(map[int]int)
		for _, d := range drifts {
			parts := parseVersion(d.Current)
			if len(parts) > 0 {
				majorVersions[parts[0]]++
			}
		}
		latestParts := parseVersion(latest)
		latestMajor := 0
		if len(latestParts) > 0 {
			latestMajor = latestParts[0]
		}

		details := []string{
			fmt.Sprintf("Latest version: %s (major %d)", latest, latestMajor),
			fmt.Sprintf("%d file(s) are on an older major version:", len(drifts)),
		}

		for major, count := range majorVersions {
			if major < latestMajor {
				details = append(details,
					fmt.Sprintf("  %d file(s) still on major %d — breaking changes expected when upgrading to %d",
						count, major, latestMajor))
			}
		}

		details = append(details, "")
		details = append(details, "Upgrade path:")
		details = append(details, fmt.Sprintf("  1. Run: tfoutdated scan --impact %s --target-version %s", source, latest))
		details = append(details, "  2. Check which modules need upgrading first")
		details = append(details, "  3. Upgrade modules in a dev/test environment")
		details = append(details, "  4. Run terraform plan to verify no unexpected changes")
		details = append(details, fmt.Sprintf("  5. Update provider constraint to version = \"%s\"", latest))

		var fix []string
		for _, d := range drifts {
			rel := shortPath(d.File)
			fix = append(fix,
				fmt.Sprintf("%s:%d  version = \"%s\"  →  version = \"%s\"",
					rel, d.Line, d.Current, latest))
		}

		recs = append(recs, analyzer.Recommendation{
			Severity: "high",
			Category: "major-drift",
			Title:    fmt.Sprintf("Provider %s: %d file(s) behind major version (%s available)", source, len(drifts), latest),
			Details:  details,
			Fix:      fix,
		})
	}

	return recs
}

// checkDeprecated flags deprecated modules/providers.
func checkDeprecated(analysis *analyzer.Analysis) []analyzer.Recommendation {
	var recs []analyzer.Recommendation
	for _, dep := range analysis.Dependencies {
		if !dep.Deprecated {
			continue
		}

		details := []string{
			fmt.Sprintf("Module %s is deprecated and will stop receiving updates.", dep.Source),
		}
		if dep.ReplacedBy != "" {
			details = append(details, fmt.Sprintf("Replacement: %s", dep.ReplacedBy))
			details = append(details, "")
			details = append(details, "Migration steps:")
			details = append(details, fmt.Sprintf("  1. Replace source = \"%s\" with source = \"%s\"", dep.Source, dep.ReplacedBy))
			details = append(details, "  2. Check the new module's inputs — some may have changed names")
			details = append(details, "  3. Run terraform plan to verify the migration")
		}

		var fix []string
		rel := shortPath(dep.FilePath)
		if dep.ReplacedBy != "" {
			fix = append(fix,
				fmt.Sprintf("%s  source = \"%s\"  →  source = \"%s\"",
					rel, dep.Source, dep.ReplacedBy))
		}

		recs = append(recs, analyzer.Recommendation{
			Severity: "critical",
			Category: "deprecated",
			Title:    fmt.Sprintf("Module %s (%s) is DEPRECATED", dep.Name, dep.Source),
			Details:  details,
			Fix:      fix,
		})
	}
	return recs
}

// checkModuleStaleness flags modules that are many minor versions behind.
func checkModuleStaleness(analysis *analyzer.Analysis) []analyzer.Recommendation {
	type stale struct {
		Name    string
		Source  string
		Current string
		Latest  string
		File    string
		Line    int
		Gap     int
	}

	var items []stale
	for _, dep := range analysis.Dependencies {
		if !dep.IsModule || dep.UpdateType != resolver.UpdateMinor {
			continue
		}
		cur := parseVersion(dep.CurrentVer)
		lat := parseVersion(dep.LatestVer)
		if len(cur) >= 2 && len(lat) >= 2 && cur[0] == lat[0] {
			gap := lat[1] - cur[1]
			if gap >= 3 {
				items = append(items, stale{
					Name:    dep.Name,
					Source:  dep.Source,
					Current: dep.CurrentVer,
					Latest:  dep.LatestVer,
					File:    dep.FilePath,
					Line:    dep.Line,
					Gap:     gap,
				})
			}
		}
	}

	if len(items) == 0 {
		return nil
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Gap > items[j].Gap })

	details := []string{
		"These modules are significantly behind. Falling this far behind makes future upgrades",
		"harder because changes accumulate. Schedule a monthly dependency review to stay current.",
		"",
		"Priority order (most behind first):",
	}

	var fix []string
	for _, item := range items {
		rel := shortPath(item.File)
		details = append(details,
			fmt.Sprintf("  %s: %s → %s (%d minor versions behind)",
				item.Name, item.Current, item.Latest, item.Gap))
		fix = append(fix,
			fmt.Sprintf("%s:%d  version = \"%s\"  →  version = \"%s\"",
				rel, item.Line, item.Current, item.Latest))
	}

	return []analyzer.Recommendation{{
		Severity: "medium",
		Category: "stale-modules",
		Title:    fmt.Sprintf("%d module(s) are 3+ minor versions behind — prioritize these upgrades", len(items)),
		Details:  details,
		Fix:      fix,
	}}
}

func hasMajorVersionSplit(versions []string) bool {
	majors := make(map[int]bool)
	for _, v := range versions {
		parts := parseVersion(v)
		if len(parts) > 0 {
			majors[parts[0]] = true
		}
	}
	return len(majors) > 1
}

func shortPath(file string) string {
	dir := filepath.Dir(file)
	parent := filepath.Base(filepath.Dir(dir))
	return filepath.Join(parent, filepath.Base(dir), filepath.Base(file))
}

func parseVersion(v string) []int {
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
	pa := parseVersion(a)
	pb := parseVersion(b)
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

// checkAVMPreOneWarnings flags AVM modules with minor version bumps that may contain breaking changes.
func checkAVMPreOneWarnings(analysis *analyzer.Analysis) []analyzer.Recommendation {
	type avmMinor struct {
		Name    string
		Source  string
		Current string
		Latest  string
		File    string
		Line    int
	}

	var items []avmMinor
	for _, dep := range analysis.Dependencies {
		if !dep.IsAVM || dep.UpdateType != resolver.UpdateMinor {
			continue
		}
		// Check if pre-1.0
		parts := parseVersion(dep.CurrentVer)
		if len(parts) >= 1 && parts[0] == 0 {
			items = append(items, avmMinor{
				Name:    dep.Name,
				Source:  dep.Source,
				Current: dep.CurrentVer,
				Latest:  dep.LatestVer,
				File:    dep.FilePath,
				Line:    dep.Line,
			})
		}
	}

	if len(items) == 0 {
		return nil
	}

	details := []string{
		"Azure Verified Modules follow a modified SemVer: pre-1.0 minor bumps MAY contain breaking changes",
		"(per AVM spec SNFR17). Treat these as potentially breaking and review changelogs before upgrading.",
		"",
	}
	for _, item := range items {
		details = append(details,
			fmt.Sprintf("  %s: %s → %s (pre-1.0 minor bump — may break)", item.Name, item.Current, item.Latest))
	}

	return []analyzer.Recommendation{{
		Severity: "high",
		Category: "avm-pre-one",
		Title:    fmt.Sprintf("%d AVM module(s) have pre-1.0 minor bumps that may contain breaking changes", len(items)),
		Details:  details,
	}}
}

// checkAVMProviderConflicts detects potential provider version conflicts across AVM modules.
func checkAVMProviderConflicts(analysis *analyzer.Analysis) []analyzer.Recommendation {
	// Group AVM modules by their provider constraints
	// If some modules pin azurerm < 4.0 and others require >= 4.0, flag it
	type moduleInfo struct {
		Name       string
		Source     string
		Current    string
		Latest     string
		Constraint string
		File       string
	}

	var avmModules []moduleInfo
	for _, dep := range analysis.Dependencies {
		if !dep.IsAVM {
			continue
		}
		avmModules = append(avmModules, moduleInfo{
			Name:       dep.Name,
			Source:     dep.Source,
			Current:    dep.CurrentVer,
			Latest:     dep.LatestVer,
			Constraint: dep.Constraint,
			File:       dep.FilePath,
		})
	}

	if len(avmModules) < 2 {
		return nil
	}

	// Check for version spread — if modules span a wide range, they likely have
	// different provider requirements
	var pre010, post010 []moduleInfo
	for _, m := range avmModules {
		parts := parseVersion(m.Current)
		if len(parts) >= 2 {
			if parts[0] == 0 && parts[1] < 10 {
				pre010 = append(pre010, m)
			} else {
				post010 = append(post010, m)
			}
		}
	}

	// If we have a mix of very old and newer AVM modules, flag it
	if len(pre010) == 0 || len(post010) == 0 {
		return nil
	}

	details := []string{
		"Your configuration uses AVM modules at very different version ranges.",
		"Older AVM modules may require azurerm < 4.0 while newer ones require >= 4.0,",
		"creating impossible-to-satisfy provider constraints.",
		"",
		"Upgrade the older modules first to ensure provider compatibility:",
		"",
	}
	for _, m := range pre010 {
		details = append(details,
			fmt.Sprintf("  %s: %s → %s (old — likely needs azurerm < 4.0)", m.Name, m.Current, m.Latest))
	}

	return []analyzer.Recommendation{{
		Severity: "high",
		Category: "avm-provider-conflict",
		Title:    fmt.Sprintf("Potential provider version conflict across %d AVM modules — upgrade older modules first", len(avmModules)),
		Details:  details,
	}}
}
