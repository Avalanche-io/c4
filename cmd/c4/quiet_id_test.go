package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// TestQuietPrintsOneIDPerPath pins the machine-output contract for
// `c4 id -q`: exactly one bare 90-char ID line per path that succeeds,
// nothing else on stdout, byte-pure — no regex over prose required.
// This is the contract whose absence caused a consumer to ship a wrong
// project ID scraped from listing output (founding complaint 3).
func TestQuietPrintsOneIDPerPath(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{
		"a.txt":     "alpha",
		"sub/b.txt": "bravo",
	})
	file := filepath.Join(dir, "single.bin")
	content := []byte("standalone content\n")
	if err := os.WriteFile(file, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Directory: one line, a valid ID.
	stdout, stderr, code := runC4(t, bin, "id", "-q", tree)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("dir: expected 1 line, got %d: %q", len(lines), stdout)
	}
	if _, err := c4.Parse(lines[0]); err != nil {
		t.Fatalf("dir: not a bare ID: %q", lines[0])
	}

	// Regular file: one line, and it is the file's content ID.
	wantFileID := c4.Identify(bytes.NewReader(content)).String()
	stdout, stderr, code = runC4(t, bin, "id", "-q", file)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != wantFileID {
		t.Fatalf("file: got %q want %q", strings.TrimSpace(stdout), wantFileID)
	}

	// A .c4m argument: -q prints the description's own canonical ID,
	// equal to what a dir scan of the same tree prints.
	c4mPath := filepath.Join(dir, "proj.c4m")
	full, _, code := runC4(t, bin, "id", tree)
	if code != 0 {
		t.Fatal("full listing failed")
	}
	if err := os.WriteFile(c4mPath, []byte(full), 0644); err != nil {
		t.Fatal(err)
	}
	wantTree, _, _ := runC4(t, bin, "id", "-q", tree)
	gotC4m, _, code := runC4(t, bin, "id", "-q", c4mPath)
	if code != 0 {
		t.Fatal("c4m -q failed")
	}
	if strings.TrimSpace(gotC4m) != strings.TrimSpace(wantTree) {
		t.Fatalf("description ID != tree scan ID:\n%s\n%s", gotC4m, wantTree)
	}

	// Failed path: prints nothing for it, exit 1, successful paths
	// still print.
	stdout, _, code = runC4(t, bin, "id", "-q", file, filepath.Join(dir, "missing.txt"))
	if code == 0 {
		t.Fatal("expected exit 1 with a missing path")
	}
	lines = strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 || strings.TrimSpace(lines[0]) != wantFileID {
		t.Fatalf("mixed: expected only the succeeded path's ID, got %q", stdout)
	}
}

// TestQuietContentMode pins T3's scripted form: two byte-identical
// trees with different metadata give equal IDs under -q -m c.
func TestQuietContentMode(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	writeTree(t, a, map[string]string{"x.txt": "same bytes"})
	writeTree(t, b, map[string]string{"x.txt": "same bytes"})
	if err := os.Chmod(filepath.Join(b, "x.txt"), 0755); err != nil {
		t.Fatal(err)
	}

	idA, _, codeA := runC4(t, bin, "id", "-q", "-m", "c", a)
	idB, _, codeB := runC4(t, bin, "id", "-q", "-m", "c", b)
	if codeA != 0 || codeB != 0 {
		t.Fatal("content-mode scans failed")
	}
	if idA != idB {
		t.Fatalf("content IDs differ:\n%s%s", idA, idB)
	}

	// The default level is content: bare -q equals -q -m c.
	idDefA, _, _ := runC4(t, bin, "id", "-q", a)
	if idDefA != idA {
		t.Fatalf("default -q should be content level:\n%s%s", idDefA, idA)
	}

	idFullA, _, _ := runC4(t, bin, "id", "-q", "-m", "f", a)
	idFullB, _, _ := runC4(t, bin, "id", "-q", "-m", "f", b)
	if idFullA == idFullB {
		t.Fatal("full-fidelity IDs should differ (mode changed)")
	}
}
