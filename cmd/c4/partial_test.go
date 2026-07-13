package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestPartialScanExitsTwo pins the partial-knowledge contract: an
// unreadable entry is declared on stderr and recorded with nulls, the
// description is still produced (with its ID), and id exits 2.
func TestPartialScanExitsTwo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 does not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads everything; permission-based partiality untestable")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{
		"open.txt":   "readable",
		"locked.txt": "unreadable",
	})
	if err := os.Chmod(filepath.Join(tree, "locked.txt"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(tree, "locked.txt"), 0644)

	stdout, stderr, code := runC4(t, bin, "id", "-q", tree)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "locked.txt") {
		t.Fatalf("unreadable entry not declared on stderr: %q", stderr)
	}
	// The description is still produced: one bare ID line.
	if lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n"); len(lines) != 1 || lines[0] == "" {
		t.Fatalf("partial scan should still print THE ID: %q", stdout)
	}

	// A fully readable tree exits 0.
	if err := os.Chmod(filepath.Join(tree, "locked.txt"), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, code = runC4(t, bin, "id", "-q", tree)
	if code != 0 {
		t.Fatalf("readable tree should exit 0, got %d", code)
	}
}

// TestPartialScanUnreadableDirectory pins pin 18: an unreadable
// DIRECTORY records with nulls (no children, null ID) and the scan
// continues — exit 2, description still produced, claim still valid.
func TestPartialScanUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 does not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{
		"locked/secret.txt": "hidden",
		"open/ok.txt":       "readable",
	})
	if err := os.Chmod(filepath.Join(tree, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(tree, "locked"), 0755)

	stdout, stderr, code := runC4(t, bin, "id", "-q", tree)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "locked/") {
		t.Fatalf("unreadable directory not declared: %q", stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("partial scan should still print THE ID")
	}
	// The readable sibling was fully scanned.
	full, _, _ := runC4(t, bin, "id", tree)
	if !strings.Contains(full, "ok.txt") {
		t.Fatal("scan did not continue past the unreadable directory")
	}
}
