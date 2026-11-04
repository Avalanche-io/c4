package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A changed file inside a subdirectory: restore --force writes the file
// back and sets the directory's recorded mtime after the write, so the
// tree matches the description on disk, verification passes (exit 0), a
// diff afterwards is clean, and a re-run plans nothing. (Reported by the
// fleet lead, 2026-11-03.) The directory's own mtime differs from its
// newest child's on purpose: under reading A of
// design/directory-timestamp.md (ruled 2026-11-03) that is the normal
// case, and the verification's projection keeps the directory's own time.
func TestRestoreSetsDirectoryMtimeAfterNestedChange(t *testing.T) {
	bin := buildC4(t)
	root := t.TempDir()
	dir := filepath.Join(root, "t")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	fileTime := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	dirTime := time.Date(2026, 11, 4, 0, 42, 6, 0, time.UTC)
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "sub", "n.txt"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(filepath.Join(dir, "sub", "n.txt"), fileTime, fileTime)
		os.Chtimes(filepath.Join(dir, "sub"), dirTime, dirTime)
	}
	env := map[string]string{"HOME": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)
	write("nested\n")
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", dir)
	if code != 0 {
		t.Fatalf("id -s exit %d\n%s", code, stderr)
	}
	c4mPath := filepath.Join(root, "p.c4m")
	if err := os.WriteFile(c4mPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	write("NESTED\n") // the directory's own mtime still matches the snapshot

	out, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", c4mPath, dir)
	if code != 0 || strings.Contains(stderr, "verification failed") {
		t.Fatalf("restore --force exit %d:\n%s%s", code, out, stderr)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "sub", "n.txt"))
	if string(got) != "nested\n" {
		t.Fatalf("n.txt holds %q after restore", got)
	}
	info, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Truncate(time.Second).Equal(dirTime) {
		t.Fatalf("sub/ mtime is %s after restore, recorded %s: the write bumped it and nothing set it back", info.ModTime().UTC(), dirTime)
	}

	diff, stderr, code := runC4WithEnv(t, bin, env, "diff", c4mPath, dir)
	if code != 0 {
		t.Fatalf("diff exit %d\n%s", code, stderr)
	}
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("the tree differs from the description after restore:\n%s", diff)
	}

	_, stderr, code = runC4WithEnv(t, bin, env, "restore", c4mPath, dir)
	if code != 0 || !strings.Contains(stderr, "0 operations planned") {
		t.Fatalf("a re-run still plans work (exit %d):\n%s", code, stderr)
	}
}
