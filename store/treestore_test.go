package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestTreeStoreBasicPutGet(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("hello world")
	id, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if id.IsNil() {
		t.Fatal("got nil ID")
	}

	// Verify the ID matches direct computation.
	expected := c4.Identify(bytes.NewReader(content))
	if id != expected {
		t.Fatalf("ID mismatch: got %s, want %s", id, expected)
	}

	// Has should return true.
	if !s.Has(id) {
		t.Fatal("Has returned false for stored content")
	}

	// Get should return the content.
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestTreeStoreHasNonExistent(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	id := c4.Identify(bytes.NewReader([]byte("does not exist")))

	if s.Has(id) {
		t.Fatal("Has returned true for non-existent content")
	}
}

func TestTreeStoreDuplicatePut(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("duplicate test")
	id1, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("duplicate Put returned different IDs: %s vs %s", id1, id2)
	}
}

func TestTreeStoreRemove(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	content := []byte("to be removed")
	id, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Has(id) {
		t.Fatal("content not found after Put")
	}
	if err := s.Remove(id); err != nil {
		t.Fatal(err)
	}
	if s.Has(id) {
		t.Fatal("content still found after Remove")
	}
}

func TestTreeStoreAdaptiveSplit(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Use a very low threshold to trigger splits quickly.
	s.SetSplitThreshold(5)

	// Store enough items to trigger a split.
	ids := make([]c4.ID, 10)
	for i := 0; i < 10; i++ {
		content := fmt.Sprintf("content-%d", i)
		id, err := s.Put(bytes.NewReader([]byte(content)))
		if err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		ids[i] = id
	}

	// All IDs start with "c4", so the first split creates c4/ directory.
	c4Dir := filepath.Join(dir, "c4")
	info, err := os.Stat(c4Dir)
	if err != nil {
		t.Fatalf("c4/ directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("c4/ is not a directory")
	}

	// After split, the c4/ directory should contain subdirectories,
	// not content files directly.
	entries, _ := os.ReadDir(c4Dir)
	hasSubdirs := false
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) == 2 {
			hasSubdirs = true
			break
		}
	}
	if !hasSubdirs {
		t.Fatal("c4/ should have 2-char subdirectories after split")
	}

	// All stored content should still be retrievable.
	for i, id := range ids {
		if !s.Has(id) {
			t.Fatalf("content %d (%s) not found after split", i, id)
		}
		rc, err := s.Open(id)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		expected := fmt.Sprintf("content-%d", i)
		if string(got) != expected {
			t.Fatalf("content %d: got %q, want %q", i, got, expected)
		}
	}
}

func TestTreeStoreDeepSplit(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Very low threshold to force multiple levels of splitting.
	s.SetSplitThreshold(3)

	// Store many items.
	ids := make([]c4.ID, 20)
	for i := 0; i < 20; i++ {
		content := fmt.Sprintf("deep-split-content-%d-%d", i, i*17)
		id, err := s.Put(bytes.NewReader([]byte(content)))
		if err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
		ids[i] = id
	}

	// All content should be retrievable regardless of depth.
	for i, id := range ids {
		if !s.Has(id) {
			t.Fatalf("content %d not found after deep split", i)
		}
		rc, err := s.Open(id)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		expected := fmt.Sprintf("deep-split-content-%d-%d", i, i*17)
		if string(got) != expected {
			t.Fatalf("content %d: got %q, want %q", i, got, expected)
		}
	}

	// Verify we actually have multiple levels of nesting.
	maxDepth := 0
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > maxDepth {
			maxDepth = depth
		}
		return nil
	})
	if maxDepth < 2 {
		t.Fatalf("expected at least depth 2, got %d", maxDepth)
	}
}

func TestTreeStorePathResolution(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Store content at depth 0 (flat).
	content := []byte("path test")
	id, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	// path() should resolve correctly at depth 0.
	p := s.path(id)
	if filepath.Dir(p) != dir {
		t.Fatalf("expected file in root dir, got %s", p)
	}

	// Manually create a trie level and move the file.
	idStr := id.String()
	sub := filepath.Join(dir, idStr[:2])
	os.MkdirAll(sub, 0755)
	os.Rename(filepath.Join(dir, idStr), filepath.Join(sub, idStr))

	// path() should now follow the trie.
	p2 := s.path(id)
	if filepath.Dir(p2) != sub {
		t.Fatalf("expected file in %s, got dir %s", sub, filepath.Dir(p2))
	}

	// Content should still be accessible.
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch after manual trie move")
	}
}

func TestTreeStoreCreate(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Compute an ID first.
	content := []byte("create test")
	id := c4.Identify(bytes.NewReader(content))

	// Create should work.
	wc, err := s.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	wc.Write(content)
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	// Should be retrievable.
	if !s.Has(id) {
		t.Fatal("content not found after Create")
	}

	// Duplicate Create should fail.
	_, err = s.Create(id)
	if err == nil {
		t.Fatal("expected error on duplicate Create")
	}
}

func TestTreeStoreEmptyContent(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Empty content should have a valid C4 ID.
	id, err := s.Put(bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	if id.IsNil() {
		t.Fatal("empty content should have a non-nil ID")
	}
	if !s.Has(id) {
		t.Fatal("empty content not found")
	}
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if len(got) != 0 {
		t.Fatalf("expected empty content, got %d bytes", len(got))
	}
}
