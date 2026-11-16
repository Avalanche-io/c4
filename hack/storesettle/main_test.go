package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// A report finds an object one level above its place and exits 2 without
// moving it; -move moves it and a second report exits 0.
func TestStoreSettle(t *testing.T) {
	root := t.TempDir()
	s, err := store.NewTreeStore(root)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(4)
	var ids []c4.ID
	for i := 0; i < 20; i++ {
		id, err := s.Put(bytes.NewReader([]byte{byte(i), 'x'}))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	var up string
	for _, id := range ids {
		p, _ := s.ContentPath(id)
		if filepath.Dir(p) == root {
			continue
		}
		up = filepath.Join(filepath.Dir(filepath.Dir(p)), id.String())
		if err := os.Rename(p, up); err != nil {
			t.Fatal(err)
		}
		break
	}
	if up == "" {
		t.Fatal("fixture: no object below the root")
	}
	if code := run(root, false); code != 2 {
		t.Errorf("report: exit %d, want 2", code)
	}
	if _, err := os.Stat(up); err != nil {
		t.Errorf("report moved the file: %v", err)
	}
	if code := run(root, true); code != 0 {
		t.Errorf("-move: exit %d, want 0", code)
	}
	if code := run(root, false); code != 0 {
		t.Errorf("report after -move: exit %d, want 0", code)
	}
	if code := run(filepath.Join(root, "absent"), false); code != 1 {
		t.Errorf("a missing directory: exit %d, want 1", code)
	}
}
