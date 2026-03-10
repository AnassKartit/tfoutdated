package fixer

import (
	"encoding/json"
	"os"
	"strings"
)

// applyCdktfChanges rewrites version strings in cdktf.json.
func applyCdktfChanges(filePath string, changes []Change) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	content := string(data)

	// We need to do targeted JSON edits. Since cdktf.json can have various
	// structures, we parse and re-serialize with modifications.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		// Fallback: string replacement
		return applyCdktfStringReplace(filePath, content, changes)
	}

	changed := false

	// Update terraformModules
	if modulesRaw, ok := raw["terraformModules"]; ok {
		var modules []map[string]interface{}
		if err := json.Unmarshal(modulesRaw, &modules); err == nil {
			for i := range modules {
				mod := modules[i]
				name, _ := mod["name"].(string)
				version, _ := mod["version"].(string)
				source, _ := mod["source"].(string)

				for _, c := range changes {
					if matchesCdktfModule(c, name, source) && version == c.OldVersion {
						modules[i]["version"] = c.NewVersion
						changed = true
					}
				}
			}
			if changed {
				updated, err := json.Marshal(modules)
				if err == nil {
					raw["terraformModules"] = updated
				}
			}
		}
	}

	// Update terraformProviders
	if providersRaw, ok := raw["terraformProviders"]; ok {
		var providers []json.RawMessage
		if err := json.Unmarshal(providersRaw, &providers); err == nil {
			for i := range providers {
				// Try as string first
				var s string
				if err := json.Unmarshal(providers[i], &s); err == nil {
					for _, c := range changes {
						// Match by provider name (e.g., "aws" in "hashicorp/aws@~> 5.30")
						if strings.Contains(s, "/"+c.Name+"@") || strings.HasPrefix(s, c.Name+"@") {
							// Replace everything after @ with new version
							if idx := strings.Index(s, "@"); idx >= 0 {
								newS := s[:idx+1] + c.NewVersion
								providers[i] = jsonString(newS)
								changed = true
							}
						} else if strings.Contains(s, c.OldVersion) {
							newS := strings.Replace(s, c.OldVersion, c.NewVersion, 1)
							providers[i] = jsonString(newS)
							changed = true
						}
					}
					continue
				}

				// Try as object
				var obj map[string]interface{}
				if err := json.Unmarshal(providers[i], &obj); err == nil {
					name, _ := obj["name"].(string)
					version, _ := obj["version"].(string)
					for _, c := range changes {
						if c.Name == name && version == c.OldVersion {
							obj["version"] = c.NewVersion
							updated, _ := marshalRawNoEscape(obj)
							if updated != nil {
								providers[i] = updated
							}
							changed = true
						}
					}
				}
			}
			if changed {
				updated, err := marshalRawNoEscape(providers)
				if err == nil {
					raw["terraformProviders"] = updated
				}
			}
		}
	}

	if !changed {
		return nil
	}

	// Re-serialize with indentation, disabling HTML escaping to preserve ~> etc.
	output, err := marshalJSONNoEscape(raw)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, output, 0644)
}

// applyCdktfStringReplace is a fallback that does simple string replacement in cdktf.json.
func applyCdktfStringReplace(filePath, content string, changes []Change) error {
	result := content
	for _, c := range changes {
		result = strings.Replace(result, `"`+c.OldVersion+`"`, `"`+c.NewVersion+`"`, 1)
	}
	if result == content {
		return nil
	}
	return os.WriteFile(filePath, []byte(result), 0644)
}

// applyPackageJSONChanges rewrites @cdktf/provider-* versions in package.json.
func applyPackageJSONChanges(filePath string, changes []Change) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Parse the full package.json preserving structure
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	changed := false

	// Build a lookup: provider name → Change
	changeLookup := make(map[string]Change)
	for _, c := range changes {
		changeLookup[c.Name] = c
	}

	// Provider name → npm package name
	providerToPackage := map[string]string{
		"aws":        "@cdktf/provider-aws",
		"azurerm":    "@cdktf/provider-azurerm",
		"google":     "@cdktf/provider-google",
		"azuread":    "@cdktf/provider-azuread",
		"azapi":      "@cdktf/provider-azapi",
		"kubernetes": "@cdktf/provider-kubernetes",
		"helm":       "@cdktf/provider-helm",
		"null":       "@cdktf/provider-null",
		"random":     "@cdktf/provider-random",
		"local":      "@cdktf/provider-local",
		"external":   "@cdktf/provider-external",
		"tls":        "@cdktf/provider-tls",
		"dns":        "@cdktf/provider-dns",
		"time":       "@cdktf/provider-time",
		"archive":    "@cdktf/provider-archive",
		"http":       "@cdktf/provider-http",
	}

	for _, section := range []string{"dependencies", "devDependencies"} {
		sectionRaw, ok := raw[section]
		if !ok {
			continue
		}

		var deps map[string]string
		if err := json.Unmarshal(sectionRaw, &deps); err != nil {
			continue
		}

		for provName, c := range changeLookup {
			pkgName, ok := providerToPackage[provName]
			if !ok {
				continue
			}

			currentVer, ok := deps[pkgName]
			if !ok {
				continue
			}

			// Preserve the npm version prefix (^, ~, etc.)
			prefix := ""
			for _, ch := range currentVer {
				if ch == '^' || ch == '~' || ch == '>' || ch == '=' || ch == '<' || ch == ' ' {
					prefix += string(ch)
				} else {
					break
				}
			}
			if prefix == "" {
				prefix = "^" // default npm prefix
			}

			// Extract just the version number from TF constraint (e.g., "~> 6.28.0" → "6.28.0")
			newVer := extractMinVersion(c.NewVersion)
			if newVer == "" {
				newVer = c.NewVersion
			}

			deps[pkgName] = prefix + newVer
			changed = true
		}

		if changed {
			updated, err := json.Marshal(deps)
			if err == nil {
				raw[section] = updated
			}
		}
	}

	if !changed {
		return nil
	}

	output, err := marshalJSONNoEscape(raw)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, output, 0644)
}

// jsonString returns a JSON-encoded string without HTML escaping.
func jsonString(s string) json.RawMessage {
	// Manually quote the string to avoid Go's HTML escaping of <, >, &
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
	return json.RawMessage(`"` + escaped + `"`)
}

// marshalRawNoEscape marshals a value without HTML escaping, returning json.RawMessage.
func marshalRawNoEscape(v interface{}) (json.RawMessage, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode adds a trailing newline, strip it
	s := strings.TrimRight(buf.String(), "\n")
	return json.RawMessage(s), nil
}

// marshalJSONNoEscape serializes to indented JSON without HTML escaping.
func marshalJSONNoEscape(v interface{}) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// matchesCdktfModule checks if a fixer.Change matches a cdktf module by name or source.
func matchesCdktfModule(c Change, name, source string) bool {
	if c.Name == name {
		return true
	}
	// Match by source path
	if source != "" && strings.Contains(source, c.Name) {
		return true
	}
	return false
}

// IsCdktfFile returns true if the file is a cdktf.json or a cdktf-related package.json.
func IsCdktfFile(path string) bool {
	base := strings.ToLower(path)
	return strings.HasSuffix(base, "cdktf.json") || strings.HasSuffix(base, "package.json")
}
