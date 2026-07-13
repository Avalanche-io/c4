package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// TestSequenceIngestStoresMembers is the regression test for folded
// sequences storing nothing: `c4 id -S -s` folds frame sequences into
// one pattern entry, and the shipped store pass reconstructed paths
// from entry names — a folded name is not a real file, so every member
// was silently skipped. Every member's bytes must be retrievable from
// the store by the member's own C4 ID.
func TestSequenceIngestStoresMembers(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "shots")
	if err := os.MkdirAll(tree, 0755); err != nil {
		t.Fatal(err)
	}

	const frames = 5
	memberIDs := make([]c4.ID, 0, frames)
	for i := 1; i <= frames; i++ {
		content := []byte(fmt.Sprintf("frame-content-%04d\n", i))
		name := fmt.Sprintf("frame.%04d.exr", i)
		if err := os.WriteFile(filepath.Join(tree, name), content, 0644); err != nil {
			t.Fatal(err)
		}
		memberIDs = append(memberIDs, c4.Identify(bytes.NewReader(content)))
	}

	storeDir := filepath.Join(dir, "store")
	_, stderr, code := runC4WithEnv(t, bin,
		map[string]string{"C4_STORE": storeDir}, "id", "-S", "-s", tree)
	if code != 0 {
		t.Fatalf("c4 id -S -s exit %d: %s", code, stderr)
	}
	listing, _, code := runC4(t, bin, "id", "-m", "f", "-S", tree)
	if code != 0 {
		t.Fatal("id -m f -S failed")
	}
	if !bytes.Contains([]byte(listing), []byte("frame.[0001-0005].exr")) {
		t.Fatalf("expected folded sequence entry in output:\n%s", listing)
	}

	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range memberIDs {
		if !s.Has(id) {
			t.Fatalf("member %d (%s) not in store after -S -s ingest", i+1, id)
		}
		rc, err := s.Open(id)
		if err != nil {
			t.Fatal(err)
		}
		got := c4.Identify(rc)
		rc.Close()
		if got != id {
			t.Fatalf("member %d bytes do not validate: %s != %s", i+1, got, id)
		}
	}
}
