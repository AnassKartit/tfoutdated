package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// cdktfJSON represents the structure of a cdktf.json file.
type cdktfJSON struct {
	TerraformModules   []cdktfModule   `json:"terraformModules"`
	TerraformProviders []cdktfProvider `json:"terraformProviders"`
}

// cdktfModule represents a module entry in cdktf.json terraformModules.
type cdktfModule struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
}

// cdktfProvider represents a provider entry in cdktf.json terraformProviders.
// Can be a string like "hashicorp/aws@~> 5.0" or an object.
type cdktfProvider struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
}

// UnmarshalJSON handles cdktf providers which can be strings or objects.
func (p *cdktfProvider) UnmarshalJSON(data []byte) error {
	// Try string first: "hashicorp/aws@~> 5.0" or "aws@~> 5.0"
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		return p.parseProviderString(s)
	}

	// Try object
	type alias cdktfProvider
	var obj alias
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*p = cdktfProvider(obj)
	return nil
}

func (p *cdktfProvider) parseProviderString(s string) error {
	// Format: "namespace/name@version" or "name@version" or "namespace/name"
	version := ""
	source := s

	if idx := strings.Index(s, "@"); idx >= 0 {
		source = s[:idx]
		version = s[idx+1:]
	}

	parts := strings.Split(source, "/")
	switch len(parts) {
	case 1:
		p.Name = parts[0]
		p.Namespace = "hashicorp"
	case 2:
		p.Namespace = parts[0]
		p.Name = parts[1]
	case 3:
		// registry.terraform.io/hashicorp/aws
		p.Namespace = parts[1]
		p.Name = parts[2]
	}

	p.Source = source
	p.Version = version
	return nil
}

// packageJSON represents the relevant parts of a package.json file.
type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// cdktfProviderPackages maps npm package prefixes to Terraform provider info.
var cdktfProviderPackages = map[string]struct {
	Namespace string
	Name      string
}{
	"@cdktf/provider-aws":       {Namespace: "hashicorp", Name: "aws"},
	"@cdktf/provider-azurerm":   {Namespace: "hashicorp", Name: "azurerm"},
	"@cdktf/provider-google":    {Namespace: "hashicorp", Name: "google"},
	"@cdktf/provider-azuread":   {Namespace: "hashicorp", Name: "azuread"},
	"@cdktf/provider-azapi":     {Namespace: "azure", Name: "azapi"},
	"@cdktf/provider-kubernetes": {Namespace: "hashicorp", Name: "kubernetes"},
	"@cdktf/provider-helm":      {Namespace: "hashicorp", Name: "helm"},
	"@cdktf/provider-null":      {Namespace: "hashicorp", Name: "null"},
	"@cdktf/provider-random":    {Namespace: "hashicorp", Name: "random"},
	"@cdktf/provider-local":     {Namespace: "hashicorp", Name: "local"},
	"@cdktf/provider-external":  {Namespace: "hashicorp", Name: "external"},
	"@cdktf/provider-tls":       {Namespace: "hashicorp", Name: "tls"},
	"@cdktf/provider-dns":       {Namespace: "hashicorp", Name: "dns"},
	"@cdktf/provider-time":      {Namespace: "hashicorp", Name: "time"},
	"@cdktf/provider-archive":   {Namespace: "hashicorp", Name: "archive"},
	"@cdktf/provider-http":      {Namespace: "hashicorp", Name: "http"},
}

// scanCdktfJSON parses a cdktf.json file and extracts module and provider dependencies.
func scanCdktfJSON(path string) ([]ModuleDependency, []ProviderDependency) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	var cfg cdktfJSON
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil
	}

	relPath, _ := filepath.Rel(".", path)
	if relPath == "" {
		relPath = path
	}

	var modules []ModuleDependency
	var providers []ProviderDependency

	// Extract modules from terraformModules
	for i, mod := range cfg.TerraformModules {
		if mod.Source == "" || mod.Version == "" {
			continue
		}

		sourceType, isAVM := classifySource(mod.Source)
		if sourceType == "local" {
			continue
		}

		name := mod.Name
		if name == "" {
			name = inferModuleName(mod.Source)
		}

		modules = append(modules, ModuleDependency{
			Name:       name,
			Source:     registrySource(mod.Source),
			Version:    mod.Version,
			FilePath:   relPath,
			Line:       i + 1, // approximate line (JSON doesn't have precise line numbers easily)
			IsAVM:      isAVM,
			SourceType: sourceType,
		})
	}

	// Extract providers from terraformProviders
	for _, prov := range cfg.TerraformProviders {
		if prov.Version == "" || prov.Name == "" {
			continue
		}

		ns := prov.Namespace
		if ns == "" {
			ns = "hashicorp"
		}

		providers = append(providers, ProviderDependency{
			Name:      prov.Name,
			Namespace: ns,
			Version:   prov.Version,
			FilePath:  relPath,
			Line:      1,
		})
	}

	return modules, providers
}

// scanPackageJSON parses a package.json file and extracts @cdktf/provider-* dependencies.
func scanPackageJSON(path string) []ProviderDependency {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(".", path)
	if relPath == "" {
		relPath = path
	}

	var providers []ProviderDependency
	seen := make(map[string]bool)

	// Check both dependencies and devDependencies
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for pkgName, version := range deps {
			info, ok := cdktfProviderPackages[pkgName]
			if !ok {
				continue
			}
			if seen[pkgName] {
				continue
			}
			seen[pkgName] = true

			// Clean npm version range to something we can resolve
			// npm uses ^, ~, >=, etc. — we extract the base version
			tfVersion := npmVersionToConstraint(version)
			if tfVersion == "" {
				continue
			}

			providers = append(providers, ProviderDependency{
				Name:      info.Name,
				Namespace: info.Namespace,
				Version:   tfVersion,
				FilePath:  relPath,
				Line:      1,
			})
		}
	}

	return providers
}

// npmVersionToConstraint converts an npm version string to a Terraform-style constraint.
// @cdktf/provider-aws versions track provider versions with a suffix, e.g.:
//   - "19.38.0" means it wraps aws provider ~> 5.x
//
// For cdktf provider packages, the npm version IS the package version (not the TF provider version).
// We need to look up the actual TF provider version from the registry.
// For now, we return the version as-is so the resolver can handle it.
func npmVersionToConstraint(version string) string {
	v := strings.TrimSpace(version)
	if v == "" || v == "*" || v == "latest" {
		return ""
	}

	// Strip npm range prefixes to get base version
	v = strings.TrimLeft(v, "^~>=<! ")

	// Handle compound ranges like ">=1.0.0 <2.0.0"
	if parts := strings.Fields(v); len(parts) > 0 {
		v = strings.TrimLeft(parts[0], "^~>=<! ")
	}

	if v == "" {
		return ""
	}

	return v
}

// inferModuleName creates a module name from a registry source.
func inferModuleName(source string) string {
	parts := strings.Split(source, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return source
}

// findCdktfFiles looks for cdktf.json and package.json in the given directory.
func findCdktfFiles(dir string) (cdktfPath, packagePath string) {
	cdktfPath = filepath.Join(dir, "cdktf.json")
	if _, err := os.Stat(cdktfPath); err != nil {
		cdktfPath = ""
	}

	packagePath = filepath.Join(dir, "package.json")
	if _, err := os.Stat(packagePath); err != nil {
		packagePath = ""
	}

	// Only return package.json if cdktf.json exists (to avoid scanning random Node projects)
	if cdktfPath == "" && packagePath != "" {
		// Check if package.json has any @cdktf dependencies
		if !hasCdktfDeps(packagePath) {
			packagePath = ""
		}
	}

	return
}

// hasCdktfDeps checks if a package.json contains @cdktf/provider-* dependencies.
func hasCdktfDeps(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// Quick check before full parse
	if !strings.Contains(string(data), "@cdktf/provider-") {
		return false
	}
	return true
}
