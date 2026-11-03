package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tree whose files and directories sit at the Unix epoch scans as a
// null-dated description. Restoring that description into an empty folder
// must create the files at the epoch, so the folder scans back at the
// description's own ID: the materialization round-trips. The description
// is made by c4 id, never written by hand.
func TestRestoreRoundTripsNullTimestamps(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{
		"a.txt":     "alpha",
		"sub/b.txt": "beta",
	})
	epoch := time.Unix(0, 0).UTC()
	for _, p := range []string{"a.txt", "sub/b.txt", "sub", "."} {
		if err := os.Chtimes(filepath.Join(src, filepath.FromSlash(p)), epoch, epoch); err != nil {
			t.Fatal(err)
		}
	}

	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", src)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, " - ") || strings.Contains(stdout, "1970-") {
		t.Fatalf("the epoch should scan as a null timestamp:\n%s", stdout)
	}
	desc := filepath.Join(dir, "n.c4m")
	if err := os.WriteFile(desc, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, code = runC4WithEnv(t, bin, env, "id", "-q", src)
	if code != 0 {
		t.Fatal("c4 id -q failed")
	}
	targetID := strings.TrimSpace(stdout)

	// the content goes into the store
	if _, stderr, code = runC4WithEnv(t, bin, env, "-s", src); code != 0 {
		t.Fatalf("c4 -s: exit %d: %s", code, stderr)
	}

	dest := filepath.Join(dir, "dest")
	_, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", desc, dest)
	if code != 0 || strings.Contains(stderr, "verification failed") {
		t.Fatalf("restore: exit %d: %s", code, stderr)
	}
	for _, p := range []string{"a.txt", "sub/b.txt", "sub"} {
		info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().UTC().Equal(epoch) {
			t.Errorf("%s created from a null timestamp has mtime %s, want the epoch", p, info.ModTime().UTC())
		}
	}
	stdout, _, code = runC4WithEnv(t, bin, env, "id", "-q", dest)
	if code != 0 {
		t.Fatal("c4 id -q of the restored folder failed")
	}
	if got := strings.TrimSpace(stdout); got != targetID {
		t.Errorf("restored folder scans as %s, want the description's %s", got, targetID)
	}
}

// Applied to a path that already exists, a null timestamp never overwrites
// the path's own: restoring a null-dated description over a folder that
// already holds the content leaves its timestamps alone.
func TestRestoreNullTimestampNeverOverwritesAnExistingFile(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	// a null-dated description of two files, one in a subdirectory: the
	// subdirectory's record and size derive from its child's line, which
	// a kept date lengthens, so verification must re-derive directories
	// after projecting (the case the fleet's lead measured)
	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"a.txt": "alpha", "d/b.txt": "beta"})
	epoch := time.Unix(0, 0).UTC()
	for _, p := range []string{"a.txt", "d/b.txt", "d", "."} {
		if err := os.Chtimes(filepath.Join(src, filepath.FromSlash(p)), epoch, epoch); err != nil {
			t.Fatal(err)
		}
	}
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", src)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	desc := filepath.Join(dir, "n.c4m")
	if err := os.WriteFile(desc, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}

	// the same files already in the destination, dated
	dest := filepath.Join(dir, "dest")
	writeTree(t, dest, map[string]string{"a.txt": "alpha", "d/b.txt": "beta"})
	when := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dest, "a.txt"), when, when); err != nil {
		t.Fatal(err)
	}
	// The file keeps its own time, and verification compares at the
	// target's per-field knowledge, so the kept time is not held against
	// the target's null: the restore succeeds.
	_, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", desc, dest)
	if code != 0 || strings.Contains(stderr, "verification failed") {
		t.Fatalf("restore: exit %d: %s", code, stderr)
	}
	info, err := os.Stat(filepath.Join(dest, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Equal(when) {
		t.Errorf("a null timestamp overwrote the existing file's: %s", info.ModTime().UTC())
	}
}
