package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// `c4 restore --force -c GUIDE` must honour -c exactly as the dry run does:
// trust is the client's stated choice. A file whose bytes changed but whose
// size and mtime match the guide (older than the guide's scan, so racy-safe)
// is trusted. The dry run planned 0 operations for it, but --force planned
// from the fully hashed pre-image and rewrote it. Without -c every file is
// hashed and the file is restored.
func TestRestoreForceHonoursTheGuideLikeTheDryRun(t *testing.T) {
	bin := buildC4(t)
	work := t.TempDir()
	tree := filepath.Join(work, "t")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(tree, "a.txt")
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setBytes := func(s string) {
		t.Helper()
		if err := os.WriteFile(a, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(a, old, old)
		os.Chtimes(tree, old, old)
	}
	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": filepath.Join(work, "store")}

	setBytes("hello world\n")
	snap, errOut, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("c4 id -s exited %d: %s", code, errOut)
	}
	guide := filepath.Join(work, "p.c4m")
	if err := os.WriteFile(guide, []byte(snap), 0o644); err != nil {
		t.Fatal(err)
	}

	// Same size, same mtime, different bytes.
	setBytes("HELLO WORLD\n")
	_, dry, _ := runC4WithEnv(t, bin, env, "restore", "-c", guide, guide, tree)
	if !strings.Contains(dry, "dry run: 0 operations") {
		t.Fatalf("dry run with -c should plan nothing for a trusted file:\n%s", dry)
	}
	_, errOut, code = runC4WithEnv(t, bin, env, "restore", "--force", "-c", guide, guide, tree)
	if code != 0 {
		t.Fatalf("restore --force -c exited %d: %.400s", code, errOut)
	}
	if b, _ := os.ReadFile(a); string(b) != "HELLO WORLD\n" {
		t.Errorf("--force -c rewrote a file the guide vouches for (a.txt = %q); the dry run planned 0 operations", b)
	}

	// Without -c nothing is trusted: the file is restored.
	setBytes("HELLO WORLD\n")
	_, errOut, code = runC4WithEnv(t, bin, env, "restore", "--force", guide, tree)
	if code != 0 {
		t.Fatalf("restore --force exited %d: %.400s", code, errOut)
	}
	if b, _ := os.ReadFile(a); string(b) != "hello world\n" {
		t.Errorf("--force without -c left a.txt = %q, want the restored bytes", b)
	}
}
