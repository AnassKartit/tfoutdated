package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
)

// ModuleDependency represents a Terraform module block dependency.
type ModuleDependency struct {
	Name       string // module block name
	Source     string // registry source path
	Version    string // version constraint string
	FilePath   string // path to .tf file
	Line       int    // line number in file
	IsAVM      bool   // Azure Verified Module detected
	SourceType string // "registry", "github", "local"
}

// ProviderDependency represents a required_providers entry.
type ProviderDependency struct {
	Name          string // provider name, e.g. "azurerm"
	Namespace     string // provider namespace, e.g. "hashicorp"
	Version       string // version constraint string
	LockedVersion string // actual installed version from .terraform.lock.hcl
	FilePath      string
	Line          int
}

// ResourceBlock represents a resource or provider block extracted from a .tf file.
type ResourceBlock struct {
	Type      string // e.g. "azurerm_app_service"
	Name      string // e.g. "main"
	FilePath  string
	StartLine int
	EndLine   int
	RawHCL    string // exact text from the user's file
}

// ScanResult contains all dependencies found during scanning.
type ScanResult struct {
	Modules   []ModuleDependency
	Providers []ProviderDependency
	Resources []ResourceBlock
	Files     []string // all .tf files scanned
}

// Options configures the scanner.
type Options struct {
	Path      string
	Recursive bool
	Ignores   []config.IgnoreRule
}

// Scanner scans Terraform configurations for dependencies.
type Scanner struct {
	opts   Options
	parser *hclparse.Parser
}

// New creates a new Scanner.
func New(opts Options) *Scanner {
	return &Scanner{
		opts:   opts,
		parser: hclparse.NewParser(),
	}
}

// Scan walks the configured path and extracts all dependencies.
func (s *Scanner) Scan() (*ScanResult, error) {
	files, err := walkTerraformFiles(s.opts.Path, s.opts.Recursive, s.opts.Ignores)
	if err != nil {
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	result := &ScanResult{
		Files: files,
	}

	for _, file := range files {
		hclFile, diags := s.parser.ParseHCLFile(file)
		if diags.HasErrors() {
			// Skip files that can't be parsed
			continue
		}

		modules := extractModules(hclFile, file)
		providers := extractProviders(hclFile, file)

		result.Modules = append(result.Modules, modules...)
		result.Providers = append(result.Providers, providers...)

		// Extract resource and provider config blocks for real-code snippets
		rawBytes, err := os.ReadFile(file)
		if err == nil {
			resources := extractResources(hclFile, file, rawBytes)
			result.Resources = append(result.Resources, resources...)
		}
	}

	// Scan for cdktf.json and package.json (cdktf support)
	cdktfPath, packagePath := findCdktfFiles(s.opts.Path)
	if cdktfPath != "" {
		modules, providers := scanCdktfJSON(cdktfPath)
		result.Modules = append(result.Modules, modules...)
		result.Providers = append(result.Providers, providers...)
		result.Files = append(result.Files, cdktfPath)
	}
	if packagePath != "" {
		providers := scanPackageJSON(packagePath)
		result.Providers = append(result.Providers, providers...)
		if cdktfPath == "" {
			// Only add package.json to files list if cdktf.json wasn't already added
			result.Files = append(result.Files, packagePath)
		}
	}

	// Scan for terragrunt.hcl files
	tgFiles := findTerragruntFiles(s.opts.Path, s.opts.Recursive)
	for _, tgFile := range tgFiles {
		modules, providers := scanTerragruntHCL(tgFile)
		result.Modules = append(result.Modules, modules...)
		result.Providers = append(result.Providers, providers...)
		result.Files = append(result.Files, tgFile)
	}

	// Read lock file to get actual installed provider versions
	lockedVersions := parseLockFile(s.opts.Path)
	if len(lockedVersions) > 0 {
		for i := range result.Providers {
			p := &result.Providers[i]
			ns := p.Namespace
			if ns == "" {
				ns = "hashicorp"
			}
			key := strings.ToLower(ns + "/" + p.Name)
			for lockKey, v := range lockedVersions {
				if strings.ToLower(lockKey) == key {
					p.LockedVersion = v
					break
				}
			}
		}
	}

	return result, nil
}

// parseLockFile reads .terraform.lock.hcl and returns provider → locked version map.
// Keys are "namespace/name" (e.g. "hashicorp/azurerm").
func parseLockFile(dir string) map[string]string {
	lockPath := filepath.Join(dir, ".terraform.lock.hcl")
	content, err := os.ReadFile(lockPath)
	if err != nil {
		return nil
	}

	versions := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	var currentProvider string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match: provider "registry.terraform.io/hashicorp/azurerm" {
		if strings.HasPrefix(line, "provider ") && strings.HasSuffix(line, "{") {
			// Extract the provider source from quotes
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start >= 0 && end > start {
				source := line[start+1 : end]
				// Strip "registry.terraform.io/" prefix
				source = strings.TrimPrefix(source, "registry.terraform.io/")
				currentProvider = source
			}
		}
		// Match: version = "4.62.0"
		if currentProvider != "" && strings.HasPrefix(line, "version") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				ver := strings.TrimSpace(parts[1])
				ver = strings.Trim(ver, "\"")
				versions[currentProvider] = ver
				currentProvider = ""
			}
		}
	}

	return versions
}

// classifySource determines the source type and cleans the source string.
func classifySource(source string) (sourceType string, isAVM bool) {
	switch {
	case strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../"):
		return "local", false
	case strings.HasPrefix(source, "git::") || strings.HasPrefix(source, "github.com/"):
		isAVM = isAVMSource(source)
		return "github", isAVM
	case strings.Contains(source, "/"):
		isAVM = isAVMSource(source)
		return "registry", isAVM
	default:
		return "local", false
	}
}

// isAVMSource checks if a source is an Azure Verified Module.
func isAVMSource(source string) bool {
	lower := strings.ToLower(source)
	return strings.Contains(lower, "azure/avm-res-") ||
		strings.Contains(lower, "azure/avm-ptn-") ||
		strings.Contains(lower, "azure/avm-utl-")
}

// registrySource extracts a clean registry source from various source formats.
func registrySource(source string) string {
	// Remove query parameters (e.g., ?ref=v1.0.0)
	if idx := strings.Index(source, "?"); idx != -1 {
		source = source[:idx]
	}
	// Remove git:: prefix
	source = strings.TrimPrefix(source, "git::")
	// Remove https:// prefix if present
	source = strings.TrimPrefix(source, "https://")
	// Remove registry.terraform.io/ prefix
	source = strings.TrimPrefix(source, "registry.terraform.io/")
	return source
}

// extractModules parses module blocks from an HCL file.
func extractModules(file *hcl.File, filePath string) []ModuleDependency {
	var modules []ModuleDependency

	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "module", LabelNames: []string{"name"}},
		},
	})
	if diags.HasErrors() {
		return nil
	}

	for _, block := range content.Blocks {
		attrs, _ := block.Body.JustAttributes()

		sourceAttr, hasSource := attrs["source"]
		if !hasSource {
			continue
		}
		sourceVal, diags := sourceAttr.Expr.Value(nil)
		if diags.HasErrors() {
			continue
		}
		source := sourceVal.AsString()

		sourceType, isAVM := classifySource(source)
		if sourceType == "local" {
			continue // skip local modules
		}

		var versionStr string
		if versionAttr, ok := attrs["version"]; ok {
			versionVal, diags := versionAttr.Expr.Value(nil)
			if !diags.HasErrors() {
				versionStr = versionVal.AsString()
			}
		}

		// Skip modules without version constraints
		if versionStr == "" {
			continue
		}

		relPath, _ := filepath.Rel(".", filePath)
		if relPath == "" {
			relPath = filePath
		}

		modules = append(modules, ModuleDependency{
			Name:       block.Labels[0],
			Source:     registrySource(source),
			Version:    versionStr,
			FilePath:   relPath,
			Line:       block.DefRange.Start.Line,
			IsAVM:      isAVM,
			SourceType: sourceType,
		})
	}

	return modules
}

// extractProviders parses required_providers from terraform blocks.
func extractProviders(file *hcl.File, filePath string) []ProviderDependency {
	var providers []ProviderDependency

	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "terraform"},
		},
	})
	if diags.HasErrors() {
		return nil
	}

	for _, block := range content.Blocks {
		innerContent, _, diags := block.Body.PartialContent(&hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{
				{Type: "required_providers"},
			},
		})
		if diags.HasErrors() {
			continue
		}

		for _, rpBlock := range innerContent.Blocks {
			attrs, _ := rpBlock.Body.JustAttributes()

			for name, attr := range attrs {
				// required_providers entries can be either a simple string or an object
				val, diags := attr.Expr.Value(nil)
				if diags.HasErrors() {
					continue
				}

				var namespace, versionStr string

				if val.Type().IsObjectType() {
					sourceVal := val.GetAttr("source")
					if sourceVal.IsKnown() && !sourceVal.IsNull() {
						parts := strings.Split(sourceVal.AsString(), "/")
						if len(parts) >= 2 {
							namespace = parts[len(parts)-2]
						}
					}
					versionVal := val.GetAttr("version")
					if versionVal.IsKnown() && !versionVal.IsNull() {
						versionStr = versionVal.AsString()
					}
				} else if val.Type() == cty.String {
					// Simple string constraint (rare but valid)
					versionStr = val.AsString()
				}

				if versionStr == "" {
					continue
				}

				relPath, _ := filepath.Rel(".", filePath)
				if relPath == "" {
					relPath = filePath
				}

				providers = append(providers, ProviderDependency{
					Name:      name,
					Namespace: namespace,
					Version:   versionStr,
					FilePath:  relPath,
					Line:      attr.Range.Start.Line,
				})
			}
		}
	}

	return providers
}

// extractResources extracts resource and provider config blocks from an HCL file.
func extractResources(file *hcl.File, filePath string, rawBytes []byte) []ResourceBlock {
	var blocks []ResourceBlock

	relPath, _ := filepath.Rel(".", filePath)
	if relPath == "" {
		relPath = filePath
	}

	// Extract resource blocks (resource "type" "name" { ... })
	content, _, diags := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "resource", LabelNames: []string{"type", "name"}},
			{Type: "provider", LabelNames: []string{"name"}},
		},
	})
	if diags.HasErrors() {
		return nil
	}

	for _, block := range content.Blocks {
		startByte := block.DefRange.Start.Byte
		startLine := block.DefRange.Start.Line

		// Find the block text using brace-counting from the opening brace
		blockText, endLine := extractBlockText(rawBytes, startByte, startLine)
		if blockText == "" {
			continue
		}

		rb := ResourceBlock{
			FilePath:  relPath,
			StartLine: startLine,
			EndLine:   endLine,
			RawHCL:    blockText,
		}

		switch block.Type {
		case "resource":
			rb.Type = block.Labels[0]
			rb.Name = block.Labels[1]
		case "provider":
			rb.Type = "provider"
			rb.Name = block.Labels[0]
		}

		blocks = append(blocks, rb)
	}

	return blocks
}

// extractBlockText uses brace-counting on raw file bytes to extract a complete block.
func extractBlockText(raw []byte, startByte int, startLine int) (string, int) {
	// Find the opening brace from the start position
	pos := startByte
	for pos < len(raw) && raw[pos] != '{' {
		pos++
	}
	if pos >= len(raw) {
		return "", startLine
	}

	// Count braces to find the matching closing brace
	depth := 0
	endPos := pos
	endLine := startLine
	inString := false
	escaped := false

	for endPos < len(raw) {
		ch := raw[endPos]

		if escaped {
			escaped = false
			endPos++
			continue
		}

		if ch == '\\' && inString {
			escaped = true
			endPos++
			continue
		}

		if ch == '"' {
			inString = !inString
		}

		if !inString {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					endPos++ // include the closing brace
					break
				}
			}
		}

		if ch == '\n' {
			endLine++
		}
		endPos++
	}

	if depth != 0 {
		return "", startLine
	}

	return string(raw[startByte:endPos]), endLine
}
