package scan

import (
	"os"
	"path/filepath"
	"testing"
)

// A symbolic link naming one of its own ancestors is an ordinary thing to
// find on a filesystem — `current` and `latest` links look exactly like it.
// Resolving one by scanning the directory it names recursed without bound
// and killed the process with `fatal error: stack overflow`, while exiting
// 2, the code that means "partial description". A crash reported as a
// partial success is worse than either.
//
// No fixed point exists for such a link: the directory it names contains
// the link itself, so every ID would be an arbitrary truncation. The answer
// is a nil ID — what the standard already gives for a target that cannot be
// identified.
func TestSymlinkCycleDoesNotRecurse(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("..", filepath.Join(sub, "up")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	m, err := NewGenerator().GenerateFromPath(root)
	if err != nil {
		t.Fatalf("scanning a tree with a symlink cycle failed: %v", err)
	}

	var found bool
	for _, e := range m.Entries {
		if filepath.Base(e.Name) != "up" {
			continue
		}
		found = true
		if !e.C4ID.IsNil() {
			t.Errorf("cyclic symlink got ID %s; want nil", e.C4ID)
		}
	}
	if !found {
		t.Fatal("the symlink entry is missing from the description")
	}
}

// The cycle guard must not stop an ordinary link to a directory that is not
// an ancestor from resolving to that directory's manifest ID, per
// C4M-STANDARD.md section 5.2.
func TestSymlinkToSiblingDirectoryStillResolves(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	other := filepath.Join(root, "other")
	for _, d := range []string{real, other} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(real, "f.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../real", filepath.Join(other, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	m, err := NewGenerator().GenerateFromPath(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Entries {
		if filepath.Base(e.Name) == "link" && e.C4ID.IsNil() {
			t.Fatal("a link to a non-ancestor directory must still resolve")
		}
	}
}
