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

	block1, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("storing block 1: exit %d", code)
	}
	block1ID := strings.TrimSpace(block1)

	// Block 2: names block 1 on its first line, then states what it learned.
	os.WriteFile(filepath.Join(tree, "second.txt"), []byte("second"), 0644)
	later, _, code := runC4WithEnv(t, bin, env, "id", "-q", filepath.Join(tree, "second.txt"))
	if code != 0 {
		t.Fatalf("identifying second.txt: exit %d", code)
	}
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
	absent, _, code := runC4WithEnv(t, bin, env, "id", "-q", absentPath)
	if code != 0 {
		t.Fatalf("identifying absent.bin: exit %d", code)
	}
	absentID := strings.TrimSpace(absent)

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

	block1, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("storing block 1: exit %d", code)
	}
	block1ID := strings.TrimSpace(block1)

	block2 := block1ID + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 5 g5.txt " + block1ID + "\n"
	block2Path := filepath.Join(dir, "block2.c4m")
	os.WriteFile(block2Path, []byte(block2), 0644)

	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", "-q", block2Path)
	if code == 0 {
		t.Fatalf("expected id to refuse an unresolved base, got exit 0: %s", stdout)
	}
	if !strings.Contains(stderr, block1ID) {
		t.Errorf("error must name the unresolved base %s, got: %s", block1ID, stderr)
	}
}
