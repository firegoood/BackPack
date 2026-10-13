package manage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledHelperCleanupPreservesOperatorFilesAndSymlinkTargets(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "helpers")
	if err := os.MkdirAll(filepath.Join(root, "xray", "pinned"), 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "operator-xray")
	if err := os.WriteFile(outside, []byte("operator-owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(base, "keep")
	if err := os.WriteFile(sibling, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeInstalledHelperTree(base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("installer helper tree was not removed")
	}
	for _, path := range []string{outside, sibling} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("operator file was removed: %s: %v", path, err)
		}
	}
	if err := removeInstalledHelperTree(base); err != nil {
		t.Fatalf("absent helper tree was not idempotent: %v", err)
	}
}

func TestInstalledHelperCleanupRefusesRedirectedOrPublicRoots(t *testing.T) {
	for _, kind := range []string{"base-symlink", "root-symlink", "public-base", "public-root"} {
		t.Run(kind, func(t *testing.T) {
			base := filepath.Join(t.TempDir(), "owned")
			root := filepath.Join(base, "helpers")
			target := t.TempDir()
			if err := os.MkdirAll(root, 0755); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "base-symlink":
				if err := os.Mkdir(filepath.Join(target, "helpers"), 0755); err != nil {
					t.Fatal(err)
				}
				base = filepath.Join(filepath.Dir(base), "redirected")
				if err := os.Symlink(target, base); err != nil {
					t.Fatal(err)
				}
			case "root-symlink":
				if err := os.Remove(root); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, root); err != nil {
					t.Fatal(err)
				}
			case "public-base":
				if err := os.Chmod(base, 0777); err != nil {
					t.Fatal(err)
				}
			case "public-root":
				if err := os.Chmod(root, 0777); err != nil {
					t.Fatal(err)
				}
			}
			if err := removeInstalledHelperTree(base); err == nil {
				t.Fatal("unsafe helper cleanup was accepted")
			}
			if _, err := os.Stat(filepath.Join(base, "helpers")); err != nil {
				t.Fatalf("refused target was modified: %v", err)
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("redirected target was modified: %v", err)
			}
		})
	}
}
