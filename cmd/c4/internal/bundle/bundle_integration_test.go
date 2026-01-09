package bundle

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/absfs/memfs"
	"github.com/absfs/absfs"
)

// TestResumeBundleWithMemFS tests the ResumeBundle functionality using an in-memory filesystem
func TestResumeBundleWithMemFS(t *testing.T) {
	// Create a memory filesystem
	fs, _ := memfs.NewFS()

	// Create a mock bundle structure
	bundleDir := "/test_bundle"
	progressDir := filepath.Join(bundleDir, "progress")

	// Create directories
	if err := fs.MkdirAll(progressDir, 0755); err != nil {
		t.Fatalf("Failed to create progress dir: %v", err)
	}

	// Create a partial scan state file
	statePath := filepath.Join(progressDir, "scan_0001_state.txt")
	stateContent := `Stage:2
Found:10
Metadata:8
C4:5
CurrentPath:/test/path
StartTime:2025-01-01T00:00:00Z`

	if err := writeFile(fs, statePath, []byte(stateContent)); err != nil {
		t.Fatalf("Failed to write state file: %v", err)
	}

	// Create a partial manifest chunk
	chunkPath := filepath.Join(progressDir, "chunk_001.c4m")
	chunkContent := `@c4m 1.0
-rw-r--r-- 2025-01-01T00:00:00Z 100 file1.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-01-01T00:00:00Z 200 file2.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp`

	if err := writeFile(fs, chunkPath, []byte(chunkContent)); err != nil {
		t.Fatalf("Failed to write chunk file: %v", err)
	}

	// Test finding resume path
	// NOTE: This is where we need to refactor the actual code to accept a filesystem interface
	// For now, this test demonstrates the approach but won't work without refactoring
	t.Skip("ResumeBundle needs refactoring to accept filesystem interface")
}

// TestLoadManifestFromFile tests loading a manifest from a file
func TestLoadManifestFromFile(t *testing.T) {
	// Create a memory filesystem
	fs, _ := memfs.NewFS()

	// Create a test manifest file
	manifestPath := "/test.c4m"
	manifestContent := `@c4m 1.0
-rw-r--r-- 2025-01-01T00:00:00Z 100 test.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
drwxr-xr-x 2025-01-01T00:00:00Z 200 dir/
  -rw-r--r-- 2025-01-01T00:00:00Z 200 file.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp`

	if err := writeFile(fs, manifestPath, []byte(manifestContent)); err != nil {
		t.Fatalf("Failed to write manifest file: %v", err)
	}

	// Read the file back
	f, err := fs.Open(manifestPath)
	if err != nil {
		t.Fatalf("Failed to open manifest file: %v", err)
	}
	defer f.Close()

	// Parse the manifest
	parser := NewParser(f)
	manifest, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Verify the manifest
	if manifest.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", manifest.Version)
	}

	if len(manifest.Entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(manifest.Entries))
	}
}

// TestBaseChainResolutionWithMemFS tests resolving base chains with multiple manifest chunks
func TestBaseChainResolutionWithMemFS(t *testing.T) {
	// Create a memory filesystem
	fs, _ := memfs.NewFS()

	// Create base manifest
	baseManifestPath := "/base.c4m"
	baseContent := `@c4m 1.0
-rw-r--r-- 2025-01-01T00:00:00Z 100 base.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp`

	if err := writeFile(fs, baseManifestPath, []byte(baseContent)); err != nil {
		t.Fatalf("Failed to write base manifest: %v", err)
	}

	// Create derived manifest with @base reference
	derivedPath := "/derived.c4m"
	derivedContent := `@c4m 1.0
@base c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp
-rw-r--r-- 2025-01-01T00:00:00Z 200 derived.txt c44aMtvPeoSPUFTRQNy6yj44qjrYtaJT4i9SzzNH2hiFHoYpjc5ecDzrz9jzuNBUgbqzHH7pYjSatjeoyh8C1UX4Bp`

	if err := writeFile(fs, derivedPath, []byte(derivedContent)); err != nil {
		t.Fatalf("Failed to write derived manifest: %v", err)
	}

	// Read derived manifest
	f, err := fs.Open(derivedPath)
	if err != nil {
		t.Fatalf("Failed to open derived manifest: %v", err)
	}
	defer f.Close()

	parser := NewParser(f)
	manifest, err := parser.ParseAll()
	if err != nil {
		t.Fatalf("Failed to parse derived manifest: %v", err)
	}

	// Verify the manifest has a base reference
	if manifest.Base.IsNil() {
		t.Error("Expected manifest to have @base reference")
	}

	// NOTE: ResolveBaseChain needs refactoring to work with filesystem interface
	t.Skip("ResolveBaseChain needs refactoring to accept filesystem interface")
}

// Helper function to write a file to the filesystem
func writeFile(fs absfs.FileSystem, path string, data []byte) error {
	f, err := fs.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// TestBundleWithMemFS demonstrates how to create and test a bundle with memfs
func TestBundleWithMemFS(t *testing.T) {
	// Create memory filesystem
	fs, _ := memfs.NewFS()

	// Create test files
	testFiles := map[string]string{
		"/data/file1.txt": "content1",
		"/data/file2.txt": "content2",
		"/data/subdir/file3.txt": "content3",
	}

	for path, content := range testFiles {
		dir := filepath.Dir(path)
		if err := fs.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
		if err := writeFile(fs, path, []byte(content)); err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	// Verify files were created by attempting to read them
	fileCount := 0
	for path := range testFiles {
		f, err := fs.Open(path)
		if err != nil {
			t.Errorf("Failed to open file %s: %v", path, err)
			continue
		}
		f.Close()
		fileCount++
	}

	// Verify we found all files
	if fileCount != 3 {
		t.Errorf("Expected 3 files, got %d", fileCount)
	}
}

// TestProgressiveScannerWithMemFS tests the progressive scanner with an in-memory filesystem
func TestProgressiveScannerWithMemFS(t *testing.T) {
	// This test demonstrates how we could test the progressive scanner
	// with a mock filesystem, but requires refactoring the scanner
	// to accept a filesystem interface

	fs, _ := memfs.NewFS()

	// Create a directory structure
	dirs := []string{
		"/root/dir1",
		"/root/dir2",
		"/root/dir1/subdir1",
		"/root/dir1/subdir2",
	}

	for _, dir := range dirs {
		if err := fs.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create files
	files := map[string]int{
		"/root/file1.txt": 100,
		"/root/dir1/file2.txt": 200,
		"/root/dir1/subdir1/file3.txt": 300,
	}

	for path, size := range files {
		content := strings.Repeat("x", size)
		if err := writeFile(fs, path, []byte(content)); err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	// NOTE: ProgressiveScanner needs refactoring to work with filesystem interface
	t.Skip("ProgressiveScanner needs refactoring to accept filesystem interface")
}