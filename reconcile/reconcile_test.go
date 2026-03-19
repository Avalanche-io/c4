package reconcile

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// helper: write a file with content and return its C4 ID.
func writeFile(t *testing.T, dir, name, content string) c4.ID {
	t.Helper()
	path := filepath.Join(dir, name)
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	id := c4.Identify(bytes.NewReader([]byte(content)))
	return id
}

// helper: build a manifest with files matching a source directory.
func buildManifest(t *testing.T, entries []testEntry) *c4m.Manifest {
	t.Helper()
	b := c4m.NewBuilder()
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, e := range entries {
		if e.isDir {
			b.AddDir(e.name, c4m.WithTimestamp(ts), c4m.WithSize(0))
		} else {
			b.AddFile(e.name,
				c4m.WithC4ID(e.id),
				c4m.WithSize(int64(len(e.content))),
				c4m.WithMode(e.mode),
				c4m.WithTimestamp(ts),
			)
		}
	}
	m, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type testEntry struct {
	name    string
	content string
	id      c4.ID
	mode    os.FileMode
	isDir   bool
}

func TestPlanEmptyToPopulated(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	id1 := writeFile(t, srcDir, "a.txt", "alpha")
	id2 := writeFile(t, srcDir, "b.txt", "bravo")
	id3 := writeFile(t, srcDir, "c.txt", "charlie")

	target := buildManifest(t, []testEntry{
		{name: "a.txt", content: "alpha", id: id1, mode: 0644},
		{name: "b.txt", content: "bravo", id: id2, mode: 0644},
		{name: "c.txt", content: "charlie", id: id3, mode: 0644},
	})

	srcManifest := buildManifest(t, []testEntry{
		{name: "a.txt", content: "alpha", id: id1, mode: 0644},
		{name: "b.txt", content: "bravo", id: id2, mode: 0644},
		{name: "c.txt", content: "charlie", id: id3, mode: 0644},
	})

	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("expected complete plan, got %d missing", len(plan.Missing))
	}

	createCount := 0
	for _, op := range plan.Operations {
		if op.Type == OpCreate {
			createCount++
		}
	}
	if createCount != 3 {
		t.Fatalf("expected 3 OpCreate, got %d", createCount)
	}
}

func TestPlanIdentical(t *testing.T) {
	dir := t.TempDir()

	id1 := writeFile(t, dir, "a.txt", "alpha")
	id2 := writeFile(t, dir, "b.txt", "bravo")

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	// Set file times to match manifest.
	os.Chtimes(filepath.Join(dir, "a.txt"), ts, ts)
	os.Chtimes(filepath.Join(dir, "b.txt"), ts, ts)

	target := buildManifest(t, []testEntry{
		{name: "a.txt", content: "alpha", id: id1},
		{name: "b.txt", content: "bravo", id: id2},
	})

	rec := New()
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 0 {
		for _, op := range plan.Operations {
			t.Logf("  unexpected op: type=%d path=%s", op.Type, op.Path)
		}
		t.Fatalf("expected 0 operations for identical dir, got %d", len(plan.Operations))
	}
}

func TestPlanFileRemoval(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "a.txt", "alpha")
	writeFile(t, dir, "b.txt", "bravo")
	writeFile(t, dir, "extra.txt", "should be removed")

	id1 := c4.Identify(bytes.NewReader([]byte("alpha")))
	id2 := c4.Identify(bytes.NewReader([]byte("bravo")))

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "a.txt"), ts, ts)
	os.Chtimes(filepath.Join(dir, "b.txt"), ts, ts)

	target := buildManifest(t, []testEntry{
		{name: "a.txt", content: "alpha", id: id1, mode: 0644},
		{name: "b.txt", content: "bravo", id: id2, mode: 0644},
	})

	rec := New()
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}

	removeCount := 0
	for _, op := range plan.Operations {
		if op.Type == OpRemove {
			removeCount++
		}
	}
	if removeCount != 1 {
		t.Fatalf("expected 1 OpRemove, got %d", removeCount)
	}
}

func TestPlanFileMove(t *testing.T) {
	dir := t.TempDir()

	// File exists at "old.txt" but target wants it at "new.txt".
	id := writeFile(t, dir, "old.txt", "moveme")

	target := buildManifest(t, []testEntry{
		{name: "new.txt", content: "moveme", id: id, mode: 0644},
	})

	rec := New()
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}

	moveCount := 0
	for _, op := range plan.Operations {
		if op.Type == OpMove {
			moveCount++
		}
	}
	if moveCount != 1 {
		t.Fatalf("expected 1 OpMove, got %d", moveCount)
	}
}

func TestPlanMissingContent(t *testing.T) {
	dstDir := t.TempDir()

	// Create a manifest referencing a C4 ID that doesn't exist anywhere.
	fakeID := c4.Identify(bytes.NewReader([]byte("nonexistent content")))

	target := buildManifest(t, []testEntry{
		{name: "missing.txt", content: "nonexistent content", id: fakeID, mode: 0644},
	})

	rec := New() // no sources
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if plan.IsComplete() {
		t.Fatal("expected plan to have missing content")
	}
	if len(plan.Missing) != 1 {
		t.Fatalf("expected 1 missing ID, got %d", len(plan.Missing))
	}
	if plan.Missing[0] != fakeID {
		t.Fatalf("wrong missing ID: got %s, want %s", plan.Missing[0], fakeID)
	}
}

func TestApplyCreatesFiles(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	id1 := writeFile(t, srcDir, "hello.txt", "hello world")
	id2 := writeFile(t, srcDir, "foo.txt", "foo bar")

	target := buildManifest(t, []testEntry{
		{name: "hello.txt", content: "hello world", id: id1, mode: 0644},
		{name: "foo.txt", content: "foo bar", id: id2, mode: 0644},
	})

	srcManifest := buildManifest(t, []testEntry{
		{name: "hello.txt", content: "hello world", id: id1, mode: 0644},
		{name: "foo.txt", content: "foo bar", id: id2, mode: 0644},
	})

	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("plan not complete, missing %d IDs", len(plan.Missing))
	}

	result, err := rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 2 {
		t.Fatalf("expected 2 created, got %d", result.Created)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// Verify files exist with correct content.
	for _, tc := range []struct {
		name    string
		content string
	}{
		{"hello.txt", "hello world"},
		{"foo.txt", "foo bar"},
	} {
		data, err := os.ReadFile(filepath.Join(dstDir, tc.name))
		if err != nil {
			t.Fatalf("reading %s: %v", tc.name, err)
		}
		if string(data) != tc.content {
			t.Fatalf("%s: got %q, want %q", tc.name, data, tc.content)
		}
	}
}

func TestApplyIdempotent(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	id := writeFile(t, srcDir, "data.txt", "some data")

	target := buildManifest(t, []testEntry{
		{name: "data.txt", content: "some data", id: id},
	})

	srcManifest := buildManifest(t, []testEntry{
		{name: "data.txt", content: "some data", id: id},
	})

	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))

	// First apply.
	plan1, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	res1, err := rec.Apply(plan1, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if res1.Created != 1 {
		t.Fatalf("first apply: expected 1 created, got %d", res1.Created)
	}

	// Second apply: re-plan against the now-populated directory.
	plan2, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}

	// The second plan should have zero operations because the dir matches.
	if len(plan2.Operations) != 0 {
		for _, op := range plan2.Operations {
			t.Logf("unexpected op: type=%d path=%s", op.Type, op.Path)
		}
		t.Fatalf("second plan: expected 0 operations, got %d", len(plan2.Operations))
	}
}

func TestApplyRemovesFiles(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, dir, "keep.txt", "keeper")
	writeFile(t, dir, "gone.txt", "doomed")

	keepID := c4.Identify(bytes.NewReader([]byte("keeper")))

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "keep.txt"), ts, ts)

	target := buildManifest(t, []testEntry{
		{name: "keep.txt", content: "keeper", id: keepID, mode: 0644},
	})

	rec := New()
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := rec.Apply(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 {
		t.Fatalf("expected 1 removed, got %d", result.Removed)
	}

	if _, err := os.Stat(filepath.Join(dir, "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("gone.txt should have been removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatal("keep.txt should still exist")
	}
}

func TestApplyMetadataUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix file permissions")
	}
	dir := t.TempDir()

	id := writeFile(t, dir, "mod.txt", "modifiable")
	os.Chmod(filepath.Join(dir, "mod.txt"), 0644)

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "mod.txt"), ts, ts)

	// Target wants mode 0755 (different from current 0644).
	target := buildManifest(t, []testEntry{
		{name: "mod.txt", content: "modifiable", id: id, mode: 0755},
	})

	rec := New()
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}

	chmodCount := 0
	for _, op := range plan.Operations {
		if op.Type == OpChmod {
			chmodCount++
		}
	}
	if chmodCount != 1 {
		t.Fatalf("expected 1 OpChmod, got %d", chmodCount)
	}

	result, err := rec.Apply(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated != 1 {
		t.Fatalf("expected 1 updated, got %d", result.Updated)
	}

	info, err := os.Stat(filepath.Join(dir, "mod.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("expected mode 0755, got %o", info.Mode().Perm())
	}
}

// TestDirSourceHasAndOpen verifies the DirSource content source.
func TestDirSourceHasAndOpen(t *testing.T) {
	dir := t.TempDir()
	content := "test content for dir source"
	id := writeFile(t, dir, "src.txt", content)

	m := buildManifest(t, []testEntry{
		{name: "src.txt", content: content, id: id, mode: 0644},
	})

	ds := NewDirSource(m, dir)

	if !ds.Has(id) {
		t.Fatal("DirSource should have the ID")
	}

	rc, err := ds.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("got %q, want %q", data, content)
	}

	// Non-existent ID.
	fakeID := c4.Identify(bytes.NewReader([]byte("nonexistent")))
	if ds.Has(fakeID) {
		t.Fatal("DirSource should not have a fake ID")
	}
}

// TestApplyDirectoryTimestamps verifies that directories get correct
// timestamps after all children are written.
func TestApplyDirectoryTimestamps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix file permissions")
	}

	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source files in a nested structure.
	id1 := writeFile(t, srcDir, "dir1/a.txt", "alpha")
	id2 := writeFile(t, srcDir, "dir1/sub/b.txt", "bravo")

	dirTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	subTime := time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC)
	fileTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "dir1/", Mode: os.ModeDir | 0755, Size: 0, Timestamp: dirTime, Depth: 0})
	target.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0644, Size: 5, C4ID: id1, Timestamp: fileTime, Depth: 1})
	target.AddEntry(&c4m.Entry{Name: "sub/", Mode: os.ModeDir | 0755, Size: 0, Timestamp: subTime, Depth: 1})
	target.AddEntry(&c4m.Entry{Name: "b.txt", Mode: 0644, Size: 5, C4ID: id2, Timestamp: fileTime, Depth: 2})

	// Build a source manifest that matches the on-disk layout.
	srcManifest := c4m.NewManifest()
	srcManifest.AddEntry(&c4m.Entry{Name: "dir1/", Mode: os.ModeDir | 0755, Size: 0, Timestamp: fileTime, Depth: 0})
	srcManifest.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0644, Size: 5, C4ID: id1, Timestamp: fileTime, Depth: 1})
	srcManifest.AddEntry(&c4m.Entry{Name: "sub/", Mode: os.ModeDir | 0755, Size: 0, Timestamp: fileTime, Depth: 1})
	srcManifest.AddEntry(&c4m.Entry{Name: "b.txt", Mode: 0644, Size: 5, C4ID: id2, Timestamp: fileTime, Depth: 2})

	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("plan has %d missing IDs", len(plan.Missing))
	}
	_, err = rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}

	// Check directory timestamps.
	dir1Info, err := os.Stat(filepath.Join(dstDir, "dir1"))
	if err != nil {
		t.Fatal(err)
	}
	if !dir1Info.ModTime().Truncate(time.Second).Equal(dirTime) {
		t.Errorf("dir1 mtime = %v, want %v", dir1Info.ModTime(), dirTime)
	}

	subInfo, err := os.Stat(filepath.Join(dstDir, "dir1", "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if !subInfo.ModTime().Truncate(time.Second).Equal(subTime) {
		t.Errorf("dir1/sub mtime = %v, want %v", subInfo.ModTime(), subTime)
	}

	// File timestamps too.
	fileInfo, err := os.Stat(filepath.Join(dstDir, "dir1", "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !fileInfo.ModTime().Truncate(time.Second).Equal(fileTime) {
		t.Errorf("a.txt mtime = %v, want %v", fileInfo.ModTime(), fileTime)
	}
}

// TestApplyRoundTrip verifies that materializing a manifest and rescanning
// produces an identical manifest (same C4 IDs for all entries).
func TestApplyRoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Create source files.
	id1 := writeFile(t, srcDir, "hello.txt", "hello world")
	id2 := writeFile(t, srcDir, "sub/data.bin", "binary data here")

	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "hello.txt", Mode: 0644, Size: 11, C4ID: id1, Timestamp: ts, Depth: 0})
	target.AddEntry(&c4m.Entry{Name: "sub/", Mode: os.ModeDir | 0755, Size: 16, Timestamp: ts, Depth: 0})
	target.AddEntry(&c4m.Entry{Name: "data.bin", Mode: 0644, Size: 16, C4ID: id2, Timestamp: ts, Depth: 1})

	srcManifest := c4m.NewManifest()
	srcManifest.AddEntry(&c4m.Entry{Name: "hello.txt", Mode: 0644, Size: 11, C4ID: id1, Timestamp: ts, Depth: 0})
	srcManifest.AddEntry(&c4m.Entry{Name: "sub/", Mode: os.ModeDir | 0755, Size: 16, Timestamp: ts, Depth: 0})
	srcManifest.AddEntry(&c4m.Entry{Name: "data.bin", Mode: 0644, Size: 16, C4ID: id2, Timestamp: ts, Depth: 1})

	// Apply target to empty directory.
	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))
	plan, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rec.Apply(plan, dstDir)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file content round-trips.
	data1, err := os.ReadFile(filepath.Join(dstDir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data1) != "hello world" {
		t.Fatalf("hello.txt content = %q, want %q", data1, "hello world")
	}

	data2, err := os.ReadFile(filepath.Join(dstDir, "sub", "data.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "binary data here" {
		t.Fatalf("data.bin content = %q, want %q", data2, "binary data here")
	}

	// Verify a second plan has zero operations (already matches).
	plan2, err := rec.Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan2.Operations) != 0 {
		for _, op := range plan2.Operations {
			t.Logf("  unexpected op: type=%d path=%s", op.Type, op.Path)
		}
		t.Fatalf("round-trip: expected 0 operations, got %d", len(plan2.Operations))
	}
}
