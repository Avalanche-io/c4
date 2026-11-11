package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A restore recreates symlinks with their own recorded times, so a tree
// holding dated symlinks (live and dangling) restores, verifies and
// re-scans to the snapshot's C4 ID. Through 2026-11-11 restore never set a
// link's own time and its verification failed (exit 3) on any tree holding
// a symlink.
func TestRestoreSetsSymlinkTimes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs privileges on Windows; the unsupported path is TestLinkTimesAsBuilt")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}
	tree := filepath.Join(dir, "work")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "bravo"})
	links := map[string]string{"live": "a.txt", "dangling": "missing.txt", "sub/up": "../a.txt"}
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, target := range links {
		p := filepath.Join(tree, filepath.FromSlash(name))
		if err := os.Symlink(target, p); err != nil {
			t.Fatal(err)
		}
		if err := setLinkTimeForTest(p, when); err != nil {
			t.Fatal(err)
		}
	}

	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	snapID := strings.TrimSpace(stdout)
	if err := os.RemoveAll(tree); err != nil {
		t.Fatal(err)
	}

	_, builtID := restoreIDs(t, bin, env, snapID, tree)
	if builtID != snapID {
		t.Fatalf("as-built %s != snapshot %s", builtID, snapID)
	}
	for name, target := range links {
		p := filepath.Join(tree, filepath.FromSlash(name))
		got, err := os.Readlink(p)
		if err != nil || got != target {
			t.Fatalf("%s: target %q, %v; want %q", name, got, err, target)
		}
		info, err := os.Lstat(p)
		if err != nil || !info.ModTime().Equal(when) {
			t.Fatalf("%s: own time %v, %v; want %v", name, info.ModTime().UTC(), err, when)
		}
	}
	stdout, _, code = runC4WithEnv(t, bin, env, "id", "-q", tree)
	if code != 0 || strings.TrimSpace(stdout) != snapID {
		t.Fatalf("re-scan %q (exit %d), want %s", strings.TrimSpace(stdout), code, snapID)
	}
}
