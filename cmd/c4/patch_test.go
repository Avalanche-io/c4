package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchLocalPath(t *testing.T) {
	bin := buildC4(t)

	// Create source directory with known structure
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("alpha\n"), 0644)
	os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("bravo\n"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "sub", "c.txt"), []byte("charlie\n"), 0644)

	// Generate desired state c4m
	desiredC4m := filepath.Join(t.TempDir(), "desired.c4m")
	out, err := runC4(t, bin, srcDir, ".")
	if err != nil {
		t.Fatalf("c4 . failed: %v\n%s", err, out)
	}
	os.WriteFile(desiredC4m, []byte(out), 0644)

	// Create a scrambled target directory with same content but different layout
	targetDir := filepath.Join(t.TempDir(), "output")
	os.MkdirAll(targetDir, 0755)
	// a.txt renamed
	os.WriteFile(filepath.Join(targetDir, "renamed_a.txt"), []byte("alpha\n"), 0644)
	// b.txt in correct place
	os.WriteFile(filepath.Join(targetDir, "b.txt"), []byte("bravo\n"), 0644)
	// c.txt at top level instead of sub/
	os.WriteFile(filepath.Join(targetDir, "c.txt"), []byte("charlie\n"), 0644)

	// Patch
	out, err = runC4(t, bin, targetDir, "patch", desiredC4m, targetDir)
	if err != nil {
		t.Fatalf("c4 patch failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "patched") {
		t.Errorf("expected 'patched' in output, got: %s", out)
	}

	// Verify result matches desired state
	out, err = runC4(t, bin, targetDir, ".")
	if err != nil {
		t.Fatalf("c4 . failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "a.txt") {
		t.Error("a.txt should exist at root")
	}
	if !strings.Contains(out, "b.txt") {
		t.Error("b.txt should exist at root")
	}
	if !strings.Contains(out, "sub/") {
		t.Error("sub/ directory should exist")
	}
	if !strings.Contains(out, "c.txt") {
		t.Error("c.txt should exist in sub/")
	}

	// renamed_a.txt should be gone
	if _, err := os.Stat(filepath.Join(targetDir, "renamed_a.txt")); err == nil {
		t.Error("renamed_a.txt should have been removed")
	}
}

func TestPatchLocalPathMissingContent(t *testing.T) {
	bin := buildC4(t)

	// Create source with content
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "unique.txt"), []byte("content not available elsewhere\n"), 0644)

	// Generate desired state
	desiredC4m := filepath.Join(t.TempDir(), "desired.c4m")
	out, _ := runC4(t, bin, srcDir, ".")
	os.WriteFile(desiredC4m, []byte(out), 0644)

	// Create empty target — content can't be resolved
	targetDir := filepath.Join(t.TempDir(), "empty")
	os.MkdirAll(targetDir, 0755)

	// Should fail because content isn't available locally
	out, err := runC4(t, bin, targetDir, "patch", desiredC4m, targetDir)
	if err == nil {
		t.Error("expected error when content not available locally")
	}
	if !strings.Contains(out, "not available locally") {
		t.Errorf("expected 'not available locally' error, got: %s", out)
	}
}

func TestPatchLocalPathNoChanges(t *testing.T) {
	bin := buildC4(t)

	// Create directory
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("content\n"), 0644)

	// Generate c4m of current state
	c4mFile := filepath.Join(t.TempDir(), "state.c4m")
	out, _ := runC4(t, bin, dir, ".")
	os.WriteFile(c4mFile, []byte(out), 0644)

	// Patch to same state — should be no changes
	out, err := runC4(t, bin, dir, "patch", c4mFile, dir)
	if err != nil {
		t.Fatalf("c4 patch failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no changes") {
		t.Errorf("expected 'no changes', got: %s", out)
	}
}
