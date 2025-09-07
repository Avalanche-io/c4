package c4m

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

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