package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fresh scan's values are already normal, including a nested tree whose
// directory mtime differs from its newest child's. Under reading A of
// design/directory-timestamp.md (ruled 2026-11-03) a directory's time is
// its own, so the normaliser has nothing to say about it; earlier it
// rewrote every such directory to its newest child's time and reported
// the scan's own output as not normal. `c4 id dir` writes a chain (a base
// with null aggregates, a boundary, a refinement patch), and the normal
// form is the resolved flat text, so the shape is checked on the scan
// flattened by `c4 patch`: there `c4 id -d` and `-l` report nothing. On
// the chain itself -d shows the reshaping and no value change.
func TestNormalizerAcceptsAFreshScan(t *testing.T) {
	bin := buildC4(t)
	root := t.TempDir()
	dir := filepath.Join(root, "t")
	if err := os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a", "b", "x.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.txt"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fileTime := time.Date(2026, 1, 1, 16, 0, 0, 0, time.UTC)
	dirTime := time.Date(2026, 1, 1, 17, 0, 0, 0, time.UTC) // newer than its newest child
	for _, p := range []string{"a/b/x.txt", "top.txt"} {
		os.Chtimes(filepath.Join(dir, filepath.FromSlash(p)), fileTime, fileTime)
	}
	for _, p := range []string{"a/b", "a"} {
		os.Chtimes(filepath.Join(dir, filepath.FromSlash(p)), dirTime, dirTime)
	}
	env := map[string]string{"HOME": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)

	out, stderr, code := runC4WithEnv(t, bin, env, "id", dir)
	if code != 0 {
		t.Fatalf("id exit %d\n%s", code, stderr)
	}
	chainPath := filepath.Join(root, "scan.c4m")
	if err := os.WriteFile(chainPath, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	// The chain: reshaped to flat, but no value reported as changed.
	_, stderr, _ = runC4WithEnv(t, bin, env, "id", "-d", chainPath)
	if strings.Contains(stderr, "normalized:") {
		t.Fatalf("the normaliser changed a value of a fresh scan:\n%s", stderr)
	}

	flat, stderr, code := runC4WithEnv(t, bin, env, "patch", chainPath)
	if code != 0 {
		t.Fatalf("patch exit %d\n%s", code, stderr)
	}
	flatPath := filepath.Join(root, "flat.c4m")
	if err := os.WriteFile(flatPath, []byte(flat), 0o644); err != nil {
		t.Fatal(err)
	}
	diff, stderr, code := runC4WithEnv(t, bin, env, "id", "-d", flatPath)
	if code != 0 || strings.TrimSpace(diff) != "" || strings.Contains(stderr, "normalized") {
		t.Fatalf("a fresh scan, flattened, is not normal (exit %d):\n%s%s", code, diff, stderr)
	}
	list, _, code := runC4WithEnv(t, bin, env, "id", "-l", flatPath)
	if code != 0 || strings.TrimSpace(list) != "" {
		t.Fatalf("id -l lists a fresh flattened scan as needing normalisation (exit %d):\n%s", code, list)
	}
}
