package scan

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// c4mContentSizeForTest mirrors the c4m byte-count helper used inside
// c4m.PropagateMetadata, kept here just so this package's integration test
// can verify expected directory sizes after propagation.
func c4mContentSizeForTest(entries []*Entry) int64 {
	var n int64
	for _, e := range entries {
		n += int64(len(e.Canonical())) + 1
	}
	return n
}

func TestFileMetadataImplementsFileInfo(t *testing.T) {
	// Create a test metadata
	md := &BasicFileMetadata{
		path:    "/test/file.txt",
		name:    "file.txt",
		size:    1024,
		mode:    0644,
		modTime: time.Now(),
		isDir:   false,
		depth:   1,
	}

	// Verify it implements os.FileInfo
	var _ os.FileInfo = md

	// Test FileInfo methods
	if md.Name() != "file.txt" {
		t.Errorf("Name() = %v, want %v", md.Name(), "file.txt")
	}
	if md.Size() != 1024 {
		t.Errorf("Size() = %v, want %v", md.Size(), 1024)
	}
	if md.IsDir() != false {
		t.Errorf("IsDir() = %v, want %v", md.IsDir(), false)
	}
}

func TestMetadataC4ID(t *testing.T) {
	md := &BasicFileMetadata{
		path:    "/test/file.txt",
		name:    "file.txt",
		size:    1024,
		mode:    0644,
		modTime: time.Now(),
		isDir:   false,
		depth:   1,
	}

	// Initially, ID should be nil
	if !md.ID().IsNil() {
		t.Errorf("Initial ID should be nil, got %v", md.ID())
	}

	// Set an ID
	testID := c4.Identify(strings.NewReader("test content"))
	md.SetID(testID)

	// Verify ID was set
	if md.ID() != testID {
		t.Errorf("ID() = %v, want %v", md.ID(), testID)
	}
}

func TestMetadataToEntry(t *testing.T) {
	modTime := time.Now()
	md := &BasicFileMetadata{
		path:    "/test/file.txt",
		name:    "file.txt",
		size:    1024,
		mode:    0644,
		modTime: modTime,
		isDir:   false,
		depth:   2,
		target:  "",
		c4id:    c4.Identify(strings.NewReader("test")),
	}

	entry := MetadataToEntry(md)

	if entry.Name != "file.txt" {
		t.Errorf("Entry.Name = %v, want %v", entry.Name, "file.txt")
	}
	if entry.Size != 1024 {
		t.Errorf("Entry.Size = %v, want %v", entry.Size, 1024)
	}
	if entry.Mode != 0644 {
		t.Errorf("Entry.Mode = %v, want %v", entry.Mode, 0644)
	}
	if entry.Depth != 2 {
		t.Errorf("Entry.Depth = %v, want %v", entry.Depth, 2)
	}
	if entry.C4ID != md.ID() {
		t.Errorf("Entry.C4ID = %v, want %v", entry.C4ID, md.ID())
	}
}

func TestMetadataToEntryDirectory(t *testing.T) {
	md := &BasicFileMetadata{
		path:    "/test/dir",
		name:    "dir",
		size:    0,
		mode:    os.ModeDir | 0755,
		modTime: time.Now(),
		isDir:   true,
		depth:   1,
	}

	entry := MetadataToEntry(md)

	// Directory names should have trailing slash
	if entry.Name != "dir/" {
		t.Errorf("Directory entry.Name = %v, want %v", entry.Name, "dir/")
	}
	if !entry.IsDir() {
		t.Errorf("Entry should be a directory")
	}
}

func TestMetadataSymlink(t *testing.T) {
	md := &BasicFileMetadata{
		path:    "/test/link",
		name:    "link",
		size:    0,
		mode:    os.ModeSymlink | 0777,
		modTime: time.Now(),
		isDir:   false,
		target:  "../target",
		depth:   1,
	}

	if md.Target() != "../target" {
		t.Errorf("Target() = %v, want %v", md.Target(), "../target")
	}

	entry := MetadataToEntry(md)
	if entry.Target != "../target" {
		t.Errorf("Entry.Target = %v, want %v", entry.Target, "../target")
	}
	if !entry.IsSymlink() {
		t.Errorf("Entry should be a symlink")
	}
}

func TestEntryToMetadata(t *testing.T) {
	entry := &Entry{
		Name:      "test.txt",
		Size:      2048,
		Mode:      0644,
		Timestamp: time.Now(),
		Depth:     3,
		C4ID:      c4.Identify(strings.NewReader("content")),
		Target:    "",
	}

	md := EntryToMetadata(entry)

	if md.Name() != "test.txt" {
		t.Errorf("Name() = %v, want %v", md.Name(), "test.txt")
	}
	if md.Size() != 2048 {
		t.Errorf("Size() = %v, want %v", md.Size(), 2048)
	}
	if md.Depth() != 3 {
		t.Errorf("Depth() = %v, want %v", md.Depth(), 3)
	}
	if md.ID() != entry.C4ID {
		t.Errorf("ID() = %v, want %v", md.ID(), entry.C4ID)
	}
}

func TestScanResultToManifest(t *testing.T) {
	// Create test metadata
	files := []FileMetadata{
		&BasicFileMetadata{
			path:    "/file1.txt",
			name:    "file1.txt",
			size:    100,
			mode:    0644,
			modTime: time.Now(),
			depth:   0,
		},
		&BasicFileMetadata{
			path:    "/file2.txt",
			name:    "file2.txt",
			size:    200,
			mode:    0644,
			modTime: time.Now(),
			depth:   0,
		},
	}

	result := &ScanResult{
		Root:     nil,
		AllFiles: files,
	}

	manifest := result.ToManifest()

	if len(manifest.Entries) != 2 {
		t.Errorf("Manifest should have 2 entries, got %v", len(manifest.Entries))
	}

	// Verify entries were converted correctly
	if manifest.Entries[0].Name != "file1.txt" {
		t.Errorf("First entry name = %v, want %v", manifest.Entries[0].Name, "file1.txt")
	}
	if manifest.Entries[1].Name != "file2.txt" {
		t.Errorf("Second entry name = %v, want %v", manifest.Entries[1].Name, "file2.txt")
	}
}

// TestPropagateMetadataDirectorySizes verifies that calling
// c4m.PropagateMetadata on a sorted scan-produced manifest gives directories
// the expected size (descendant file sizes + own c4m one-level listing
// bytes). The propagation algorithm itself is tested in package c4m; this
// is an integration check that the scan layer wires it up correctly.
func TestPropagateMetadataDirectorySizes(t *testing.T) {
	now := time.Now().UTC()
	entries := []*Entry{
		{Name: "dir/", Mode: os.ModeDir | 0755, Timestamp: now, Size: -1, Depth: 0},
		{Name: "file1.txt", Mode: 0644, Timestamp: now, Size: 100, Depth: 1},
		{Name: "subdir/", Mode: os.ModeDir | 0755, Timestamp: now, Size: -1, Depth: 1},
		{Name: "file2.txt", Mode: 0644, Timestamp: now, Size: 200, Depth: 2},
	}

	// subdir has one child: file2.txt
	wantSubdir := int64(200) + c4mContentSizeForTest([]*Entry{entries[3]})

	c4m.PropagateMetadata(entries)

	if entries[2].Size != wantSubdir {
		t.Errorf("subdir/ size = %d, want %d", entries[2].Size, wantSubdir)
	}
	// dir/ children are file1 and subdir (now with updated size). Use the
	// post-propagation entries so subdir's canonical form has the right Size.
	wantDir := entries[1].Size + entries[2].Size + c4mContentSizeForTest([]*Entry{entries[1], entries[2]})
	if entries[0].Size != wantDir {
		t.Errorf("dir/ size = %d, want %d", entries[0].Size, wantDir)
	}
}

func TestNewFileMetadataDirSizeNull(t *testing.T) {
	// Create a temp directory to get real os.FileInfo
	dir := t.TempDir()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	md := NewFileMetadata(dir, info, 0)
	if md.Size() != -1 {
		t.Errorf("NewFileMetadata for dir should set Size=-1, got %d", md.Size())
	}
}

func TestEntryHasNullValues(t *testing.T) {
	tests := []struct {
		name     string
		entry    *Entry
		expected bool
	}{
		{
			name: "fully specified file",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Now().UTC(),
				Size:      100,
			},
			expected: false,
		},
		{
			name: "null size",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Now().UTC(),
				Size:      -1,
			},
			expected: true,
		},
		{
			name: "null timestamp",
			entry: &Entry{
				Mode:      0644,
				Timestamp: time.Unix(0, 0),
				Size:      100,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.entry.HasNullValues()
			if result != tt.expected {
				t.Errorf("HasNullValues() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestManifestCanonicalize(t *testing.T) {
	manifest := NewManifest()

	// Add entry with null values
	manifest.AddEntry(&Entry{
		Name:      "file.txt",
		Mode:      0,              // Null
		Timestamp: time.Unix(0, 0), // Null
		Size:      -1,             // Null
	})

	// Should have null values
	if !manifest.HasNullValues() {
		t.Error("Expected manifest to have null values before canonicalization")
	}

	// Canonicalize
	manifest.Canonicalize()

	// Null values stay null — they render as "-" in canonical form
	entry := manifest.Entries[0]
	if entry.Mode != 0 {
		t.Errorf("Null mode should stay 0 after canonicalization, got %o", entry.Mode)
	}
	if !entry.Timestamp.Equal(time.Unix(0, 0).UTC()) {
		t.Error("Null timestamp should stay null after canonicalization")
	}
	if entry.Size != -1 {
		t.Errorf("Null size should stay -1 after canonicalization, got %d", entry.Size)
	}

	// Verify canonical form renders null values as "-"
	canonical := entry.Canonical()
	if !strings.HasPrefix(canonical, "- - - file.txt") {
		t.Errorf("Expected canonical to start with '- - - file.txt', got %q", canonical)
	}
}

func TestComputeC4IDDeterministic(t *testing.T) {
	// Create two manifests with same content — both should produce same ID
	m1 := NewManifest()
	m1.AddEntry(&Entry{
		Name:      "file.txt",
		Mode:      0644,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Size:      100,
	})

	m2 := NewManifest()
	m2.AddEntry(&Entry{
		Name:      "file.txt",
		Mode:      0644,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Size:      100,
	})

	id1 := m1.ComputeC4ID()
	id2 := m2.ComputeC4ID()

	if id1 != id2 {
		t.Errorf("Same content produced different IDs:\n  ID1: %s\n  ID2: %s",
			id1.String(), id2.String())
	}

	// Null size (-1) and explicit zero (0) are now semantically different
	m3 := NewManifest()
	m3.AddEntry(&Entry{
		Name:      "file.txt",
		Mode:      0644,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Size:      -1, // Null — renders as "-"
	})

	id3 := m3.ComputeC4ID()
	if id1 == id3 {
		t.Error("Null size and explicit size should produce different IDs")
	}
}