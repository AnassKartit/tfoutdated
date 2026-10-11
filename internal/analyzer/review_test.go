package analyzer

import (
	"testing"

	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
)

func TestNeedsReview(t *testing.T) {
	azurerm := DependencyAnalysis{Name: "azurerm", Source: "hashicorp/azurerm", LatestVer: "5.9.0", UpdateType: resolver.UpdateMajor}

	if !NeedsReview(azurerm, nil) {
		t.Error("provider major upgrade with no known change should need review")
	}
	known := []breaking.BreakingChange{{Provider: "azurerm"}}
	if NeedsReview(azurerm, known) {
		t.Error("provider with known breaking changes should not need review")
	}

	minor := azurerm
	minor.UpdateType = resolver.UpdateMinor
	if NeedsReview(minor, nil) {
		t.Error("minor upgrade should not need review")
	}

	module := azurerm
	module.IsModule = true
	if NeedsReview(module, nil) {
		t.Error("modules are diffed against the registry, an empty result is an answer")
	}
}

func TestUpgradeGuideURL(t *testing.T) {
	dep := DependencyAnalysis{Name: "hashicorp/azurerm", LatestVer: "5.9.0"}
	want := "https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/guides/5.0-upgrade-guide"
	if got := UpgradeGuideURL(dep); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := UpgradeGuideURL(DependencyAnalysis{Name: "random", LatestVer: "3.0.0"}); got != "" {
		t.Errorf("unknown provider should have no guide, got %q", got)
	}
}
