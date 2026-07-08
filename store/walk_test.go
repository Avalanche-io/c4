package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestTreeStoreWalk(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Force trie splits so the walk crosses interior nodes.
	s.SetSplitThreshold(4)

	want := make(map[c4.ID]int64)
	for i := 0; i < 32; i++ {
		content := fmt.Sprintf("walk content %d", i)
		id, err := s.Put(strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		want[id] = int64(len(content))
	}

	got := make(map[c4.ID]int64)
	err = s.Walk(func(id c4.ID, size int64) error {
		if _, dup := got[id]; dup {
			t.Fatalf("Walk yielded %s twice", id)
		}
		got[id] = size
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("Walk found %d objects, want %d", len(got), len(want))
	}
	for id, size := range want {
		if got[id] != size {
			t.Fatalf("object %s: size %d, want %d", id, got[id], size)
		}
	}
}

func TestTreeStoreWalkStopsOnError(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := s.Put(strings.NewReader(fmt.Sprintf("stop content %d", i))); err != nil {
			t.Fatal(err)
		}
	}

	sentinel := errors.New("stop")
	calls := 0
	err = s.Walk(func(id c4.ID, size int64) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Walk error = %v, want sentinel", err)
	}
	if calls != 1 {
		t.Fatalf("fn called %d times after error, want 1", calls)
	}
}

func TestTreeStoreWalkSkipsNonObjects(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Put(strings.NewReader("real object"))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a stray temp file and an unrelated file in the store root.
	for name, content := range map[string]string{
		".ingest.12345": "partial",
		"README":        "not an object",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var seen []c4.ID
	err = s.Walk(func(got c4.ID, size int64) error {
		seen = append(seen, got)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != id {
		t.Fatalf("Walk = %v, want exactly [%s]", seen, id)
	}
}
