package output

import (
	"encoding/json"
	"io"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

// JSONRenderer renders analysis results as JSON for pipeline consumption.
type JSONRenderer struct{}

type jsonOutput struct {
	ScannedFiles    int                           `json:"scannedFiles"`
	TotalDeps       int                           `json:"totalDeps"`
	Outdated        int                           `json:"outdated"`
	UpToDate        int                           `json:"upToDate"`
	Deprecated      int                           `json:"deprecated"`
	Summary         string                        `json:"summary"`
	DurationMs      int64                         `json:"durationMs"`
	Dependencies    []jsonDependency              `json:"dependencies"`
	BreakingChanges []jsonBreakingChange          `json:"breaking_changes,omitempty"`
	UpgradePaths    []jsonUpgradePath             `json:"upgrade_paths,omitempty"`
	Alignments      []jsonAlignment               `json:"alignment_issues,omitempty"`
	Recommendations []jsonRecommendation          `json:"recommendations,omitempty"`
	ProviderImpact  *jsonProviderImpact           `json:"provider_impact,omitempty"`
}

type jsonRecommendation struct {
	Severity string   `json:"severity"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Details  []string `json:"details"`
	Fix      []string `json:"fix,omitempty"`
}

type jsonProviderImpact struct {
	TargetProvider string                    `json:"target_provider"`
	TargetVersion  string                    `json:"target_version"`
	TotalModules   int                       `json:"total_modules"`
	Compatible     int                       `json:"compatible"`
	NeedUpgrade    int                       `json:"need_upgrade"`
	Incompatible   int                       `json:"incompatible"`
	Results        []jsonProviderImpactResult `json:"results"`
	DurationMs     int64                     `json:"duration_ms"`
}

type jsonProviderImpactResult struct {
	Module               string `json:"module"`
	CurrentModuleVer     string `json:"current_module_version"`
	LatestModuleVer      string `json:"latest_module_version"`
	Compatible           bool   `json:"compatible"`
	RequiresModuleUpgrade bool  `json:"requires_module_upgrade"`
	MinCompatibleVer     string `json:"min_compatible_version,omitempty"`
	File                 string `json:"file"`
	Line                 int    `json:"line"`
}

type jsonDependency struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Current    string   `json:"current"`
	Latest     string   `json:"latest"`
	Constraint string   `json:"constraint,omitempty"`
	UpdateType string   `json:"update_type"`
	Status     string   `json:"status"`
	Type       string   `json:"type"`
	SourceType string   `json:"source_type,omitempty"`
	FilePath   string   `json:"file_path"`
	Line       int      `json:"line"`
	IsModule   bool     `json:"is_module"`
	IsAVM      bool     `json:"is_avm,omitempty"`
	Deprecated bool     `json:"deprecated,omitempty"`
	ReplacedBy string   `json:"replaced_by,omitempty"`
	AllVersions []string `json:"all_versions,omitempty"`
}

type jsonBreakingChange struct {
	Provider       string             `json:"provider"`
	Version        string             `json:"version"`
	ResourceType   string             `json:"resource_type,omitempty"`
	Attribute      string             `json:"attribute,omitempty"`
	Kind           string             `json:"kind"`
	Severity       string             `json:"severity"`
	Description    string             `json:"description"`
	MigrationGuide string             `json:"migration_guide,omitempty"`
	AutoFixable    bool               `json:"auto_fixable"`
	BeforeSnippet  string             `json:"before_snippet,omitempty"`
	AfterSnippet   string             `json:"after_snippet,omitempty"`
	EffortLevel       string             `json:"effort_level,omitempty"`
	DynamicDetected   bool               `json:"dynamic_detected,omitempty"`
	AffectedResources []jsonAffectedResource `json:"affected_resources,omitempty"`
}

type jsonAffectedResource struct {
	ResourceName string `json:"resource_name"`
	File         string `json:"file"`
	Line         int    `json:"line"`
	LinesChanged int    `json:"lines_changed"`
	Before       string `json:"before"`
	After        string `json:"after,omitempty"`
}

type jsonUpgradePath struct {
	Name              string            `json:"name"`
	Steps             []jsonUpgradeStep `json:"steps"`
	NonBreakingTarget string            `json:"non_breaking_target,omitempty"`
}

type jsonUpgradeStep struct {
	From            string `json:"from"`
	To              string `json:"to"`
	Safe            bool   `json:"safe"`
	BreakingChanges int    `json:"breaking_changes,omitempty"`
}

type jsonAlignment struct {
	Name     string              `json:"name"`
	Versions map[string][]string `json:"versions"`
}

func (r *JSONRenderer) Render(w io.Writer, analysis *analyzer.Analysis) error {
	outdated := 0
	deprecated := 0
	for _, dep := range analysis.Dependencies {
		if dep.UpdateType > resolver.UpdateNone {
			outdated++
		}
		if dep.Deprecated {
			deprecated++
		}
	}

	out := jsonOutput{
		ScannedFiles: analysis.ScannedFiles,
		TotalDeps:    analysis.TotalDeps,
		Outdated:     outdated,
		UpToDate:     analysis.UpToDate,
		Deprecated:   deprecated,
		Summary:      analysis.Summary(),
		DurationMs:   analysis.DurationMs,
	}

	for _, dep := range analysis.Dependencies {
		status := "outdated"
		if dep.UpdateType == resolver.UpdateNone {
			status = "current"
		}
		depType := "provider"
		if dep.IsModule {
			depType = "module"
		}
		sourceType := dep.SourceType
		if sourceType == "" && !dep.IsModule {
			sourceType = "provider"
		}
		out.Dependencies = append(out.Dependencies, jsonDependency{
			Name:        dep.Name,
			Source:      dep.Source,
			Current:     dep.CurrentVer,
			Latest:      dep.LatestVer,
			Constraint:  dep.Constraint,
			UpdateType:  dep.UpdateType.String(),
			Status:      status,
			Type:        depType,
			SourceType:  sourceType,
			FilePath:    dep.FilePath,
			Line:        dep.Line,
			IsModule:    dep.IsModule,
			IsAVM:       dep.IsAVM,
			Deprecated:  dep.Deprecated,
			ReplacedBy:  dep.ReplacedBy,
			AllVersions: dep.AllVersions,
		})
	}

	for _, bc := range analysis.BreakingChanges {
		jbc := jsonBreakingChange{
			Provider:       bc.Provider,
			Version:        bc.Version,
			ResourceType:   bc.ResourceType,
			Attribute:      bc.Attribute,
			Kind:           bc.Kind.String(),
			Severity:       bc.Severity.String(),
			Description:    bc.Description,
			MigrationGuide: bc.MigrationGuide,
			AutoFixable:    bc.AutoFixable,
			BeforeSnippet:  bc.BeforeSnippet,
			AfterSnippet:   bc.AfterSnippet,
			EffortLevel:     bc.EffortLevel,
			DynamicDetected: bc.DynamicDetected,
		}

		// Attach real user code snippets from impact analysis
		for _, impact := range analysis.Impacts {
			if impact.ActualBefore == "" {
				continue
			}
			if impact.BreakingChange.ResourceType != bc.ResourceType || impact.BreakingChange.Attribute != bc.Attribute {
				continue
			}
			jbc.AffectedResources = append(jbc.AffectedResources, jsonAffectedResource{
				ResourceName: impact.ResourceName,
				File:         impact.AffectedFile,
				Line:         impact.AffectedLine,
				LinesChanged: impact.LinesChanged,
				Before:       impact.ActualBefore,
				After:        impact.ActualAfter,
			})
		}

		out.BreakingChanges = append(out.BreakingChanges, jbc)
	}

	for _, path := range analysis.UpgradePaths {
		jp := jsonUpgradePath{
			Name:              path.Name,
			NonBreakingTarget: path.NonBreakingTarget,
		}
		for _, step := range path.Steps {
			jp.Steps = append(jp.Steps, jsonUpgradeStep{
				From:            step.From,
				To:              step.To,
				Safe:            step.Safe,
				BreakingChanges: step.BreakingChanges,
			})
		}
		out.UpgradePaths = append(out.UpgradePaths, jp)
	}

	for _, a := range analysis.Alignments {
		out.Alignments = append(out.Alignments, jsonAlignment{
			Name:     a.Name,
			Versions: a.Versions,
		})
	}

	// Recommendations
	for _, rec := range analysis.Recommendations {
		out.Recommendations = append(out.Recommendations, jsonRecommendation{
			Severity: rec.Severity,
			Category: rec.Category,
			Title:    rec.Title,
			Details:  rec.Details,
			Fix:      rec.Fix,
		})
	}

	// Provider Impact
	if analysis.ProviderImpact != nil {
		imp := analysis.ProviderImpact
		ji := &jsonProviderImpact{
			TargetProvider: imp.TargetProvider,
			TargetVersion:  imp.TargetVersion,
			TotalModules:   imp.TotalModules,
			Compatible:     imp.Compatible,
			NeedUpgrade:    imp.NeedUpgrade,
			Incompatible:   imp.Incompatible,
			DurationMs:     imp.DurationMs,
		}
		for _, r := range imp.Results {
			ji.Results = append(ji.Results, jsonProviderImpactResult{
				Module:                r.Module,
				CurrentModuleVer:      r.CurrentModuleVer,
				LatestModuleVer:       r.LatestModuleVer,
				Compatible:            r.Compatible,
				RequiresModuleUpgrade: r.RequiresModuleUpgrade,
				MinCompatibleVer:      r.MinCompatibleVer,
				File:                  r.File,
				Line:                  r.Line,
			})
		}
		out.ProviderImpact = ji
	}

	// Filter out zero-value update types
	var filtered []jsonDependency
	for _, d := range out.Dependencies {
		if d.UpdateType != resolver.UpdateNone.String() {
			filtered = append(filtered, d)
		}
	}
	out.Dependencies = filtered

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
