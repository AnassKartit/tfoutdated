package multirepo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://github.com/user/repo.git", true},
		{"https://github.com/user/repo", true},
		{"http://github.com/user/repo", true},
		{"git@github.com:user/repo.git", true},
		{"ssh://git@github.com/user/repo", true},
		{"some-repo.git", true},
		{"/home/user/repos/my-repo", false},
		{"./local-path", false},
		{"relative-path", false},
	}

	for _, tt := range tests {
		got := isGitURL(tt.input)
		if got != tt.expected {
			t.Errorf("isGitURL(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestRepoName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://github.com/user/my-repo.git", "my-repo"},
		{"https://github.com/user/my-repo", "my-repo"},
		{"git@github.com:user/my-repo.git", "my-repo"},
		{"https://dev.azure.com/org/project/_git/repo-name", "repo-name"},
	}

	for _, tt := range tests {
		got := repoName(tt.input)
		if got != tt.expected {
			t.Errorf("repoName(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveReposLocalPaths(t *testing.T) {
	// Create temp dirs that act as local repos
	tmp := t.TempDir()
	repoA := filepath.Join(tmp, "repo-a")
	repoB := filepath.Join(tmp, "repo-b")
	os.MkdirAll(repoA, 0o755)
	os.MkdirAll(repoB, 0o755)

	// Write repos file
	reposFile := filepath.Join(tmp, "repos.txt")
	content := repoA + "\n" +
		"# this is a comment\n" +
		"\n" +
		repoB + "\n"
	os.WriteFile(reposFile, []byte(content), 0o644)

	entries, err := ResolveRepos(reposFile, tmp)
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Name != "repo-a" {
		t.Errorf("expected name repo-a, got %s", entries[0].Name)
	}
	if entries[0].Cloned {
		t.Error("local path should not be marked as cloned")
	}
	if entries[1].Name != "repo-b" {
		t.Errorf("expected name repo-b, got %s", entries[1].Name)
	}
}

func TestResolveReposSkipsBadPaths(t *testing.T) {
	tmp := t.TempDir()

	reposFile := filepath.Join(tmp, "repos.txt")
	content := "/nonexistent/path/that/does/not/exist\n"
	os.WriteFile(reposFile, []byte(content), 0o644)

	entries, err := ResolveRepos(reposFile, tmp)
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for bad paths, got %d", len(entries))
	}
}

func TestResolveReposMissingFile(t *testing.T) {
	_, err := ResolveRepos("/nonexistent/repos.txt", "/tmp")
	if err == nil {
		t.Error("expected error for missing repos file")
	}
}

func TestCleanupCloned(t *testing.T) {
	tmp := t.TempDir()
	clonedDir := filepath.Join(tmp, "cloned-repo")
	os.MkdirAll(clonedDir, 0o755)
	os.WriteFile(filepath.Join(clonedDir, "test.txt"), []byte("test"), 0o644)

	localDir := filepath.Join(tmp, "local-repo")
	os.MkdirAll(localDir, 0o755)

	entries := []RepoEntry{
		{LocalPath: clonedDir, Cloned: true},
		{LocalPath: localDir, Cloned: false},
	}

	CleanupCloned(entries)

	if _, err := os.Stat(clonedDir); !os.IsNotExist(err) {
		t.Error("expected cloned dir to be removed")
	}
	if _, err := os.Stat(localDir); err != nil {
		t.Error("expected local dir to be preserved")
	}
}
