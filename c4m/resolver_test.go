package c4m

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// mockStorage implements the Storage interface for testing
type mockStorage struct {
	manifests map[string]string // C4 ID -> manifest content
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		manifests: make(map[string]string),
	}
}

func (ms *mockStorage) Get(id c4.ID) (io.ReadCloser, error) {
	content, ok := ms.manifests[id.String()]
	if !ok {
		return nil, fmt.Errorf("manifest not found: %s", id.String())
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (ms *mockStorage) addManifest(content string) c4.ID {
	id := c4.Identify(strings.NewReader(content))
	ms.manifests[id.String()] = "@c4m 1.0\n" + content
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
	_, err := m.WriteTo(&buf)
	if err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
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
