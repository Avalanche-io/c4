package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

func makeSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested content"), 0644)
	// Backdate for deterministic timestamps.
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "hello.txt"), ts, ts)
	os.Chtimes(filepath.Join(dir, "sub", "nested.txt"), ts, ts)
	os.Chtimes(filepath.Join(dir, "sub"), ts, ts)
	return dir
}

func TestDistributeSingleDirTarget(t *testing.T) {
	src := makeSourceDir(t)
	dst := t.TempDir()

	result, err := Distribute(src, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}

	if result.Manifest == nil || len(result.Manifest.Entries) == 0 {
		t.Fatal("expected non-empty manifest")
	}

	// Check files exist in destination.
	data, err := os.ReadFile(filepath.Join(dst, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("got %q, want %q", data, "hello world")
	}

	data, err = os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested content" {
		t.Fatalf("got %q, want %q", data, "nested content")
	}

	if result.Targets[0].Created != 2 {
		t.Errorf("expected 2 files created, got %d", result.Targets[0].Created)
	}
}

func TestDistributeTwoDirTargets(t *testing.T) {
	src := makeSourceDir(t)
	dst1 := t.TempDir()
	dst2 := t.TempDir()

	result, err := Distribute(src, ToDir(dst1), ToDir(dst2))
	if err != nil {
		t.Fatal(err)
	}

	// Both should have identical files.
	for _, dst := range []string{dst1, dst2} {
		data, err := os.ReadFile(filepath.Join(dst, "hello.txt"))
		if err != nil {
			t.Fatalf("missing hello.txt in %s: %v", dst, err)
		}
		if string(data) != "hello world" {
			t.Fatalf("%s: got %q, want %q", dst, data, "hello world")
		}

		data, err = os.ReadFile(filepath.Join(dst, "sub", "nested.txt"))
		if err != nil {
			t.Fatalf("missing nested.txt in %s: %v", dst, err)
		}
		if string(data) != "nested content" {
			t.Fatalf("%s: got %q, want %q", dst, data, "nested content")
		}
	}

	// Both targets should report 2 creates.
	for i, tr := range result.Targets {
		if tr.Created != 2 {
			t.Errorf("target %d: expected 2 created, got %d", i, tr.Created)
		}
	}
}

func TestDistributeDirAndStore(t *testing.T) {
	src := makeSourceDir(t)
	dst := t.TempDir()

	storeDir := t.TempDir()
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Distribute(src, ToDir(dst), ToStore(s))
	if err != nil {
		t.Fatal(err)
	}

	// Dir should have files.
	if _, err := os.Stat(filepath.Join(dst, "hello.txt")); err != nil {
		t.Fatal("hello.txt missing from dir target")
	}

	// Store should have content.
	for _, e := range result.Manifest.Entries {
		if e.IsDir() || e.C4ID.IsNil() {
			continue
		}
		if !s.Has(e.C4ID) {
			t.Errorf("store missing content for %s (%s)", e.Name, e.C4ID)
		}
	}

	// Dir target should have 2 creates, store should have 2.
	if result.Targets[0].Created != 2 {
		t.Errorf("dir target: expected 2 created, got %d", result.Targets[0].Created)
	}
	if result.Targets[1].Created != 2 {
		t.Errorf("store target: expected 2 created, got %d", result.Targets[1].Created)
	}
}

func TestDistributeStoreOnly(t *testing.T) {
	src := makeSourceDir(t)

	storeDir := t.TempDir()
	s, err := store.NewTreeStore(storeDir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Distribute(src, ToStore(s))
	if err != nil {
		t.Fatal(err)
	}

	// Manifest should have entries with C4 IDs.
	if result.Manifest == nil {
		t.Fatal("expected manifest")
	}
	for _, e := range result.Manifest.Entries {
		if e.IsDir() {
			continue
		}
		if e.C4ID.IsNil() {
			t.Errorf("entry %s missing C4 ID", e.Name)
		}
		if !s.Has(e.C4ID) {
			t.Errorf("store missing %s", e.C4ID)
		}
	}
}

func TestDistributeEmptySource(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	result, err := Distribute(src, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Manifest.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result.Manifest.Entries))
	}
	if result.Targets[0].Created != 0 {
		t.Errorf("expected 0 created, got %d", result.Targets[0].Created)
	}
}

func TestDistributeNestedDirectoryTimestamps(t *testing.T) {
	src := makeSourceDir(t)
	dst := t.TempDir()

	_, err := Distribute(src, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}

	// Check that sub/ directory has the backdated timestamp.
	info, err := os.Stat(filepath.Join(dst, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	if !info.ModTime().Truncate(time.Second).Equal(expected) {
		t.Errorf("sub/ mtime = %v, want %v", info.ModTime(), expected)
	}
}

func TestDistributeC4IDsMatchScan(t *testing.T) {
	src := makeSourceDir(t)
	dst := t.TempDir()

	result, err := Distribute(src, ToDir(dst))
	if err != nil {
		t.Fatal(err)
	}

	// Verify C4 IDs are correct by checking content.
	helloID := c4.Identify(strings.NewReader("hello world"))
	nestedID := c4.Identify(strings.NewReader("nested content"))

	foundHello := false
	foundNested := false
	for _, e := range result.Manifest.Entries {
		if e.Name == "hello.txt" {
			if e.C4ID != helloID {
				t.Errorf("hello.txt C4 ID = %s, want %s", e.C4ID, helloID)
			}
			foundHello = true
		}
		if e.Name == "nested.txt" {
			if e.C4ID != nestedID {
				t.Errorf("nested.txt C4 ID = %s, want %s", e.C4ID, nestedID)
			}
			foundNested = true
		}
	}
	if !foundHello || !foundNested {
		t.Error("missing entries in manifest")
	}
}

func TestDistributeMultipleStoreTargets(t *testing.T) {
	src := makeSourceDir(t)

	s1Dir := t.TempDir()
	s2Dir := t.TempDir()
	s1, err := store.NewTreeStore(s1Dir)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := store.NewTreeStore(s2Dir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := Distribute(src, ToStore(s1), ToStore(s2))
	if err != nil {
		t.Fatal(err)
	}

	// Both stores should have all content.
	for _, e := range result.Manifest.Entries {
		if e.IsDir() || e.C4ID.IsNil() {
			continue
		}
		if !s1.Has(e.C4ID) {
			t.Errorf("store1 missing %s (%s)", e.Name, e.C4ID)
		}
		if !s2.Has(e.C4ID) {
			t.Errorf("store2 missing %s (%s)", e.Name, e.C4ID)
		}
	}
}

func TestDistributeDepthCalculation(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "a", "b", "c"), 0755)
	os.WriteFile(filepath.Join(src, "a", "b", "c", "deep.txt"), []byte("deep"), 0644)
	os.WriteFile(filepath.Join(src, "top.txt"), []byte("top"), 0644)

	result, err := Distribute(src)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range result.Manifest.Entries {
		switch e.Name {
		case "top.txt":
			if e.Depth != 0 {
				t.Errorf("top.txt depth = %d, want 0", e.Depth)
			}
		case "a/":
			if e.Depth != 0 {
				t.Errorf("a/ depth = %d, want 0", e.Depth)
			}
		case "b/":
			if e.Depth != 1 {
				t.Errorf("b/ depth = %d, want 1", e.Depth)
			}
		case "c/":
			if e.Depth != 2 {
				t.Errorf("c/ depth = %d, want 2", e.Depth)
			}
		case "deep.txt":
			if e.Depth != 3 {
				t.Errorf("deep.txt depth = %d, want 3", e.Depth)
			}
		}
	}
}

func TestDistributeNoTargets(t *testing.T) {
	// Distribute with no targets should still produce a manifest (scan only).
	src := makeSourceDir(t)

	result, err := Distribute(src)
	if err != nil {
		t.Fatal(err)
	}

	if result.Manifest == nil || len(result.Manifest.Entries) == 0 {
		t.Fatal("expected manifest even with no targets")
	}
	// All files should have C4 IDs.
	for _, e := range result.Manifest.Entries {
		if !e.IsDir() && e.C4ID.IsNil() {
			t.Errorf("%s missing C4 ID", e.Name)
		}
	}
}

func TestDistributeNonexistentSource(t *testing.T) {
	_, err := Distribute("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestDistributeContentIdentical(t *testing.T) {
	src := makeSourceDir(t)
	dst1 := t.TempDir()
	dst2 := t.TempDir()

	_, err := Distribute(src, ToDir(dst1), ToDir(dst2))
	if err != nil {
		t.Fatal(err)
	}

	// Both destinations should have byte-identical files.
	for _, rel := range []string{"hello.txt", "sub/nested.txt"} {
		d1, err := os.ReadFile(filepath.Join(dst1, rel))
		if err != nil {
			t.Fatal(err)
		}
		d2, err := os.ReadFile(filepath.Join(dst2, rel))
		if err != nil {
			t.Fatal(err)
		}
		if string(d1) != string(d2) {
			t.Errorf("%s: destinations differ", rel)
		}
	}
}
