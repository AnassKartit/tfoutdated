package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLock(t *testing.T, dir, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`# generated
provider "registry.terraform.io/hashicorp/azurerm" {
  version     = "` + version + `"
  constraints = "~> 4.0"
}
`)
	if err := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Each Terraform root owns its lock file. Reading only the lock at the scan root
// made every root below it fall back to constraint satisfaction, so a provider
// pinned at 4.56.0 was reported as the newest 4.x instead.
func TestLockIndexUsesNearestLock(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, "4.10.0")
	subA := filepath.Join(root, "platform", "vnets")
	subB := filepath.Join(root, "platform", "identity")
	writeLock(t, subA, "4.56.0")
	// subB deliberately has no lock of its own and must inherit the root's.
	if err := os.MkdirAll(subB, 0o755); err != nil {
		t.Fatal(err)
	}

	idx := BuildLockIndex(root, true)
	if len(idx) != 2 {
		t.Fatalf("want 2 lock files indexed, got %d", len(idx))
	}

	if _, v := idx.Nearest(subA, root); v["hashicorp/azurerm"] != "4.56.0" {
		t.Errorf("subdirectory must use its own lock, got %v", v)
	}
	if _, v := idx.Nearest(subB, root); v["hashicorp/azurerm"] != "4.10.0" {
		t.Errorf("directory without a lock must inherit the nearest one above, got %v", v)
	}
	if _, v := idx.Nearest(root, root); v["hashicorp/azurerm"] != "4.10.0" {
		t.Errorf("root must use the root lock, got %v", v)
	}
}

// A vendored .terraform directory holds other modules' lock files; they describe
// someone else's root and must not be mistaken for ours.
func TestLockIndexSkipsVendoredLocks(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, "4.10.0")
	writeLock(t, filepath.Join(root, ".terraform", "modules", "vendored"), "1.0.0")

	idx := BuildLockIndex(root, true)
	if len(idx) != 1 {
		t.Errorf("vendored lock must be ignored, indexed %d", len(idx))
	}
}

// A root with a bare `provider "azurerm" {}` and no required_providers block is
// valid Terraform. Those providers used to be invisible, so such a root reported
// no dependencies at all while sitting a major version behind.
func TestLockedProvidersSurfacesUndeclared(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, "4.80.0")
	idx := BuildLockIndex(root, false)
	_, versions := idx.Nearest(root, root)

	got := LockedProviders(root, versions, map[string]bool{})
	if len(got) != 1 {
		t.Fatalf("want the lock-only provider reported, got %d", len(got))
	}
	p := got[0]
	if p.Name != "azurerm" || p.Namespace != "hashicorp" {
		t.Errorf("bad split: namespace=%q name=%q", p.Namespace, p.Name)
	}
	if p.LockedVersion != "4.80.0" {
		t.Errorf("want locked 4.80.0, got %q", p.LockedVersion)
	}
	if p.Version != "~> 4.0" {
		t.Errorf("want the constraint carried across, got %q", p.Version)
	}

	// Already declared in required_providers: must not be duplicated.
	known := map[string]bool{"hashicorp/azurerm": true}
	if got := LockedProviders(root, versions, known); len(got) != 0 {
		t.Errorf("declared provider must not be added twice, got %d", len(got))
	}
}
