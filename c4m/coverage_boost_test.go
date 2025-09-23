package c4m

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// Test manifest writing operations
func TestManifestWriting(t *testing.T) {
	manifest := NewManifest()
	manifest.Version = "1.0"

	// Add various entries
	manifest.AddEntry(&Entry{
		Name:      "test.txt",
		Size:      100,
		Mode:      0644,
		Timestamp: time.Now(),
	})

	manifest.AddEntry(&Entry{
		Name: "dir/",
		Mode: 0755 | os.ModeDir,
	})

	// Test WriteTo
	var buf bytes.Buffer
	n, err := manifest.WriteTo(&buf)
	if err != nil {
		t.Errorf("WriteTo failed: %v", err)
	}
	if n == 0 {
		t.Error("WriteTo wrote 0 bytes")
	}

	// Test WritePretty
	buf.Reset()
	n, err = manifest.WritePretty(&buf)
	if err != nil {
		t.Errorf("WritePretty failed: %v", err)
	}
	if n == 0 {
		t.Error("WritePretty wrote 0 bytes")
	}
}

// Test entry methods
func TestEntryMethods(t *testing.T) {
	// Test regular file
	entry := &Entry{
		Name: "file.txt",
		Mode: 0644,
		Size: 100,
	}

	if entry.IsDir() {
		t.Error("Regular file marked as directory")
	}

	if entry.IsSymlink() {
		t.Error("Regular file marked as symlink")
	}

	// Test directory
	dirEntry := &Entry{
		Name: "dir/",
		Mode: 0755 | os.ModeDir,
	}

	if !dirEntry.IsDir() {
		t.Error("Directory not marked as directory")
	}

	// Test symlink
	linkEntry := &Entry{
		Name:   "link",
		Mode:   0777 | os.ModeSymlink,
		Target: "target",
	}

	if !linkEntry.IsSymlink() {
		t.Error("Symlink not marked as symlink")
	}

	// Test String methods
	_ = entry.String()
}

// Test manifest operations
func TestManifestOperations(t *testing.T) {
	m1 := NewManifest()
	m1.AddEntry(&Entry{Name: "a.txt", Size: 100})
	m1.AddEntry(&Entry{Name: "b.txt", Size: 200})

	m2 := NewManifest()
	m2.AddEntry(&Entry{Name: "b.txt", Size: 200})
	m2.AddEntry(&Entry{Name: "c.txt", Size: 300})

	// Test that we have entries
	if len(m1.Entries) != 2 {
		t.Errorf("Expected 2 entries in m1, got %d", len(m1.Entries))
	}

	if len(m2.Entries) != 2 {
		t.Errorf("Expected 2 entries in m2, got %d", len(m2.Entries))
	}
}

// Test sorting operations
func TestSortingOperations(t *testing.T) {
	manifest := NewManifest()

	// Add entries in reverse order
	manifest.AddEntry(&Entry{Name: "z.txt", Mode: 0644})
	manifest.AddEntry(&Entry{Name: "a.txt", Mode: 0644})
	manifest.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755})
	manifest.AddEntry(&Entry{Name: "m.txt", Mode: 0644})

	// Test Sort
	manifest.Sort()
	if manifest.Entries[0].Name != "a.txt" {
		t.Errorf("Expected first entry to be a.txt after sort, got %s", manifest.Entries[0].Name)
	}

	// Test SortSiblingsHierarchically
	manifest2 := NewManifest()
	manifest2.AddEntry(&Entry{Name: "file.txt", Mode: 0644, Depth: 0})
	manifest2.AddEntry(&Entry{Name: "dir/", Mode: os.ModeDir | 0755, Depth: 0})
	manifest2.AddEntry(&Entry{Name: "another.txt", Mode: 0644, Depth: 0})

	manifest2.SortSiblingsHierarchically()
	// Files should come before directories at same depth
	if manifest2.Entries[0].Mode.IsDir() {
		t.Error("Directory came before file at same depth")
	}
}

// Test base chain resolver basic functionality
func TestBaseChainBasic(t *testing.T) {
	resolver := NewBaseChainResolver("/test/path")

	// Test that resolver was created
	if resolver == nil {
		t.Error("Failed to create base chain resolver")
	}
}

// Test natural sorting additional cases
func TestNaturalSortAdditional(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"file1.txt", "file2.txt", true},
		{"file2.txt", "file10.txt", true},
		{"file10.txt", "file2.txt", false},
		{"abc", "def", true},
		{"def", "abc", false},
	}

	for _, tt := range tests {
		got := NaturalLess(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("NaturalLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// Test manifest sources
func TestManifestSources(t *testing.T) {
	manifest := NewManifest()
	manifest.AddEntry(&Entry{Name: "test.txt", Size: 100})

	// Test ManifestSource
	source := ManifestSource{manifest}
	// Just ensure we can create the source
	if source.Manifest == nil {
		t.Error("ManifestSource has nil manifest")
	}

	// Test FileSource
	// This would need a real file, so we'll skip the actual file test
	// but test the structure exists
	_ = FileSource{Path: "test.c4m"}
}