package cmd

import (
	"testing"

	"github.com/anasskartit/tfoutdated/internal/scanner"
)

func TestUnpinnedDeps(t *testing.T) {
	scan := &scanner.ScanResult{
		Providers: []scanner.ProviderDependency{
			{Name: "azurerm", Version: ">= 3.116.0", FilePath: "versions.tf", Line: 7},
			{Name: "azapi", Version: "~> 2.4", FilePath: "versions.tf", Line: 11},
			{Name: "random", Version: "", FilePath: "versions.tf", Line: 15},
			{Name: "azurerm", Version: ">= 3.116.0", FilePath: ".terraform.lock.hcl"},
			{Name: "time", Version: ">= 0.9, < 1.0", FilePath: "versions.tf", Line: 19},
		},
		Modules: []scanner.ModuleDependency{
			{Name: "vnet", Version: "~> 0.15.0", SourceType: "registry"},
			{Name: "nsg", Version: "", SourceType: "registry"},
			{Name: "local", Version: "", SourceType: "local"},
			{Name: "gitmod", SourceType: "git", Unpinned: true},
		},
	}
	got := map[string]bool{}
	for _, u := range unpinnedDeps(scan) {
		if u.File == ".terraform.lock.hcl" {
			t.Errorf("lock file entry reported: %+v", u)
		}
		got[u.Name] = true
	}
	for _, want := range []string{"azurerm", "random", "nsg", "gitmod"} {
		if !got[want] {
			t.Errorf("%s should be reported as unpinned", want)
		}
	}
	for _, notWant := range []string{"azapi", "time", "vnet", "local"} {
		if got[notWant] {
			t.Errorf("%s has a bounded constraint or is local, should not be reported", notWant)
		}
	}
}
