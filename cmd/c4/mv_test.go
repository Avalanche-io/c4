package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

func TestMvRename(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Create a test file and establish a c4m
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644)
	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "cp", "hello.txt", "test.c4m:")

	// Rename hello.txt -> greeting.txt
	run(t, bin, dir, "mv", "test.c4m:hello.txt", "test.c4m:greeting.txt")

	// Load manifest and verify
	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	found := false
	for _, e := range m.Entries {
		if e.Name == "greeting.txt" {
			found = true
			// C4 ID should be preserved
			expected := c4.Identify(strings.NewReader("hello"))
			if e.C4ID != expected {
				t.Errorf("C4 ID changed after rename: got %s, want %s", e.C4ID, expected)
			}
		}
		if e.Name == "hello.txt" {
			t.Error("old name 'hello.txt' still exists after mv")
		}
	}
	if !found {
		t.Error("new name 'greeting.txt' not found after mv")
	}
}

func TestMvToSubdir(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)
	run(t, bin, dir, "mk", "test.c4m:")
	run(t, bin, dir, "cp", "file.txt", "test.c4m:")
	run(t, bin, dir, "mkdir", "-p", "test.c4m:archive/")

	// Move file.txt into archive/
	run(t, bin, dir, "mv", "test.c4m:file.txt", "test.c4m:archive/file.txt")

	m := loadTestManifest(t, filepath.Join(dir, "test.c4m"))

	// file.txt should be at depth 1 under archive/
	foundInArchive := false
	for _, e := range m.Entries {
		if e.Name == "file.txt" && e.Depth == 1 {
			foundInArchive = true
		}
		if e.Name == "file.txt" && e.Depth == 0 {
			t.Error("file.txt still at root after mv to archive/")
		}
	}
	if !foundInArchive {
		t.Error("file.txt not found in archive/ after mv")
	}
}

func TestMvNotFound(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	run(t, bin, dir, "mk", "test.c4m:")
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x"), 0644)
	run(t, bin, dir, "cp", "x.txt", "test.c4m:")

	cmd := exec.Command(bin, "mv", "test.c4m:nonexistent.txt", "test.c4m:new.txt")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for mv of nonexistent file")
	}
	if !strings.Contains(string(out), "not found") {
		t.Errorf("expected 'not found' error, got %q", out)
	}
}

func TestMvNoEstablishment(t *testing.T) {
	bin := buildTestBinary(t)
	dir := t.TempDir()

	// Create a c4m file without establishment
	os.WriteFile(filepath.Join(dir, "test.c4m"), []byte("-rw-r--r-- 2026-01-01T00:00:00Z 5 file.txt -\n"), 0644)

	cmd := exec.Command(bin, "mv", "test.c4m:file.txt", "test.c4m:new.txt")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected error for mv without establishment")
	}
	if !strings.Contains(string(out), "not established") {
		t.Errorf("expected 'not established' error, got %q", out)
	}
}

// Helpers

func run(t *testing.T, bin, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Allow cp to fail on c4d push — the c4m file is still written
		if args[0] == "cp" && strings.Contains(string(out), "c4d push failed") {
			return
		}
		t.Fatalf("%s %s: %v\n%s", bin, strings.Join(args, " "), err, out)
	}
}

func loadTestManifest(t *testing.T, path string) *c4m.Manifest {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer f.Close()
	m, err := c4m.NewDecoder(f).Decode()
	if err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return m
}
