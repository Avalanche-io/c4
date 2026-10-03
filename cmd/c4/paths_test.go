package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTreeFile writes content at root/rel with a fixed mtime so that
// unchanged files compare equal across two trees.
func writeTreeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	mt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

// diffFixture builds two trees, identifies both, and returns the
// `c4 diff` patch stream between them.
func diffFixture(t *testing.T, bin string) string {
	t.Helper()
	dir := t.TempDir()
	v1 := filepath.Join(dir, "v1")
	v2 := filepath.Join(dir, "v2")

	writeTreeFile(t, v1, "README.md", "a\n")
	writeTreeFile(t, v1, "src/main.go", "b\n")
	writeTreeFile(t, v1, "old/gone.txt", "z\n")
	writeTreeFile(t, v1, "old/keep.txt", "k\n")

	writeTreeFile(t, v2, "README.md", "a\n")           // unchanged
	writeTreeFile(t, v2, "src/main.go", "b\nc\n")      // modified
	writeTreeFile(t, v2, "old/keep.txt", "k\n")        // unchanged; old/gone.txt removed
	writeTreeFile(t, v2, "new.txt", "x\n")             // added
	writeTreeFile(t, v2, "src/sub/deep.txt", "deep\n") // nested add

	for _, v := range []string{v1, v2} {
		out, stderr, code := runC4(t, bin, "id", v)
		if code != 0 {
			t.Fatalf("c4 id %s: exit %d: %s", v, code, stderr)
		}
		if err := os.WriteFile(v+".c4m", []byte(out), 0644); err != nil {
			t.Fatal(err)
		}
	}
	diff, stderr, code := runC4(t, bin, "diff", v1+".c4m", v2+".c4m")
	if code != 0 {
		t.Fatalf("c4 diff: exit %d: %s", code, stderr)
	}
	return diff
}

// TestPathsFromDiffPatch: `c4 diff a b | c4 paths` lists the changed
// paths (adds, modifications, removals, nested), not escaped lines.
func TestPathsFromDiffPatch(t *testing.T) {
	bin := buildC4(t)
	diff := diffFixture(t, bin)

	out, stderr, code := runC4WithStdin(t, bin, diff, "paths")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := "new.txt\nold/\nold/gone.txt\nsrc/\nsrc/main.go\nsrc/sub/\nsrc/sub/deep.txt\n"
	if out != want {
		t.Fatalf("diff | paths:\n got: %q\nwant: %q\ndiff input:\n%s", out, want, diff)
	}

	// Same result reading the patch from a file argument.
	p := filepath.Join(t.TempDir(), "d.c4m")
	if err := os.WriteFile(p, []byte(diff), 0644); err != nil {
		t.Fatal(err)
	}
	out2, stderr, code := runC4(t, bin, "paths", p)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if out2 != want {
		t.Fatalf("paths <patch file>:\n got: %q\nwant: %q", out2, want)
	}
}

// TestPathsPlainC4mUnchanged: plain c4m input keeps the 1.0.17 output.
func TestPathsPlainC4mUnchanged(t *testing.T) {
	bin := buildC4(t)
	in := "-rw-r--r-- 2026-01-02T03:04:05Z 2 README.md -\n" +
		"drwxr-xr-x 2026-01-02T03:04:05Z 2 old/ -\n" +
		"  -rw-r--r-- 2026-01-02T03:04:05Z 2 gone.txt -\n" +
		"drwxr-xr-x 2026-01-02T03:04:05Z 2 src/ -\n" +
		"  -rw-r--r-- 2026-01-02T03:04:05Z 2 main.go -\n"
	out, stderr, code := runC4WithStdin(t, bin, in, "paths")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := "README.md\nold/\nold/gone.txt\nsrc/\nsrc/main.go\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// TestPathsIntersectUnchanged: `c4 intersect id a b | c4 paths` still works.
func TestPathsIntersectUnchanged(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	writeTreeFile(t, filepath.Join(dir, "a"), "same.txt", "s\n")
	writeTreeFile(t, filepath.Join(dir, "a"), "only-a.txt", "a\n")
	writeTreeFile(t, filepath.Join(dir, "b"), "same.txt", "s\n")
	for _, v := range []string{"a", "b"} {
		out, _, _ := runC4(t, bin, "id", filepath.Join(dir, v))
		os.WriteFile(filepath.Join(dir, v+".c4m"), []byte(out), 0644)
	}
	inter, stderr, code := runC4(t, bin, "intersect", "id", filepath.Join(dir, "a.c4m"), filepath.Join(dir, "b.c4m"))
	if code != 0 {
		t.Fatalf("intersect exit %d: %s", code, stderr)
	}
	out, stderr, code := runC4WithStdin(t, bin, inter, "paths")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if out != "same.txt\n" {
		t.Fatalf("got %q", out)
	}
}

// TestPathsListToC4mUnchanged: the reverse direction (path list to c4m)
// keeps the 1.0.17 output, including lists whose names are C4 IDs.
func TestPathsListToC4mUnchanged(t *testing.T) {
	bin := buildC4(t)
	out, stderr, code := runC4WithStdin(t, bin, "src/main.go\nREADME.md\ndocs/a b.txt\n", "paths")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	want := "- - - README.md -\n" +
		"- - - docs/ -\n" +
		"  - - - a\\ b.txt -\n" +
		"- - - src/ -\n" +
		"  - - - main.go -\n"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}

	// A list of files named by C4 ID (e.g. a store listing) is a path
	// list, not a patch stream: there are no entry lines.
	id := "c441o2bU2Fkzfxpdez54a3SyVJ2VQyNgq8kt8FQkiNti4C2ZiaPETESRBihuUgyNQs2uLCcvbviMaEHG7wZDXn7Rrf"
	out, stderr, code = runC4WithStdin(t, bin, id+"\n"+id+"\n", "paths")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if want := "- - - " + id + " -\n"; out != want {
		t.Fatalf("ID-named path list: got %q, want %q", out, want)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("expected one entry, got %q", out)
	}
}
