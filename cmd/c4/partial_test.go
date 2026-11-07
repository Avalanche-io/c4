package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A scan with an unreadable file is partial knowledge: the entry is
// recorded with a null ID, declared on stderr, the description is still
// produced, and id exits 2 so a script can tell it from a complete scan.
// Through 1.0.23 it exited 0 (measured on a mount with a non-local file,
// 2026-11-07). A fully readable tree exits 0.
func TestPartialScanExitsTwo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0 does not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads everything; permission-based partiality untestable")
	}
	bin := buildC4(t)
	tree := filepath.Join(t.TempDir(), "tree")
	writeTree(t, tree, map[string]string{
		"open.txt":   "readable",
		"locked.txt": "unreadable",
	})
	if err := os.Chmod(filepath.Join(tree, "locked.txt"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(tree, "locked.txt"), 0o644)

	stdout, stderr, code := runC4(t, bin, "id", tree)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d (stderr: %s)", code, stderr)
	}
	if !strings.Contains(stderr, "partial:") || !strings.Contains(stderr, "locked.txt") {
		t.Fatalf("unreadable entry not declared on stderr: %q", stderr)
	}
	var lockedLine string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, "locked.txt") {
			lockedLine = line
		}
	}
	if lockedLine == "" || !strings.HasSuffix(strings.TrimSpace(lockedLine), " -") {
		t.Fatalf("the unreadable file is not recorded with a null ID:\n%s", stdout)
	}
	if !strings.Contains(stdout, "open.txt") {
		t.Fatalf("the readable file is missing from the description:\n%s", stdout)
	}

	if err := os.Chmod(filepath.Join(tree, "locked.txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, code = runC4(t, bin, "id", tree)
	if code != 0 {
		t.Fatalf("readable tree should exit 0, got %d", code)
	}
}
