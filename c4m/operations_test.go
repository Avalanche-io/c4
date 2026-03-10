package c4m

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func createTestEntry(name string, size int64) *Entry {
	return &Entry{
		Name:      name,
		Mode:      0644,
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Size:      size,
		C4ID:      c4.Identify(strings.NewReader(name)), // Use name for unique IDs
	}
}

func createTestManifest(names ...string) *Manifest {
	m := NewManifest()
	for i, name := range names {
		m.AddEntry(createTestEntry(name, int64(i*100)))
	}
	return m
}

func TestManifestSource(t *testing.T) {
	m := createTestManifest("file1.txt", "file2.txt")
	ms := ManifestSource{Manifest: m}
	
	result, err := ms.ToManifest()
	if err != nil {
		t.Fatalf("ToManifest() error = %v", err)
	}
	
	if result != m {
		t.Error("ToManifest() should return the same manifest")
	}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name   string
		left   *Manifest
		right  *Manifest
		want   []string // Expected file names in diff
	}{
		{
			name:  "identical manifests",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file1.txt", "file2.txt"),
			want:  []string{},
		},
		{
			name:  "different files",
			left:  createTestManifest("file1.txt", "file2.txt", "file3.txt"),
			right: createTestManifest("file1.txt", "file4.txt"),
			want:  []string{"file2.txt", "file3.txt", "file4.txt"},
		},
		{
			name:  "left empty",
			left:  createTestManifest(),
			right: createTestManifest("file1.txt"),
			want:  []string{"file1.txt"},
		},
		{
			name:  "right empty",
			left:  createTestManifest("file1.txt"),
			right: createTestManifest(),
			want:  []string{"file1.txt"},
		},
		{
			name: "same name different content",
			left: &Manifest{
				Entries: []*Entry{
					{
						Name:      "file1.txt",
						Mode:      0644,
						Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
						Size:      100,
						C4ID:      c4.Identify(strings.NewReader("content1")),
					},
				},
			},
			right: &Manifest{
				Entries: []*Entry{
					{
						Name:      "file1.txt",
						Mode:      0644,
						Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
						Size:      100,
						C4ID:      c4.Identify(strings.NewReader("content2")),
					},
				},
			},
			want: []string{"file1.txt"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms1 := ManifestSource{Manifest: tt.left}
			ms2 := ManifestSource{Manifest: tt.right}
			
			diff, err := Diff(&ms1, &ms2)
			if err != nil {
				t.Fatalf("Diff() error = %v", err)
			}
			
			// Collect all changed entries (added, removed, modified)
			var allChanges []*Entry
			allChanges = append(allChanges, diff.Added.Entries...)
			allChanges = append(allChanges, diff.Removed.Entries...)
			allChanges = append(allChanges, diff.Modified.Entries...)
			
			if len(allChanges) != len(tt.want) {
				t.Errorf("Diff returned %d changes, want %d", len(allChanges), len(tt.want))
			}
			
			// Check that all expected files are in the diff
			diffNames := make(map[string]bool)
			for _, e := range allChanges {
				diffNames[e.Name] = true
			}
			
			for _, name := range tt.want {
				if !diffNames[name] {
					t.Errorf("Expected %q in diff, not found", name)
				}
			}
		})
	}
}

func TestDiffResultIsEmpty(t *testing.T) {
	tests := []struct {
		name string
		diff *DiffResult
		want bool
	}{
		{
			name: "empty diff",
			diff: &DiffResult{
				Added:    NewManifest(),
				Removed:  NewManifest(),
				Modified: NewManifest(),
				Same:     NewManifest(),
			},
			want: true,
		},
		{
			name: "diff with additions",
			diff: &DiffResult{
				Added:    createTestManifest("file1.txt"),
				Removed:  NewManifest(),
				Modified: NewManifest(),
				Same:     NewManifest(),
			},
			want: false,
		},
		{
			name: "diff with removals",
			diff: &DiffResult{
				Added:    NewManifest(),
				Removed:  createTestManifest("file1.txt"),
				Modified: NewManifest(),
				Same:     NewManifest(),
			},
			want: false,
		},
		{
			name: "diff with modifications",
			diff: &DiffResult{
				Added:    NewManifest(),
				Removed:  NewManifest(),
				Modified: createTestManifest("file1.txt"),
				Same:     NewManifest(),
			},
			want: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.diff.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathList(t *testing.T) {
	m := NewManifest()
	m.AddEntry(&Entry{Name: "file1.txt", Size: 0, Depth: 0})
	m.AddEntry(&Entry{Name: "dir/", Size: 0, Depth: 0, Mode: os.ModeDir})
	m.AddEntry(&Entry{Name: "file2.txt", Size: 100, Depth: 1})
	m.AddEntry(&Entry{Name: "sub/", Size: 0, Depth: 1, Mode: os.ModeDir})
	m.AddEntry(&Entry{Name: "file3.txt", Size: 200, Depth: 2})

	paths := m.PathList()

	// PathList returns bare entry names sorted
	expected := []string{"dir/", "file1.txt", "file2.txt", "file3.txt", "sub/"}
	if len(paths) != len(expected) {
		t.Errorf("PathList() returned %d paths, want %d", len(paths), len(expected))
	}

	for i, p := range paths {
		if i < len(expected) && p != expected[i] {
			t.Errorf("PathList()[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

func TestFilterByPath(t *testing.T) {
	// Use bare names (entries have proper depth, not path-like names)
	m := createTestManifest("file1.txt", "file2.txt", "file3.txt", "test.doc")

	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "filter txt files",
			pattern: "*.txt",
			want:    []string{"file1.txt", "file2.txt", "file3.txt"},
		},
		{
			name:    "filter all files",
			pattern: "*",
			want:    []string{"file1.txt", "file2.txt", "file3.txt", "test.doc"},
		},
		{
			name:    "filter specific file",
			pattern: "file1.txt",
			want:    []string{"file1.txt"},
		},
		{
			name:    "no matches",
			pattern: "*.pdf",
			want:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := m.FilterByPath(tt.pattern)

			if len(filtered.Entries) != len(tt.want) {
				t.Errorf("FilterByPath(%q) returned %d entries, want %d", tt.pattern, len(filtered.Entries), len(tt.want))
			}

			filteredNames := make(map[string]bool)
			for _, e := range filtered.Entries {
				filteredNames[e.Name] = true
			}

			for _, name := range tt.want {
				if !filteredNames[name] {
					t.Errorf("Expected %q in filtered result, not found", name)
				}
			}
		})
	}
}

func TestFilterByPrefix(t *testing.T) {
	// Use bare names (no path separators in entry names)
	m := createTestManifest("file1.txt", "file2.txt", "file3.txt", "file4.txt", "other.txt")

	tests := []struct {
		name   string
		prefix string
		want   []string
	}{
		{
			name:   "filter by file prefix",
			prefix: "file",
			want:   []string{"file1.txt", "file2.txt", "file3.txt", "file4.txt"},
		},
		{
			name:   "no matches",
			prefix: "nonexistent",
			want:   []string{},
		},
		{
			name:   "empty prefix matches all",
			prefix: "",
			want:   []string{"file1.txt", "file2.txt", "file3.txt", "file4.txt", "other.txt"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := m.FilterByPrefix(tt.prefix)
			
			if len(filtered.Entries) != len(tt.want) {
				t.Errorf("FilterByPrefix(%q) returned %d entries, want %d", tt.prefix, len(filtered.Entries), len(tt.want))
			}
			
			filteredNames := make(map[string]bool)
			for _, e := range filtered.Entries {
				filteredNames[e.Name] = true
			}
			
			for _, name := range tt.want {
				if !filteredNames[name] {
					t.Errorf("Expected %q in filtered result, not found", name)
				}
			}
		})
	}
}

func TestEntriesEqual(t *testing.T) {
	testTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	testID := c4.Identify(strings.NewReader("test"))
	
	tests := []struct {
		name string
		e1   *Entry
		e2   *Entry
		want bool
	}{
		{
			name: "identical entries",
			e1: &Entry{
				Name:      "file.txt",
				Mode:      0644,
				Timestamp: testTime,
				Size:      100,
				C4ID:      testID,
			},
			e2: &Entry{
				Name:      "file.txt",
				Mode:      0644,
				Timestamp: testTime,
				Size:      100,
				C4ID:      testID,
			},
			want: true,
		},
		{
			name: "different names",
			e1:   &Entry{Name: "file1.txt", Mode: 0644, Timestamp: testTime, Size: 100, C4ID: testID},
			e2:   &Entry{Name: "file2.txt", Mode: 0644, Timestamp: testTime, Size: 100, C4ID: testID},
			want: false,
		},
		{
			name: "different modes",
			e1:   &Entry{Name: "file.txt", Mode: 0644, Timestamp: testTime, Size: 100, C4ID: testID},
			e2:   &Entry{Name: "file.txt", Mode: 0755, Timestamp: testTime, Size: 100, C4ID: testID},
			want: false,
		},
		{
			name: "different C4 IDs",
			e1:   &Entry{Name: "file.txt", Mode: 0644, Timestamp: testTime, Size: 100, C4ID: testID},
			e2:   &Entry{Name: "file.txt", Mode: 0644, Timestamp: testTime, Size: 100, C4ID: c4.Identify(strings.NewReader("other"))},
			want: false,
		},
		{
			name: "different timestamps ignored if C4 IDs match",
			e1:   &Entry{Name: "file.txt", Mode: 0644, Timestamp: testTime, Size: 100, C4ID: testID},
			e2:   &Entry{Name: "file.txt", Mode: 0644, Timestamp: testTime.Add(time.Hour), Size: 100, C4ID: testID},
			want: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entriesEqual(tt.e1, tt.e2); got != tt.want {
				t.Errorf("entriesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// PatchDiff Tests
// ----------------------------------------------------------------------------

func makeDir(name string, id c4.ID, depth int) *Entry {
	return &Entry{
		Name:      name,
		Mode:      os.ModeDir | 0755,
		Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		Size:      -1,
		C4ID:      id,
		Depth:     depth,
	}
}

func makeFile(name string, content string, depth int) *Entry {
	return &Entry{
		Name:      name,
		Mode:      0644,
		Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		Size:      int64(len(content)),
		C4ID:      c4.Identify(strings.NewReader(content)),
		Depth:     depth,
	}
}

func TestPatchDiffIdentical(t *testing.T) {
	m := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeFile("b.txt", "bbb", 0),
	}}
	result := PatchDiff(m, m)
	if !result.IsEmpty() {
		t.Errorf("expected empty patch for identical manifests, got %d entries", len(result.Patch.Entries))
	}
}

func TestPatchDiffAddition(t *testing.T) {
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}
	new := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeFile("b.txt", "bbb", 0),
	}}
	result := PatchDiff(old, new)
	if result.IsEmpty() {
		t.Fatal("expected non-empty patch")
	}
	if len(result.Patch.Entries) != 1 {
		t.Fatalf("expected 1 patch entry, got %d", len(result.Patch.Entries))
	}
	if result.Patch.Entries[0].Name != "b.txt" {
		t.Errorf("expected addition of b.txt, got %s", result.Patch.Entries[0].Name)
	}
}

func TestPatchDiffRemoval(t *testing.T) {
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeFile("b.txt", "bbb", 0),
	}}
	new := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}
	result := PatchDiff(old, new)
	if len(result.Patch.Entries) != 1 {
		t.Fatalf("expected 1 patch entry (removal), got %d", len(result.Patch.Entries))
	}
	// Removal re-emits the old entry
	if result.Patch.Entries[0].Name != "b.txt" {
		t.Errorf("expected removal of b.txt, got %s", result.Patch.Entries[0].Name)
	}
	if result.Patch.Entries[0].C4ID != c4.Identify(strings.NewReader("bbb")) {
		t.Error("removal entry should have the OLD C4 ID")
	}
}

func TestPatchDiffModification(t *testing.T) {
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "old content", 0),
	}}
	new := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "new content", 0),
	}}
	result := PatchDiff(old, new)
	if len(result.Patch.Entries) != 1 {
		t.Fatalf("expected 1 patch entry (modification), got %d", len(result.Patch.Entries))
	}
	// Modification emits the NEW entry (clobber)
	if result.Patch.Entries[0].C4ID != c4.Identify(strings.NewReader("new content")) {
		t.Error("modification entry should have the NEW C4 ID")
	}
}

func TestPatchDiffMetadataOnly(t *testing.T) {
	content := "same content"
	id := c4.Identify(strings.NewReader(content))

	old := &Manifest{Entries: []*Entry{{
		Name:      "a.txt",
		Mode:      0644,
		Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		Size:      int64(len(content)),
		C4ID:      id,
		Depth:     0,
	}}}

	// Same content (same C4 ID) but different mode and timestamp (touch + chmod)
	new := &Manifest{Entries: []*Entry{{
		Name:      "a.txt",
		Mode:      0755,
		Timestamp: time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC),
		Size:      int64(len(content)),
		C4ID:      id,
		Depth:     0,
	}}}

	result := PatchDiff(old, new)
	if len(result.Patch.Entries) != 1 {
		t.Fatalf("expected 1 patch entry for metadata change, got %d", len(result.Patch.Entries))
	}
	pe := result.Patch.Entries[0]
	if pe.Mode != 0755 {
		t.Errorf("patch entry mode = %v, want 0755", pe.Mode)
	}
	if !pe.Timestamp.Equal(time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("patch entry timestamp = %v, want 2026-03-07T08:00:00Z", pe.Timestamp)
	}
	if pe.C4ID != id {
		t.Error("patch entry should preserve original C4 ID")
	}

	// Round-trip: applying the patch should produce the new state
	applied := ApplyPatch(old, result.Patch)
	if len(applied.Entries) != 1 {
		t.Fatalf("applied manifest has %d entries, want 1", len(applied.Entries))
	}
	ae := applied.Entries[0]
	if ae.Mode != 0755 {
		t.Errorf("applied mode = %v, want 0755", ae.Mode)
	}
	if !ae.Timestamp.Equal(time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("applied timestamp = %v, want 2026-03-07T08:00:00Z", ae.Timestamp)
	}
}

func TestPatchDiffNested(t *testing.T) {
	// Old: src/ contains main.go and util.go
	srcOldID := c4.Identify(strings.NewReader("src-old"))
	old := &Manifest{Entries: []*Entry{
		makeFile("README.md", "readme", 0),
		makeDir("src/", srcOldID, 0),
		makeFile("main.go", "main-old", 1),
		makeFile("util.go", "util", 1),
	}}

	// New: src/ contains main.go (modified) and util.go (unchanged)
	srcNewID := c4.Identify(strings.NewReader("src-new"))
	new := &Manifest{Entries: []*Entry{
		makeFile("README.md", "readme", 0),
		makeDir("src/", srcNewID, 0),
		makeFile("main.go", "main-new", 1),
		makeFile("util.go", "util", 1),
	}}

	result := PatchDiff(old, new)

	// Should emit: src/ (new ID) then main.go (new content)
	// README.md is unchanged (same C4 ID), util.go is unchanged
	if len(result.Patch.Entries) != 2 {
		t.Fatalf("expected 2 patch entries (dir + modified file), got %d", len(result.Patch.Entries))
	}

	if result.Patch.Entries[0].Name != "src/" {
		t.Errorf("first entry should be src/, got %s", result.Patch.Entries[0].Name)
	}
	if result.Patch.Entries[0].Depth != 0 {
		t.Errorf("src/ should be at depth 0, got %d", result.Patch.Entries[0].Depth)
	}

	if result.Patch.Entries[1].Name != "main.go" {
		t.Errorf("second entry should be main.go, got %s", result.Patch.Entries[1].Name)
	}
	if result.Patch.Entries[1].Depth != 1 {
		t.Errorf("main.go should be at depth 1, got %d", result.Patch.Entries[1].Depth)
	}
}

func TestPatchDiffSkipsIdenticalSubtrees(t *testing.T) {
	// Both manifests have the same src/ directory (same C4 ID)
	srcID := c4.Identify(strings.NewReader("src-shared"))
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeDir("src/", srcID, 0),
		makeFile("main.go", "main", 1),
		makeFile("util.go", "util", 1),
	}}
	new := &Manifest{Entries: []*Entry{
		makeFile("b.txt", "bbb", 0),
		makeDir("src/", srcID, 0),
		makeFile("main.go", "main", 1),
		makeFile("util.go", "util", 1),
	}}

	result := PatchDiff(old, new)

	// Should emit: a.txt (removal) and b.txt (addition)
	// src/ is skipped entirely (same C4 ID)
	if len(result.Patch.Entries) != 2 {
		t.Fatalf("expected 2 patch entries, got %d", len(result.Patch.Entries))
	}

	names := map[string]bool{}
	for _, e := range result.Patch.Entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("expected a.txt and b.txt in patch, got %v", names)
	}
}

func TestPatchDiffAddedDirectory(t *testing.T) {
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}

	newDirID := c4.Identify(strings.NewReader("newdir"))
	new := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeDir("docs/", newDirID, 0),
		makeFile("README.md", "readme", 1),
		makeFile("guide.md", "guide", 1),
	}}

	result := PatchDiff(old, new)

	// Should emit: docs/ + README.md + guide.md (dir + full subtree)
	if len(result.Patch.Entries) != 3 {
		t.Fatalf("expected 3 patch entries (dir + 2 files), got %d", len(result.Patch.Entries))
	}

	if result.Patch.Entries[0].Name != "docs/" {
		t.Errorf("first entry should be docs/, got %s", result.Patch.Entries[0].Name)
	}
	if result.Patch.Entries[0].Depth != 0 {
		t.Errorf("docs/ should be at depth 0")
	}
	// Children should be at depth 1
	for _, e := range result.Patch.Entries[1:] {
		if e.Depth != 1 {
			t.Errorf("child %s should be at depth 1, got %d", e.Name, e.Depth)
		}
	}
}

func TestPatchDiffPageBoundaries(t *testing.T) {
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}
	new := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa-new", 0),
	}}

	result := PatchDiff(old, new)

	// OldID and NewID should be different (content changed)
	if result.OldID == result.NewID {
		t.Error("OldID and NewID should differ for modified manifest")
	}

	// Both should be non-nil
	if result.OldID.IsNil() || result.NewID.IsNil() {
		t.Error("page boundary IDs should not be nil")
	}
}

func TestPatchDiffFilesBeforeDirs(t *testing.T) {
	dirID := c4.Identify(strings.NewReader("dir"))
	old := &Manifest{Entries: []*Entry{}}
	new := &Manifest{Entries: []*Entry{
		makeFile("z.txt", "zzz", 0),
		makeDir("a-dir/", dirID, 0),
		makeFile("a.txt", "aaa", 0),
	}}

	result := PatchDiff(old, new)

	// Files should come before directories in patch output
	if len(result.Patch.Entries) < 3 {
		t.Fatalf("expected at least 3 entries, got %d", len(result.Patch.Entries))
	}

	// First two should be files, last should be directory
	if result.Patch.Entries[0].IsDir() || result.Patch.Entries[1].IsDir() {
		t.Error("files should appear before directories in patch")
	}
	if !result.Patch.Entries[2].IsDir() {
		t.Error("directory should appear after files in patch")
	}
}

// ----------------------------------------------------------------------------
// ApplyPatch Tests
// ----------------------------------------------------------------------------

func TestApplyPatchRoundTrip(t *testing.T) {
	// Verify: ApplyPatch(base, PatchDiff(base, target).Patch) == target
	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeFile("b.txt", "bbb", 0),
		makeFile("c.txt", "ccc", 0),
	}}
	target := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa-new", 0),
		makeFile("c.txt", "ccc", 0),
		makeFile("d.txt", "ddd", 0),
	}}

	patch := PatchDiff(old, target)
	result := ApplyPatch(old, patch.Patch)

	// Result should match target
	if len(result.Entries) != len(target.Entries) {
		t.Fatalf("expected %d entries, got %d", len(target.Entries), len(result.Entries))
	}

	for i, e := range result.Entries {
		te := target.Entries[i]
		if e.Name != te.Name {
			t.Errorf("entry %d: name %q != %q", i, e.Name, te.Name)
		}
		if e.C4ID != te.C4ID {
			t.Errorf("entry %d (%s): C4 ID mismatch", i, e.Name)
		}
	}
}

func TestApplyPatchNestedRoundTrip(t *testing.T) {
	srcOldID := c4.Identify(strings.NewReader("src-old"))
	srcNewID := c4.Identify(strings.NewReader("src-new"))

	old := &Manifest{Entries: []*Entry{
		makeFile("README.md", "readme", 0),
		makeDir("src/", srcOldID, 0),
		makeFile("main.go", "main-old", 1),
		makeFile("util.go", "util", 1),
	}}

	target := &Manifest{Entries: []*Entry{
		makeFile("README.md", "readme", 0),
		makeDir("src/", srcNewID, 0),
		makeFile("main.go", "main-new", 1),
		makeFile("util.go", "util", 1),
	}}

	patch := PatchDiff(old, target)
	result := ApplyPatch(old, patch.Patch)

	// Should have same entries as target
	if len(result.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(result.Entries))
	}

	// Find main.go in result
	var mainEntry *Entry
	for _, e := range result.Entries {
		if e.Name == "main.go" {
			mainEntry = e
			break
		}
	}
	if mainEntry == nil {
		t.Fatal("main.go not found in result")
	}
	if mainEntry.C4ID != c4.Identify(strings.NewReader("main-new")) {
		t.Error("main.go should have the new C4 ID after patch")
	}

	// util.go should be unchanged
	var utilEntry *Entry
	for _, e := range result.Entries {
		if e.Name == "util.go" {
			utilEntry = e
			break
		}
	}
	if utilEntry == nil {
		t.Fatal("util.go not found in result")
	}
	if utilEntry.C4ID != c4.Identify(strings.NewReader("util")) {
		t.Error("util.go should be unchanged after patch")
	}
}

func TestApplyPatchDirectoryRemoval(t *testing.T) {
	dirID := c4.Identify(strings.NewReader("dir"))

	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeDir("docs/", dirID, 0),
		makeFile("readme.md", "readme", 1),
		makeFile("guide.md", "guide", 1),
	}}

	target := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}

	patch := PatchDiff(old, target)
	result := ApplyPatch(old, patch.Patch)

	// Should only have a.txt
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry after removing docs/, got %d", len(result.Entries))
	}
	if result.Entries[0].Name != "a.txt" {
		t.Errorf("expected a.txt, got %s", result.Entries[0].Name)
	}
}

func TestApplyPatchDirectoryAddition(t *testing.T) {
	dirID := c4.Identify(strings.NewReader("newdir"))

	old := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}

	target := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
		makeDir("docs/", dirID, 0),
		makeFile("readme.md", "readme", 1),
	}}

	patch := PatchDiff(old, target)
	result := ApplyPatch(old, patch.Patch)

	// Should have a.txt + docs/ + readme.md
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}
}

func TestApplyPatchEmptyPatch(t *testing.T) {
	m := &Manifest{Entries: []*Entry{
		makeFile("a.txt", "aaa", 0),
	}}

	patch := PatchDiff(m, m)
	if !patch.IsEmpty() {
		t.Fatal("expected empty patch for identical manifests")
	}

	result := ApplyPatch(m, patch.Patch)
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
}

// ----------------------------------------------------------------------------
// Resolver Tests (merged from resolver_test.go)
// ----------------------------------------------------------------------------

// mockStorage implements store.Source for testing
type mockStorage struct {
	manifests map[string]string // C4 ID -> manifest content
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		manifests: make(map[string]string),
	}
}

func (ms *mockStorage) Open(id c4.ID) (io.ReadCloser, error) {
	content, ok := ms.manifests[id.String()]
	if !ok {
		return nil, fmt.Errorf("manifest not found: %s", id.String())
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (ms *mockStorage) addManifest(content string) c4.ID {
	id := c4.Identify(strings.NewReader(content))
	ms.manifests[id.String()] = content
	return id
}

func TestResolverBasicFileResolution(t *testing.T) {
	storage := newMockStorage()

	// Create a C4 ID for test content
	fileContentID := c4.Identify(strings.NewReader("test file content"))

	// Create a simple manifest with one file
	manifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 100 file.txt %s
`, fileContentID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving the file
	result, err := resolver.Resolve(rootID, "file.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file: %v", err)
	}

	if result.IsDir {
		t.Error("Expected file, got directory")
	}

	if result.ID != fileContentID {
		t.Errorf("Expected ID %s, got %s", fileContentID, result.ID)
	}
}

func TestResolverBasicDirectoryResolution(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for subdirectory file
	subfileContentID := c4.Identify(strings.NewReader("subfile content"))

	// Create subdirectory manifest
	subManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 50 subfile.txt %s
`, subfileContentID.String())
	subID := storage.addManifest(subManifest)

	// Create root manifest with directory
	rootManifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 50 subdir/ %s
`, subID.String())
	rootID := storage.addManifest(rootManifest)

	resolver := NewResolver(storage)

	// Test resolving the directory
	result, err := resolver.Resolve(rootID, "subdir")
	if err != nil {
		t.Fatalf("Failed to resolve directory: %v", err)
	}

	if !result.IsDir {
		t.Error("Expected directory, got file")
	}

	if result.ID != subID {
		t.Errorf("Expected ID %s, got %s", subID, result.ID)
	}

	if result.Manifest == nil {
		t.Error("Expected manifest to be loaded for directory")
	}
}

func TestResolverNestedPathResolution(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for deep file
	deepfileContentID := c4.Identify(strings.NewReader("deep file content"))

	// Create deepest level manifest
	deepManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 25 deep.txt %s
`, deepfileContentID.String())
	deepID := storage.addManifest(deepManifest)

	// Create middle level manifest
	midManifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 25 level3/ %s
`, deepID.String())
	midID := storage.addManifest(midManifest)

	// Create second level manifest
	level2Manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 25 level2/ %s
`, midID.String())
	level2ID := storage.addManifest(level2Manifest)

	// Create root manifest
	rootManifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 25 level1/ %s
`, level2ID.String())
	rootID := storage.addManifest(rootManifest)

	resolver := NewResolver(storage)

	// Test resolving nested file path
	result, err := resolver.Resolve(rootID, "level1/level2/level3/deep.txt")
	if err != nil {
		t.Fatalf("Failed to resolve nested path: %v", err)
	}

	if result.IsDir {
		t.Error("Expected file, got directory")
	}

	if result.ID != deepfileContentID {
		t.Errorf("Expected ID %s, got %s", deepfileContentID, result.ID)
	}
}

func TestResolverRootPathResolution(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for file
	fileContentID := c4.Identify(strings.NewReader("file content"))

	// Create a simple manifest
	manifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 100 file.txt %s
`, fileContentID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving root path with empty string
	result, err := resolver.Resolve(rootID, "")
	if err != nil {
		t.Fatalf("Failed to resolve root path: %v", err)
	}

	if !result.IsDir {
		t.Error("Expected root to be directory")
	}

	if result.ID != rootID {
		t.Errorf("Expected root ID %s, got %s", rootID, result.ID)
	}

	if result.Manifest == nil {
		t.Error("Expected manifest to be loaded for root")
	}

	// Test with leading/trailing slashes
	result, err = resolver.Resolve(rootID, "/")
	if err != nil {
		t.Fatalf("Failed to resolve root path with slash: %v", err)
	}

	if result.ID != rootID {
		t.Errorf("Expected root ID %s, got %s", rootID, result.ID)
	}
}

func TestResolverPathNotFound(t *testing.T) {
	storage := newMockStorage()

	// Create C4 IDs for files
	file1ContentID := c4.Identify(strings.NewReader("file1 content"))
	file2ContentID := c4.Identify(strings.NewReader("file2 content"))

	// Create a manifest with specific files
	manifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 100 file1.txt %s
-rw-r--r-- 2025-01-15T10:00:00Z 200 file2.txt %s
`, file1ContentID.String(), file2ContentID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving non-existent file
	_, err := resolver.Resolve(rootID, "nonexistent.txt")
	if err == nil {
		t.Fatal("Expected error for non-existent path")
	}

	// Check that error message contains available entries
	errMsg := err.Error()
	if !strings.Contains(errMsg, "path not found") {
		t.Errorf("Expected 'path not found' in error, got: %s", errMsg)
	}

	if !strings.Contains(errMsg, "file1.txt") || !strings.Contains(errMsg, "file2.txt") {
		t.Errorf("Expected available entries in error message, got: %s", errMsg)
	}
}

func TestResolverCannotTraverseThroughFile(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for file
	fileContentID := c4.Identify(strings.NewReader("file content"))

	// Create a manifest with a file
	manifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 100 file.txt %s
`, fileContentID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Try to traverse through the file
	_, err := resolver.Resolve(rootID, "file.txt/something")
	if err == nil {
		t.Fatal("Expected error when traversing through file")
	}

	if !strings.Contains(err.Error(), "cannot traverse through file") {
		t.Errorf("Expected 'cannot traverse through file' error, got: %s", err.Error())
	}
}

func TestResolverManifestCaching(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for subdirectory file
	subfileContentID := c4.Identify(strings.NewReader("subfile content"))

	// Create subdirectory manifest
	subManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 50 subfile.txt %s
`, subfileContentID.String())
	subID := storage.addManifest(subManifest)

	// Create root manifest
	rootManifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 50 subdir/ %s
`, subID.String())
	rootID := storage.addManifest(rootManifest)

	resolver := NewResolver(storage)

	// First resolution - loads from storage
	result1, err := resolver.Resolve(rootID, "subdir")
	if err != nil {
		t.Fatalf("First resolution failed: %v", err)
	}

	// Remove manifest from storage to test caching
	delete(storage.manifests, rootID.String())

	// Second resolution - should use cache
	result2, err := resolver.Resolve(rootID, "subdir")
	if err != nil {
		t.Fatalf("Second resolution failed (cache not working): %v", err)
	}

	if result1.ID != result2.ID {
		t.Error("Cache returned different result")
	}
}

func TestResolverNilRootManifest(t *testing.T) {
	storage := newMockStorage()
	resolver := NewResolver(storage)

	// Test with nil root manifest ID
	nilID := c4.ID{}
	_, err := resolver.Resolve(nilID, "anything")
	if err == nil {
		t.Fatal("Expected error for nil root manifest ID")
	}

	if !strings.Contains(err.Error(), "nil root manifest") {
		t.Errorf("Expected 'nil root manifest' error, got: %s", err.Error())
	}
}

func TestResolverDirectoryWithAndWithoutSlash(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for subdirectory file
	fileContentID := c4.Identify(strings.NewReader("file content"))

	// Create subdirectory manifest
	subManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 50 file.txt %s
`, fileContentID.String())
	subID := storage.addManifest(subManifest)

	// Create root manifest with directory (with trailing slash)
	rootManifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 50 mydir/ %s
`, subID.String())
	rootID := storage.addManifest(rootManifest)

	resolver := NewResolver(storage)

	// Test resolving directory without trailing slash
	result1, err := resolver.Resolve(rootID, "mydir")
	if err != nil {
		t.Fatalf("Failed to resolve directory without slash: %v", err)
	}

	if !result1.IsDir {
		t.Error("Expected directory")
	}

	// Test resolving directory with trailing slash in path
	result2, err := resolver.Resolve(rootID, "mydir/")
	if err != nil {
		t.Fatalf("Failed to resolve directory with slash: %v", err)
	}

	if !result2.IsDir {
		t.Error("Expected directory")
	}

	// Both should resolve to same ID
	if result1.ID != result2.ID {
		t.Error("Directory resolution should be same with or without trailing slash")
	}
}

func TestManifestCacheClear(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for file
	fileContentID := c4.Identify(strings.NewReader("file content"))

	// Create a simple manifest
	manifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 100 file.txt %s
`, fileContentID.String())
	id := storage.addManifest(manifest)

	cache := NewManifestCache(storage)

	// Load manifest into cache
	_, err := cache.Get(id)
	if err != nil {
		t.Fatalf("Failed to load manifest: %v", err)
	}

	// Verify it's cached
	cache.mu.RLock()
	_, cached := cache.cache[id.String()]
	cache.mu.RUnlock()

	if !cached {
		t.Error("Manifest should be in cache")
	}

	// Clear cache
	cache.Clear()

	// Verify cache is empty
	cache.mu.RLock()
	_, stillCached := cache.cache[id.String()]
	cache.mu.RUnlock()

	if stillCached {
		t.Error("Cache should be empty after Clear()")
	}
}

func TestResolverMixedContentDirectory(t *testing.T) {
	storage := newMockStorage()

	// Create C4 IDs for all files
	nestedContentID := c4.Identify(strings.NewReader("nested content"))
	file1ContentID := c4.Identify(strings.NewReader("file1 content"))
	file2ContentID := c4.Identify(strings.NewReader("file2 content"))

	// Create subdirectory manifest
	subManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 25 nested.txt %s
`, nestedContentID.String())
	subID := storage.addManifest(subManifest)

	// Create root manifest with both files and directories
	rootManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 100 file1.txt %s
-rw-r--r-- 2025-01-15T10:00:00Z 200 file2.txt %s
drwxr-xr-x 2025-01-15T10:00:00Z 25 subdir/ %s
`, file1ContentID.String(), file2ContentID.String(), subID.String())
	rootID := storage.addManifest(rootManifest)

	resolver := NewResolver(storage)

	// Test resolving file in root
	result, err := resolver.Resolve(rootID, "file1.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file: %v", err)
	}
	if result.IsDir {
		t.Error("Expected file, got directory")
	}

	// Test resolving directory
	result, err = resolver.Resolve(rootID, "subdir")
	if err != nil {
		t.Fatalf("Failed to resolve directory: %v", err)
	}
	if !result.IsDir {
		t.Error("Expected directory, got file")
	}

	// Test resolving file in subdirectory
	result, err = resolver.Resolve(rootID, "subdir/nested.txt")
	if err != nil {
		t.Fatalf("Failed to resolve nested file: %v", err)
	}
	if result.IsDir {
		t.Error("Expected file, got directory")
	}
}

func TestResolverStorageError(t *testing.T) {
	storage := newMockStorage()

	// Create a manifest that references a non-existent subdirectory
	fakeID := c4.Identify(strings.NewReader("fake"))
	rootManifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 baddir/ %s
`, fakeID.String())
	rootID := storage.addManifest(rootManifest)

	resolver := NewResolver(storage)

	// Try to resolve into the directory with missing manifest
	_, err := resolver.Resolve(rootID, "baddir")
	if err == nil {
		t.Fatal("Expected error when loading missing manifest")
	}

	if !strings.Contains(err.Error(), "loading manifest") {
		t.Errorf("Expected 'loading manifest' error, got: %s", err.Error())
	}
}

func TestResolverRealManifestGeneration(t *testing.T) {
	// Test with actual manifest generation and C4 IDs
	storage := newMockStorage()

	// Create a real manifest using the generator
	m := NewManifest()
	m.AddEntry(&Entry{
		Name:      "readme.txt",
		Mode:      0644,
		Size:      100,
		Timestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		C4ID:      c4.Identify(strings.NewReader("readme content")),
	})
	m.AddEntry(&Entry{
		Name:      "docs/",
		Mode:      0755 | (1 << 31), // directory bit
		Size:      50,
		Timestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		C4ID:      c4.Identify(strings.NewReader("dummy")), // Will be replaced with actual manifest ID
	})

	// Generate canonical form
	var buf bytes.Buffer
	err := NewEncoder(&buf).Encode(m)
	if err != nil {
		t.Fatalf("Failed to encode manifest: %v", err)
	}

	// Calculate actual C4 ID
	manifestID := c4.Identify(bytes.NewReader(buf.Bytes()))
	storage.manifests[manifestID.String()] = buf.String()

	resolver := NewResolver(storage)

	// Resolve the file
	result, err := resolver.Resolve(manifestID, "readme.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file: %v", err)
	}

	if result.IsDir {
		t.Error("Expected file, got directory")
	}

	expectedID := c4.Identify(strings.NewReader("readme content"))
	if result.ID != expectedID {
		t.Errorf("Expected ID %s, got %s", expectedID, result.ID)
	}
}

// TestResolverFlatManifestBasic tests basic flat manifest with depth-based nesting
func TestResolverFlatManifestBasic(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for file
	fileContentID := c4.Identify(strings.NewReader("final.txt content"))

	// Create a flat manifest with all entries at various depths
	// This simulates: projects/2024/renders/final.txt
	manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 projects/
  drwxr-xr-x 2025-01-15T10:00:00Z 0 2024/
    drwxr-xr-x 2025-01-15T10:00:00Z 0 renders/
      -rw-r--r-- 2025-01-15T10:00:00Z 100 final.txt %s
`, fileContentID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving the file through flat manifest
	result, err := resolver.Resolve(rootID, "projects/2024/renders/final.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file in flat manifest: %v", err)
	}

	if result.IsDir {
		t.Error("Expected file, got directory")
	}

	if result.ID != fileContentID {
		t.Errorf("Expected ID %s, got %s", fileContentID, result.ID)
	}
}

// TestResolverFlatManifestDirectory tests resolving to a directory in flat manifest
func TestResolverFlatManifestDirectory(t *testing.T) {
	storage := newMockStorage()

	// Create C4 IDs for files
	file1ID := c4.Identify(strings.NewReader("file1.txt content"))
	file2ID := c4.Identify(strings.NewReader("file2.txt content"))

	// Create a flat manifest with directory structure
	manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 projects/
  drwxr-xr-x 2025-01-15T10:00:00Z 0 code/
    -rw-r--r-- 2025-01-15T10:00:00Z 50 file1.txt %s
    -rw-r--r-- 2025-01-15T10:00:00Z 75 file2.txt %s
`, file1ID.String(), file2ID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving to the directory
	result, err := resolver.Resolve(rootID, "projects/code")
	if err != nil {
		t.Fatalf("Failed to resolve directory in flat manifest: %v", err)
	}

	if !result.IsDir {
		t.Error("Expected directory, got file")
	}

	// For flat manifests, the directory itself has no C4 ID
	if !result.ID.IsNil() {
		t.Error("Expected nil C4 ID for flat manifest directory")
	}

	// The manifest should be the same (contains all entries including children)
	if result.Manifest == nil {
		t.Error("Expected manifest to be present")
	}
}

// TestResolverFlatManifestMultipleLevels tests deeply nested flat manifest
func TestResolverFlatManifestMultipleLevels(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for deeply nested file
	deepFileID := c4.Identify(strings.NewReader("deep file content"))

	// Create a flat manifest with many depth levels
	manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 a/
  drwxr-xr-x 2025-01-15T10:00:00Z 0 b/
    drwxr-xr-x 2025-01-15T10:00:00Z 0 c/
      drwxr-xr-x 2025-01-15T10:00:00Z 0 d/
        drwxr-xr-x 2025-01-15T10:00:00Z 0 e/
          -rw-r--r-- 2025-01-15T10:00:00Z 25 deep.txt %s
`, deepFileID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving the deeply nested file
	result, err := resolver.Resolve(rootID, "a/b/c/d/e/deep.txt")
	if err != nil {
		t.Fatalf("Failed to resolve deeply nested file: %v", err)
	}

	if result.IsDir {
		t.Error("Expected file, got directory")
	}

	if result.ID != deepFileID {
		t.Errorf("Expected ID %s, got %s", deepFileID, result.ID)
	}
}

// TestResolverMixedHierarchicalAndFlat tests mixed manifest styles
func TestResolverMixedHierarchicalAndFlat(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for file in flat section
	flatFileID := c4.Identify(strings.NewReader("flat file content"))

	// Create a sub-manifest (hierarchical)
	subFileID := c4.Identify(strings.NewReader("hierarchical file content"))
	subManifest := fmt.Sprintf(`-rw-r--r-- 2025-01-15T10:00:00Z 50 subfile.txt %s
`, subFileID.String())
	subID := storage.addManifest(subManifest)

	// Create root manifest with both flat and hierarchical parts
	// The "flat/" directory has null C4 ID (flat manifest)
	// The "hierarchical/" directory has a C4 ID (separate manifest)
	manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 flat/
  drwxr-xr-x 2025-01-15T10:00:00Z 0 nested/
    -rw-r--r-- 2025-01-15T10:00:00Z 100 flatfile.txt %s
drwxr-xr-x 2025-01-15T10:00:00Z 50 hierarchical/ %s
`, flatFileID.String(), subID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving file in flat section
	result, err := resolver.Resolve(rootID, "flat/nested/flatfile.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file in flat section: %v", err)
	}
	if result.ID != flatFileID {
		t.Errorf("Expected ID %s, got %s", flatFileID, result.ID)
	}

	// Test resolving file in hierarchical section
	result, err = resolver.Resolve(rootID, "hierarchical/subfile.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file in hierarchical section: %v", err)
	}
	if result.ID != subFileID {
		t.Errorf("Expected ID %s, got %s", subFileID, result.ID)
	}
}

// TestResolverFlatManifestSiblings tests resolving when there are sibling directories
func TestResolverFlatManifestSiblings(t *testing.T) {
	storage := newMockStorage()

	// Create C4 IDs for files
	file1ID := c4.Identify(strings.NewReader("file1 content"))
	file2ID := c4.Identify(strings.NewReader("file2 content"))

	// Create a flat manifest with sibling directories
	manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 dir1/
  -rw-r--r-- 2025-01-15T10:00:00Z 50 file1.txt %s
drwxr-xr-x 2025-01-15T10:00:00Z 0 dir2/
  -rw-r--r-- 2025-01-15T10:00:00Z 75 file2.txt %s
`, file1ID.String(), file2ID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving file in first directory
	result, err := resolver.Resolve(rootID, "dir1/file1.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file in dir1: %v", err)
	}
	if result.ID != file1ID {
		t.Errorf("Expected ID %s, got %s", file1ID, result.ID)
	}

	// Test resolving file in second directory
	result, err = resolver.Resolve(rootID, "dir2/file2.txt")
	if err != nil {
		t.Fatalf("Failed to resolve file in dir2: %v", err)
	}
	if result.ID != file2ID {
		t.Errorf("Expected ID %s, got %s", file2ID, result.ID)
	}
}

// TestResolverFlatManifestPathNotFound tests error handling in flat manifests
func TestResolverFlatManifestPathNotFound(t *testing.T) {
	storage := newMockStorage()

	// Create C4 ID for file
	fileID := c4.Identify(strings.NewReader("file content"))

	// Create a flat manifest
	manifest := fmt.Sprintf(`drwxr-xr-x 2025-01-15T10:00:00Z 0 projects/
  drwxr-xr-x 2025-01-15T10:00:00Z 0 2024/
    -rw-r--r-- 2025-01-15T10:00:00Z 100 file.txt %s
`, fileID.String())
	rootID := storage.addManifest(manifest)

	resolver := NewResolver(storage)

	// Test resolving non-existent path
	_, err := resolver.Resolve(rootID, "projects/2025/file.txt")
	if err == nil {
		t.Fatal("Expected error for non-existent path")
	}

	if !strings.Contains(err.Error(), "path not found") {
		t.Errorf("Expected 'path not found' error, got: %s", err.Error())
	}
}
