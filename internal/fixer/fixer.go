package fixer

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/analyzer"
	"github.com/anasskartit/tfoutdated/internal/breaking"
	"github.com/anasskartit/tfoutdated/internal/resolver"
	"github.com/anasskartit/tfoutdated/internal/scanner"
)

// Change describes a version update applied to a file.
type Change struct {
	FilePath   string
	Name       string
	OldVersion string
	NewVersion string
	Line       int
}

// Options configures the fixer.
type Options struct {
	DryRun   bool
	SafeOnly bool // use NonBreakingTarget from upgrade paths
}

// Fixer applies safe version upgrades to .tf files.
type Fixer struct {
	opts Options
}

// New creates a new Fixer.
func New(opts Options) *Fixer {
	return &Fixer{opts: opts}
}

// Fix applies non-breaking upgrades and returns the list of changes.
func (f *Fixer) Fix(analysis *analyzer.Analysis) ([]Change, error) {
	var changes []Change

	// Group changes by file for efficient editing
	byFile := make(map[string][]Change)

	// Build a map of safe targets from upgrade paths
	safeTargets := make(map[string]string) // dep name → non-breaking target version
	for _, up := range analysis.UpgradePaths {
		if up.NonBreakingTarget != "" {
			safeTargets[up.Name] = up.NonBreakingTarget
		}
	}

	// Check if any modules are being upgraded — if so, skip standalone provider bumps
	// because FixProviderConstraints will set the correct constraint based on module deps
	hasModuleUpgrades := false
	for _, dep := range analysis.Dependencies {
		if dep.IsModule && dep.CurrentVer != dep.LatestVer {
			hasModuleUpgrades = true
			break
		}
	}

	for _, dep := range analysis.Dependencies {
		// Skip standalone provider upgrades when modules are being upgraded —
		// FixProviderConstraints will determine the correct constraint
		if !dep.IsModule && hasModuleUpgrades {
			continue
		}

		// Determine the target version
		targetVer := dep.LatestVer

		if f.opts.SafeOnly && dep.UpdateType == resolver.UpdateMajor {
			// In safe mode, use the non-breaking target from upgrade paths
			if safeTarget, ok := safeTargets[dep.Name]; ok && safeTarget != dep.CurrentVer {
				targetVer = safeTarget
			} else {
				continue // no safe target available
			}
		} else if f.opts.SafeOnly && hasBreakingChanges(analysis.BreakingChanges, dep) {
			// For minor/patch with breaking changes, use safe target or skip
			if safeTarget, ok := safeTargets[dep.Name]; ok && safeTarget != dep.CurrentVer {
				targetVer = safeTarget
			} else {
				continue // breaking changes detected, no safe target
			}
		}

		if targetVer == dep.CurrentVer {
			continue
		}

		c := Change{
			FilePath:   dep.FilePath,
			Name:       dep.Name,
			OldVersion: dep.CurrentVer,
			NewVersion: targetVer,
			Line:       dep.Line,
		}

		changes = append(changes, c)
		byFile[dep.FilePath] = append(byFile[dep.FilePath], c)
	}

	if f.opts.DryRun || len(changes) == 0 {
		return changes, nil
	}

	// Apply changes file by file
	for filePath, fileChanges := range byFile {
		if IsCdktfFile(filePath) {
			if strings.HasSuffix(filePath, "cdktf.json") {
				if err := applyCdktfChanges(filePath, fileChanges); err != nil {
					return changes, fmt.Errorf("applying cdktf changes to %s: %w", filePath, err)
				}
			} else if strings.HasSuffix(filePath, "package.json") {
				if err := applyPackageJSONChanges(filePath, fileChanges); err != nil {
					return changes, fmt.Errorf("applying package.json changes to %s: %w", filePath, err)
				}
			}
			continue
		}
		if IsTerragruntFile(filePath) {
			if err := applyTerragruntChanges(filePath, fileChanges); err != nil {
				return changes, fmt.Errorf("applying terragrunt changes to %s: %w", filePath, err)
			}
			continue
		}
		if err := applyChanges(filePath, fileChanges); err != nil {
			return changes, fmt.Errorf("applying changes to %s: %w", filePath, err)
		}
	}

	return changes, nil
}

func hasBreakingChanges(bcs []breaking.BreakingChange, dep analyzer.DependencyAnalysis) bool {
	for _, bc := range bcs {
		if bc.Provider == dep.Name || bc.Provider == dep.Source {
			return true
		}
	}
	return false
}

func applyChanges(filePath string, changes []Change) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	result, err := rewriteVersions(string(content), changes)
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, []byte(result), 0644)
}

// rewriteVersions performs token-based version replacement in HCL content.
func rewriteVersions(content string, changes []Change) (string, error) {
	// Sort changes by line number in reverse order to maintain accurate positions
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Line > changes[j].Line
	})

	lines := splitLines(content)

	for _, change := range changes {
		if change.Line <= 0 || change.Line > len(lines) {
			continue
		}

		// The reported line is the block start (e.g., `module "x" {`).
		// The version attribute may be on a subsequent line within the block.
		// Search up to 10 lines ahead for the version string.
		replaced := false
		for offset := 0; offset < 10 && change.Line-1+offset < len(lines); offset++ {
			idx := change.Line - 1 + offset
			updated := replaceVersionInLine(lines[idx], change.OldVersion, change.NewVersion)
			if updated != lines[idx] {
				lines[idx] = updated
				replaced = true
				break
			}
		}
		if !replaced {
			// Fallback: try exact line
			lines[change.Line-1] = replaceVersionInLine(lines[change.Line-1], change.OldVersion, change.NewVersion)
		}
	}

	return joinLines(lines), nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for _, l := range lines {
		result += l
	}
	return result
}

// FixModuleCalls applies breaking change transforms (variable renames/removals) to module calls.
func (f *Fixer) FixModuleCalls(analysis *analyzer.Analysis, scan *scanner.ScanResult, upgradedModules []Change) ([]ModuleChange, error) {
	var changes []ModuleChange

	if scan == nil {
		return nil, nil
	}

	// Build set of modules that were actually version-bumped
	upgraded := make(map[string]bool) // dep.Name → true
	for _, c := range upgradedModules {
		upgraded[c.Name] = true
	}

	for _, bc := range analysis.BreakingChanges {
		if bc.Transform == nil || !bc.IsModule {
			continue
		}

		// Find module blocks that use this module source
		for _, dep := range analysis.Dependencies {
			if !dep.IsModule || dep.Source != bc.Provider {
				continue
			}

			// Only apply transforms for modules that were actually upgraded
			if !upgraded[dep.Name] {
				continue
			}

			// Find the raw module block in scan results
			// Module blocks aren't in Resources, we need to find them in the .tf files
			if len(bc.Transform.RenameAttrs) > 0 || len(bc.Transform.RemoveAttrs) > 0 {
				applied, err := applyModuleTransform(dep.FilePath, dep.Line, dep.Name, bc.Transform, f.opts.DryRun)
				if err != nil {
					continue
				}
				if applied {
					for old, new := range bc.Transform.RenameAttrs {
						changes = append(changes, ModuleChange{
							FilePath: dep.FilePath,
							Module:   dep.Name,
							Type:     "rename",
							Old:      old,
							New:      new,
							Line:     dep.Line,
						})
					}
					for _, attr := range bc.Transform.RemoveAttrs {
						changes = append(changes, ModuleChange{
							FilePath: dep.FilePath,
							Module:   dep.Name,
							Type:     "remove",
							Old:      attr,
							Line:     dep.Line,
						})
					}
				}
			}
		}
	}

	return changes, nil
}

// ModuleChange describes a transform applied to a module call.
type ModuleChange struct {
	FilePath string
	Module   string
	Type     string // "rename" or "remove"
	Old      string
	New      string
	Line     int
}

// applyModuleTransform reads a .tf file, finds the module block starting at the given line,
// and applies attribute renames/removals.
func applyModuleTransform(filePath string, blockLine int, moduleName string, transform *breaking.Transform, dryRun bool) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	lines := splitLines(string(content))
	if blockLine <= 0 || blockLine > len(lines) {
		return false, nil
	}

	changed := false

	// Find the module block boundaries (brace counting)
	depth := 0
	blockStart := blockLine - 1
	blockEnd := len(lines) - 1
	foundOpen := false

	for i := blockStart; i < len(lines); i++ {
		line := lines[i]
		for _, ch := range line {
			if ch == '{' {
				depth++
				foundOpen = true
			}
			if ch == '}' {
				depth--
				if foundOpen && depth == 0 {
					blockEnd = i
					goto done
				}
			}
		}
	}
done:

	// Apply renames within the block
	for old, new := range transform.RenameAttrs {
		for i := blockStart; i <= blockEnd; i++ {
			trimmed := strings.TrimSpace(lines[i])
			// Match "old_attr = ..." or "old_attr=" patterns
			if strings.HasPrefix(trimmed, old+" ") || strings.HasPrefix(trimmed, old+"=") || strings.HasPrefix(trimmed, old+"\t") {
				// Preserve indentation
				indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
				rest := trimmed[len(old):]
				// Apply value hint if available
				if transform.ValueHints != nil {
					if hint, ok := transform.ValueHints[old]; ok {
						rest = applyValueHint(rest, hint)
					}
				}
				lines[i] = indent + new + rest
				if !strings.HasSuffix(lines[i], "\n") && i < len(lines)-1 {
					lines[i] += "\n"
				}
				changed = true
				break
			}
		}
	}

	// Apply removals within the block
	for _, attr := range transform.RemoveAttrs {
		for i := blockStart; i <= blockEnd; i++ {
			trimmed := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trimmed, attr+" ") || strings.HasPrefix(trimmed, attr+"=") || strings.HasPrefix(trimmed, attr+"\t") {
				// Determine the full extent of the attribute value (may span multiple lines)
				_, attrEnd := findAttributeExtent(lines, i)
				if attrEnd > blockEnd {
					attrEnd = blockEnd
				}
				// Remove lines[i..attrEnd] inclusive
				lines = append(lines[:i], lines[attrEnd+1:]...)
				removed := attrEnd - i + 1
				blockEnd -= removed
				changed = true
				break
			}
		}
	}

	if !changed || dryRun {
		return changed, nil
	}

	return true, os.WriteFile(filePath, []byte(joinLines(lines)), 0644)
}

// applyValueHint rewrites the value portion of an attribute line based on a ValueHint.
// rest is the portion of the line after the attribute name (e.g., " = var.foo.name\n").
func applyValueHint(rest string, hint breaking.ValueHint) string {
	switch hint.Confidence {
	case breaking.ValueHintAuto:
		// If the value expression ends with hint.OldSuffix, replace with hint.NewSuffix
		trimmed := strings.TrimRight(rest, " \t\n")
		if strings.HasSuffix(trimmed, hint.OldSuffix) {
			suffix := rest[len(trimmed):]
			newVal := trimmed[:len(trimmed)-len(hint.OldSuffix)] + hint.NewSuffix
			return newVal + suffix
		}
		return rest
	case breaking.ValueHintSuggest:
		// Append a TODO comment
		trimmed := strings.TrimRight(rest, " \t\n")
		suffix := rest[len(trimmed):]
		// Only add comment if there isn't one already
		if !strings.Contains(trimmed, "# TODO(tfoutdated)") {
			return trimmed + " # TODO(tfoutdated): value may need update" + suffix
		}
		return rest
	default:
		return rest
	}
}
