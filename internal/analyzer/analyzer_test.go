package analyzer

import (
	"testing"

	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"

	goversion "github.com/hashicorp/go-version"
)

func mustVersion(t *testing.T, s string) *goversion.Version {
	t.Helper()
	v, err := goversion.NewVersion(s)
	if err != nil {
		t.Fatalf("invalid version %q: %v", s, err)
	}
	return v
}

func TestSummaryAllUpToDate(t *testing.T) {
	a := &Analysis{}
	got := a.Summary()
	want := "All dependencies are up to date."
	if got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestSummaryWithDependencies(t *testing.T) {
	a := &Analysis{
		Dependencies: []DependencyAnalysis{
			{Name: "azurerm", UpdateType: resolver.UpdateMajor},
			{Name: "random", UpdateType: resolver.UpdateMinor},
			{Name: "null", UpdateType: resolver.UpdatePatch},
			{Name: "tls", UpdateType: resolver.UpdatePatch},
		},
	}

	got := a.Summary()
	want := "4 dependencies outdated (1 major, 1 minor, 2 patchs)"
	if got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestSummarySingleDependency(t *testing.T) {
	a := &Analysis{
		Dependencies: []DependencyAnalysis{
			{Name: "azurerm", UpdateType: resolver.UpdateMajor},
		},
	}

	got := a.Summary()
	want := "1 dependency outdated (1 major)"
	if got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestHasBreakingChanges(t *testing.T) {
	tests := []struct {
		name string
		bcs  []breaking.BreakingChange
		want bool
	}{
		{
			name: "no changes",
			bcs:  nil,
			want: false,
		},
		{
			name: "info only",
			bcs: []breaking.BreakingChange{
				{Severity: breaking.SeverityInfo},
			},
			want: false,
		},
		{
			name: "warning only",
			bcs: []breaking.BreakingChange{
				{Severity: breaking.SeverityWarning},
			},
			want: false,
		},
		{
			name: "breaking",
			bcs: []breaking.BreakingChange{
				{Severity: breaking.SeverityBreaking},
			},
			want: true,
		},
		{
			name: "critical",
			bcs: []breaking.BreakingChange{
				{Severity: breaking.SeverityCritical},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Analysis{BreakingChanges: tt.bcs}
			if got := a.HasBreakingChanges(); got != tt.want {
				t.Errorf("HasBreakingChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterBySeverity(t *testing.T) {
	a := &Analysis{
		Dependencies: []DependencyAnalysis{
			{Name: "p1", UpdateType: resolver.UpdatePatch},
			{Name: "p2", UpdateType: resolver.UpdateMinor},
			{Name: "p3", UpdateType: resolver.UpdateMajor},
		},
	}

	// Filter for minor and above
	filtered := a.FilterBySeverity("minor")
	if len(filtered.Dependencies) != 2 {
		t.Errorf("expected 2 deps with minor filter, got %d", len(filtered.Dependencies))
	}
	for _, dep := range filtered.Dependencies {
		if dep.UpdateType < resolver.UpdateMinor {
			t.Errorf("unexpected patch update %q in minor-filtered results", dep.Name)
		}
	}

	// Filter for major only
	filtered = a.FilterBySeverity("major")
	if len(filtered.Dependencies) != 1 {
		t.Errorf("expected 1 dep with major filter, got %d", len(filtered.Dependencies))
	}
	if filtered.Dependencies[0].Name != "p3" {
		t.Errorf("expected p3, got %s", filtered.Dependencies[0].Name)
	}

	// Filter for patch (all)
	filtered = a.FilterBySeverity("patch")
	if len(filtered.Dependencies) != 3 {
		t.Errorf("expected 3 deps with patch filter, got %d", len(filtered.Dependencies))
	}
}

func TestFilterBySeverityPreservesOtherFields(t *testing.T) {
	a := &Analysis{
		Dependencies: []DependencyAnalysis{
			{Name: "p1", UpdateType: resolver.UpdatePatch},
		},
		BreakingChanges: []breaking.BreakingChange{
			{Provider: "test", Severity: breaking.SeverityBreaking},
		},
		Alignments: []AlignmentIssue{
			{Name: "test", Versions: map[string][]string{"1.0.0": {"a.tf"}}},
		},
	}

	filtered := a.FilterBySeverity("major")
	if len(filtered.BreakingChanges) != 1 {
		t.Error("expected breaking changes to be preserved after filtering")
	}
	if len(filtered.Alignments) != 1 {
		t.Error("expected alignment issues to be preserved after filtering")
	}
}

func TestAnalyze(t *testing.T) {
	current := mustVersion(t, "3.75.0")
	latest := mustVersion(t, "4.1.0")

	resolved := &resolver.ResolvedResult{
		Dependencies: []resolver.ResolvedDependency{
			{
				Name:       "azurerm",
				Source:     "hashicorp/azurerm",
				FilePath:   "main.tf",
				Line:       5,
				IsModule:   false,
				Current:    current,
				Latest:     latest,
				UpdateType: resolver.UpdateMajor,
			},
		},
	}

	analyzer := New()
	analysis := analyzer.Analyze(resolved)

	if len(analysis.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(analysis.Dependencies))
	}

	dep := analysis.Dependencies[0]
	if dep.Name != "azurerm" {
		t.Errorf("name = %q, want %q", dep.Name, "azurerm")
	}
	if dep.CurrentVer != "3.75.0" {
		t.Errorf("currentVer = %q, want %q", dep.CurrentVer, "3.75.0")
	}
	if dep.LatestVer != "4.1.0" {
		t.Errorf("latestVer = %q, want %q", dep.LatestVer, "4.1.0")
	}
	if dep.UpdateType != resolver.UpdateMajor {
		t.Errorf("updateType = %v, want %v", dep.UpdateType, resolver.UpdateMajor)
	}
}

func TestAnalyzeSortOrder(t *testing.T) {
	resolved := &resolver.ResolvedResult{
		Dependencies: []resolver.ResolvedDependency{
			{
				Name:       "zeta",
				Current:    mustVersion(t, "1.0.0"),
				Latest:     mustVersion(t, "1.0.1"),
				UpdateType: resolver.UpdatePatch,
			},
			{
				Name:       "alpha",
				Current:    mustVersion(t, "1.0.0"),
				Latest:     mustVersion(t, "2.0.0"),
				UpdateType: resolver.UpdateMajor,
			},
			{
				Name:       "beta",
				Current:    mustVersion(t, "1.0.0"),
				Latest:     mustVersion(t, "1.1.0"),
				UpdateType: resolver.UpdateMinor,
			},
		},
	}

	analyzer := New()
	analysis := analyzer.Analyze(resolved)

	// Major first, then minor, then patch
	if analysis.Dependencies[0].Name != "alpha" {
		t.Errorf("first dep should be alpha (major), got %s", analysis.Dependencies[0].Name)
	}
	if analysis.Dependencies[1].Name != "beta" {
		t.Errorf("second dep should be beta (minor), got %s", analysis.Dependencies[1].Name)
	}
	if analysis.Dependencies[2].Name != "zeta" {
		t.Errorf("third dep should be zeta (patch), got %s", analysis.Dependencies[2].Name)
	}
}

func TestDetectAlignmentIssues(t *testing.T) {
	resolved := &resolver.ResolvedResult{
		Dependencies: []resolver.ResolvedDependency{
			{
				Source:   "hashicorp/azurerm",
				FilePath: "envs/dev/main.tf",
				Current:  mustVersion(t, "3.75.0"),
				Latest:   mustVersion(t, "4.0.0"),
			},
			{
				Source:   "hashicorp/azurerm",
				FilePath: "envs/prod/main.tf",
				Current:  mustVersion(t, "3.80.0"),
				Latest:   mustVersion(t, "4.0.0"),
			},
		},
	}

	issues := detectAlignmentIssues(resolved)
	if len(issues) != 1 {
		t.Fatalf("expected 1 alignment issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.Name != "hashicorp/azurerm" {
		t.Errorf("name = %q, want %q", issue.Name, "hashicorp/azurerm")
	}
	if len(issue.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(issue.Versions))
	}
}

func TestNoAlignmentIssuesWhenSameVersion(t *testing.T) {
	resolved := &resolver.ResolvedResult{
		Dependencies: []resolver.ResolvedDependency{
			{
				Source:   "hashicorp/azurerm",
				FilePath: "envs/dev/main.tf",
				Current:  mustVersion(t, "3.75.0"),
				Latest:   mustVersion(t, "4.0.0"),
			},
			{
				Source:   "hashicorp/azurerm",
				FilePath: "envs/prod/main.tf",
				Current:  mustVersion(t, "3.75.0"),
				Latest:   mustVersion(t, "4.0.0"),
			},
		},
	}

	issues := detectAlignmentIssues(resolved)
	if len(issues) != 0 {
		t.Errorf("expected 0 alignment issues when versions match, got %d", len(issues))
	}
}

func TestComputeDistance(t *testing.T) {
	dep := resolver.ResolvedDependency{
		Current: mustVersion(t, "3.75.0"),
		Latest:  mustVersion(t, "4.1.2"),
	}

	dist := computeDistance(dep)
	if dist.MajorsBehind != 1 {
		t.Errorf("MajorsBehind = %d, want 1", dist.MajorsBehind)
	}
}

func TestRecommendAlignment(t *testing.T) {
	issues := []AlignmentIssue{
		{
			Name: "hashicorp/azurerm",
			Versions: map[string][]string{
				"3.75.0": {"dev/main.tf"},
				"3.80.0": {"prod/main.tf"},
			},
		},
	}

	recs := RecommendAlignment(issues)
	if recs["hashicorp/azurerm"] != "3.80.0" {
		t.Errorf("recommended = %q, want %q", recs["hashicorp/azurerm"], "3.80.0")
	}
}

func TestUpdateTypeString(t *testing.T) {
	tests := []struct {
		ut   resolver.UpdateType
		want string
	}{
		{resolver.UpdateNone, "NONE"},
		{resolver.UpdatePatch, "PATCH"},
		{resolver.UpdateMinor, "MINOR"},
		{resolver.UpdateMajor, "MAJOR"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.ut.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
