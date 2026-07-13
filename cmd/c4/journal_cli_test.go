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
	if c.ScanStart.IsZero() {
		t.Fatal("claim scan start missing")
	}
	// Roots are purely virtual: the journal carries scan-start and ID,
	// nothing else — no names, sizes, or origins (draft-v9 §5).
	data, err := os.ReadFile(filepath.Join(storeDir, "journal"))
	if err != nil {
		t.Fatalf("journal file not at <store>/journal: %v", err)
	}
	if !strings.HasPrefix(string(data), "@c4 journal 1\n") {
		t.Fatalf("journal missing magic header: %q", string(data)[:40])
	}
	if strings.Contains(string(data), tree) {
		t.Fatal("journal must not record filesystem origins")
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
	// index, scan-start, ID — the ID is the last field (awk $NF).
	fields := strings.Fields(lines[0])
	if len(fields) != 3 || fields[0] != "1" || fields[len(fields)-1] != id {
		t.Fatalf("journal line should be 'index scan-start ID': %q", lines[0])
	}
}

// Restore's pre-image journaling is pinned by TestRestorePreImageJournaled
// in restore_test.go — patch no longer touches directories.
