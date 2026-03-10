package analyzer

// RecommendAlignment suggests a single version to align all uses of a dependency.
func RecommendAlignment(issues []AlignmentIssue) map[string]string {
	recommendations := make(map[string]string)

	for _, issue := range issues {
		// Recommend the highest version currently in use
		var highest string
		for ver := range issue.Versions {
			if highest == "" || ver > highest {
				highest = ver
			}
		}
		recommendations[issue.Name] = highest
	}

	return recommendations
}
