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

	// Dry run first: the plan in c4m patch form — the current ID (the
	// empty description for an absent dir), differing entries, the
	// target ID — and nothing touched.
	stdout, _, code = runC4WithEnv(t, bin, env, "restore", snapID, tree)
	if code != 0 {
		t.Fatalf("dry run exit %d", code)
	}
	planLines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(planLines) < 2 {
		t.Fatalf("dry run plan too short: %q", stdout)
	}
	if planLines[len(planLines)-1] != snapID {
		t.Fatalf("plan must end with the target ID:\n%s", stdout)
	}
	if planLines[0] == snapID {
		t.Fatal("plan first line should be the (differing) current ID")
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

// TestRestoreSwitchesViewsOfFilesTheFolderHolds: two views of the same
// files, the folder in view A and the store empty. Every file B names is
// already in the folder, so restore moves them into B with nothing from
// the store, and its line 1 undoes it back to A. Content the folder holds
// counts only by its ID, hashed: a file whose bytes changed is missing.
func TestRestoreSwitchesViewsOfFilesTheFolderHolds(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	// view A: the shot's frames by department; view B: the same files by
	// shot, renamed
	a := map[string]string{
		"comp/sh010_v001.exr":   "frame one",
		"comp/sh020_v001.exr":   "frame two",
		"lighting/key_rig.usda": "the key light",
		"notes.txt":             "the notes",
	}
	b := map[string]string{
		"sh010/comp.exr":         "frame one",
		"sh020/comp.exr":         "frame two",
		"shared/lights/key.usda": "the key light",
		"README.txt":             "the notes",
	}
	describe := func(name string, files map[string]string) string {
		tree := filepath.Join(dir, "describe-"+name)
		writeTree(t, tree, files)
		stdout, stderr, code := runC4WithEnv(t, bin, env, "id", tree)
		if code != 0 {
			t.Fatalf("c4 id %s: exit %d: %s", name, code, stderr)
		}
		p := filepath.Join(dir, name+".c4m")
		if err := os.WriteFile(p, []byte(stdout), 0o644); err != nil {
			t.Fatal(err)
		}
		os.RemoveAll(tree)
		return p
	}
	viewB := describe("b", b)

	folder := filepath.Join(dir, "show")
	writeTree(t, folder, a)
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-q", folder)
	if code != 0 {
		t.Fatal("c4 id of the folder failed")
	}
	idA := strings.TrimSpace(stdout)

	// the store holds none of it
	if entries, _ := os.ReadDir(storeDir); len(entries) != 0 {
		t.Fatalf("the store is not empty: %d entries", len(entries))
	}

	// the dry run plans moves, and refuses nothing
	if _, stderr, code := runC4WithEnv(t, bin, env, "restore", viewB, folder); code != 0 || strings.Contains(stderr, "Missing content") {
		t.Fatalf("dry run: exit %d: %s", code, stderr)
	}

	preID, builtID := restoreIDs(t, bin, env, viewB, folder)
	if preID != idA {
		t.Errorf("line 1 %s, want view A's %s", preID, idA)
	}
	for p, body := range b {
		if data, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(p))); err != nil || string(data) != body {
			t.Errorf("view B's %s: %q, %v", p, data, err)
		}
	}
	for p := range a {
		if _, err := os.Stat(filepath.Join(folder, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Errorf("view A's %s is still there", p)
		}
	}

	// the undo, line 1, brings view A back
	undoPre, undoBuilt := restoreIDs(t, bin, env, preID, folder)
	if undoPre != builtID || undoBuilt != idA {
		t.Errorf("undo: line 1 %s (want %s), line 2 %s (want %s)", undoPre, builtID, undoBuilt, idA)
	}
	for p, body := range a {
		if data, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(p))); err != nil || string(data) != body {
			t.Errorf("view A's %s after the undo: %q, %v", p, data, err)
		}
	}

	// a file the folder holds under the right name but with other bytes
	// is not the content B names: missing, and nothing is touched
	empty := filepath.Join(dir, "store2")
	os.MkdirAll(empty, 0o755)
	env2 := map[string]string{"C4_STORE": empty, "HOME": filepath.Join(dir, "home")}
	changed := filepath.Join(dir, "changed")
	writeTree(t, changed, a)
	os.WriteFile(filepath.Join(changed, "notes.txt"), []byte("the notes, edited"), 0o644)
	_, stderr, code := runC4WithEnv(t, bin, env2, "restore", "--force", viewB, changed)
	if code != 1 || !strings.Contains(stderr, "Missing content for 1 entries; nothing restored") {
		t.Errorf("changed bytes: exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(changed, "comp", "sh010_v001.exr")); err != nil {
		t.Errorf("the refusal touched the folder: %v", err)
	}
}
