package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// restoreIDs runs `c4 restore --force` and returns its two stdout
// lines: the pre-image ID and the as-built ID.
func restoreIDs(t *testing.T, bin string, env map[string]string, target, dest string) (string, string) {
	t.Helper()
	stdout, stderr, code := runC4WithEnv(t, bin, env, "restore", "--force", target, dest)
	if code != 0 {
		t.Fatalf("restore --force exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("restore must print exactly 2 lines, got %d: %q", len(lines), stdout)
	}
	return lines[0], lines[1]
}

// TestRestoreRecoverFromStoreAlone is T1: snapshot a tree, delete the
// tree and every file outside the store, keep only the printed ID —
// then restore rebuilds it byte-exact, and line 2 verifies as-built.
func TestRestoreRecoverFromStoreAlone(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	tree := filepath.Join(dir, "work")
	writeTree(t, tree, map[string]string{
		"a.txt":         "alpha",
		"sub/b.txt":     "bravo",
		"sub/deep/c.md": "charlie",
	})

	// Snapshot; keep only what was PRINTED.
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	snapID := strings.TrimSpace(stdout)

	// Total loss of everything outside the store.
	if err := os.RemoveAll(tree); err != nil {
		t.Fatal(err)
	}

	// Dry run first: prints nothing on stdout, changes nothing.
	stdout, _, code = runC4WithEnv(t, bin, env, "restore", snapID, tree)
	if code != 0 {
		t.Fatalf("dry run exit %d", code)
	}
	if stdout != "" {
		t.Fatalf("dry run printed to stdout: %q", stdout)
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Fatal("dry run touched the filesystem")
	}

	// Force: two lines; the tree returns byte-exact.
	preID, builtID := restoreIDs(t, bin, env, snapID, tree)
	if builtID != snapID {
		t.Fatalf("as-built %s != snapshot %s", builtID, snapID)
	}
	// The pre-image of an absent dir is the empty description.
	if preID == "" || preID == builtID {
		t.Fatalf("suspicious pre-image ID: %q", preID)
	}
	data, err := os.ReadFile(filepath.Join(tree, "sub", "deep", "c.md"))
	if err != nil || string(data) != "charlie" {
		t.Fatalf("content not restored: %q, %v", data, err)
	}
}

// TestRestoreUndoTheUndo is T2: restore the WRONG directory, undo it
// via line 1, then undo the undo — arbitrary jumps, each verified.
func TestRestoreUndoTheUndo(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	// A snapshot of some other project.
	other := filepath.Join(dir, "other")
	writeTree(t, other, map[string]string{"release.txt": "other project"})
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", other)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	otherSnap := strings.TrimSpace(stdout)

	// The precious directory, mistakenly restored over.
	precious := filepath.Join(dir, "precious")
	writeTree(t, precious, map[string]string{
		"notes.txt":      "irreplaceable",
		"docs/design.md": "the design",
	})

	// The accident.
	preID, builtID := restoreIDs(t, bin, env, otherSnap, precious)
	if builtID != otherSnap {
		t.Fatal("accident restore did not verify")
	}
	if _, err := os.Stat(filepath.Join(precious, "notes.txt")); !os.IsNotExist(err) {
		t.Fatal("accident did not destroy the prior state")
	}

	// The undo: line 1 feeds straight back as a restore target.
	undoPre, undoBuilt := restoreIDs(t, bin, env, preID, precious)
	if undoBuilt != preID {
		t.Fatalf("undo did not verify: %s != %s", undoBuilt, preID)
	}
	data, err := os.ReadFile(filepath.Join(precious, "notes.txt"))
	if err != nil || string(data) != "irreplaceable" {
		t.Fatalf("undo incomplete: %q, %v", data, err)
	}

	// The undo of the undo: forward again to the accident state.
	if undoPre != otherSnap {
		// The undo's pre-image is the accident state — restoring it
		// re-applies the accident. IDs must agree.
		t.Fatalf("undo pre-image %s != accident state %s", undoPre, otherSnap)
	}
	_, redoBuilt := restoreIDs(t, bin, env, undoPre, precious)
	if redoBuilt != otherSnap {
		t.Fatal("redo did not verify")
	}
}

// TestRestoreRefusesMissingContent pins the pre-flight: a target whose
// content is absent from the store lists the missing IDs, exits 1, and
// touches nothing.
func TestRestoreRefusesMissingContent(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "not stored"})

	// A description whose content was never stored (-x scan, no -s).
	c4mText, _, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 0 {
		t.Fatal("id failed")
	}
	c4mPath := filepath.Join(dir, "tree.c4m")
	if err := os.WriteFile(c4mPath, []byte(c4mText), 0644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	writeTree(t, dest, map[string]string{"existing.txt": "untouched"})

	stdout, stderr, code := runC4WithEnv(t, bin, env, "restore", "--force", c4mPath, dest)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d (stderr: %s)", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("refusal must print nothing on stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "Missing content") {
		t.Fatalf("no missing-content listing: %s", stderr)
	}
	if data, _ := os.ReadFile(filepath.Join(dest, "existing.txt")); string(data) != "untouched" {
		t.Fatal("refusal touched the destination")
	}
}

// TestRestorePreImageJournaled pins the print barrier: line 1's
// pre-image ID must be a journaled claim before it prints.
func TestRestorePreImageJournaled(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	env := map[string]string{"C4_STORE": storeDir}

	tree := filepath.Join(dir, "work")
	writeTree(t, tree, map[string]string{"a.txt": "v1"})
	stdout, _, _ := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	snap := strings.TrimSpace(stdout)

	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("v2 dirty"), 0644); err != nil {
		t.Fatal(err)
	}
	preID, _ := restoreIDs(t, bin, env, snap, tree)

	logOut, _, code := runC4WithEnv(t, bin, env, "log")
	if code != 0 {
		t.Fatal("c4 log failed")
	}
	if !strings.Contains(logOut, preID) {
		t.Fatalf("pre-image %s not in journal:\n%s", preID, logOut)
	}
}
