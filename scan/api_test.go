package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// TestDir tests the primary public entry point.
func TestDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("nested"), 0644)

	m, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 3 { // hello.txt, sub/, nested.txt
		t.Fatalf("expected 3 entries, got %d", len(m.Entries))
	}

	// All files should have C4 IDs (default mode is full).
	for _, e := range m.Entries {
		if !e.IsDir() && e.C4ID.IsNil() {
			t.Errorf("entry %s missing C4 ID in full mode", e.Name)
		}
	}
}

// TestDirWithModeStructure tests structure-only scan.
func TestDirWithModeStructure(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)

	m, err := Dir(dir, WithMode(ModeStructure))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}

	e := m.Entries[0]
	if !e.C4ID.IsNil() {
		t.Error("structure mode should not compute C4 IDs")
	}
	if e.Size != -1 {
		t.Errorf("structure mode should have null size, got %d", e.Size)
	}
}

// TestDirWithModeMetadata tests metadata scan.
func TestDirWithModeMetadata(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644)

	m, err := Dir(dir, WithMode(ModeMetadata))
	if err != nil {
		t.Fatal(err)
	}

	e := m.Entries[0]
	if e.C4ID.IsNil() == false {
		t.Error("metadata mode should not compute C4 IDs")
	}
	if e.Size != 4 {
		t.Errorf("metadata mode should have real size, got %d", e.Size)
	}
	if e.Mode == 0 {
		t.Error("metadata mode should have real permissions")
	}
}

// TestDirWithExclude tests exclude patterns.
func TestDirWithExclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.tmp"), []byte("skip"), 0644)

	m, err := Dir(dir, WithExclude([]string{"*.log", "*.tmp"}))
	if err != nil {
		t.Fatal(err)
	}

	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry after exclude, got %d", len(m.Entries))
	}
	if m.Entries[0].Name != "keep.txt" {
		t.Errorf("expected keep.txt, got %s", m.Entries[0].Name)
	}
}

// TestDirWithGuide tests guided scan optimization.
func TestDirWithGuide(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bravo"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("charlie"), 0644)

	// Create a guide manifest with only a.txt and b.txt.
	guide := c4m.NewManifest()
	guide.AddEntry(&c4m.Entry{Name: "a.txt", Size: 5, Depth: 0})
	guide.AddEntry(&c4m.Entry{Name: "b.txt", Size: 5, Depth: 0})

	m, err := Dir(dir, WithGuide(guide))
	if err != nil {
		t.Fatal(err)
	}

	// Only a.txt and b.txt should be in the result (c.txt excluded by guide).
	names := make(map[string]bool)
	for _, e := range m.Entries {
		names[e.Name] = true
	}
	if names["c.txt"] {
		t.Error("c.txt should be excluded by guide")
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Error("a.txt and b.txt should be included by guide")
	}
}

// TestDirWithSequenceDetection tests sequence folding.
func TestDirWithSequenceDetection(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= 5; i++ {
		name := filepath.Join(dir, "frame."+strings.Replace(
			strings.Replace("000"+string(rune('0'+i)), "", "", 0),
			"", "", 0))
		// Simpler: just use Sprintf
		_ = name
	}
	// Use fmt.Sprintf for clarity
	for i := 1; i <= 5; i++ {
		fname := fmt.Sprintf("frame.%04d.exr", i)
		os.WriteFile(filepath.Join(dir, fname), []byte(fname), 0644)
	}

	m, err := Dir(dir, WithSequenceDetection(true))
	if err != nil {
		t.Fatal(err)
	}

	// Should be folded into a single sequence entry.
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 sequence entry, got %d", len(m.Entries))
	}
	if !m.Entries[0].IsSequence {
		t.Error("entry should be a sequence")
	}
}

// TestDirNonexistent tests error on nonexistent path.
func TestDirNonexistent(t *testing.T) {
	_, err := Dir("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

// TestParseScanMode tests all mode string variants.
func TestParseScanMode(t *testing.T) {
	tests := []struct {
		input string
		want  ScanMode
		err   bool
	}{
		{"s", ModeStructure, false},
		{"1", ModeStructure, false},
		{"structure", ModeStructure, false},
		{"m", ModeMetadata, false},
		{"2", ModeMetadata, false},
		{"metadata", ModeMetadata, false},
		{"f", ModeFull, false},
		{"3", ModeFull, false},
		{"full", ModeFull, false},
		{"", ModeFull, false},
		{"S", ModeStructure, false}, // case insensitive
		{"FULL", ModeFull, false},
		{"invalid", ModeFull, true},
		{"4", ModeFull, true},
	}

	for _, tt := range tests {
		got, err := ParseScanMode(tt.input)
		if tt.err && err == nil {
			t.Errorf("ParseScanMode(%q): expected error", tt.input)
		}
		if !tt.err && err != nil {
			t.Errorf("ParseScanMode(%q): unexpected error: %v", tt.input, err)
		}
		if !tt.err && got != tt.want {
			t.Errorf("ParseScanMode(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// TestFileSourceToManifest tests the FileSource adapter.
func TestFileSourceToManifest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("test"), 0644)

	gen := NewGeneratorWithOptions(WithMode(ModeMetadata))
	fs := FileSource{Path: dir, Generator: gen}

	m, err := fs.ToManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
}

// TestDirC4IDDeterministic verifies that scanning the same directory
// twice produces the same C4 IDs.
func TestDirC4IDDeterministic(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644)

	// Backdate to ensure consistent timestamps.
	past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "a.txt"), past, past)
	os.Chtimes(filepath.Join(dir, "b.txt"), past, past)

	m1, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}

	id1 := m1.ComputeC4ID()
	id2 := m2.ComputeC4ID()
	if id1 != id2 {
		t.Errorf("same directory should produce same C4 ID:\n  first:  %s\n  second: %s", id1, id2)
	}
}

// TestDirExcludeFile tests loading exclude patterns from a file.
func TestDirExcludeFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(dir, "skip.log"), []byte("skip"), 0644)

	excludeFile := filepath.Join(t.TempDir(), "excludes.txt")
	os.WriteFile(excludeFile, []byte("*.log\n"), 0644)

	m, err := Dir(dir, WithExcludeFile(excludeFile))
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range m.Entries {
		if e.Name == "skip.log" {
			t.Error("skip.log should be excluded by exclude file")
		}
	}
}

// TestDirEmptyDirectory tests scanning an empty directory.
func TestDirEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	m, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Entries) != 0 {
		t.Fatalf("expected 0 entries for empty dir, got %d", len(m.Entries))
	}
}

// TestDirContentIdentity verifies that files with the same content
// get the same C4 ID regardless of name or location.
func TestDirContentIdentity(t *testing.T) {
	dir := t.TempDir()
	content := "identical content"
	os.WriteFile(filepath.Join(dir, "copy1.txt"), []byte(content), 0644)
	os.WriteFile(filepath.Join(dir, "copy2.txt"), []byte(content), 0644)

	m, err := Dir(dir)
	if err != nil {
		t.Fatal(err)
	}

	expectedID := c4.Identify(strings.NewReader(content))
	for _, e := range m.Entries {
		if e.C4ID != expectedID {
			t.Errorf("%s: got C4 ID %s, want %s", e.Name, e.C4ID, expectedID)
		}
	}
}
