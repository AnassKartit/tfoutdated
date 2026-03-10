package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func fixturesDir(t *testing.T) string {
	t.Helper()
	// Walk up to project root from internal/scanner/
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "..", "..", "testdata", "fixtures")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("fixtures dir not found at %s: %v", root, err)
	}
	return root
}

func TestExtractProviders(t *testing.T) {
	dir := fixturesDir(t)
	s := New(Options{Path: dir, Recursive: false})
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(result.Providers) == 0 {
		t.Fatal("expected providers to be parsed, got none")
	}

	// Build a map for easy lookup
	provByName := make(map[string]ProviderDependency)
	for _, p := range result.Providers {
		provByName[p.Name] = p
	}

	// Check azurerm
	azurerm, ok := provByName["azurerm"]
	if !ok {
		t.Fatal("expected to find provider azurerm")
	}
	if azurerm.Version != "~> 3.75.0" {
		t.Errorf("azurerm version = %q, want %q", azurerm.Version, "~> 3.75.0")
	}
	if azurerm.Namespace != "hashicorp" {
		t.Errorf("azurerm namespace = %q, want %q", azurerm.Namespace, "hashicorp")
	}

	// Check azuread
	azuread, ok := provByName["azuread"]
	if !ok {
		t.Fatal("expected to find provider azuread")
	}
	if azuread.Version != "~> 2.47.0" {
		t.Errorf("azuread version = %q, want %q", azuread.Version, "~> 2.47.0")
	}
	if azuread.Namespace != "hashicorp" {
		t.Errorf("azuread namespace = %q, want %q", azuread.Namespace, "hashicorp")
	}
}

func TestExtractModulesFromFixtures(t *testing.T) {
	dir := fixturesDir(t)
	s := New(Options{Path: dir, Recursive: false})
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(result.Modules) == 0 {
		t.Fatal("expected modules to be parsed, got none")
	}

	found := false
	for _, m := range result.Modules {
		if m.Name == "vm" {
			found = true
			if m.Version != "~> 0.6.0" {
				t.Errorf("module vm version = %q, want %q", m.Version, "~> 0.6.0")
			}
			if m.SourceType != "registry" {
				t.Errorf("module vm sourceType = %q, want %q", m.SourceType, "registry")
			}
			if !m.IsAVM {
				t.Error("module vm should be detected as AVM module")
			}
		}
	}
	if !found {
		t.Error("expected to find module 'vm' in scan results")
	}
}

func TestSkipLocalModules(t *testing.T) {
	dir := fixturesDir(t)
	s := New(Options{Path: dir, Recursive: false})
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	for _, m := range result.Modules {
		if m.Name == "local_mod" {
			t.Error("local module 'local_mod' should be skipped")
		}
		if m.SourceType == "local" {
			t.Errorf("module %q with local sourceType should not appear in results", m.Name)
		}
	}
}

func TestClassifySource(t *testing.T) {
	tests := []struct {
		source     string
		wantType   string
		wantIsAVM  bool
	}{
		{"./modules/local", "local", false},
		{"../shared", "local", false},
		{"hashicorp/consul/aws", "registry", false},
		{"Azure/avm-res-compute-virtualmachine/azurerm", "registry", true},
		{"Azure/avm-ptn-network-hub/azurerm", "registry", true},
		{"git::https://github.com/example/mod.git", "github", false},
		{"github.com/example/mod", "github", false},
		{"git::https://github.com/Azure/avm-res-storage/azurerm", "github", true},
		{"noSlash", "local", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			gotType, gotAVM := classifySource(tt.source)
			if gotType != tt.wantType {
				t.Errorf("classifySource(%q) type = %q, want %q", tt.source, gotType, tt.wantType)
			}
			if gotAVM != tt.wantIsAVM {
				t.Errorf("classifySource(%q) isAVM = %v, want %v", tt.source, gotAVM, tt.wantIsAVM)
			}
		})
	}
}

func TestIsAVMSource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"Azure/avm-res-compute-virtualmachine/azurerm", true},
		{"Azure/avm-ptn-network-hub/azurerm", true},
		{"azure/AVM-RES-Compute-VirtualMachine/azurerm", true}, // case-insensitive
		{"hashicorp/consul/aws", false},
		{"someuser/some-module/azure", false},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := isAVMSource(tt.source)
			if got != tt.want {
				t.Errorf("isAVMSource(%q) = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

func TestRegistrySource(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hashicorp/consul/aws", "hashicorp/consul/aws"},
		{"git::https://github.com/example/mod.git?ref=v1.0.0", "github.com/example/mod.git"},
		{"https://registry.terraform.io/modules/hashicorp/consul/aws", "modules/hashicorp/consul/aws"},
		{"registry.terraform.io/hashicorp/consul/aws", "hashicorp/consul/aws"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := registrySource(tt.input)
			if got != tt.want {
				t.Errorf("registrySource(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractProvidersFromTempFile(t *testing.T) {
	// Create a temp dir with a custom providers file
	tmp := t.TempDir()
	content := `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0, < 6.0.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.6.0"
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(Options{Path: tmp, Recursive: false})
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(result.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(result.Providers))
	}

	provByName := make(map[string]ProviderDependency)
	for _, p := range result.Providers {
		provByName[p.Name] = p
	}

	aws, ok := provByName["aws"]
	if !ok {
		t.Fatal("expected provider aws")
	}
	if aws.Version != ">= 5.0.0, < 6.0.0" {
		t.Errorf("aws version = %q, want %q", aws.Version, ">= 5.0.0, < 6.0.0")
	}

	random, ok := provByName["random"]
	if !ok {
		t.Fatal("expected provider random")
	}
	if random.Version != "3.6.0" {
		t.Errorf("random version = %q, want %q", random.Version, "3.6.0")
	}
}

func TestExtractModulesRegistryAndGitHub(t *testing.T) {
	tmp := t.TempDir()
	content := `
module "consul" {
  source  = "hashicorp/consul/aws"
  version = "0.1.0"
}

module "from_git" {
  source  = "git::https://github.com/example/terraform-module.git"
  version = "1.2.3"
}

module "no_version" {
  source  = "hashicorp/nomad/aws"
}

module "local" {
  source = "../modules/local"
}
`
	if err := os.WriteFile(filepath.Join(tmp, "modules.tf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(Options{Path: tmp, Recursive: false})
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	// Should find consul and from_git, skip no_version (empty version) and local
	modByName := make(map[string]ModuleDependency)
	for _, m := range result.Modules {
		modByName[m.Name] = m
	}

	if _, ok := modByName["consul"]; !ok {
		t.Error("expected module 'consul' to be found")
	}
	if _, ok := modByName["from_git"]; !ok {
		t.Error("expected module 'from_git' to be found")
	}
	if _, ok := modByName["local"]; ok {
		t.Error("local module should be skipped")
	}
	if _, ok := modByName["no_version"]; ok {
		t.Error("module without version should be skipped")
	}
}

func TestExtractResources(t *testing.T) {
	tmp := t.TempDir()
	content := `resource "azurerm_app_service" "main" {
  name                = "my-app"
  location            = "westeurope"
  resource_group_name = "rg-main"
  app_service_plan_id = "plan-id"

  site_config {
    dotnet_framework_version = "v6.0"
    always_on                = true
  }
}

resource "azurerm_app_service_plan" "main" {
  name                = "my-plan"
  location            = "westeurope"
  resource_group_name = "rg-main"

  sku {
    tier = "Standard"
    size = "S1"
  }
}

provider "azurerm" {
  features {}
}
`
	if err := os.WriteFile(filepath.Join(tmp, "main.tf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(Options{Path: tmp, Recursive: false})
	result, err := s.Scan()
	if err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	if len(result.Resources) == 0 {
		t.Fatal("expected resources to be extracted, got none")
	}

	// Build map by type.name
	resByKey := make(map[string]ResourceBlock)
	for _, r := range result.Resources {
		key := r.Type + "." + r.Name
		resByKey[key] = r
	}

	// Check resource blocks
	appService, ok := resByKey["azurerm_app_service.main"]
	if !ok {
		t.Fatal("expected to find azurerm_app_service.main")
	}
	if appService.StartLine != 1 {
		t.Errorf("appService start line = %d, want 1", appService.StartLine)
	}
	if appService.RawHCL == "" {
		t.Error("expected RawHCL to be populated")
	}
	if appService.EndLine <= appService.StartLine {
		t.Errorf("endLine (%d) should be > startLine (%d)", appService.EndLine, appService.StartLine)
	}

	plan, ok := resByKey["azurerm_app_service_plan.main"]
	if !ok {
		t.Fatal("expected to find azurerm_app_service_plan.main")
	}
	if plan.RawHCL == "" {
		t.Error("expected RawHCL to be populated for app_service_plan")
	}

	// Check provider block
	provBlock, ok := resByKey["provider.azurerm"]
	if !ok {
		t.Fatal("expected to find provider.azurerm block")
	}
	if provBlock.RawHCL == "" {
		t.Error("expected RawHCL to be populated for provider block")
	}
}
