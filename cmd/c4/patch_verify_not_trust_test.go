package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A byte change that keeps the file's size and mtime. Through 1.0.23,
// c4 patch scanned the destination with the target as an implicit guide
// and planned with trusted metadata, so it reported "Reconciled" and
// left the changed bytes in place. Trust is the caller's choice (-c),
// never the default. (Found on the release line, 2026-11-03.)
func tamperedTree(t *testing.T) (bin, dir, c4mPath string, env map[string]string) {
	t.Helper()
	bin = buildC4(t)
	root := t.TempDir()
	dir = filepath.Join(root, "t")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		os.Chtimes(filepath.Join(dir, "a.txt"), at, at)
		os.Chtimes(dir, at, at)
	}
	env = map[string]string{"HOME": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)
	write("hello world\n")
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", dir)
	if code != 0 {
		t.Fatalf("id -s exit %d\n%s", code, stderr)
	}
	c4mPath = filepath.Join(root, "p.c4m")
	if err := os.WriteFile(c4mPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	write("HELLO WORLD\n") // same size, same mtime, different bytes
	return bin, dir, c4mPath, env
}

func TestPatchRepairsSameSizeSameMtimeByteChange(t *testing.T) {
	bin, dir, c4mPath, env := tamperedTree(t)
	out, stderr, code := runC4WithEnv(t, bin, env, "patch", c4mPath, dir)
	if code != 0 {
		t.Fatalf("patch exit %d\n%s", code, stderr)
	}
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("the changeset does not list a.txt:\n%s%s", out, stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world\n" {
		t.Fatalf("a.txt holds %q after patch: the byte change was trusted away", got)
	}
	// Re-running finds nothing to do.
	out, _, code = runC4WithEnv(t, bin, env, "patch", c4mPath, dir)
	if code != 0 || strings.Contains(out, "a.txt") {
		t.Fatalf("a second patch still changes a.txt (exit %d):\n%s", code, out)
	}
}

func TestPatchContinueTrustsTheGuideOutLoud(t *testing.T) {
	bin, dir, c4mPath, env := tamperedTree(t)
	// -c names the guide: unchanged size+mtime reuse its IDs, so the
	// tampered bytes are, by the caller's choice, not read.
	out, stderr, code := runC4WithEnv(t, bin, env, "patch", "-c", c4mPath, c4mPath, dir)
	if code != 0 {
		t.Fatalf("patch -c exit %d\n%s", code, stderr)
	}
	if strings.Contains(out, "a.txt") {
		t.Fatalf("-c was asked to trust the guide, yet a.txt was rehashed:\n%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if string(got) != "HELLO WORLD\n" {
		t.Fatalf("a.txt holds %q: -c did not trust the guide", got)
	}
}
