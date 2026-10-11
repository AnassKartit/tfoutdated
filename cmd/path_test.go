package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A positional argument must win over the --path default. Before this was wired up,
// cobra accepted the argument and dropped it, so the command silently ran against
// the working directory instead of the directory the user named.
func TestResolveTargetPath(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "infra")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "main.tf")
	if err := os.WriteFile(file, []byte("# empty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := flagPath
	t.Cleanup(func() { flagPath = restore })

	t.Run("argument wins over default flag", func(t *testing.T) {
		flagPath = "."
		got, err := resolveTargetPath([]string{sub})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != sub {
			t.Errorf("got %q, want %q", got, sub)
		}
	})

	t.Run("flag used when no argument", func(t *testing.T) {
		flagPath = sub
		got, err := resolveTargetPath(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != sub {
			t.Errorf("got %q, want %q", got, sub)
		}
	})

	t.Run("conflicting argument and flag are rejected", func(t *testing.T) {
		flagPath = dir
		_, err := resolveTargetPath([]string{sub})
		if err == nil || !strings.Contains(err.Error(), "conflicting paths") {
			t.Errorf("want conflicting-paths error, got %v", err)
		}
	})

	t.Run("missing path is an error, not a silent fallback", func(t *testing.T) {
		flagPath = "."
		_, err := resolveTargetPath([]string{filepath.Join(dir, "nope")})
		if err == nil {
			t.Fatal("want error for a path that does not exist")
		}
	})

	t.Run("a file is rejected with a directory hint", func(t *testing.T) {
		flagPath = "."
		_, err := resolveTargetPath([]string{file})
		if err == nil || !strings.Contains(err.Error(), "directory") {
			t.Errorf("want directory hint, got %v", err)
		}
	})

	t.Run("comma-separated multi-path passes through untouched", func(t *testing.T) {
		flagPath = "."
		multi := sub + "," + dir
		got, err := resolveTargetPath([]string{multi})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != multi {
			t.Errorf("got %q, want it passed through as %q", got, multi)
		}
	})
}
