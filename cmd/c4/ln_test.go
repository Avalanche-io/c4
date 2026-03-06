package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestLnHardLink(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "original.txt"), []byte("content"), 0644)
	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "cp", "original.txt", "test.c4m:")

	// Create hard link
	run(t, bin, dir, "ln", "test.c4m:original.txt", "test.c4m:copy.txt")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	expectedID := c4.Identify(strings.NewReader("content"))
	var foundOrig, foundCopy bool
	for _, e := range m.Entries {
		if e.Name == "original.txt" {
			foundOrig = true
			if e.C4ID != expectedID {
				t.Errorf("original C4 ID wrong: got %s", e.C4ID)
			}
		}
		if e.Name == "copy.txt" {
			foundCopy = true
			if e.C4ID != expectedID {
				t.Errorf("copy C4 ID should match original: got %s", e.C4ID)
			}
		}
	}
	if !foundOrig {
		t.Error("original.txt not found")
	}
	if !foundCopy {
		t.Error("copy.txt not found after ln")
	}
}

func TestLnSymlink(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")

	// Create symlink
	run(t, bin, dir, "ln", "-s", "../shared/config.yaml", "test.c4m:config.yaml")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "config.yaml" {
			found = true
			if !e.IsSymlink() {
				t.Error("expected symlink mode")
			}
			if e.Target != "../shared/config.yaml" {
				t.Errorf("wrong target: got %q, want %q", e.Target, "../shared/config.yaml")
			}
		}
	}
	if !found {
		t.Error("config.yaml symlink not found")
	}
}

func TestLnHardLinkNotFound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)
	run(t, bin, dir, "cp", "x.txt", "test.c4m:")

	cmd := exec.Command(bin, "ln", "test.c4m:nonexistent.txt", "test.c4m:link.txt")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for ln of nonexistent source")
	}
	if !strings.Contains(string(out), "not found") {
		t.Errorf("expected 'not found' error, got %q", out)
	}
}

func TestLnHardLinkDirectory(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "mkdir", "test.c4m:mydir/")

	cmd := exec.Command(bin, "ln", "test.c4m:mydir/", "test.c4m:link/")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for hard linking a directory")
	}
	if !strings.Contains(string(out), "cannot hard link directories") {
		t.Errorf("expected directory error, got %q", out)
	}
}

func TestLnSymlinkInSubdir(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "mkdir", "-p", "test.c4m:config/")

	// Create symlink inside a subdirectory
	run(t, bin, dir, "ln", "-s", "../../defaults.yaml", "test.c4m:config/settings.yaml")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "settings.yaml" && e.Depth == 1 {
			found = true
			if !e.IsSymlink() {
				t.Error("expected symlink mode")
			}
			if e.Target != "../../defaults.yaml" {
				t.Errorf("wrong target: got %q", e.Target)
			}
		}
	}
	if !found {
		t.Error("config/settings.yaml symlink not found at depth 1")
	}
}
