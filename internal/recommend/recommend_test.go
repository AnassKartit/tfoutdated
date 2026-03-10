package recommend

import (
	"strings"
	"testing"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

func TestRecommendEmpty(t *testing.T) {
	analysis := &analyzer.Analysis{}
	recs := Recommend(analysis)
	if len(recs) != 0 {
		t.Fatalf("expected 0 recommendations, got %d", len(recs))
	}
}

func TestCheckProviderFragmentation(t *testing.T) {
	analysis := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{Name: "azurerm", Source: "hashicorp/azurerm", CurrentVer: "3.75.0", LatestVer: "4.0.0", FilePath: "/a/main.tf", Line: 5, UpdateType: resolver.UpdateMajor},
			{Name: "azurerm", Source: "hashicorp/azurerm", CurrentVer: "3.80.0", LatestVer: "4.0.0", FilePath: "/b/main.tf", Line: 10, UpdateType: resolver.UpdateMajor},
		},
	}

	recs := Recommend(analysis)

	found := false
	for _, r := range recs {
		if r.Category == "version-fragmentation" {
			found = true
			if r.Severity != "medium" {
				t.Errorf("expected severity medium, got %s", r.Severity)
			}
			if len(r.Fix) == 0 {
				t.Error("expected fix suggestions")
			}
		}
	}
	if !found {
		t.Error("expected version-fragmentation recommendation")
	}
}

func TestCheckMajorDrift(t *testing.T) {
	analysis := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{Name: "azurerm", Source: "hashicorp/azurerm", CurrentVer: "3.75.0", LatestVer: "4.0.0", FilePath: "/a/main.tf", Line: 5, UpdateType: resolver.UpdateMajor},
		},
	}

	recs := Recommend(analysis)

	found := false
	for _, r := range recs {
		if r.Category == "major-drift" {
			found = true
			if r.Severity != "high" {
				t.Errorf("expected severity high, got %s", r.Severity)
			}
			// Should reference tfoutdated, not aztfoutdated
			for _, d := range r.Details {
				if strings.Contains(d, "aztfoutdated") {
					t.Error("recommendation should reference tfoutdated, not aztfoutdated")
				}
			}
		}
	}
	if !found {
		t.Error("expected major-drift recommendation")
	}
}

func TestCheckDeprecated(t *testing.T) {
	analysis := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{
				Name:       "old-module",
				Source:     "Azure/old-module/azurerm",
				CurrentVer: "1.0.0",
				LatestVer:  "1.0.0",
				FilePath:   "/a/main.tf",
				Line:       5,
				IsModule:   true,
				Deprecated: true,
				ReplacedBy: "Azure/new-module/azurerm",
			},
		},
	}

	recs := Recommend(analysis)

	found := false
	for _, r := range recs {
		if r.Category == "deprecated" {
			found = true
			if r.Severity != "critical" {
				t.Errorf("expected severity critical, got %s", r.Severity)
			}
			if len(r.Fix) == 0 {
				t.Error("expected fix suggestion for deprecated module")
			}
		}
	}
	if !found {
		t.Error("expected deprecated recommendation")
	}
}

func TestCheckUnpinnedVersions(t *testing.T) {
	analysis := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{Name: "vm", Source: "Azure/vm/azurerm", CurrentVer: "0.1.0", LatestVer: "1.0.0", FilePath: "/a/main.tf", Line: 5, IsModule: true, Constraint: "latest", UpdateType: resolver.UpdateMajor},
		},
	}

	recs := Recommend(analysis)

	found := false
	for _, r := range recs {
		if r.Category == "unpinned" {
			found = true
			if r.Severity != "high" {
				t.Errorf("expected severity high, got %s", r.Severity)
			}
		}
	}
	if !found {
		t.Error("expected unpinned recommendation")
	}
}

func TestCheckModuleStaleness(t *testing.T) {
	analysis := &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{
			{Name: "vm", Source: "Azure/vm/azurerm", CurrentVer: "0.1.0", LatestVer: "0.5.0", FilePath: "/a/main.tf", Line: 5, IsModule: true, UpdateType: resolver.UpdateMinor},
		},
	}

	recs := Recommend(analysis)

	found := false
	for _, r := range recs {
		if r.Category == "stale-modules" {
			found = true
			if r.Severity != "medium" {
				t.Errorf("expected severity medium, got %s", r.Severity)
			}
		}
	}
	if !found {
		t.Error("expected stale-modules recommendation")
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected []int
	}{
		{"3.75.0", []int{3, 75, 0}},
		{"v4.0.0", []int{4, 0, 0}},
		{"1.2.3-beta1", []int{1, 2, 3}},
		{"", nil},
	}

	for _, tt := range tests {
		got := parseVersion(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("parseVersion(%q) = %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestCompareVersionNums(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"3.75.0", "4.0.0", -1},
		{"4.0.0", "3.75.0", 1},
		{"3.75.0", "3.75.0", 0},
		{"3.80.0", "3.75.0", 1},
	}

	for _, tt := range tests {
		got := compareVersionNums(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("compareVersionNums(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}
