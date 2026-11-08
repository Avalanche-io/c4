package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// partialSnapshot stores a three-level tree, then removes sub/'s listing
// from the store, and returns the snapshot ID, the removed listing's ID
// and the tree path. The snapshot's root record is intact; one of the
// records it names is gone.
func partialSnapshot(t *testing.T, bin string, env map[string]string, dir string) (snap string, subID c4.ID, project string) {
	t.Helper()
	project = filepath.Join(dir, "project")
	writeTree(t, project, map[string]string{
		"root.txt":          "root",
		"sub/child.txt":     "child",
		"sub/deep/leaf.txt": "leaf",
	})
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", project)
	if code != 0 {
		t.Fatalf("id -s -q: exit %d: %s", code, stderr)
	}
	snap = strings.TrimSpace(out)

	full, _, code := runC4WithEnv(t, bin, env, "id", project)
	if code != 0 {
		t.Fatal("id")
	}
	m, err := c4m.Unmarshal([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Entries {
		if e.Name == "sub/" && e.Depth == 0 {
			subID = e.C4ID
		}
	}
	if subID.IsNil() {
		t.Fatal("sub/ must carry its listing ID")
	}
	s, err := store.NewTreeStore(env["C4_STORE"])
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(subID); err != nil {
		t.Fatalf("remove sub/ listing: %v", err)
	}
	return snap, subID, project
}

// TestCatRecursivePartialTreeExits2 pins the rule from the 2026-11-08
// c4d finding: `c4 cat -r` on a snapshot whose store lacks a listing
// prints the tree as far as it goes, names each missing listing on
// stderr, and exits 2. Until then it exited 0 and said nothing, so a
// check that read only the exit status believed a partial tree complete.
func TestCatRecursivePartialTreeExits2(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	snap, subID, project := partialSnapshot(t, bin, env, dir)

	out, stderr, code := runC4WithEnv(t, bin, env, "cat", "-r", snap)
	if code != 2 {
		t.Fatalf("cat -r on a partial tree must exit 2, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(out, "root.txt") || !strings.Contains(out, "sub/") {
		t.Fatalf("the tree must still print as far as it goes:\n%s", out)
	}
	if strings.Contains(out, "child.txt") {
		t.Fatalf("sub/'s children cannot appear: its listing is gone:\n%s", out)
	}
	want := "partial: sub/ listing " + subID.String() + " not in store"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr must name the missing listing:\nwant %q\ngot  %q", want, stderr)
	}

	// Verbs that need the whole tree refuse it, naming the gap.
	for _, args := range [][]string{
		{"diff", snap, project},
		{"restore", snap, project},
	} {
		_, stderr, code := runC4WithEnv(t, bin, env, args...)
		if code != 1 {
			t.Fatalf("%v on a partial tree must exit 1, got %d: %s", args, code, stderr)
		}
		if !strings.Contains(stderr, want) || !strings.Contains(stderr, "incomplete in the store") {
			t.Fatalf("%v must name the missing listing and refuse:\n%s", args, stderr)
		}
	}
}

// TestCatRecursiveNoStoreNamesCollapsed pins -r without a store: a
// one-level record cannot be expanded, so every collapsed directory is
// named and cat exits 2, instead of printing the record and exiting 0.
// An empty directory is complete as a line and is not named.
func TestCatRecursiveNoStoreNamesCollapsed(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	project := filepath.Join(dir, "project")
	writeTree(t, project, map[string]string{"root.txt": "root", "sub/child.txt": "child"})
	if err := os.MkdirAll(filepath.Join(project, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", project)
	if code != 0 {
		t.Fatalf("id -s -q: %s", stderr)
	}
	record, _, code := runC4WithEnv(t, bin, env, "cat", strings.TrimSpace(out))
	if code != 0 {
		t.Fatal("cat root record")
	}
	recordPath := filepath.Join(dir, "record.c4m")
	if err := os.WriteFile(recordPath, []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}

	noStore := map[string]string{"HOME": filepath.Join(dir, "nohome")}
	out, stderr, code = runC4WithEnv(t, bin, noStore, "cat", "-r", recordPath)
	if code != 2 {
		t.Fatalf("cat -r without a store must exit 2, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, "partial: sub/ listing ") || !strings.Contains(stderr, "no store configured") {
		t.Fatalf("stderr must name sub/ and the reason: %s", stderr)
	}
	if strings.Contains(stderr, "empty/") {
		t.Fatalf("an empty directory is complete and must not be named: %s", stderr)
	}
	if !strings.Contains(out, "sub/") {
		t.Fatalf("the record must still print: %s", out)
	}

	// A full-form file lists its own children: nothing to expand, exit 0.
	full, _, _ := runC4WithEnv(t, bin, env, "id", project)
	fullPath := filepath.Join(dir, "full.c4m")
	if err := os.WriteFile(fullPath, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runC4WithEnv(t, bin, noStore, "cat", "-r", fullPath); code != 0 {
		t.Fatalf("cat -r on a full-form file needs no store: exit %d: %s", code, stderr)
	}
}
