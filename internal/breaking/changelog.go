package breaking

import (
	"regexp"
	"strings"
)

var (
	versionHeaderRe   = regexp.MustCompile(`(?m)^##\s+v?(\d+\.\d+\.\d+)`)
	breakingSectionRe = regexp.MustCompile(`(?i)(?:BREAKING\s+CHANGE|breaking|removed|deprecated)`)

	// broadSectionRe matches sections that may contain notable changes even without
	// the word "breaking" — used to widen the net for module changelogs that often
	// document changes under generic headings.
	broadSectionRe = regexp.MustCompile(`(?i)(?:BREAKING\s+CHANGE|breaking|removed|deprecated|variable|migrated|required)`)

	removedPattern  = regexp.MustCompile(`(?i)\b(?:removed?|deleted?|dropped?)\b.*` + "`" + `(\w+)` + "`")
	renamedPattern  = regexp.MustCompile(`(?i)\b(?:renamed?|replaced?|changed?)\b.*` + "`" + `(\w+)` + "`" + `.*(?:to|with|by).*` + "`" + `(\w+)` + "`")
	requiredPattern = regexp.MustCompile(`(?i)\b(?:now required|is required|must be set)\b.*` + "`" + `(\w+)` + "`")
	resourcePattern = regexp.MustCompile(`(?i)` + "`" + `((?:azurerm|azuread|azapi|aws|google)_\w+)` + "`")

	// Module-specific patterns for AVM and similar Terraform module changelogs

	// variableRemovedPattern matches lines like "variable `foo` removed" or "removed variable `foo`"
	variableRemovedPattern = regexp.MustCompile(`(?i)(?:variable\s+` + "`" + `(\w+)` + "`" + `\s+(?:removed|deleted|dropped)` +
		`|(?:removed?|deleted?|dropped?)\s+(?:the\s+)?variable\s+` + "`" + `(\w+)` + "`" + `)`)

	// variableRenamedPattern matches lines like "variable `old` renamed to `new`" or "renamed variable `old` to `new`"
	variableRenamedPattern = regexp.MustCompile(`(?i)(?:variable\s+` + "`" + `(\w+)` + "`" + `\s+(?:renamed|replaced|changed)\s+(?:to|with|by)\s+` + "`" + `(\w+)` + "`" +
		`|(?:renamed?|replaced?|changed?)\s+(?:the\s+)?variable\s+` + "`" + `(\w+)` + "`" + `\s+(?:to|with|by)\s+` + "`" + `(\w+)` + "`" + `)`)

	// deprecatedPattern matches lines mentioning deprecation of a specific item
	deprecatedPattern = regexp.MustCompile(`(?i)(?:` + "`" + `(\w+)` + "`" + `\s+(?:is\s+)?deprecated|deprecated\s+(?:the\s+)?` + "`" + `(\w+)` + "`" + `)`)

	// requiredAddedPattern matches "required" in the reverse order: "`field` is now required" or "added required `field`"
	requiredAddedPattern = regexp.MustCompile("(?i)(?:" + "`" + `(\w+)` + "`" + `\s+(?:is\s+)?(?:now\s+)?required|added\s+required\s+(?:variable\s+)?` + "`" + `(\w+)` + "`)")

	// providerMigratedPattern matches lines about provider migration (e.g., azurerm to azapi)
	providerMigratedPattern = regexp.MustCompile(`(?i)(?:migrated?\s+(?:from\s+)?(?:the\s+)?(?:azurerm|azapi)\s+(?:provider\s+)?(?:to|provider)|` +
		`(?:azurerm|azapi)\s+(?:to|provider)\s+(?:azurerm|azapi)|` +
		`(?:now\s+)?uses?\s+(?:the\s+)?azapi\s+provider|` +
		`(?:switched|moved|transitioned)\s+(?:to|from)\s+(?:the\s+)?(?:azurerm|azapi)\s+provider)`)
)

// ParseChangelog extracts breaking changes from a markdown changelog.
func ParseChangelog(provider, content string) []BreakingChange {
	var changes []BreakingChange

	// Split by version headers
	sections := splitByVersionHeaders(content)

	for _, section := range sections {
		version := section.version
		text := section.text

		// Only look at sections that mention breaking changes
		if !breakingSectionRe.MatchString(text) {
			continue
		}

		// Parse individual lines for breaking changes
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			if !breakingSectionRe.MatchString(line) {
				continue
			}

			bc := parseLine(provider, version, line)
			if bc != nil {
				changes = append(changes, *bc)
			}
		}
	}

	return changes
}

// ParseModuleChangelog extracts breaking changes from a module changelog with
// broader detection heuristics. Module changelogs (especially AVM) often use
// different conventions than provider changelogs.
func ParseModuleChangelog(source, content string) []BreakingChange {
	var changes []BreakingChange

	sections := splitByVersionHeaders(content)

	for _, section := range sections {
		version := section.version
		text := section.text

		// Use the broader matcher for module changelogs
		if !broadSectionRe.MatchString(text) {
			continue
		}

		lines := strings.Split(text, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			bc := parseModuleLine(source, version, trimmed)
			if bc != nil {
				changes = append(changes, *bc)
			}
		}
	}

	return changes
}

type changelogSection struct {
	version string
	text    string
}

func splitByVersionHeaders(content string) []changelogSection {
	var sections []changelogSection

	matches := versionHeaderRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil
	}

	for i, match := range matches {
		version := content[match[2]:match[3]]
		start := match[1]
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}

		sections = append(sections, changelogSection{
			version: version,
			text:    content[start:end],
		})
	}

	return sections
}

func parseLine(provider, version, line string) *BreakingChange {
	bc := &BreakingChange{
		Provider: provider,
		Version:  version,
		Severity: SeverityBreaking,
	}

	// Extract resource type if mentioned
	if m := resourcePattern.FindStringSubmatch(line); len(m) > 1 {
		bc.ResourceType = m[1]
	}

	// Check for renamed attributes/resources
	if m := renamedPattern.FindStringSubmatch(line); len(m) > 2 {
		bc.Kind = AttributeRenamed
		bc.OldValue = m[1]
		bc.NewValue = m[2]
		bc.Attribute = m[1]
		bc.Description = line
		return bc
	}

	// Check for removed attributes/resources
	if m := removedPattern.FindStringSubmatch(line); len(m) > 1 {
		bc.Kind = AttributeRemoved
		bc.Attribute = m[1]
		bc.Description = line
		return bc
	}

	// Check for newly required attributes
	if m := requiredPattern.FindStringSubmatch(line); len(m) > 1 {
		bc.Kind = RequiredAdded
		bc.Attribute = m[1]
		bc.Description = line
		return bc
	}

	// Generic breaking change
	if breakingSectionRe.MatchString(line) && bc.ResourceType != "" {
		bc.Kind = BehaviorChanged
		bc.Description = strings.TrimSpace(line)
		return bc
	}

	return nil
}

// parseModuleLine handles module-specific changelog patterns. It tries the
// module-specific patterns first, then falls back to the standard parseLine logic.
func parseModuleLine(source, version, line string) *BreakingChange {
	// Check for provider migration (azurerm <-> azapi)
	if providerMigratedPattern.MatchString(line) {
		return &BreakingChange{
			Provider:    source,
			Version:     version,
			Kind:        ProviderMigrated,
			Severity:    SeverityBreaking,
			IsModule:    true,
			Description: strings.TrimSpace(line),
		}
	}

	// Check for variable renamed (module-specific pattern)
	if m := variableRenamedPattern.FindStringSubmatch(line); m != nil {
		oldVal, newVal := firstNonEmpty(m[1], m[3]), firstNonEmpty(m[2], m[4])
		if oldVal != "" && newVal != "" {
			return &BreakingChange{
				Provider:    source,
				Version:     version,
				Kind:        VariableRenamed,
				Severity:    SeverityBreaking,
				IsModule:    true,
				Attribute:   oldVal,
				OldValue:    oldVal,
				NewValue:    newVal,
				Description: strings.TrimSpace(line),
			}
		}
	}

	// Check for variable removed (module-specific pattern)
	if m := variableRemovedPattern.FindStringSubmatch(line); m != nil {
		attr := firstNonEmpty(m[1], m[2])
		if attr != "" {
			return &BreakingChange{
				Provider:    source,
				Version:     version,
				Kind:        VariableRemoved,
				Severity:    SeverityBreaking,
				IsModule:    true,
				Attribute:   attr,
				Description: strings.TrimSpace(line),
			}
		}
	}

	// Check for required variable added (reverse-order pattern)
	if m := requiredAddedPattern.FindStringSubmatch(line); m != nil {
		attr := firstNonEmpty(m[1], m[2])
		if attr != "" {
			return &BreakingChange{
				Provider:    source,
				Version:     version,
				Kind:        RequiredAdded,
				Severity:    SeverityWarning,
				IsModule:    true,
				Attribute:   attr,
				Description: strings.TrimSpace(line),
			}
		}
	}

	// Check for deprecation notices
	if m := deprecatedPattern.FindStringSubmatch(line); m != nil {
		attr := firstNonEmpty(m[1], m[2])
		if attr != "" {
			return &BreakingChange{
				Provider:    source,
				Version:     version,
				Kind:        BehaviorChanged,
				Severity:    SeverityWarning,
				IsModule:    true,
				Attribute:   attr,
				Description: strings.TrimSpace(line),
			}
		}
	}

	// Fall back to standard parseLine for general patterns (renamed, removed, required, resource-based)
	bc := parseLine(source, version, line)
	if bc != nil {
		bc.IsModule = true
		return bc
	}

	return nil
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
