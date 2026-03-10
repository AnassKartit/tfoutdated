package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/anasskartit/tfoutdated/internal/config"
)

// walkTerraformFiles finds all .tf files under the given path.
func walkTerraformFiles(root string, recursive bool, ignores []config.IgnoreRule) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			name := info.Name()

			// Skip hidden directories and .terraform
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if name == ".terraform" {
				return filepath.SkipDir
			}

			// Skip non-recursive subdirectories
			if !recursive && path != root {
				return filepath.SkipDir
			}

			// Check ignore rules
			relPath, _ := filepath.Rel(root, path)
			for _, rule := range ignores {
				if rule.Path != "" {
					matched, _ := filepath.Match(rule.Path, relPath)
					if matched {
						return filepath.SkipDir
					}
					// Also check with glob-style matching
					if strings.HasSuffix(rule.Path, "/**") {
						prefix := strings.TrimSuffix(rule.Path, "/**")
						if strings.HasPrefix(relPath, prefix) {
							return filepath.SkipDir
						}
					}
				}
			}

			return nil
		}

		// Only process .tf files
		if !strings.HasSuffix(path, ".tf") {
			return nil
		}

		// Skip override files
		if strings.HasSuffix(path, "_override.tf") || path == "override.tf" {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}
