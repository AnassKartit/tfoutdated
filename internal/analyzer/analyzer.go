package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/lifecycle"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// Recommendation represents an actionable suggestion.
type Recommendation struct {
	Severity string   `json:"severity"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Details  []string `json:"details"`
	Fix      []string `json:"fix,omitempty"`
}

// ProviderImpactResult shows what happens to a module if you upgrade a provider.
type ProviderImpactResult struct {
	Module                string                 `json:"module"`
	ModuleSource          string                 `json:"moduleSource"`
	CurrentModuleVer      string                 `json:"currentModuleVersion"`
	LatestModuleVer       string                 `json:"latestModuleVersion"`
	CurrentProviderDeps   []resolver.ProviderDep `json:"currentProviderDeps"`
	LatestProviderDeps    []resolver.ProviderDep `json:"latestProviderDeps"`
	Compatible            bool                   `json:"compatible"`
	RequiresModuleUpgrade bool                   `json:"requiresModuleUpgrade"`
	MinCompatibleVer      string                 `json:"minCompatibleVersion,omitempty"`
	File                  string                 `json:"file"`
	Line                  int                    `json:"line"`
}

// ProviderImpactSummary is the provider impact analysis report.
type ProviderImpactSummary struct {
	TargetProvider string                 `json:"targetProvider"`
	TargetVersion  string                 `json:"targetVersion"`
	TotalModules   int                    `json:"totalModules"`
	Compatible     int                    `json:"compatible"`
	NeedUpgrade    int                    `json:"needUpgrade"`
	Incompatible   int                    `json:"incompatible"`
	Results        []ProviderImpactResult `json:"results"`
	DurationMs     int64                  `json:"durationMs"`
}

// DependencyAnalysis contains analysis results for a single dependency.
type DependencyAnalysis struct {
	Name       string
	Source     string
	FilePath   string
	Line       int
	IsModule   bool
	IsAVM      bool
	Namespace  string
	SourceType string
	CurrentVer string
	LatestVer  string
	UpdateType resolver.UpdateType
	Distance   VersionDistance
	Constraint string // raw constraint string
	Deprecated bool
	ReplacedBy string
	// ReplacedByVersion is the version to pin on the replacement.
	ReplacedByVersion string
	// DeprecationReason lists the signals that marked it deprecated
	// (README notice, release notes, AVM index, archived repository).
	DeprecationReason string
	// Evidence links each of those signals to the page that shows it.
	Evidence    []lifecycle.Evidence
	AllVersions []string // all available versions
}

// VersionDistance measures how far behind a dependency is.
type VersionDistance struct {
	MajorsBehind  int
	MinorsBehind  int
	PatchesBehind int
}

// UpgradePath recommends a safe upgrade sequence.
type UpgradePath struct {
	Name              string
	Steps             []UpgradeStep
	NonBreakingTarget string // highest version reachable without code changes
}

// HasBreakingSteps returns true if any step in the path has breaking changes.
func (p UpgradePath) HasBreakingSteps() bool {
	for _, step := range p.Steps {
		if !step.Safe || step.BreakingChanges > 0 {
			return true
		}
	}
	return false
}

// UpgradeStep is a single version upgrade step.
type UpgradeStep struct {
	From            string
	To              string
	BreakingChanges int
	Safe            bool // no breaking changes
}

// ImpactItem describes a breaking change impact on a specific file.
type ImpactItem struct {
	FilePath          string
	Line              int
	ResourceType      string
	Attribute         string
	BreakingChange    breaking.BreakingChange
	ChangeDescription string
	ActualBefore      string // user's real code (from scanned .tf file)
	ActualAfter       string // deterministically transformed code
	LinesChanged      int    // how many lines differ
	AffectedFile      string // which .tf file contains the resource
	AffectedLine      int    // start line of the resource block
	ResourceName      string // e.g. "azurerm_app_service.main"
}

// Analysis is the complete analysis result.
type Analysis struct {
	Dependencies    []DependencyAnalysis
	BreakingChanges []breaking.BreakingChange
	Impacts         []ImpactItem
	UpgradePaths    []UpgradePath
	Alignments      []AlignmentIssue
	Recommendations []Recommendation
	ProviderImpact  *ProviderImpactSummary

	// Unresolved lists dependencies that could not be checked at all, so a clean
	// report can be told apart from a blind one.
	Unresolved []UnresolvedDependency

	// Scan metadata
	// Target is what was scanned: a directory, or a repository URL in
	// multi-repo runs. Reports show it so a reader knows what they look at.
	Target       string
	ScannedFiles int
	TotalDeps    int
	UpToDate     int
	DurationMs   int64
}

// AlignmentIssue indicates the same dependency used at different versions.
type AlignmentIssue struct {
	Name     string
	Versions map[string][]string // version → list of file paths
}

// HasBreakingChanges returns true if any breaking changes were detected.
func (a *Analysis) HasBreakingChanges() bool {
	for _, bc := range a.BreakingChanges {
		if bc.Severity >= breaking.SeverityBreaking {
			return true
		}
	}
	return false
}

// FilterBySeverity returns a copy filtered to the minimum severity.
func (a *Analysis) FilterBySeverity(minSeverity string) *Analysis {
	minUpdate := resolver.UpdatePatch
	switch strings.ToLower(minSeverity) {
	case "minor":
		minUpdate = resolver.UpdateMinor
	case "major":
		minUpdate = resolver.UpdateMajor
	}

	filtered := &Analysis{
		BreakingChanges: a.BreakingChanges,
		Impacts:         a.Impacts,
		UpgradePaths:    a.UpgradePaths,
		Alignments:      a.Alignments,
		Recommendations: a.Recommendations,
		ProviderImpact:  a.ProviderImpact,
		Target:          a.Target,
		ScannedFiles:    a.ScannedFiles,
		TotalDeps:       a.TotalDeps,
		UpToDate:        a.UpToDate,
		DurationMs:      a.DurationMs,
	}

	for _, dep := range a.Dependencies {
		// A deprecated module is reported at any severity: being on its latest
		// version does not help when that version is the last one.
		if dep.UpdateType >= minUpdate || dep.Deprecated {
			filtered.Dependencies = append(filtered.Dependencies, dep)
		}
	}

	return filtered
}

// Summary returns a human-readable summary string.
func (a *Analysis) Summary() string {
	if len(a.Dependencies) == 0 {
		return "All dependencies are up to date."
	}

	var major, minor, patch, deprecated int
	for _, dep := range a.Dependencies {
		if dep.Deprecated {
			deprecated++
		}
		switch dep.UpdateType {
		case resolver.UpdateMajor:
			major++
		case resolver.UpdateMinor:
			minor++
		case resolver.UpdatePatch:
			patch++
		}
	}

	parts := []string{}
	if major > 0 {
		parts = append(parts, pluralize(major, "major"))
	}
	if minor > 0 {
		parts = append(parts, pluralize(minor, "minor"))
	}
	if patch > 0 {
		parts = append(parts, pluralize(patch, "patch", "patches"))
	}

	var out []string
	if outdated := major + minor + patch; outdated > 0 {
		out = append(out, pluralize(outdated, "dependency", "dependencies")+" outdated ("+strings.Join(parts, ", ")+")")
	}
	if deprecated > 0 {
		out = append(out, pluralize(deprecated, "dependency", "dependencies")+" deprecated")
	}
	return strings.Join(out, ", ")
}

func pluralize(n int, singular string, plurals ...string) string {
	plural := singular + "s"
	if len(plurals) > 0 {
		plural = plurals[0]
	}
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// Analyzer performs version analysis on resolved dependencies.
type Analyzer struct{}

// New creates a new Analyzer.
func New() *Analyzer {
	return &Analyzer{}
}

// UnresolvedDependency is a dependency whose available versions are unknown.
type UnresolvedDependency struct {
	Name       string
	Source     string
	Declared   string // what the code pins, e.g. the git ref
	FilePath   string
	Line       int
	IsModule   bool
	SourceType string
	Reason     string
	Unpinned   bool
}

// Analyze processes resolved dependencies into analysis results.
func (a *Analyzer) Analyze(resolved *resolver.ResolvedResult) *Analysis {
	analysis := &Analysis{}

	for _, u := range resolved.Unresolved {
		declared := u.ConstraintRaw
		if declared == "" && u.Current != nil {
			declared = u.Current.String()
		}
		analysis.Unresolved = append(analysis.Unresolved, UnresolvedDependency{
			Name:       u.Name,
			Source:     u.Source,
			Declared:   declared,
			FilePath:   u.FilePath,
			Line:       u.Line,
			IsModule:   u.IsModule,
			SourceType: u.SourceType,
			Reason:     u.UnresolvedReason,
		})
	}

	for _, dep := range resolved.Dependencies {
		var allVers []string
		for _, v := range dep.AllVersions {
			allVers = append(allVers, v.String())
		}
		// An unresolved dependency has no Latest to compare against, and may have
		// no parseable Current either. Report what the code declares and leave the
		// comparison blank rather than asserting an upgrade we never verified.
		currentVer, latestVer := "", ""
		if dep.Current != nil {
			currentVer = dep.Current.String()
		} else if dep.ConstraintRaw != "" {
			currentVer = dep.ConstraintRaw
		}
		if dep.Latest != nil {
			latestVer = dep.Latest.String()
		}
		da := DependencyAnalysis{
			Name:              dep.Name,
			Source:            dep.Source,
			FilePath:          dep.FilePath,
			Line:              dep.Line,
			IsModule:          dep.IsModule,
			IsAVM:             dep.IsAVM,
			Namespace:         dep.Namespace,
			SourceType:        dep.SourceType,
			CurrentVer:        currentVer,
			LatestVer:         latestVer,
			UpdateType:        dep.UpdateType,
			Distance:          computeDistance(dep),
			Constraint:        dep.ConstraintRaw,
			Deprecated:        dep.Deprecated,
			ReplacedBy:        dep.ReplacedBy,
			ReplacedByVersion: dep.ReplacedByVersion,
			DeprecationReason: dep.DeprecationMessage,
			Evidence:          dep.DeprecationEvidence,
			AllVersions:       allVers,
		}
		analysis.Dependencies = append(analysis.Dependencies, da)
	}

	// Sort by update type (major first), then by name
	sort.Slice(analysis.Dependencies, func(i, j int) bool {
		if analysis.Dependencies[i].UpdateType != analysis.Dependencies[j].UpdateType {
			return analysis.Dependencies[i].UpdateType > analysis.Dependencies[j].UpdateType
		}
		return analysis.Dependencies[i].Name < analysis.Dependencies[j].Name
	})

	// Detect alignment issues
	analysis.Alignments = detectAlignmentIssues(resolved)

	return analysis
}

func computeDistance(dep resolver.ResolvedDependency) VersionDistance {
	// Nothing to measure when either end is unknown, which is the case for a
	// dependency whose versions could not be resolved.
	if dep.Current == nil || dep.Latest == nil {
		return VersionDistance{}
	}

	cs := dep.Current.Segments()
	ls := dep.Latest.Segments()

	for len(cs) < 3 {
		cs = append(cs, 0)
	}
	for len(ls) < 3 {
		ls = append(ls, 0)
	}

	return VersionDistance{
		MajorsBehind:  ls[0] - cs[0],
		MinorsBehind:  ls[1] - cs[1],
		PatchesBehind: ls[2] - cs[2],
	}
}

func detectAlignmentIssues(resolved *resolver.ResolvedResult) []AlignmentIssue {
	// Group by dependency source
	bySource := make(map[string]map[string][]string) // source → version → files

	for _, dep := range resolved.Dependencies {
		key := dep.Source
		if _, ok := bySource[key]; !ok {
			bySource[key] = make(map[string][]string)
		}
		verStr := dep.Current.String()
		bySource[key][verStr] = append(bySource[key][verStr], dep.FilePath)
	}

	var issues []AlignmentIssue
	for source, versions := range bySource {
		if len(versions) > 1 {
			issues = append(issues, AlignmentIssue{
				Name:     source,
				Versions: versions,
			})
		}
	}

	return issues
}
