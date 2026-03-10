package schemadiff

import (
	"strings"

	"github.com/anasskartit/tfoutdated/internal/breaking"
)

// InferValueHint analyzes old and new variable names/descriptions to infer
// whether the value expression should be rewritten (e.g., .name → .id).
// It derives everything dynamically from the variable names — no hardcoded mappings.
func InferValueHint(oldName string, oldDesc string, newName string, newDesc string) breaking.ValueHint {
	// Extract the last underscore-delimited segment from each variable name.
	// e.g., "resource_group_name" → "name", "parent_id" → "id"
	oldStem := lastSegment(oldName)
	newStem := lastSegment(newName)

	if oldStem == "" || newStem == "" || oldStem == newStem {
		return breaking.ValueHint{Confidence: breaking.ValueHintNone}
	}

	// The value hint: if the HCL expression ends with .{oldStem},
	// it likely needs to change to .{newStem}.
	// e.g., resource_group_name → parent_id means .name → .id
	oldAccessor := "." + oldStem
	newAccessor := "." + newStem

	// Score based on description analysis
	score := 0.6 // base: the suffix changed
	score += analyzeDescriptionDelta(oldDesc, newDesc, oldStem, newStem)

	var confidence breaking.ValueHintConfidence
	var reason string

	switch {
	case score >= 0.8:
		confidence = breaking.ValueHintAuto
		reason = "variable semantics changed from " + oldStem + " to " + newStem
	case score >= 0.5:
		confidence = breaking.ValueHintSuggest
		reason = "variable suffix changed from " + oldStem + " to " + newStem
	default:
		return breaking.ValueHint{Confidence: breaking.ValueHintNone}
	}

	return breaking.ValueHint{
		Confidence: confidence,
		OldSuffix:  oldAccessor,
		NewSuffix:  newAccessor,
		Reason:     reason,
	}
}

// lastSegment returns the last underscore-delimited part of a variable name.
// "resource_group_name" → "name", "parent_id" → "id", "location" → "location"
func lastSegment(name string) string {
	idx := strings.LastIndex(name, "_")
	if idx < 0 || idx == len(name)-1 {
		return name
	}
	return name[idx+1:]
}

// analyzeDescriptionDelta checks if descriptions confirm or contradict the semantic change.
// Returns +0.3 if confirmed, -0.2 if contradicted, 0 if inconclusive.
func analyzeDescriptionDelta(oldDesc, newDesc, oldStem, newStem string) float64 {
	if oldDesc == "" || newDesc == "" {
		return 0
	}

	oldLower := strings.ToLower(oldDesc)
	newLower := strings.ToLower(newDesc)

	oldConfirms := strings.Contains(oldLower, oldStem+" of") || strings.Contains(oldLower, "the "+oldStem)
	newConfirms := strings.Contains(newLower, newStem+" of") || strings.Contains(newLower, "the "+newStem)

	if oldConfirms && newConfirms {
		return 0.3
	}

	// Contradiction: old desc mentions new stem, or new desc mentions old stem
	oldContradicts := strings.Contains(oldLower, newStem+" of") || strings.Contains(oldLower, "the "+newStem)
	newContradicts := strings.Contains(newLower, oldStem+" of") || strings.Contains(newLower, "the "+oldStem)

	if oldContradicts || newContradicts {
		return -0.2
	}

	return 0
}
