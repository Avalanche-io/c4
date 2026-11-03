package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExplainRestoreCountsContentTheFolderHolds: explain restore must
// report what restore would do. A folder in view A, the store empty, and
// view B naming the same bytes under other names: restore moves the
// files, so explain says nothing is missing. A file whose bytes changed
// is missing for both.
func TestExplainRestoreCountsContentTheFolderHolds(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	a := map[string]string{
		"comp/sh010_v001.exr": "frame one",
		"comp/sh020_v001.exr": "frame two",
		"notes.txt":           "the notes",
	}
	b := map[string]string{
		"sh010/comp.exr": "frame one",
		"sh020/comp.exr": "frame two",
		"README.txt":     "the notes",
	}

	treeB := filepath.Join(dir, "describe-b")
	writeTree(t, treeB, b)
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", treeB)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	viewB := filepath.Join(dir, "b.c4m")
	if err := os.WriteFile(viewB, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(treeB)
	if entries, _ := os.ReadDir(storeDir); len(entries) != 0 {
		t.Fatalf("the store is not empty: %d entries", len(entries))
	}

	folder := filepath.Join(dir, "work")
	writeTree(t, folder, a)
	stdout, stderr, code = runC4WithEnv(t, bin, env, "explain", "restore", viewB, folder)
	if code != 0 {
		t.Fatalf("explain restore: exit %d: %s", code, stderr)
	}
	if strings.Contains(stdout, "Missing:") {
		t.Errorf("explain reports missing content the folder holds:\n%s", stdout)
	}
	if !strings.Contains(stdout, "all content available") {
		t.Errorf("explain should say all content is available:\n%s", stdout)
	}

	// restore agrees: it moves the files
	_, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", viewB, folder)
	if code != 0 {
		t.Fatalf("restore --force: exit %d: %s", code, stderr)
	}
	for p, body := range b {
		if data, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(p))); err != nil || string(data) != body {
			t.Errorf("view B's %s after restore: %q, %v", p, data, err)
		}
	}

	// a file with the right name but other bytes is missing, for explain
	// as for restore
	empty := filepath.Join(dir, "store2")
	os.MkdirAll(empty, 0o755)
	env2 := map[string]string{"C4_STORE": empty, "HOME": filepath.Join(dir, "home")}
	changed := filepath.Join(dir, "changed")
	writeTree(t, changed, a)
	os.WriteFile(filepath.Join(changed, "notes.txt"), []byte("the notes, edited"), 0o644)
	stdout, _, code = runC4WithEnv(t, bin, env2, "explain", "restore", viewB, changed)
	if code != 0 || !strings.Contains(stdout, "Missing: 1 file") {
		t.Errorf("changed bytes: exit %d:\n%s", code, stdout)
	}
	_, stderr, code = runC4WithEnv(t, bin, env2, "restore", "--force", viewB, changed)
	if code != 1 || !strings.Contains(stderr, "Missing content for 1 entries") {
		t.Errorf("restore should refuse the changed file too: exit %d: %s", code, stderr)
	}
}
