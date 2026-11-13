package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A byte change that keeps the file's size and mtime. Through f4f29a0,
// diff's directory side and restore's plan trusted size and mtime
// against the description silently: diff printed nothing, restore
// planned nothing, then failed its own verification with the changed
// bytes still in place. Trust is a client choice (-c), never the
// default. (Design review and the release lead, 2026-11-03.)
func sameSizeSameMtimeTamper(t *testing.T) (bin, root, dir, c4mPath string, env map[string]string) {
	t.Helper()
	bin = buildC4(t)
	root = t.TempDir()
	dir = filepath.Join(root, "t")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(dir, "a.txt"), at, at); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(dir, at, at); err != nil {
			t.Fatal(err)
		}
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
	return bin, root, dir, c4mPath, env
}

func TestDiffSeesSameSizeSameMtimeByteChange(t *testing.T) {
	bin, _, dir, c4mPath, env := sameSizeSameMtimeTamper(t)
	out, stderr, code := runC4WithEnv(t, bin, env, "diff", c4mPath, dir)
	if code != 0 {
		t.Fatalf("diff exit %d\n%s", code, stderr)
	}
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("diff missed a byte change under unchanged size and mtime:\n%s%s", out, stderr)
	}
}

func TestDiffContinueTrustsTheGuideOutLoud(t *testing.T) {
	bin, _, dir, c4mPath, env := sameSizeSameMtimeTamper(t)
	// With -c the guide's IDs are reused for unchanged size+mtime (the
	// file's mtime is older than the guide's scan), and stderr says so.
	out, stderr, code := runC4WithEnv(t, bin, env, "diff", "-c", c4mPath, c4mPath, dir)
	if code != 0 {
		t.Fatalf("diff -c exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "reuse:") {
		t.Fatalf("-c did not report its reuse on stderr:\n%s", stderr)
	}
	if strings.Contains(out, "a.txt") {
		t.Fatalf("-c was asked to trust the guide, yet the file was rehashed:\n%s", out)
	}
}

func TestRestoreRepairsSameSizeSameMtimeByteChange(t *testing.T) {
	bin, _, dir, c4mPath, env := sameSizeSameMtimeTamper(t)

	// Dry run: the patch and the plan agree that a.txt changes.
	out, stderr, code := runC4WithEnv(t, bin, env, "restore", c4mPath, dir)
	if code != 0 {
		t.Fatalf("dry run exit %d\n%s", code, stderr)
	}
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("dry-run patch does not list a.txt:\n%s", out)
	}
	if strings.Contains(stderr, "0 operations planned") || !strings.Contains(stderr, "a.txt") {
		t.Fatalf("the plan does not touch a.txt:\n%s", stderr)
	}

	// --force: restores the recorded bytes and verifies.
	out, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", c4mPath, dir)
	if code != 0 {
		t.Fatalf("restore --force exit %d\n%s%s", code, out, stderr)
	}
	if strings.Contains(stderr, "verification failed") {
		t.Fatalf("verification failed after restore:\n%s", stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world\n" {
		t.Fatalf("a.txt holds %q after restore", got)
	}
	// Two lines, each an ID: the pre-image and the as-built state.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || len(lines[0]) != 90 || len(lines[1]) != 90 {
		t.Fatalf("stdout is not two IDs:\n%s", out)
	}
}
