package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// terragruntSourceRegex parses tfr:/// URLs:
//
//	tfr:///terraform-aws-modules/eks/aws?version=19.0.0
//	tfr:///terraform-aws-modules/eks/aws//?version=19.0.0
//	tfr://terraform-aws-modules/eks/aws?version=19.0.0
var terragruntSourceRegex = regexp.MustCompile(
	`^tfr:/{2,3}([^?/]+/[^?/]+/[^?/]+)(?://)?(?:\?version=(.+))?$`,
)

// scanTerragruntHCL parses a terragrunt.hcl file and extracts module dependencies.
func scanTerragruntHCL(path string) ([]ModuleDependency, []ProviderDependency) {
	parser := hclparse.NewParser()
	hclFile, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		// Terragrunt HCL may use functions like find_in_parent_folders() that
		// the plain HCL parser can't evaluate. Fall back to regex.
		return scanTerragruntRegex(path)
	}

	relPath, _ := filepath.Rel(".", path)
	if relPath == "" {
		relPath = path
	}

	var modules []ModuleDependency

	// Look for terraform { source = "..." } blocks
	content, _, diags := hclFile.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "terraform"},
		},
	})
	if diags.HasErrors() {
		return scanTerragruntRegex(path)
	}

	for _, block := range content.Blocks {
		attrs, _ := block.Body.JustAttributes()
		sourceAttr, ok := attrs["source"]
		if !ok {
			continue
		}

		sourceVal, diags := sourceAttr.Expr.Value(nil)
		if diags.HasErrors() {
			// Source uses terragrunt functions — fall back to regex
			return scanTerragruntRegex(path)
		}

		source := sourceVal.AsString()
		mod := parseTerragruntSource(source, relPath, block.DefRange.Start.Line)
		if mod != nil {
			modules = append(modules, *mod)
		}
	}

	return modules, nil
}

// parseTerragruntSource extracts a ModuleDependency from a terragrunt source string.
func parseTerragruntSource(source, filePath string, line int) *ModuleDependency {
	// Handle tfr:/// registry sources
	matches := terragruntSourceRegex.FindStringSubmatch(source)
	if len(matches) >= 2 {
		registrySrc := matches[1]
		version := ""
		if len(matches) >= 3 {
			version = matches[2]
		}
		if version == "" {
			return nil
		}

		sourceType, isAVM := classifySource(registrySrc)
		name := inferTerragruntModuleName(registrySrc)

		return &ModuleDependency{
			Name:       name,
			Source:     registrySource(registrySrc),
			Version:    version,
			FilePath:   filePath,
			Line:       line,
			IsAVM:      isAVM,
			SourceType: sourceType,
		}
	}

	// Handle git:: sources with ?ref=vX.Y.Z
	if strings.HasPrefix(source, "git::") {
		return parseGitTerragruntSource(source, filePath, line)
	}

	return nil
}

// parseGitTerragruntSource handles git::https://github.com/org/module.git?ref=v1.0.0
func parseGitTerragruntSource(source, filePath string, line int) *ModuleDependency {
	// Extract ref parameter
	version := ""
	if idx := strings.Index(source, "?ref="); idx >= 0 {
		version = strings.TrimPrefix(source[idx:], "?ref=")
		version = strings.TrimPrefix(version, "v")
	}
	if version == "" {
		return nil
	}

	// Try to extract registry-compatible source from git URL
	// git::https://github.com/terraform-aws-modules/terraform-aws-eks.git
	cleaned := strings.TrimPrefix(source, "git::")
	cleaned = strings.TrimPrefix(cleaned, "https://")
	cleaned = strings.TrimPrefix(cleaned, "github.com/")
	if idx := strings.Index(cleaned, "?"); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSuffix(cleaned, ".git")

	// Try to convert github path to registry format
	// terraform-aws-modules/terraform-aws-eks → terraform-aws-modules/eks/aws
	parts := strings.Split(cleaned, "/")
	if len(parts) >= 2 {
		sourceType, isAVM := classifySource(source)
		name := parts[len(parts)-1]
		// Strip terraform- prefix patterns
		name = strings.TrimPrefix(name, "terraform-")
		for _, cloud := range []string{"aws-", "azurerm-", "google-"} {
			name = strings.TrimPrefix(name, cloud)
		}

		return &ModuleDependency{
			Name:       name,
			Source:     cleaned,
			Version:    version,
			FilePath:   filePath,
			Line:       line,
			IsAVM:      isAVM,
			SourceType: sourceType,
		}
	}

	return nil
}

// scanTerragruntRegex is a fallback parser for terragrunt.hcl files that use
// functions the HCL parser can't evaluate.
func scanTerragruntRegex(path string) ([]ModuleDependency, []ProviderDependency) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}

	relPath, _ := filepath.Rel(".", path)
	if relPath == "" {
		relPath = path
	}

	content := string(data)
	var modules []ModuleDependency

	// Match source = "tfr:///...?version=..." patterns
	sourceRegex := regexp.MustCompile(`source\s*=\s*"(tfr://[^"]+)"`)
	for _, match := range sourceRegex.FindAllStringSubmatchIndex(content, -1) {
		if len(match) >= 4 {
			sourceStr := content[match[2]:match[3]]
			line := strings.Count(content[:match[0]], "\n") + 1
			mod := parseTerragruntSource(sourceStr, relPath, line)
			if mod != nil {
				modules = append(modules, *mod)
			}
		}
	}

	// Match source = "git::...?ref=..." patterns
	gitRegex := regexp.MustCompile(`source\s*=\s*"(git::[^"]+)"`)
	for _, match := range gitRegex.FindAllStringSubmatchIndex(content, -1) {
		if len(match) >= 4 {
			sourceStr := content[match[2]:match[3]]
			line := strings.Count(content[:match[0]], "\n") + 1
			mod := parseTerragruntSource(sourceStr, relPath, line)
			if mod != nil {
				modules = append(modules, *mod)
			}
		}
	}

	return modules, nil
}

// inferTerragruntModuleName generates a short name from a registry source.
func inferTerragruntModuleName(source string) string {
	parts := strings.Split(source, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return source
}

// findTerragruntFiles discovers all terragrunt.hcl files under a directory.
func findTerragruntFiles(root string, recursive bool) []string {
	var files []string

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if name == ".terragrunt-cache" {
				return filepath.SkipDir
			}
			if !recursive && path != root {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == "terragrunt.hcl" {
			files = append(files, path)
		}
		return nil
	})

	return files
}
