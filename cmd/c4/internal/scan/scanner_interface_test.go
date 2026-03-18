package scan

import (
	"context"
	"testing"
)

func TestScannerInterface(t *testing.T) {
	// Create a scanner factory
	factory := &ScannerFactory{}
	
	// Create a filesystem scanner
	opts := ScanOptions{
		ComputeC4IDs:  false, // Disable for test speed
		IncludeHidden: false,
	}
	
	scanner := factory.NewFilesystemScanner(".", opts)
	
	// Verify it implements the interface
	var _ FilesystemScanner = scanner
	
	// Try to scan current directory (should at least have this test file)
	ctx := context.Background()
	result, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	
	if len(result.AllFiles) == 0 {
		t.Error("Expected to find at least one file")
	}
}

func TestManifestBuilder(t *testing.T) {
	// Create test metadata
	md1 := &BasicFileMetadata{
		path:  "file1.txt",
		name:  "file1.txt",
		size:  100,
		depth: 0,
	}
	
	md2 := &BasicFileMetadata{
		path:  "file2.txt",
		name:  "file2.txt",
		size:  200,
		depth: 0,
	}
	
	// Create scan result
	result := &ScanResult{
		AllFiles: []FileMetadata{md1, md2},
	}
	
	// Build manifest
	builder := NewManifestBuilder(BuildOptions{
		SortEntries: true,
	})
	
	manifest := builder.BuildFromScanResult(result)
	
	if len(manifest.Entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(manifest.Entries))
	}
}

func TestGeneratorAdapter(t *testing.T) {
	// Test that the generator adapter works correctly
	gen := NewGenerator()
	adapter := &generatorAdapter{
		generator: gen,
		path:      ".",
	}
	
	// Test SetPath
	adapter.SetPath("/tmp")
	if adapter.path != "/tmp" {
		t.Errorf("SetPath failed: got %s, want /tmp", adapter.path)
	}
	
	// Test SetOptions
	opts := ScanOptions{
		ComputeC4IDs:   true,
		FollowSymlinks: true,
		IncludeHidden:  true,
	}
	adapter.SetOptions(opts)
	
	if adapter.generator.mode != ModeFull {
		t.Error("SetOptions failed to set mode to ModeFull")
	}
	if !adapter.generator.followSymlinks {
		t.Error("SetOptions failed to set followSymlinks")
	}
	if !adapter.generator.includeHidden {
		t.Error("SetOptions failed to set includeHidden")
	}
}