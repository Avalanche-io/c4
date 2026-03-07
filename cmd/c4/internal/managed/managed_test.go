package managed

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// setupTestDir creates a temporary directory with some test files.
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create some files
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)

	sub := filepath.Join(dir, "src")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "main.go"), []byte("package main\n"), 0644)

	return dir
}

func TestIsManaged(t *testing.T) {
	dir := setupTestDir(t)

	if IsManaged(dir) {
		t.Fatal("should not be managed before Init")
	}

	_, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if !IsManaged(dir) {
		t.Fatal("should be managed after Init")
	}
}

func TestInitAndCurrent(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	manifest, err := d.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}

	if len(manifest.Entries) == 0 {
		t.Fatal("initial snapshot should have entries")
	}

	// Check that our test files are in the manifest
	names := make(map[string]bool)
	for _, e := range manifest.Entries {
		names[e.Name] = true
	}
	for _, want := range []string{"hello.txt", "README.md", "src/", "main.go"} {
		if !names[want] {
			t.Errorf("missing entry %q in manifest", want)
		}
	}
}

func TestInitExcludesC4Dir(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	manifest, err := d.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}

	for _, e := range manifest.Entries {
		if e.Name == ".c4/" || e.Name == ".c4" {
			t.Error(".c4/ should be excluded from snapshots")
		}
	}
}

func TestInitDoubleInit(t *testing.T) {
	dir := setupTestDir(t)

	_, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, err = Init(dir, nil)
	if err == nil {
		t.Fatal("second Init should fail")
	}
}

func TestSnapshot(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Add a new file
	os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file\n"), 0644)

	id, err := d.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if id.String() == "" {
		t.Fatal("snapshot should have a non-empty ID")
	}

	n, err := d.HistoryLen()
	if err != nil {
		t.Fatalf("HistoryLen: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 history entries, got %d", n)
	}
}

func TestGetSnapshot(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Add a file and snapshot
	os.WriteFile(filepath.Join(dir, "added.txt"), []byte("added\n"), 0644)
	d.Snapshot()

	// :~0 should have added.txt
	snap0, err := d.GetSnapshot(0)
	if err != nil {
		t.Fatalf("GetSnapshot(0): %v", err)
	}
	found := false
	for _, e := range snap0.Entries {
		if e.Name == "added.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("snapshot 0 should contain added.txt")
	}

	// :~1 should NOT have added.txt
	snap1, err := d.GetSnapshot(1)
	if err != nil {
		t.Fatalf("GetSnapshot(1): %v", err)
	}
	for _, e := range snap1.Entries {
		if e.Name == "added.txt" {
			t.Error("snapshot 1 should not contain added.txt")
		}
	}
}

func TestGetSnapshotOutOfRange(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	_, err = d.GetSnapshot(5)
	if err == nil {
		t.Fatal("should fail for out-of-range snapshot")
	}
}

func TestUndoRedo(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Capture initial state entry count
	initial, _ := d.Current()
	initialCount := len(initial.Entries)

	// Add file and snapshot
	os.WriteFile(filepath.Join(dir, "extra.txt"), []byte("extra\n"), 0644)
	d.Snapshot()

	after, _ := d.Current()
	if len(after.Entries) <= initialCount {
		t.Fatal("snapshot after adding file should have more entries")
	}

	// Undo
	if err := d.Undo(); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	undone, _ := d.Current()
	if len(undone.Entries) != initialCount {
		t.Errorf("after undo, expected %d entries, got %d", initialCount, len(undone.Entries))
	}

	// Redo
	if err := d.Redo(); err != nil {
		t.Fatalf("Redo: %v", err)
	}

	redone, _ := d.Current()
	if len(redone.Entries) != len(after.Entries) {
		t.Errorf("after redo, expected %d entries, got %d", len(after.Entries), len(redone.Entries))
	}
}

func TestUndoNothingToUndo(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := d.Undo(); err == nil {
		t.Fatal("should fail when nothing to undo")
	}
}

func TestRedoNothingToRedo(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := d.Redo(); err == nil {
		t.Fatal("should fail when nothing to redo")
	}
}

func TestSnapshotClearsRedo(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create two snapshots
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644)
	d.Snapshot()
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0644)
	d.Snapshot()

	// Undo once
	d.Undo()

	// New snapshot should clear redo
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("c\n"), 0644)
	d.Snapshot()

	if err := d.Redo(); err == nil {
		t.Fatal("redo should fail after new snapshot (forward history detached)")
	}
}

func TestTags(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	history, _ := d.History()
	initialID := history[0].ID

	// Set a tag
	if err := d.SetTag("v1", initialID); err != nil {
		t.Fatalf("SetTag: %v", err)
	}

	// Get the tag
	manifest, err := d.GetTag("v1")
	if err != nil {
		t.Fatalf("GetTag: %v", err)
	}
	if len(manifest.Entries) == 0 {
		t.Fatal("tagged snapshot should have entries")
	}

	// List tags
	tags, err := d.ListTags()
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if tags["v1"] != initialID {
		t.Errorf("tag v1 = %q, want %q", tags["v1"], initialID)
	}

	// Remove tag
	if err := d.RemoveTag("v1"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	_, err = d.GetTag("v1")
	if err == nil {
		t.Fatal("tag should be gone after RemoveTag")
	}
}

func TestTagNonexistentSnapshot(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	err = d.SetTag("bad", "c4notarealid")
	if err == nil {
		t.Fatal("should fail to tag nonexistent snapshot")
	}
}

func TestIgnorePatterns(t *testing.T) {
	dir := setupTestDir(t)

	// Init with exclude patterns
	d, err := Init(dir, []string{"*.log", "tmp/"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	patterns, err := d.IgnorePatterns()
	if err != nil {
		t.Fatalf("IgnorePatterns: %v", err)
	}
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(patterns))
	}

	// Add more patterns
	if err := d.AddIgnorePatterns([]string{"*.tmp"}); err != nil {
		t.Fatalf("AddIgnorePatterns: %v", err)
	}
	patterns, _ = d.IgnorePatterns()
	if len(patterns) != 3 {
		t.Fatalf("expected 3 patterns, got %d", len(patterns))
	}

	// Remove a pattern
	if err := d.RemoveIgnorePattern("*.log"); err != nil {
		t.Fatalf("RemoveIgnorePattern: %v", err)
	}
	patterns, _ = d.IgnorePatterns()
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns after removal, got %d", len(patterns))
	}
}

func TestIgnorePatternsExcludeFiles(t *testing.T) {
	dir := setupTestDir(t)

	// Create a log file
	os.WriteFile(filepath.Join(dir, "app.log"), []byte("log data\n"), 0644)

	d, err := Init(dir, []string{"*.log"})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	manifest, _ := d.Current()
	for _, e := range manifest.Entries {
		if e.Name == "app.log" {
			t.Error("app.log should be excluded by *.log pattern")
		}
	}
}

func TestTeardown(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := d.Teardown(); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	if IsManaged(dir) {
		t.Fatal("should not be managed after Teardown")
	}

	// Original files should still exist
	if _, err := os.Stat(filepath.Join(dir, "hello.txt")); err != nil {
		t.Error("hello.txt should still exist after Teardown")
	}
}

func TestOpen(t *testing.T) {
	dir := setupTestDir(t)

	_, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	d, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	manifest, err := d.Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if len(manifest.Entries) == 0 {
		t.Fatal("opened dir should have entries")
	}
}

func TestOpenNotManaged(t *testing.T) {
	dir := t.TempDir()
	_, err := Open(dir)
	if err == nil {
		t.Fatal("Open should fail on unmanaged directory")
	}
}

func TestHistory(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Add two snapshots
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644)
	d.Snapshot()
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0644)
	d.Snapshot()

	entries, err := d.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(entries))
	}

	// Verify ordering: 0 is newest
	if entries[0].Index != 0 || entries[2].Index != 2 {
		t.Error("history entries should be indexed 0 (newest) to N (oldest)")
	}

	// All IDs should be different
	seen := make(map[string]bool)
	for _, e := range entries {
		if seen[e.ID] {
			t.Errorf("duplicate ID in history: %s", e.ID)
		}
		seen[e.ID] = true
	}
}

func TestConcurrentSnapshots(t *testing.T) {
	dir := setupTestDir(t)

	d, err := Init(dir, nil)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Run concurrent snapshots — locking should serialize them
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", n)), []byte(fmt.Sprintf("%d\n", n)), 0644)
			_, err := d.Snapshot()
			errs <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Snapshot failed: %v", err)
		}
	}

	// History should have 11 entries (1 from Init + 10 snapshots)
	entries, err := d.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(entries) != 11 {
		t.Errorf("expected 11 history entries, got %d", len(entries))
	}

	// Lock file should exist
	if _, err := os.Stat(filepath.Join(dir, ".c4", "lock")); err != nil {
		t.Error("lock file should exist after locked operations")
	}
}
