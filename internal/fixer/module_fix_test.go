package fixer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

const nsgModuleCall = `module "nsg" {
  source              = "Azure/avm-res-network-networksecuritygroup/azurerm"
  version             = "0.6.0"
  name                = "nsg-example"
  resource_group_name = "rg-example"
  location            = "westeurope"
}
`

func writeModuleCall(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "main.tf")
	if err := os.WriteFile(path, []byte(nsgModuleCall), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func nsgAnalysis(path string, bc breaking.BreakingChange) *analyzer.Analysis {
	return &analyzer.Analysis{
		Dependencies: []analyzer.DependencyAnalysis{{
			Name:     "nsg",
			Source:   "Azure/avm-res-network-networksecuritygroup/azurerm",
			FilePath: path,
			Line:     1,
			IsModule: true,
		}},
		BreakingChanges: []breaking.BreakingChange{bc},
	}
}

func renameChange(autoFixable bool) breaking.BreakingChange {
	return breaking.BreakingChange{
		Provider:    "Azure/avm-res-network-networksecuritygroup/azurerm",
		IsModule:    true,
		Kind:        breaking.VariableRenamed,
		Attribute:   "resource_group_name",
		AutoFixable: autoFixable,
		Transform: &breaking.Transform{
			RenameAttrs: map[string]string{"resource_group_name": "parent_id"},
		},
	}
}

func TestFixModuleCalls_SkipsSuggestedRename(t *testing.T) {
	path := writeModuleCall(t)
	f := New(Options{})

	changes, err := f.FixModuleCalls(nsgAnalysis(path, renameChange(false)),
		&scanner.ScanResult{Files: []string{path}}, []Change{{Name: "nsg", FilePath: path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("a guessed rename must not be applied, got %v", changes)
	}
	got, _ := os.ReadFile(path)
	if string(got) != nsgModuleCall {
		t.Errorf("file was modified:\n%s", got)
	}
}

func TestFixModuleCalls_AppliesAutoFixableRename(t *testing.T) {
	path := writeModuleCall(t)
	f := New(Options{})

	changes, err := f.FixModuleCalls(nsgAnalysis(path, renameChange(true)),
		&scanner.ScanResult{Files: []string{path}}, []Change{{Name: "nsg", FilePath: path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Type != "rename" {
		t.Fatalf("expected one rename, got %v", changes)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "parent_id") || strings.Contains(string(got), "resource_group_name") {
		t.Errorf("rename not applied:\n%s", got)
	}
}
