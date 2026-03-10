package multirepo

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoEntry represents a repo to scan — either a local path or a git URL.
type RepoEntry struct {
	URL       string
	LocalPath string
	Name      string
	Cloned    bool
}

// ResolveRepos takes a repos file path and returns resolved RepoEntry list.
func ResolveRepos(reposFile string, tempDir string) ([]RepoEntry, error) {
	f, err := os.Open(reposFile)
	if err != nil {
		return nil, fmt.Errorf("open repos file: %w", err)
	}
	defer f.Close()

	var entries []RepoEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry, err := resolveEntry(line, tempDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tfoutdated: skipping %s: %v\n", line, err)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

func resolveEntry(line string, tempDir string) (RepoEntry, error) {
	if isGitURL(line) {
		name := repoName(line)
		cloneDir := filepath.Join(tempDir, name)

		fmt.Fprintf(os.Stderr, "tfoutdated: cloning %s ...\n", line)
		cmd := exec.Command("git", "clone", "--depth=1", "--single-branch", line, cloneDir)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return RepoEntry{}, fmt.Errorf("git clone failed: %w", err)
		}

		return RepoEntry{
			URL:       line,
			LocalPath: cloneDir,
			Name:      name,
			Cloned:    true,
		}, nil
	}

	abs, err := filepath.Abs(line)
	if err != nil {
		return RepoEntry{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return RepoEntry{}, fmt.Errorf("path not found: %s", abs)
	}
	if !info.IsDir() {
		return RepoEntry{}, fmt.Errorf("not a directory: %s", abs)
	}

	return RepoEntry{
		URL:       line,
		LocalPath: abs,
		Name:      filepath.Base(abs),
	}, nil
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasSuffix(s, ".git")
}

func repoName(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	name := parts[len(parts)-1]
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		after := name[idx+1:]
		subparts := strings.Split(after, "/")
		name = subparts[len(subparts)-1]
	}
	return name
}

// CleanupCloned removes temp directories for cloned repos.
func CleanupCloned(entries []RepoEntry) {
	for _, e := range entries {
		if e.Cloned {
			os.RemoveAll(e.LocalPath)
		}
	}
}
