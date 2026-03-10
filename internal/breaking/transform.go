package breaking

import (
	"fmt"
	"strings"
)

// ApplyTransform applies a deterministic Transform to raw HCL text.
// Returns the transformed text and the number of lines that changed.
func ApplyTransform(rawHCL string, t *Transform) (string, int) {
	if t == nil {
		return rawHCL, 0
	}

	lines := strings.Split(rawHCL, "\n")
	changed := make([]bool, len(lines))

	// 1. Rename resource type on the first line
	if t.RenameResource != "" {
		for i, line := range lines {
			if idx := strings.Index(line, `resource "`); idx != -1 {
				// Find the resource type between the first pair of quotes
				start := idx + len(`resource "`)
				end := strings.Index(line[start:], `"`)
				if end != -1 {
					oldType := line[start : start+end]
					lines[i] = strings.Replace(line, fmt.Sprintf(`resource "%s"`, oldType), fmt.Sprintf(`resource "%s"`, t.RenameResource), 1)
					changed[i] = true
				}
				break
			}
		}
	}

	// 2. Rename attributes
	for oldAttr, newAttr := range t.RenameAttrs {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Match lines that start with the attribute name followed by = or space
			if strings.HasPrefix(trimmed, oldAttr+" ") || strings.HasPrefix(trimmed, oldAttr+"=") {
				lines[i] = strings.Replace(line, oldAttr, newAttr, 1)
				changed[i] = true
			}
		}
	}

	// 3. Remove attributes (comment them out)
	for _, attr := range t.RemoveAttrs {
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, attr+" ") || strings.HasPrefix(trimmed, attr+"=") {
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = indent + "# REMOVED: " + trimmed
				changed[i] = true
			}
			// Also handle block-style attributes (e.g., "sku {")
			if strings.HasPrefix(trimmed, attr+" {") || trimmed == attr+"{" {
				// Comment out this line and all nested lines until the block closes
				indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
				lines[i] = indent + "# REMOVED: " + trimmed
				changed[i] = true
				depth := 0
				for j := i; j < len(lines); j++ {
					for _, ch := range lines[j] {
						if ch == '{' {
							depth++
						} else if ch == '}' {
							depth--
						}
					}
					if j > i {
						innerIndent := lines[j][:len(lines[j])-len(strings.TrimLeft(lines[j], " \t"))]
						lines[j] = innerIndent + "# REMOVED: " + strings.TrimSpace(lines[j])
						changed[j] = true
					}
					if depth == 0 {
						break
					}
				}
			}
		}
	}

	// 4. Add new attributes before the closing brace
	if len(t.AddAttrs) > 0 {
		// Find the last closing brace (the resource block's end)
		lastBrace := -1
		for i := len(lines) - 1; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "}" {
				lastBrace = i
				break
			}
		}

		if lastBrace >= 0 {
			// Determine indentation from surrounding lines
			indent := "  "
			if lastBrace > 0 {
				prevLine := lines[lastBrace-1]
				trimmed := strings.TrimLeft(prevLine, " \t")
				if len(trimmed) > 0 {
					indent = prevLine[:len(prevLine)-len(trimmed)]
				}
			}

			var newLines []string
			for attr, val := range t.AddAttrs {
				newLine := fmt.Sprintf("%s%-*s = %s", indent, 0, attr, val)
				newLines = append(newLines, newLine)
			}

			// Insert before the closing brace
			result := make([]string, 0, len(lines)+len(newLines))
			result = append(result, lines[:lastBrace]...)
			result = append(result, newLines...)
			result = append(result, lines[lastBrace:]...)

			// Update changed tracking
			newChanged := make([]bool, len(result))
			copy(newChanged, changed[:lastBrace])
			for i := 0; i < len(newLines); i++ {
				newChanged[lastBrace+i] = true
			}
			copy(newChanged[lastBrace+len(newLines):], changed[lastBrace:])

			lines = result
			changed = newChanged
		}
	}

	// Count changed lines
	linesChanged := 0
	for _, c := range changed {
		if c {
			linesChanged++
		}
	}

	return strings.Join(lines, "\n"), linesChanged
}
