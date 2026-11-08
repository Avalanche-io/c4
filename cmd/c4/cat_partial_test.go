package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// partialRecord stores a tree, derives a one-level record of its root
// through the c4m package (depth-0 entries only, so sub/ is a collapsed
// line carrying its listing ID), stores that record, removes sub/'s
// listing from the store, and returns the record's ID, its file path,
// the removed listing's ID and the tree path. On 1.0.x `c4 id -s` stores
// the full tree as the root record, so a collapsed record is the shape
// that needs expanding; master's root records are one-level already.
func partialRecord(t *testing.T, bin string, env map[string]string, dir string) (recID c4.ID, recPath string, subID c4.ID, project string) {
	t.Helper()
	project = filepath.Join(dir, "project")
	writeTree(t, project, map[string]string{
		"root.txt":          "root",
		"sub/child.txt":     "child",
		"sub/deep/leaf.txt": "leaf",
	})
	if err := os.MkdirAll(filepath.Join(project, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", project)
	if code != 0 {
		t.Fatalf("id -s -q: exit %d: %s", code, stderr)
	}
	full, stderr, code := runC4WithEnv(t, bin, env, "cat", storedID(t, stderr))
	if code != 0 {
		t.Fatalf("cat root record: %s", stderr)
	}
	m, err := c4m.Unmarshal([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	record := c4m.NewManifest()
	for _, e := range m.Entries {
		if e.Depth != 0 {
			continue
		}
		cp := *e
		record.AddEntry(&cp)
		if e.Name == "sub/" {
			subID = e.C4ID
		}
	}
	if subID.IsNil() {
		t.Fatal("sub/ must carry its listing ID")
	}
	var buf bytes.Buffer
	if err := c4m.NewEncoder(&buf).Encode(record); err != nil {
		t.Fatal(err)
	}
	recPath = filepath.Join(dir, "record.c4m")
	if err := os.WriteFile(recPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := store.NewTreeStore(env["C4_STORE"])
	if err != nil {
		t.Fatal(err)
	}
	recID, err = s.Put(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(subID); err != nil {
		t.Fatalf("remove sub/ listing: %v", err)
	}
	return recID, recPath, subID, project
}

// TestCatRecursivePartialTreeExits2 pins the rule from the 2026-11-08
// c4d finding: `c4 cat -r` on a record whose store lacks a listing
// prints the tree as far as it goes, names each missing listing on
// stderr, and exits 2. Until then it exited 0 and said nothing, so a
// check that read only the exit status believed a partial tree complete.
func TestCatRecursivePartialTreeExits2(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	recID, recPath, subID, project := partialRecord(t, bin, env, dir)
	want := "partial: sub/ listing " + subID.String() + " not in store"

	for _, target := range []string{recID.String(), recPath} {
		out, stderr, code := runC4WithEnv(t, bin, env, "cat", "-r", target)
		if code != 2 {
			t.Fatalf("cat -r %s on a partial tree must exit 2, got %d; stderr: %s", target, code, stderr)
		}
		if !strings.Contains(out, "root.txt") || !strings.Contains(out, "sub/") {
			t.Fatalf("the tree must still print as far as it goes:\n%s", out)
		}
		if strings.Contains(out, "child.txt") {
			t.Fatalf("sub/'s children cannot appear: its listing is gone:\n%s", out)
		}
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr must name the missing listing:\nwant %q\ngot  %q", want, stderr)
		}
		if strings.Contains(stderr, "empty/") {
			t.Fatalf("an empty directory is complete and must not be named: %s", stderr)
		}
	}

	// A verb that needs the whole tree (patch -r <ID> <dir>) refuses it, naming the gap.
	_, stderr, code := runC4WithEnv(t, bin, env, "patch", "-r", recID.String(), project)
	if code != 1 {
		t.Fatalf("patch of a partial tree must exit 1, got %d: %s", code, stderr)
	}
	if !strings.Contains(stderr, want) || !strings.Contains(stderr, "incomplete in the store") {
		t.Fatalf("patch must name the missing listing and refuse:\n%s", stderr)
	}
}

// TestCatRecursiveNoStoreNamesCollapsed pins -r without a store: a
// one-level record cannot be expanded, so every collapsed directory is
// named and cat exits 2, instead of printing the record and exiting 0.
// A full-form file lists its own children and needs no store.
func TestCatRecursiveNoStoreNamesCollapsed(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	_, recPath, _, project := partialRecord(t, bin, env, dir)

	noStore := map[string]string{"HOME": filepath.Join(dir, "nohome")}
	out, stderr, code := runC4WithEnv(t, bin, noStore, "cat", "-r", recPath)
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

	full, _, code := runC4WithEnv(t, bin, env, "id", project)
	if code != 0 {
		t.Fatal("id")
	}
	fullPath := filepath.Join(dir, "full.c4m")
	if err := os.WriteFile(fullPath, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runC4WithEnv(t, bin, noStore, "cat", "-r", fullPath); code != 0 {
		t.Fatalf("cat -r on a full-form file needs no store: exit %d: %s", code, stderr)
	}
}
