package fixer

import (
	"os"
	"regexp"
	"strings"
)

// IsTerragruntFile returns true if the path is a terragrunt.hcl file.
func IsTerragruntFile(path string) bool {
	return strings.HasSuffix(path, "terragrunt.hcl")
}

// applyTerragruntChanges rewrites version strings in a terragrunt.hcl file.
// Handles both tfr:/// and git:: source formats.
func applyTerragruntChanges(filePath string, changes []Change) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	content := string(data)
	result := content

	for _, c := range changes {
		// Replace ?version=OLD with ?version=NEW in tfr:/// URLs
		result = strings.Replace(result, "?version="+c.OldVersion, "?version="+c.NewVersion, 1)

		// Replace ?ref=vOLD with ?ref=vNEW in git:: URLs
		result = strings.Replace(result, "?ref=v"+c.OldVersion, "?ref=v"+c.NewVersion, 1)
		result = strings.Replace(result, "?ref="+c.OldVersion, "?ref="+c.NewVersion, 1)
	}

	if result == content {
		// Try regex-based replacement as fallback
		for _, c := range changes {
			re := regexp.MustCompile(`(\?version=)` + regexp.QuoteMeta(c.OldVersion))
			result = re.ReplaceAllString(result, "${1}"+c.NewVersion)

			re = regexp.MustCompile(`(\?ref=v?)` + regexp.QuoteMeta(c.OldVersion))
			result = re.ReplaceAllString(result, "${1}"+c.NewVersion)
		}
	}

	if result == content {
		return nil
	}

	return os.WriteFile(filePath, []byte(result), 0644)
}
