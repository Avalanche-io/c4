package bundle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestFindResumePathWithManifest tests finding the resume path from a manifest
func TestFindResumePathWithManifest(t *testing.T) {
	cli := &SimpleBundleCLI{verbose: true}

	// Test case 1: Empty manifest
	manifest := NewManifest()
	resumePath := cli.findResumePath(manifest)
	if resumePath != "" {
		t.Errorf("Expected empty resume path for empty manifest, got %s", resumePath)
	}

	// Test case 2: Manifest with entries
	manifest.AddEntry(&Entry{
		Name: "file1.txt",
		Size: 100,
		Mode: 0644,
		Timestamp: time.Now(),
		Depth: 0,
	})

	manifest.AddEntry(&Entry{
		Name: "dir/",
		Size: 0,
		Mode: 0755 | os.ModeDir,
		Timestamp: time.Now(),
		Depth: 0,
	})

	manifest.AddEntry(&Entry{
		Name: "subdir/",
		Size: 200,
		Mode: 0755 | os.ModeDir,
		Timestamp: time.Now(),
		Depth: 1,
	})

	// The function returns the deepest directory found
	resumePath = cli.findResumePath(manifest)
	if resumePath != "subdir" {
		t.Errorf("Expected resume path 'subdir', got %s", resumePath)
	}
}

// TestBundleConfigCreation tests bundle configuration creation
func TestBundleConfigCreation(t *testing.T) {
	// Test DefaultBundleConfig
	defaultConfig := DefaultBundleConfig()
	if defaultConfig.MaxEntriesPerChunk != 100000 {
		t.Errorf("Expected default MaxEntriesPerChunk 100000, got %d", defaultConfig.MaxEntriesPerChunk)
	}
	// BundleDir is not set by DefaultBundleConfig, it's empty by default

	// Test DevBundleConfig
	devConfig := DevBundleConfig()
	if devConfig.MaxEntriesPerChunk != 10 {
		t.Errorf("Expected dev MaxEntriesPerChunk 10, got %d", devConfig.MaxEntriesPerChunk)
	}
	if devConfig.MaxBytesPerChunk != 1024*1024 {
		t.Errorf("Expected dev MaxBytesPerChunk 1MB, got %d", devConfig.MaxBytesPerChunk)
	}
}

// TestBundleCreation tests basic bundle creation
func TestBundleCreation(t *testing.T) {
	tmpDir := t.TempDir()

	config := DevBundleConfig()
	config.BundleDir = tmpDir

	// Create a test data directory
	dataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("Failed to create data dir: %v", err)
	}

	// Create a test file
	testFile := filepath.Join(dataDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	bundle, err := CreateBundle(dataDir, config)
	if err != nil {
		t.Fatalf("Failed to create bundle: %v", err)
	}

	if bundle.Path == "" {
		t.Error("Bundle path should not be empty")
	}

	// Verify the bundle directory exists
	if _, err := os.Stat(bundle.Path); os.IsNotExist(err) {
		t.Errorf("Bundle directory was not created at %s", bundle.Path)
	}
}

// TestManifestOperationsCLI tests basic manifest operations in CLI context
func TestManifestOperationsCLI(t *testing.T) {
	// Test creating a manifest
	manifest := NewManifest()
	if manifest.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", manifest.Version)
	}

	// Test adding entries
	entry1 := &Entry{
		Name: "file1.txt",
		Size: 100,
		Mode: 0644,
		Timestamp: time.Now(),
	}

	entry2 := &Entry{
		Name: "dir/",
		Size: 0,
		Mode: 0755 | os.ModeDir,
		Timestamp: time.Now(),
	}

	manifest.AddEntry(entry1)
	manifest.AddEntry(entry2)

	if len(manifest.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(manifest.Entries))
	}

	// Test sorting
	manifest.SortEntries()

	// NaturalLess sorts by natural order, so "dir/" comes before "file1.txt"
	if manifest.Entries[0].Name != "dir/" {
		t.Errorf("Expected dir/ first after sorting, got %s", manifest.Entries[0].Name)
	}
}

// TestBundleProgressTracking tests progress tracking in bundles
func TestBundleProgressTracking(t *testing.T) {
	tmpDir := t.TempDir()

	config := DevBundleConfig()
	config.BundleDir = tmpDir

	bundle, err := CreateBundle(tmpDir, config)
	if err != nil {
		t.Fatalf("Failed to create bundle: %v", err)
	}

	// Create a scan
	scan, err := bundle.NewScan(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create scan: %v", err)
	}

	// Add progress chunk
	manifest := NewManifest()
	manifest.AddEntry(&Entry{
		Name: "test.txt",
		Size: 100,
		Mode: 0644,
		Timestamp: time.Now(),
	})

	bundle.AddProgressChunk(scan, manifest)

	// Verify progress was tracked
	// Check if any progress chunks were created in the scan
	if len(scan.ProgressChunks) == 0 {
		t.Error("Expected progress chunks to be created")
	}

	// Complete the scan
	bundle.CompleteScan(scan)
}