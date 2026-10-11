package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// Terraform forbids a `version` argument on a non-registry source: a git module
// states its version in the ref. Reading only the version attribute made every
// git-hosted module invisible — precisely the modules an organisation maintains
// itself — and the scan then reported "no dependencies found".
func TestExtractGitModules(t *testing.T) {
	dir := t.TempDir()
	body := `
module "subnets" {
  source = "git::https://dev.azure.com/org/Project/_git/network_subnet?ref=v1.2.3"
}

module "floating" {
  source = "git::https://github.com/org/repo.git"
}

module "registry" {
  source  = "Azure/naming/azurerm"
  version = "0.4.0"
}

module "local" {
  source = "./modules/thing"
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := New(Options{Path: dir, Recursive: false}).Scan()
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]ModuleDependency{}
	for _, m := range res.Modules {
		byName[m.Name] = m
	}

	subnets, ok := byName["subnets"]
	if !ok {
		t.Fatal("git module with a ref was not extracted")
	}
	if subnets.Version != "v1.2.3" {
		t.Errorf("version should come from the ref, got %q", subnets.Version)
	}
	if subnets.Unpinned {
		t.Error("a module with a ref is pinned")
	}
	// The full URL must survive: the resolver needs the host and the ref.
	if subnets.Source != "git::https://dev.azure.com/org/Project/_git/network_subnet?ref=v1.2.3" {
		t.Errorf("git source must keep its URL and ref, got %q", subnets.Source)
	}

	floating, ok := byName["floating"]
	if !ok {
		t.Fatal("a git module without a ref must still be reported, not skipped")
	}
	if !floating.Unpinned {
		t.Error("a module with no ref tracks the default branch and is unpinned")
	}

	if byName["registry"].Version != "0.4.0" {
		t.Errorf("registry module unchanged, got %q", byName["registry"].Version)
	}
	if _, ok := byName["local"]; ok {
		t.Error("a local module has no version of its own and must be skipped")
	}
}
