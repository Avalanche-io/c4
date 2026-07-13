package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCatPathDescent pins the PATHS contract: <ID>/a/b selects entries
// by recorded name through verified store fetches — a directory yields
// its listing, a file its bytes; a file mid-path errors; unknown names
// error; a trailing slash asserts a listing.
func TestCatPathDescent(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}

	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{
		"top.txt":            "top content",
		"src/parser.go":      "package parser\n",
		"src/deep/notes.md":  "deep notes",
		"src/deep/other.txt": "other",
	})

	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	snap := strings.TrimSpace(stdout)

	// File extraction by recorded path, verified.
	out, stderr, code := runC4WithEnv(t, bin, env, "cat", snap+"/src/parser.go")
	if code != 0 {
		t.Fatalf("cat ID/path exit %d: %s", code, stderr)
	}
	if out != "package parser\n" {
		t.Fatalf("wrong bytes: %q", out)
	}

	// Deep extraction.
	out, _, code = runC4WithEnv(t, bin, env, "cat", snap+"/src/deep/notes.md")
	if code != 0 || out != "deep notes" {
		t.Fatalf("deep extraction failed (exit %d): %q", code, out)
	}

	// A directory final component yields its one-level listing.
	out, _, code = runC4WithEnv(t, bin, env, "cat", snap+"/src/deep")
	if code != 0 {
		t.Fatalf("dir descent exit %d", code)
	}
	if !strings.Contains(out, "notes.md") || !strings.Contains(out, "other.txt") {
		t.Fatalf("listing missing children: %q", out)
	}

	// Trailing slash asserts a listing: fine on a dir, refused on a file.
	_, _, code = runC4WithEnv(t, bin, env, "cat", snap+"/src/")
	if code != 0 {
		t.Fatal("trailing slash on a directory should succeed")
	}
	_, stderr, code = runC4WithEnv(t, bin, env, "cat", snap+"/top.txt/")
	if code == 0 {
		t.Fatal("trailing slash on a file must be refused")
	}

	// A file mid-path is an error.
	_, stderr, code = runC4WithEnv(t, bin, env, "cat", snap+"/top.txt/nested")
	if code == 0 {
		t.Fatal("file mid-path must error")
	}
	if !strings.Contains(stderr, "not a directory") {
		t.Fatalf("unexpected mid-path error: %q", stderr)
	}

	// An unknown name errors.
	_, _, code = runC4WithEnv(t, bin, env, "cat", snap+"/nope.txt")
	if code == 0 {
		t.Fatal("unknown name must error")
	}

	// "." and ".." components are errors.
	for _, p := range []string{snap + "/./top.txt", snap + "/../top.txt"} {
		if _, _, code = runC4WithEnv(t, bin, env, "cat", p); code == 0 {
			t.Fatalf("%q should be refused", p)
		}
	}
}

// TestRestoreAndDiffPathTargets pins ID/path acceptance beyond cat:
// restore targets and diff sides descend the same way.
func TestRestoreAndDiffPathTargets(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store")}

	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{
		"src/a.txt":      "alpha",
		"src/deep/b.txt": "bravo",
		"other.txt":      "unrelated",
	})
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("snapshot failed")
	}
	snap := strings.TrimSpace(stdout)

	// Restore only the src subtree into a fresh directory.
	dest := filepath.Join(dir, "srconly")
	_, stderr, code := runC4WithEnv(t, bin, env, "restore", "--force", snap+"/src", dest)
	if code != 0 {
		t.Fatalf("restore ID/path exit %d: %s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dest, "deep", "b.txt"))
	if err != nil || string(data) != "bravo" {
		t.Fatalf("subtree not restored: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "other.txt")); !os.IsNotExist(err) {
		t.Fatal("restore of a subtree must not include siblings")
	}

	// A file target is refused: restore targets must resolve to listings.
	_, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", snap+"/other.txt", dest)
	if code == 0 {
		t.Fatal("file restore target must be refused")
	}

	// Diff side: the restored subtree matches the recorded subtree.
	out, _, code := runC4WithEnv(t, bin, env, "diff", snap+"/src", dest)
	if code != 0 {
		t.Fatalf("diff ID/path exit %d", code)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty diff, got: %q", out)
	}
}
