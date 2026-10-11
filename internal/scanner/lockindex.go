package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LockIndex maps each directory holding a .terraform.lock.hcl to the provider
// versions that lock pins.
//
// A repository is normally a set of Terraform roots, each with its own lock file.
// Reading only the lock at the scan root — which is what this package used to do —
// means every root below it silently falls back to constraint satisfaction, so a
// provider pinned at 4.56.0 gets reported as 4.81.0 and real drift disappears.
type LockIndex map[string]map[string]string

// BuildLockIndex finds every lock file at or below root.
func BuildLockIndex(root string, recursive bool) LockIndex {
	idx := LockIndex{}
	if v := parseLockFile(root); len(v) > 0 {
		idx[filepath.Clean(root)] = v
	}
	if !recursive {
		return idx
	}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable corner of the tree is not fatal for a scan
		}
		if d.IsDir() {
			// .terraform holds vendored copies of other modules' lock files;
			// they describe someone else's root, not ours.
			if name := d.Name(); name == ".terraform" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != ".terraform.lock.hcl" {
			return nil
		}
		dir := filepath.Dir(path)
		if v := parseLockFile(dir); len(v) > 0 {
			idx[filepath.Clean(dir)] = v
		}
		return nil
	})
	return idx
}

// Nearest returns the lock that governs startDir: the closest one at or above it,
// never crossing above root. Terraform resolves providers per root directory, so
// the nearest lock is the one that actually describes what is installed.
func (li LockIndex) Nearest(startDir, root string) (string, map[string]string) {
	if len(li) == 0 {
		return "", nil
	}
	root = filepath.Clean(root)
	dir := filepath.Clean(startDir)
	for {
		if v, ok := li[dir]; ok {
			return dir, v
		}
		if dir == root || dir == "." || dir == string(filepath.Separator) {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// LockedProviders turns a lock entry into provider dependencies.
//
// Providers can be pinned in a lock file without ever appearing in a
// required_providers block — a root with a bare `provider "azurerm" {}` is
// perfectly valid Terraform. Those used to be invisible, so such a root reported
// "no dependencies found" while sitting a major version behind.
func LockedProviders(lockDir string, versions map[string]string, known map[string]bool) []ProviderDependency {
	lockPath := filepath.Join(lockDir, ".terraform.lock.hcl")
	constraints := parseLockConstraints(lockDir)

	out := make([]ProviderDependency, 0, len(versions))
	for source, version := range versions {
		key := strings.ToLower(strings.TrimPrefix(source, "registry.terraform.io/"))
		if known[key] || known[strings.ToLower(source)] {
			continue // already declared in required_providers
		}
		ns, name := splitProviderSource(source)
		out = append(out, ProviderDependency{
			Name:          name,
			Namespace:     ns,
			Version:       constraints[key],
			LockedVersion: version,
			FilePath:      lockPath,
			Line:          0,
		})
	}
	return out
}

// splitProviderSource turns "registry.terraform.io/hashicorp/azurerm" or
// "hashicorp/azurerm" into namespace and name.
func splitProviderSource(source string) (namespace, name string) {
	parts := strings.Split(strings.TrimPrefix(source, "registry.terraform.io/"), "/")
	switch len(parts) {
	case 0:
		return "hashicorp", source
	case 1:
		return "hashicorp", parts[0]
	default:
		return parts[len(parts)-2], parts[len(parts)-1]
	}
}

// parseLockConstraints reads the `constraints = "..."` line of each provider block,
// so a lock-only provider still shows the constraint the code asked for.
func parseLockConstraints(dir string) map[string]string {
	content, err := os.ReadFile(filepath.Join(dir, ".terraform.lock.hcl"))
	if err != nil {
		return nil
	}
	out := map[string]string{}
	var current string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "provider ") && strings.HasSuffix(line, "{") {
			start := strings.Index(line, "\"")
			end := strings.LastIndex(line, "\"")
			if start >= 0 && end > start {
				// parseLockFile drops the registry host from its keys; both maps
				// have to agree or the constraint lookup silently misses.
				current = strings.ToLower(strings.TrimPrefix(line[start+1:end], "registry.terraform.io/"))
			}
			continue
		}
		if current != "" && strings.HasPrefix(line, "constraints") {
			if v := strings.Trim(strings.TrimSpace(strings.SplitN(line, "=", 2)[1]), `"`); v != "" {
				out[current] = v
			}
			current = ""
		}
	}
	return out
}
