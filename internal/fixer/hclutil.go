package fixer

import (
	"strings"
	"unicode"
)

// findAttributeExtent returns the inclusive line range [start, end] for an
// attribute starting at lines[start]. Handles single-line values, brace blocks,
// bracket lists, and heredocs.
func findAttributeExtent(lines []string, start int) (int, int) {
	if start < 0 || start >= len(lines) {
		return start, start
	}

	line := lines[start]

	// Check for heredoc (<<MARKER or <<-MARKER)
	if marker := extractHeredocMarker(line); marker != "" {
		for i := start + 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == marker {
				return start, i
			}
		}
		// If closing marker not found, return just the start line
		return start, start
	}

	// Count braces and brackets on the start line (string-aware)
	braceDepth, bracketDepth := countDelimiters(line)

	// If balanced, it's a single-line value
	if braceDepth == 0 && bracketDepth == 0 {
		return start, start
	}

	// Scan forward until balanced
	for i := start + 1; i < len(lines); i++ {
		bd, bkd := countDelimiters(lines[i])
		braceDepth += bd
		bracketDepth += bkd
		if braceDepth <= 0 && bracketDepth <= 0 {
			return start, i
		}
	}

	// If never balanced, return to end
	return start, len(lines) - 1
}

// countDelimiters returns net brace and bracket depth for a line,
// ignoring delimiters inside string literals.
func countDelimiters(line string) (braceDepth, bracketDepth int) {
	inString := false
	escaped := false
	commentReached := false

	for _, ch := range line {
		if commentReached {
			break
		}

		if escaped {
			escaped = false
			continue
		}

		if ch == '\\' && inString {
			escaped = true
			continue
		}

		if ch == '"' {
			inString = !inString
			continue
		}

		if inString {
			continue
		}

		// Outside string: check for comment
		if ch == '#' {
			commentReached = true
			continue
		}

		switch ch {
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}
	}

	return braceDepth, bracketDepth
}

// extractHeredocMarker extracts the heredoc delimiter from a line containing <<MARKER or <<-MARKER.
// Returns empty string if no heredoc is found.
func extractHeredocMarker(line string) string {
	// Look for << after the = sign (attribute value context)
	idx := strings.Index(line, "<<")
	if idx < 0 {
		return ""
	}

	rest := line[idx+2:]
	// Skip optional - for indented heredocs
	if len(rest) > 0 && rest[0] == '-' {
		rest = rest[1:]
	}

	// Trim leading/trailing whitespace
	rest = strings.TrimSpace(rest)

	if isValidHeredocDelimiter(rest) {
		return rest
	}

	return ""
}

// isValidHeredocDelimiter checks if s is a valid heredoc delimiter.
// A valid delimiter is a non-empty string consisting of letters, digits, and underscores,
// starting with a letter or underscore.
func isValidHeredocDelimiter(s string) bool {
	if len(s) == 0 {
		return false
	}

	for i, ch := range s {
		if i == 0 {
			if !unicode.IsLetter(ch) && ch != '_' {
				return false
			}
		} else {
			if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
				return false
			}
		}
	}

	return true
}
