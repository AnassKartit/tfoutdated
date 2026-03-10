package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/anasskartit/tfoutdated/internal/config"
)

func createTFFile(t *testing.T, path string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# empty tf\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestWalkRecursive(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, "sub", "sub.tf"))
	createTFFile(t, filepath.Join(tmp, "sub", "deep", "deep.tf"))

	files, err := walkTerraformFiles(tmp, true, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}
}

func TestWalkNonRecursive(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, "sub", "sub.tf"))

	files, err := walkTerraformFiles(tmp, false, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file (only root), got %d: %v", len(files), files)
	}

	expected := filepath.Join(tmp, "main.tf")
	if files[0] != expected {
		t.Errorf("got %q, want %q", files[0], expected)
	}
}

func TestWalkSkipsDotTerraformDir(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, ".terraform", "modules", "mod.tf"))

	files, err := walkTerraformFiles(tmp, true, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file (should skip .terraform), got %d: %v", len(files), files)
	}
}

func TestWalkSkipsHiddenDirs(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, ".hidden", "secret.tf"))
	createTFFile(t, filepath.Join(tmp, ".git", "hooks.tf"))

	files, err := walkTerraformFiles(tmp, true, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file (should skip hidden dirs), got %d: %v", len(files), files)
	}
}

func TestWalkRespectsIgnoreRules(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, "legacy", "old.tf"))
	createTFFile(t, filepath.Join(tmp, "staging", "infra.tf"))

	ignores := []config.IgnoreRule{
		{Path: "legacy"},
	}

	files, err := walkTerraformFiles(tmp, true, ignores)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 files (should skip legacy), got %d: %v", len(files), files)
	}

	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) == "legacy" {
			t.Errorf("file in 'legacy' directory should be skipped: %s", f)
		}
	}
}

func TestWalkIgnoreGlobPattern(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, "env", "dev", "infra.tf"))
	createTFFile(t, filepath.Join(tmp, "env", "prod", "infra.tf"))

	ignores := []config.IgnoreRule{
		{Path: "env/**"},
	}

	files, err := walkTerraformFiles(tmp, true, ignores)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	// The "env" dir itself does not match "env/**" via filepath.Match,
	// but its children "env/dev" and "env/prod" match the glob suffix rule.
	// So we should only get main.tf (env is traversed but sub-dirs are skipped).
	for _, f := range files {
		if filepath.Dir(f) != tmp {
			// If it has env in the path, it's from a subdirectory of env
			rel, _ := filepath.Rel(tmp, f)
			if len(rel) > 3 && rel[:3] == "env" {
				t.Errorf("file in 'env/**' should be skipped: %s", f)
			}
		}
	}
}

func TestWalkSkipsOverrideFiles(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	createTFFile(t, filepath.Join(tmp, "main_override.tf"))

	files, err := walkTerraformFiles(tmp, false, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file (should skip override), got %d: %v", len(files), files)
	}

	if filepath.Base(files[0]) != "main.tf" {
		t.Errorf("expected main.tf, got %s", files[0])
	}
}

func TestWalkOnlyTFFiles(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "main.tf"))
	if err := os.WriteFile(filepath.Join(tmp, "readme.md"), []byte("# README"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "vars.tfvars"), []byte("var=1"), 0644); err != nil {
		t.Fatal(err)
	}

	files, err := walkTerraformFiles(tmp, false, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 .tf file, got %d: %v", len(files), files)
	}
}

func TestWalkEmptyDir(t *testing.T) {
	tmp := t.TempDir()

	files, err := walkTerraformFiles(tmp, true, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files in empty dir, got %d", len(files))
	}
}

func TestWalkMultipleFilesReturnsSorted(t *testing.T) {
	tmp := t.TempDir()
	createTFFile(t, filepath.Join(tmp, "c.tf"))
	createTFFile(t, filepath.Join(tmp, "a.tf"))
	createTFFile(t, filepath.Join(tmp, "b.tf"))

	files, err := walkTerraformFiles(tmp, false, nil)
	if err != nil {
		t.Fatalf("walkTerraformFiles() error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// filepath.Walk traverses in lexical order
	sorted := make([]string, len(files))
	copy(sorted, files)
	sort.Strings(sorted)
	for i := range files {
		if files[i] != sorted[i] {
			t.Errorf("files not in lexical order: got %v", files)
			break
		}
	}
}
