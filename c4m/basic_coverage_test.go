package c4m

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// Test basic Manifest functions
func TestManifestBasic(t *testing.T) {
	m := NewManifest()
	if m == nil {
		t.Fatal("NewManifest returned nil")
	}

	// Test AddEntry
	entry := &Entry{
		Name:      "test.txt",
		Mode:      0644,
		Size:      100,
		Timestamp: time.Now(),
		C4ID:      c4.Identify(strings.NewReader("test")),
	}
	m.AddEntry(entry)

	// Test Sort
	m.Sort()

	// Test WriteTo
	var buf bytes.Buffer
	n, err := m.WriteTo(&buf)
	if err != nil {
		t.Errorf("WriteTo failed: %v", err)
	}
	if n == 0 {
		t.Error("WriteTo wrote 0 bytes")
	}

	// Test GetEntry
	e := m.GetEntry("test.txt")
	if e == nil {
		t.Error("GetEntry returned nil")
	}

	// Test ComputeC4ID
	id := m.ComputeC4ID()
	var emptyID c4.ID
	if id == emptyID {
		t.Error("ComputeC4ID returned empty ID")
	}

	// Test Canonical
	canonical := m.Canonical()
	if canonical == "" {
		t.Error("Canonical returned empty string")
	}

	// Test AllEntriesString
	str := m.AllEntriesString()
	if str == "" {
		t.Error("AllEntriesString returned empty")
	}

	// Test GetEntriesAtDepth
	entries := m.GetEntriesAtDepth(0)
	if len(entries) == 0 {
		t.Error("GetEntriesAtDepth returned no entries")
	}
}

// Test Entry methods
func TestEntryBasic(t *testing.T) {
	e := &Entry{
		Name:      "test.txt",
		Mode:      0644,
		Size:      100,
		Timestamp: time.Now(),
	}

	// Test IsDir
	if e.IsDir() {
		t.Error("IsDir returned true for file")
	}

	// Test IsSymlink
	if e.IsSymlink() {
		t.Error("IsSymlink returned true for regular file")
	}

	// Test BaseName
	base := e.BaseName()
	if base != "test.txt" {
		t.Errorf("BaseName returned %q, expected test.txt", base)
	}

	// Test String
	str := e.String()
	if str == "" {
		t.Error("String returned empty")
	}

	// Test Canonical
	canonical := e.Canonical()
	if canonical == "" {
		t.Error("Canonical returned empty")
	}
}

// Test Sequence functions
func TestSequenceBasic(t *testing.T) {
	// Test IsSequence
	if !IsSequence("file_[001-005].txt") {
		t.Error("Expected IsSequence to return true")
	}

	// Test ParseSequence
	seq, err := ParseSequence("file_[001-005].txt")
	if err != nil {
		t.Errorf("ParseSequence failed: %v", err)
	}
	if seq != nil {
		// Test Expand
		files := seq.Expand()
		if len(files) != 5 {
			t.Errorf("Expected 5 files, got %d", len(files))
		}

		// Test Count
		if seq.Count() != 5 {
			t.Errorf("Expected count 5, got %d", seq.Count())
		}
	}
}

// Test Validator
func TestValidatorBasic(t *testing.T) {
	v := NewValidator(false)
	if v == nil {
		t.Fatal("NewValidator returned nil")
	}

	// Test GetStats
	stats := v.GetStats()
	// Check if stats has expected initial values
	if stats.Files < 0 {
		t.Error("GetStats returned invalid Files count")
	}

	// Test GetCurrentPath
	path := v.GetCurrentPath()
	if path != "" {
		t.Errorf("Expected empty path, got %q", path)
	}
}

// Test Parser
func TestParserBasic(t *testing.T) {
	p := NewParser(nil)
	if p == nil {
		t.Fatal("NewParser returned nil")
	}

	// Test NewStrictParser
	p2 := NewStrictParser(nil)
	if p2 == nil {
		t.Fatal("NewStrictParser returned nil")
	}
}

// Test Operations with ManifestSource
func TestOperationsBasic(t *testing.T) {
	m1 := NewManifest()
	m1.AddEntry(&Entry{Name: "a.txt", Mode: 0644, Size: 100, Timestamp: time.Now()})

	m2 := NewManifest()
	m2.AddEntry(&Entry{Name: "b.txt", Mode: 0644, Size: 200, Timestamp: time.Now()})

	// Test Diff
	diff, err := Diff(ManifestSource{m1}, ManifestSource{m2})
	if err != nil {
		t.Errorf("Diff failed: %v", err)
	}
	if diff == nil {
		t.Error("Diff returned nil results")
	}

	// Test Union
	union, err := Union(ManifestSource{m1}, ManifestSource{m2})
	if err != nil {
		t.Errorf("Union failed: %v", err)
	}
	if union == nil {
		t.Error("Union returned nil")
	}

	// Test Intersect
	intersect, err := Intersect(ManifestSource{m1}, ManifestSource{m2})
	if err != nil {
		t.Errorf("Intersect failed: %v", err)
	}
	if intersect == nil {
		t.Error("Intersect returned nil")
	}

	// Test Subtract
	subtract, err := Subtract(ManifestSource{m1}, ManifestSource{m2})
	if err != nil {
		t.Errorf("Subtract failed: %v", err)
	}
	if subtract == nil {
		t.Error("Subtract returned nil")
	}
}
