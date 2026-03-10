package fixer

import (
	"strings"
)

// replaceVersionInLine replaces a version constraint in a line of HCL.
// It handles various constraint formats:
//   - version = "~> 3.75.0"
//   - version = ">= 3.0, < 4.0"
//   - version = "3.75.0"
func replaceVersionInLine(line, oldVersion, newVersion string) string {
	// Find the version string in the line
	// Look for the old version number and replace it with the new one
	if strings.Contains(line, oldVersion) {
		return strings.Replace(line, oldVersion, newVersion, 1)
	}

	// Try replacing with constraint prefix handling
	// e.g., "~> 3.75.0" → "~> 3.116.0"
	constraintPrefixes := []string{"~> ", ">= ", "<= ", "!= ", "> ", "< ", "= "}
	for _, prefix := range constraintPrefixes {
		old := prefix + oldVersion
		if strings.Contains(line, old) {
			return strings.Replace(line, old, prefix+newVersion, 1)
		}
	}

	return line
}

// buildConstraint creates a version constraint string from a target version.
func buildConstraint(constraintType, version string) string {
	switch constraintType {
	case "pessimistic":
		return "~> " + version
	case "exact":
		return version
	case "minimum":
		return ">= " + version
	default:
		return "~> " + version
	}
}
