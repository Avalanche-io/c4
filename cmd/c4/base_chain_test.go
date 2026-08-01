package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A block naming a prior block resolves to both — the accreting-chain shape
// a long-running scan emits, where each committed block refines the last and
// the newest block's ID alone reconstructs everything before it.
func TestPatchBackfillsExternalBaseFromStore(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	// Block 1: the first thing the scan learned, stored so later blocks can
	// name it.
	tree := filepath.Join(dir, "tree")
	os.MkdirAll(filepath.Join(tree, "sub"), 0755)
	os.WriteFile(filepath.Join(tree, "first.txt"), []byte("first"), 0644)

	block1ID := storeTreeID(t, bin, env, tree)

	// Block 2: names block 1 on its first line, then states what it learned.
	os.WriteFile(filepath.Join(tree, "second.txt"), []byte("second"), 0644)
	later := bytesID(t, bin, env, "second")
	block2 := block1ID + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 6 second.txt " + strings.TrimSpace(later) + "\n"
	block2Path := filepath.Join(dir, "block2.c4m")
	os.WriteFile(block2Path, []byte(block2), 0644)

	resolved, stderr, code := runC4WithEnv(t, bin, env, "patch", block2Path)
	if code != 0 {
		t.Fatalf("patch exit %d: %s", code, stderr)
	}
	if !strings.Contains(resolved, "second.txt") {
		t.Errorf("this block's own entry missing:\n%s", resolved)
	}
	if !strings.Contains(resolved, "first.txt") {
		t.Errorf("base block's content was not backfilled:\n%s", resolved)
	}
	if !strings.Contains(resolved, "sub/") {
		t.Errorf("base block's directory was not backfilled:\n%s", resolved)
	}
}

// A base that is not in the store must fail loudly, naming the ID. Returning
// this block's entries alone would be a manifest silently missing everything
// it was built on.
func TestPatchUnresolvableBaseFailsLoudly(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	absentPath := filepath.Join(dir, "absent.bin")
	os.WriteFile(absentPath, []byte("never stored anywhere"), 0644)
	absentID := bytesID(t, bin, env, "never stored anywhere")

	orphan := absentID + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 5 g5.txt " + absentID + "\n"
	orphanPath := filepath.Join(dir, "orphan.c4m")
	os.WriteFile(orphanPath, []byte(orphan), 0644)

	stdout, stderr, code := runC4WithEnv(t, bin, env, "patch", orphanPath)
	if code == 0 {
		t.Fatalf("expected failure, got exit 0 with:\n%s", stdout)
	}
	if !strings.Contains(stderr, absentID) {
		t.Errorf("error must name the unresolved base %s, got: %s", absentID, stderr)
	}
}

// id never reads the store, so it cannot resolve a base — and must say so
// rather than print an ID for a manifest missing its base. That ID would
// name a different tree than the file describes.
func TestIDRefusesUnresolvedExternalBase(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	tree := filepath.Join(dir, "tree")
	os.MkdirAll(tree, 0755)
	os.WriteFile(filepath.Join(tree, "first.txt"), []byte("first"), 0644)

	block1ID := storeTreeID(t, bin, env, tree)

	block2 := block1ID + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 5 g5.txt " + block1ID + "\n"
	block2Path := filepath.Join(dir, "block2.c4m")
	os.WriteFile(block2Path, []byte(block2), 0644)

	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", block2Path)
	if code == 0 {
		t.Fatalf("expected id to refuse an unresolved base, got exit 0: %s", stdout)
	}
	if !strings.Contains(stderr, block1ID) {
		t.Errorf("error must name the unresolved base %s, got: %s", block1ID, stderr)
	}
}

// storeTreeID stores a tree's description and returns its ID. On 1.0.x -q
// prints nothing; the stored ID is the stored: line on stderr.
func storeTreeID(t *testing.T, bin string, env map[string]string, tree string) string {
	t.Helper()
	_, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("storing %s: exit %d: %s", tree, code, stderr)
	}
	for _, line := range strings.Split(stderr, "\n") {
		if id := strings.TrimSpace(strings.TrimPrefix(line, "stored:")); strings.HasPrefix(line, "stored:") && len(id) == 90 {
			return id
		}
	}
	t.Fatalf("no stored: line in %q", stderr)
	return ""
}

// bytesID returns the C4 ID of content, read on stdin (its bytes).
func bytesID(t *testing.T, bin string, env map[string]string, content string) string {
	t.Helper()
	out, stderr, code := runC4WithStdinEnv(t, bin, env, content, "id")
	if code != 0 {
		t.Fatalf("c4 id < content: exit %d: %s", code, stderr)
	}
	return strings.TrimSpace(out)
}

// The two-argument forms read their target through the same backfill.
// Reconciling a directory to a block whose base was ignored would delete
// every file only the base names; through 1.0.26 it did, exit 0.
func TestPatchPairFormsBackfillExternalBase(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	tree := filepath.Join(dir, "tree")
	os.MkdirAll(tree, 0755)
	os.WriteFile(filepath.Join(tree, "first.txt"), []byte("first"), 0644)
	block1ID := storeTreeID(t, bin, env, tree)

	secondID := bytesID(t, bin, env, "second")
	os.WriteFile(filepath.Join(tree, "second.txt"), []byte("second"), 0644)
	if _, _, code := runC4WithEnv(t, bin, env, "id", "-s", tree); code != 0 {
		t.Fatalf("storing second.txt's bytes: exit %d", code)
	}
	block2 := block1ID + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 6 second.txt " + secondID + "\n"
	block2Path := filepath.Join(dir, "block2.c4m")
	os.WriteFile(block2Path, []byte(block2), 0644)

	// c4m to c4m: the written description holds the base's entry too.
	out := filepath.Join(dir, "out.c4m")
	if _, stderr, code := runC4WithEnv(t, bin, env, "patch", block2Path, out); code != 0 {
		t.Fatalf("patch block2 out.c4m: exit %d: %s", code, stderr)
	}
	written, _ := os.ReadFile(out)
	if !strings.Contains(string(written), "first.txt") || !strings.Contains(string(written), "second.txt") {
		t.Errorf("out.c4m must hold the base's first.txt and the block's second.txt:\n%s", written)
	}

	// c4m to dir: reconciling keeps the file only the base names.
	dest := filepath.Join(dir, "dest")
	os.MkdirAll(dest, 0755)
	os.WriteFile(filepath.Join(dest, "first.txt"), []byte("first"), 0644)
	os.WriteFile(filepath.Join(dest, "second.txt"), []byte("second"), 0644)
	if _, stderr, code := runC4WithEnv(t, bin, env, "patch", block2Path, dest); code != 0 {
		t.Fatalf("patch block2 dest/: exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dest, "first.txt")); err != nil {
		t.Errorf("reconciling to the block deleted first.txt, which its base names: %v", err)
	}
}
