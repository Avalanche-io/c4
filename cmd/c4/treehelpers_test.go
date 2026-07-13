package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// writeTree materializes files (path → content) under root, creating
// parent directories as needed.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// countObjects returns the number of objects in the store at storeDir.
func countObjects(t *testing.T, storeDir string) int {
	t.Helper()
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	err = s.Walk(func(id c4.ID, size int64) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// stripValidator removes the trailing bare root-ID line a listing
// stream ends with (draft-v9: the final line of a listing-emitting id
// invocation is the root ID), yielding the flat description text.
func stripValidator(s string) string {
	trimmed := strings.TrimRight(s, "\n")
	cut := strings.LastIndexByte(trimmed, '\n')
	last := trimmed
	if cut >= 0 {
		last = trimmed[cut+1:]
	}
	f := strings.TrimSpace(last)
	if len(f) == 90 && strings.HasPrefix(f, "c4") {
		if cut < 0 {
			return ""
		}
		return trimmed[:cut+1]
	}
	return s
}
