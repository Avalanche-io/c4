package main

import (
	"os"
	"path/filepath"
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
