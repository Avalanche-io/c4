package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// TestIngestJournalsClaim pins the print barrier's journal leg: after
// `c4 id -s -q`, the printed snapshot ID must already be a journaled
// claim in <store>/log.c4m — anything printed is recorded.
func TestIngestJournalsClaim(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{
		"a.txt":     "alpha",
		"sub/b.txt": "bravo",
	})
	storeDir := filepath.Join(dir, "store")

	stdout, stderr, code := runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	printedID := strings.TrimSpace(stdout)

	claims, err := c4m.OpenJournal(storeDir).Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected 1 journal claim, got %d", len(claims))
	}
	c := claims[0]
	if c.ID.String() != printedID {
		t.Fatalf("journal claim %s != printed ID %s", c.ID, printedID)
	}
	if c.Name != "proj.c4m" {
		t.Fatalf("claim name = %q, want proj.c4m", c.Name)
	}
	if !strings.Contains(c.Origin, ":") || !strings.HasSuffix(c.Origin, tree) {
		t.Fatalf("claim origin = %q, want <host>:%s", c.Origin, tree)
	}
	if c.Size <= 0 {
		t.Fatalf("claim size = %d", c.Size)
	}

	// A second snapshot after a change appends a second claim.
	if err := os.WriteFile(filepath.Join(tree, "a.txt"), []byte("alpha2"), 0644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("second snapshot exit %d: %s", code, stderr)
	}
	claims, err = c4m.OpenJournal(storeDir).Claims()
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 {
		t.Fatalf("expected 2 claims, got %d", len(claims))
	}
}

// TestLogNoArgsListsJournal pins the no-arg `c4 log`: one line per
// claim, containing the claim's ID.
func TestLogNoArgsListsJournal(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "proj")
	writeTree(t, tree, map[string]string{"x.txt": "content"})
	storeDir := filepath.Join(dir, "store")

	stdout, _, code := runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatal("ingest failed")
	}
	id := strings.TrimSpace(stdout)

	stdout, stderr, code := runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "log")
	if code != 0 {
		t.Fatalf("c4 log exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 journal line, got %d: %q", len(lines), stdout)
	}
	if !strings.Contains(lines[0], id) || !strings.Contains(lines[0], "proj.c4m") {
		t.Fatalf("journal line missing claim data: %q", lines[0])
	}
}

// Restore's pre-image journaling is pinned by TestRestorePreImageJournaled
// in restore_test.go — patch no longer touches directories.
