package c4m

import (
	"os"
	"path/filepath"
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

func TestFileSource(t *testing.T) {
	// Create a temp directory with a test file
	tmpdir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	
	// Create a test file
	testFile := filepath.Join(tmpdir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	
	fs := FileSource{Path: tmpdir}
	manifest, err := fs.ToManifest()
	if err != nil {
		t.Fatalf("ToManifest() error = %v", err)
	}
	
	if len(manifest.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(manifest.Entries))
	}
	if manifest.Entries[0].Name != "test.txt" {
		t.Errorf("Entry name = %q, want %q", manifest.Entries[0].Name, "test.txt")
	}
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

func TestUnion(t *testing.T) {
	tests := []struct {
		name   string
		left   *Manifest
		right  *Manifest
		want   []string
	}{
		{
			name:  "disjoint sets",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file3.txt", "file4.txt"),
			want:  []string{"file1.txt", "file2.txt", "file3.txt", "file4.txt"},
		},
		{
			name:  "overlapping sets",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file2.txt", "file3.txt"),
			want:  []string{"file1.txt", "file2.txt", "file3.txt"},
		},
		{
			name:  "one empty",
			left:  createTestManifest(),
			right: createTestManifest("file1.txt"),
			want:  []string{"file1.txt"},
		},
		{
			name:  "both empty",
			left:  createTestManifest(),
			right: createTestManifest(),
			want:  []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms1 := ManifestSource{Manifest: tt.left}
			ms2 := ManifestSource{Manifest: tt.right}
			
			union, err := Union(&ms1, &ms2)
			if err != nil {
				t.Fatalf("Union() error = %v", err)
			}
			
			if len(union.Entries) != len(tt.want) {
				t.Errorf("Union returned %d entries, want %d", len(union.Entries), len(tt.want))
			}
			
			// Check all expected files are present
			unionNames := make(map[string]bool)
			for _, e := range union.Entries {
				unionNames[e.Name] = true
			}
			
			for _, name := range tt.want {
				if !unionNames[name] {
					t.Errorf("Expected %q in union, not found", name)
				}
			}
		})
	}
}

func TestIntersect(t *testing.T) {
	tests := []struct {
		name   string
		left   *Manifest
		right  *Manifest
		want   []string
	}{
		{
			name:  "overlapping sets",
			left:  createTestManifest("file1.txt", "file2.txt", "file3.txt"),
			right: createTestManifest("file2.txt", "file3.txt", "file4.txt"),
			want:  []string{"file2.txt", "file3.txt"},
		},
		{
			name:  "disjoint sets",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file3.txt", "file4.txt"),
			want:  []string{},
		},
		{
			name:  "identical sets",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file1.txt", "file2.txt"),
			want:  []string{"file1.txt", "file2.txt"},
		},
		{
			name:  "one empty",
			left:  createTestManifest(),
			right: createTestManifest("file1.txt"),
			want:  []string{},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms1 := ManifestSource{Manifest: tt.left}
			ms2 := ManifestSource{Manifest: tt.right}
			
			intersect, err := Intersect(&ms1, &ms2)
			if err != nil {
				t.Fatalf("Intersect() error = %v", err)
			}
			
			if len(intersect.Entries) != len(tt.want) {
				t.Errorf("Intersect returned %d entries, want %d", len(intersect.Entries), len(tt.want))
			}
			
			// Check all expected files are present
			intersectNames := make(map[string]bool)
			for _, e := range intersect.Entries {
				intersectNames[e.Name] = true
			}
			
			for _, name := range tt.want {
				if !intersectNames[name] {
					t.Errorf("Expected %q in intersection, not found", name)
				}
			}
		})
	}
}

func TestSubtract(t *testing.T) {
	tests := []struct {
		name   string
		left   *Manifest
		right  *Manifest
		want   []string
	}{
		{
			name:  "remove some files",
			left:  createTestManifest("file1.txt", "file2.txt", "file3.txt"),
			right: createTestManifest("file2.txt"),
			want:  []string{"file1.txt", "file3.txt"},
		},
		{
			name:  "remove all files",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file1.txt", "file2.txt"),
			want:  []string{},
		},
		{
			name:  "remove none",
			left:  createTestManifest("file1.txt", "file2.txt"),
			right: createTestManifest("file3.txt", "file4.txt"),
			want:  []string{"file1.txt", "file2.txt"},
		},
		{
			name:  "empty right",
			left:  createTestManifest("file1.txt"),
			right: createTestManifest(),
			want:  []string{"file1.txt"},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms1 := ManifestSource{Manifest: tt.left}
			ms2 := ManifestSource{Manifest: tt.right}
			
			subtract, err := Subtract(&ms1, &ms2)
			if err != nil {
				t.Fatalf("Subtract() error = %v", err)
			}
			
			if len(subtract.Entries) != len(tt.want) {
				t.Errorf("Subtract returned %d entries, want %d", len(subtract.Entries), len(tt.want))
			}
			
			// Check all expected files are present
			subtractNames := make(map[string]bool)
			for _, e := range subtract.Entries {
				subtractNames[e.Name] = true
			}
			
			for _, name := range tt.want {
				if !subtractNames[name] {
					t.Errorf("Expected %q in result, not found", name)
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
	m := createTestManifest("file1.txt", "dir/file2.txt", "dir/sub/file3.txt")
	
	paths := m.PathList()
	
	expected := []string{"dir/file2.txt", "dir/sub/file3.txt", "file1.txt"}
	if len(paths) != len(expected) {
		t.Errorf("PathList() returned %d paths, want %d", len(paths), len(expected))
	}
	
	for i, path := range paths {
		if path != expected[i] {
			t.Errorf("PathList()[%d] = %q, want %q", i, path, expected[i])
		}
	}
}

func TestFilterByPath(t *testing.T) {
	m := createTestManifest("file1.txt", "file2.txt", "dir/file3.txt", "test.doc")
	
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "filter txt files",
			pattern: "*.txt",
			want:    []string{"file1.txt", "file2.txt"},
		},
		{
			name:    "filter files in dir",
			pattern: "dir/*",
			want:    []string{"dir/file3.txt"},
		},
		{
			name:    "filter all files",
			pattern: "*",
			want:    []string{"file1.txt", "file2.txt", "test.doc"},
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
			
			// Check all expected files are present
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
	m := createTestManifest("file1.txt", "file2.txt", "dir/file3.txt", "dir/sub/file4.txt", "other/file5.txt")
	
	tests := []struct {
		name   string
		prefix string
		want   []string
	}{
		{
			name:   "filter by dir prefix",
			prefix: "dir/",
			want:   []string{"dir/file3.txt", "dir/sub/file4.txt"},
		},
		{
			name:   "filter by file prefix",
			prefix: "file",
			want:   []string{"file1.txt", "file2.txt"},
		},
		{
			name:   "no matches",
			prefix: "nonexistent",
			want:   []string{},
		},
		{
			name:   "empty prefix matches all",
			prefix: "",
			want:   []string{"file1.txt", "file2.txt", "dir/file3.txt", "dir/sub/file4.txt", "other/file5.txt"},
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