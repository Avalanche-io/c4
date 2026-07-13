package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRescanReuseCounts pins the metadata-trusted re-scan surface:
// `c4 id -c <guide> -q <tree>` on an unchanged tree reuses every file
// ID, reports the counts on stderr, and prints the same root ID.
func TestRescanReuseCounts(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}

	tree := filepath.Join(dir, "work")
	writeTree(t, tree, map[string]string{
		"a.txt":     "alpha",
		"sub/b.txt": "bravo",
	})
	ageTree(t, tree, time.Now().Add(-time.Hour))

	// Snapshot (journals the claim with its scan start).
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	snapID := strings.TrimSpace(stdout)

	// Capture the guide c4m.
	c4mText, _, code := runC4WithEnv(t, bin, env, "id", "-m", "f", tree)
	if code != 0 {
		t.Fatal("id failed")
	}
	guidePath := filepath.Join(dir, "guide.c4m")
	if err := os.WriteFile(guidePath, []byte(c4mText), 0644); err != nil {
		t.Fatal(err)
	}

	// Re-scan with the guide: everything reuses, root ID identical.
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", "-m", "f", "-c", guidePath, "-q", tree)
	if code != 0 {
		t.Fatalf("re-scan failed: %s", stderr)
	}
	if got := strings.TrimSpace(stdout); got != snapID {
		t.Fatalf("re-scan ID %s != snapshot %s", got, snapID)
	}
	if !strings.Contains(stderr, "reuse: 2 reused, 0 rehashed") {
		t.Fatalf("missing/incorrect reuse summary: %q", stderr)
	}

	// Change one file (bytes and mtime): only it re-hashes.
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("ALPHA2"), 0644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code = runC4WithEnv(t, bin, env, "id", "-m", "f", "-c", guidePath, "-q", tree)
	if code != 0 {
		t.Fatalf("re-scan failed: %s", stderr)
	}
	if got := strings.TrimSpace(stdout); got == snapID {
		t.Fatal("changed tree must not reproduce the old root ID")
	}
	if !strings.Contains(stderr, "reuse: 1 reused, 1 rehashed") {
		t.Fatalf("missing/incorrect reuse summary after change: %q", stderr)
	}
}

// TestRescanVerifyCatchesObfuscation pins both halves of the posture:
// a same-size change with a restored mtime is invisible to the default
// re-scan (documented accepted risk), and `--verify` catches it,
// naming the file on stderr.
func TestRescanVerifyCatchesObfuscation(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}

	tree := filepath.Join(dir, "work")
	writeTree(t, tree, map[string]string{"x.bin": "0123456789"})
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	ageTree(t, tree, past)

	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	snapID := strings.TrimSpace(stdout)

	c4mText, _, _ := runC4WithEnv(t, bin, env, "id", "-m", "f", tree)
	guidePath := filepath.Join(dir, "guide.c4m")
	if err := os.WriteFile(guidePath, []byte(c4mText), 0644); err != nil {
		t.Fatal(err)
	}

	// The obfuscated change: same size, mtime restored.
	xPath := filepath.Join(tree, "x.bin")
	if err := os.WriteFile(xPath, []byte("9876543210"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(xPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Default re-scan: invisible by design — stale ID reused.
	stdout, _, code = runC4WithEnv(t, bin, env, "id", "-m", "f", "-c", guidePath, "-q", tree)
	if code != 0 {
		t.Fatal("re-scan failed")
	}
	if got := strings.TrimSpace(stdout); got != snapID {
		t.Fatalf("posture: obfuscated change should be invisible to default re-scan (got %s)", got)
	}

	// --verify: full re-hash, obfuscation named on stderr, true ID out.
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", "-m", "f", "-c", guidePath, "--verify", "-q", tree)
	if code != 0 {
		t.Fatalf("verify re-scan failed: %s", stderr)
	}
	if got := strings.TrimSpace(stdout); got == snapID {
		t.Fatal("--verify must produce the true (changed) root ID")
	}
	if !strings.Contains(stderr, "content changed under unchanged metadata") ||
		!strings.Contains(stderr, "x.bin") {
		t.Fatalf("verify did not report the obfuscated file: %q", stderr)
	}
}

// ageTree sets every file and directory mtime under root to when.
func ageTree(t *testing.T, root string, when time.Time) {
	t.Helper()
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, when, when)
	})
	if err != nil {
		t.Fatal(err)
	}
}
