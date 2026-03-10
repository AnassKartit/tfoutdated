package schemadiff

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
)

// renameCandidate holds a scored (old, new) pair for rename detection.
type renameCandidate struct {
	oldName string
	newName string
	score   float64
}

// detectRenames uses multi-signal bipartite matching to find likely renames
// among removed and added variables. Returns a map from old name to new name.
func detectRenames(removed, added map[string]*tfconfig.Variable, oldMod, newMod *tfconfig.Module) map[string]string {
	const threshold = 0.45

	var candidates []renameCandidate
	for oldName, oldVar := range removed {
		for newName, newVar := range added {
			score := computeVariableScore(oldName, newName, oldVar, newVar)
			if score >= threshold {
				candidates = append(candidates, renameCandidate{
					oldName: oldName,
					newName: newName,
					score:   score,
				})
			}
		}
	}

	// Sort descending by score, break ties by old name then new name
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].oldName != candidates[j].oldName {
			return candidates[i].oldName < candidates[j].oldName
		}
		return candidates[i].newName < candidates[j].newName
	})

	// Greedy 1:1 assignment
	result := make(map[string]string)
	usedOld := make(map[string]bool)
	usedNew := make(map[string]bool)
	for _, c := range candidates {
		if usedOld[c.oldName] || usedNew[c.newName] {
			continue
		}
		result[c.oldName] = c.newName
		usedOld[c.oldName] = true
		usedNew[c.newName] = true
	}

	return result
}

// computeVariableScore calculates the weighted similarity score between two variables.
func computeVariableScore(oldName, newName string, oldVar, newVar *tfconfig.Variable) float64 {
	score := 0.0
	score += 0.40 * nameSimilarity(oldName, newName)
	score += 0.20 * descriptionSimilarity(oldVar.Description, newVar.Description)
	score += 0.15 * typeCompatibility(oldVar.Type, newVar.Type)
	score += 0.10 * defaultSimilarity(oldVar.Default, newVar.Default)
	score += 0.10 * requiredMatch(oldVar.Required, newVar.Required)
	score += 0.05 * sensitiveMatch(oldVar.Sensitive, newVar.Sensitive)
	return score
}

// outputRenameCandidate holds a scored (old, new) pair for output rename detection.
type outputRenameCandidate struct {
	oldName string
	newName string
	score   float64
}

// detectOutputRenames uses multi-signal matching to find likely renames
// among removed and added outputs. Returns a map from old name to new name.
func detectOutputRenames(removed, added map[string]*tfconfig.Output) map[string]string {
	const threshold = 0.45

	var candidates []outputRenameCandidate
	for oldName, oldOut := range removed {
		for newName, newOut := range added {
			score := computeOutputScore(oldName, newName, oldOut, newOut)
			if score >= threshold {
				candidates = append(candidates, outputRenameCandidate{
					oldName: oldName,
					newName: newName,
					score:   score,
				})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].oldName != candidates[j].oldName {
			return candidates[i].oldName < candidates[j].oldName
		}
		return candidates[i].newName < candidates[j].newName
	})

	result := make(map[string]string)
	usedOld := make(map[string]bool)
	usedNew := make(map[string]bool)
	for _, c := range candidates {
		if usedOld[c.oldName] || usedNew[c.newName] {
			continue
		}
		result[c.oldName] = c.newName
		usedOld[c.oldName] = true
		usedNew[c.newName] = true
	}

	return result
}

// computeOutputScore calculates the weighted similarity score between two outputs.
func computeOutputScore(oldName, newName string, oldOut, newOut *tfconfig.Output) float64 {
	score := 0.0
	score += 0.50 * nameSimilarity(oldName, newName)
	score += 0.25 * descriptionSimilarity(oldOut.Description, newOut.Description)
	score += 0.15 * typeCompatibility(oldOut.Type, newOut.Type)
	score += 0.10 * sensitiveMatch(oldOut.Sensitive, newOut.Sensitive)
	return score
}

// --- Signal functions ---

// nameSimilarity combines token Jaccard, LCS ratio, and normalized Levenshtein.
func nameSimilarity(a, b string) float64 {
	a = strings.ToLower(a)
	b = strings.ToLower(b)
	if a == b {
		return 1.0
	}

	tokensA := tokenize(a)
	tokensB := tokenize(b)

	// Token Jaccard
	jaccard := tokenJaccard(tokensA, tokensB)

	// LCS ratio
	lcsLen := lcs(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	lcsRatio := 0.0
	if maxLen > 0 {
		lcsRatio = float64(lcsLen) / float64(maxLen)
	}

	// Normalized Levenshtein similarity
	levDist := levenshtein(a, b)
	levSim := 0.0
	if maxLen > 0 {
		levSim = 1.0 - float64(levDist)/float64(maxLen)
	}

	// Weighted combination
	return 0.4*jaccard + 0.3*lcsRatio + 0.3*levSim
}

// descriptionSimilarity returns word Jaccard after stop-word removal.
// Returns 0.5 if either description is empty.
func descriptionSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0.5
	}
	if a == b {
		return 1.0
	}

	wordsA := removeStopWords(strings.Fields(strings.ToLower(a)))
	wordsB := removeStopWords(strings.Fields(strings.ToLower(b)))

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.5
	}

	return tokenJaccard(wordsA, wordsB)
}

// typeCompatibility scores type similarity between two Terraform type strings.
func typeCompatibility(a, b string) float64 {
	a = normalizeType(a)
	b = normalizeType(b)

	if a == b && a != "" {
		return 1.0
	}
	if a == "" || b == "" {
		return 0.5
	}
	if sameTypeFamily(a, b) {
		return 0.6
	}
	return 0.0
}

// defaultSimilarity compares two default values.
func defaultSimilarity(a, b interface{}) float64 {
	if a == nil && b == nil {
		return 1.0
	}
	if a == nil || b == nil {
		return 0.3
	}

	// Compare by JSON representation for deep equality
	jsonA, errA := json.Marshal(a)
	jsonB, errB := json.Marshal(b)
	if errA == nil && errB == nil && string(jsonA) == string(jsonB) {
		return 1.0
	}

	// Same Go type
	if fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b) {
		return 0.5
	}

	return 0.0
}

// requiredMatch returns 1.0 if same, 0.2 if different.
func requiredMatch(a, b bool) float64 {
	if a == b {
		return 1.0
	}
	return 0.2
}

// sensitiveMatch returns 1.0 if same, 0.0 if different.
func sensitiveMatch(a, b bool) float64 {
	if a == b {
		return 1.0
	}
	return 0.0
}

// --- Helper functions ---

// tokenize splits a name on underscores into lowercase tokens.
func tokenize(s string) []string {
	parts := strings.Split(s, "_")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, strings.ToLower(p))
		}
	}
	return result
}

// tokenJaccard computes the Jaccard similarity between two token slices.
func tokenJaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	setA := make(map[string]bool, len(a))
	for _, t := range a {
		setA[t] = true
	}
	setB := make(map[string]bool, len(b))
	for _, t := range b {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	union := len(setA)
	for t := range setB {
		if !setA[t] {
			union++
		}
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// lcs returns the length of the longest common subsequence of two strings.
func lcs(a, b string) int {
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return 0
	}

	// Use two rows to save memory
	prev := make([]int, n+1)
	curr := make([]int, n+1)

	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else {
				curr[j] = curr[j-1]
				if prev[j] > curr[j] {
					curr[j] = prev[j]
				}
			}
		}
		prev, curr = curr, make([]int, n+1)
	}
	return prev[n]
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			curr[j] = ins
			if del < curr[j] {
				curr[j] = del
			}
			if sub < curr[j] {
				curr[j] = sub
			}
		}
		prev = curr
	}

	return prev[lb]
}

// stopWords is a set of common English stop words to filter from descriptions.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"of": true, "in": true, "to": true, "for": true, "with": true,
	"on": true, "at": true, "by": true, "from": true, "as": true,
	"into": true, "through": true, "during": true, "before": true,
	"after": true, "above": true, "below": true, "between": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true,
	"so": true, "yet": true, "both": true, "either": true, "neither": true,
	"this": true, "that": true, "these": true, "those": true,
	"it": true, "its": true, "if": true, "then": true, "else": true,
}

// removeStopWords filters common English stop words from a word slice.
func removeStopWords(words []string) []string {
	var result []string
	for _, w := range words {
		// Strip common punctuation
		w = strings.Trim(w, ".,;:!?()[]{}\"'")
		if w != "" && !stopWords[w] {
			result = append(result, w)
		}
	}
	return result
}

// normalizeType lowercases and trims whitespace from a Terraform type string.
func normalizeType(t string) string {
	return strings.TrimSpace(strings.ToLower(t))
}

// typeFamily maps base Terraform types to family groups.
var typeFamilies = map[string]string{
	"string": "scalar",
	"number": "scalar",
	"bool":   "scalar",
	"list":   "collection",
	"set":    "collection",
	"tuple":  "collection",
	"map":    "mapping",
	"object": "mapping",
}

// sameTypeFamily returns true if two types belong to the same family.
func sameTypeFamily(a, b string) bool {
	// Extract base type (e.g., "list(string)" -> "list")
	baseA := extractBaseType(a)
	baseB := extractBaseType(b)

	famA, okA := typeFamilies[baseA]
	famB, okB := typeFamilies[baseB]

	return okA && okB && famA == famB
}

// extractBaseType extracts the base type from a potentially parameterized type.
func extractBaseType(t string) string {
	t = normalizeType(t)
	if idx := strings.Index(t, "("); idx >= 0 {
		return t[:idx]
	}
	return t
}
