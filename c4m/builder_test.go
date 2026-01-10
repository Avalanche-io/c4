package c4m

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// mustParseID parses a C4 ID string or panics
func mustParseID(s string) c4.ID {
	id, err := c4.Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}

func TestManifestBuilder_Basic(t *testing.T) {
	m := NewBuilder().
		AddFile("readme.txt", WithSize(100)).
		AddFile("go.mod", WithSize(50)).
		MustBuild()

	if len(m.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.Entries))
	}

	entry := m.GetByPath("readme.txt")
	if entry == nil {
		t.Fatal("expected to find readme.txt")
	}
	if entry.Size != 100 {
		t.Errorf("expected size 100, got %d", entry.Size)
	}
	if entry.Depth != 0 {
		t.Errorf("expected depth 0, got %d", entry.Depth)
	}
}

func TestManifestBuilder_WithDirectory(t *testing.T) {
	m := NewBuilder().
		AddFile("readme.txt").
		AddDir("src").
			AddFile("main.go", WithSize(500)).
			AddFile("util.go", WithSize(200)).
		End().
		AddFile("go.mod").
		MustBuild()

	if len(m.Entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(m.Entries))
	}

	// Check src directory
	src := m.GetByPath("src/")
	if src == nil {
		t.Fatal("expected to find src/")
	}
	if src.Depth != 0 {
		t.Errorf("src/ expected depth 0, got %d", src.Depth)
	}

	// Check main.go inside src
	mainGo := m.GetByPath("main.go")
	if mainGo == nil {
		t.Fatal("expected to find main.go")
	}
	if mainGo.Depth != 1 {
		t.Errorf("main.go expected depth 1, got %d", mainGo.Depth)
	}
	if mainGo.Size != 500 {
		t.Errorf("main.go expected size 500, got %d", mainGo.Size)
	}
}

func TestManifestBuilder_NestedDirectories(t *testing.T) {
	m := NewBuilder().
		AddDir("src").
			AddFile("main.go").
			AddDir("internal").
				AddFile("helper.go").
				AddDir("deep").
					AddFile("nested.go").
				EndDir().
			EndDir().
			AddFile("util.go").
		End().
		MustBuild()

	// Check depths
	tests := []struct {
		path  string
		depth int
	}{
		{"src/", 0},
		{"main.go", 1},
		{"internal/", 1},
		{"helper.go", 2},
		{"deep/", 2},
		{"nested.go", 3},
		{"util.go", 1},
	}

	for _, tt := range tests {
		entry := m.GetByPath(tt.path)
		if entry == nil {
			t.Errorf("expected to find %s", tt.path)
			continue
		}
		if entry.Depth != tt.depth {
			t.Errorf("%s expected depth %d, got %d", tt.path, tt.depth, entry.Depth)
		}
	}
}

func TestManifestBuilder_Options(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	m := NewBuilder().
		AddFile("test.txt",
			WithSize(1000),
			WithMode(0644),
			WithTimestamp(ts),
		).
		MustBuild()

	entry := m.GetByPath("test.txt")
	if entry == nil {
		t.Fatal("expected to find test.txt")
	}
	if entry.Size != 1000 {
		t.Errorf("expected size 1000, got %d", entry.Size)
	}
	if entry.Mode != 0644 {
		t.Errorf("expected mode 0644, got %o", entry.Mode)
	}
	if !entry.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, entry.Timestamp)
	}
}

func TestManifestBuilder_WithTarget(t *testing.T) {
	m := NewBuilder().
		AddFile("link", WithTarget("/usr/bin/something")).
		MustBuild()

	entry := m.GetByPath("link")
	if entry == nil {
		t.Fatal("expected to find link")
	}
	if entry.Target != "/usr/bin/something" {
		t.Errorf("expected target /usr/bin/something, got %s", entry.Target)
	}
	if entry.Mode&os.ModeSymlink == 0 {
		t.Error("expected symlink mode to be set")
	}
}

func TestManifestBuilder_DirTrailingSlash(t *testing.T) {
	// Test that AddDir adds trailing slash automatically
	m := NewBuilder().
		AddDir("nodash").
			AddFile("file.txt").
		End().
		MustBuild()

	// Should find with trailing slash
	if m.GetByPath("nodash/") == nil {
		t.Error("expected to find nodash/")
	}
	// Should NOT find without trailing slash
	if m.GetByPath("nodash") != nil {
		t.Error("should not find nodash without trailing slash")
	}
}

func TestManifest_Builder(t *testing.T) {
	// Test Builder() on existing manifest
	m := NewManifest()
	m.AddEntry(&Entry{Name: "existing.txt", Depth: 0})

	m.Builder().
		AddFile("new.txt").
		AddDir("newdir").
			AddFile("nested.txt").
		End()

	if len(m.Entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(m.Entries))
	}

	if m.GetByPath("existing.txt") == nil {
		t.Error("existing.txt should still exist")
	}
	if m.GetByPath("new.txt") == nil {
		t.Error("new.txt should exist")
	}
	if m.GetByPath("newdir/") == nil {
		t.Error("newdir/ should exist")
	}
	if m.GetByPath("nested.txt") == nil {
		t.Error("nested.txt should exist")
	}
}

// ----------------------------------------------------------------------------
// Tree Navigation Tests
// ----------------------------------------------------------------------------

func TestTreeNavigation_GetByPath(t *testing.T) {
	m := NewBuilder().
		AddFile("root.txt").
		AddDir("dir").
			AddFile("nested.txt").
		End().
		MustBuild()

	if m.GetByPath("root.txt") == nil {
		t.Error("GetByPath failed for root.txt")
	}
	if m.GetByPath("dir/") == nil {
		t.Error("GetByPath failed for dir/")
	}
	if m.GetByPath("nested.txt") == nil {
		t.Error("GetByPath failed for nested.txt")
	}
	if m.GetByPath("nonexistent") != nil {
		t.Error("GetByPath should return nil for nonexistent")
	}
}

func TestTreeNavigation_Children(t *testing.T) {
	m := NewBuilder().
		AddDir("parent").
			AddFile("child1.txt").
			AddFile("child2.txt").
			AddDir("subdir").
				AddFile("grandchild.txt").
			EndDir().
		End().
		MustBuild()

	parent := m.GetByPath("parent/")
	children := m.Children(parent)

	if len(children) != 3 {
		t.Errorf("expected 3 children, got %d", len(children))
	}

	// Check children names
	names := make(map[string]bool)
	for _, c := range children {
		names[c.Name] = true
	}
	if !names["child1.txt"] || !names["child2.txt"] || !names["subdir/"] {
		t.Error("missing expected children")
	}

	// Test Children on file (should return nil)
	file := m.GetByPath("child1.txt")
	if m.Children(file) != nil {
		t.Error("Children of file should be nil")
	}

	// Test Children on nil
	if m.Children(nil) != nil {
		t.Error("Children of nil should be nil")
	}
}

func TestTreeNavigation_Parent(t *testing.T) {
	m := NewBuilder().
		AddDir("level1").
			AddDir("level2").
				AddFile("deep.txt").
			EndDir().
		End().
		MustBuild()

	deep := m.GetByPath("deep.txt")
	parent := m.Parent(deep)
	if parent == nil || parent.Name != "level2/" {
		t.Error("Parent of deep.txt should be level2/")
	}

	grandparent := m.Parent(parent)
	if grandparent == nil || grandparent.Name != "level1/" {
		t.Error("Parent of level2/ should be level1/")
	}

	// Root level has no parent
	root := m.GetByPath("level1/")
	if m.Parent(root) != nil {
		t.Error("Root level entry should have nil parent")
	}

	// Test Parent on nil
	if m.Parent(nil) != nil {
		t.Error("Parent of nil should be nil")
	}
}

func TestTreeNavigation_Siblings(t *testing.T) {
	m := NewBuilder().
		AddFile("file1.txt").
		AddFile("file2.txt").
		AddDir("dir").
			AddFile("nested1.txt").
			AddFile("nested2.txt").
		End().
		MustBuild()

	// Test root-level siblings
	file1 := m.GetByPath("file1.txt")
	siblings := m.Siblings(file1)
	if len(siblings) != 2 {
		t.Errorf("expected 2 root siblings, got %d", len(siblings))
	}

	// Test nested siblings
	nested1 := m.GetByPath("nested1.txt")
	nestedSiblings := m.Siblings(nested1)
	if len(nestedSiblings) != 1 {
		t.Errorf("expected 1 nested sibling, got %d", len(nestedSiblings))
	}
	if nestedSiblings[0].Name != "nested2.txt" {
		t.Error("sibling should be nested2.txt")
	}

	// Test Siblings on nil
	if m.Siblings(nil) != nil {
		t.Error("Siblings of nil should be nil")
	}
}

func TestTreeNavigation_Ancestors(t *testing.T) {
	m := NewBuilder().
		AddDir("a").
			AddDir("b").
				AddDir("c").
					AddFile("deep.txt").
				EndDir().
			EndDir().
		End().
		MustBuild()

	deep := m.GetByPath("deep.txt")
	ancestors := m.Ancestors(deep)

	if len(ancestors) != 3 {
		t.Errorf("expected 3 ancestors, got %d", len(ancestors))
	}

	// Check order: immediate parent first, root last
	if ancestors[0].Name != "c/" {
		t.Errorf("first ancestor should be c/, got %s", ancestors[0].Name)
	}
	if ancestors[1].Name != "b/" {
		t.Errorf("second ancestor should be b/, got %s", ancestors[1].Name)
	}
	if ancestors[2].Name != "a/" {
		t.Errorf("third ancestor should be a/, got %s", ancestors[2].Name)
	}

	// Test Ancestors on root (should be nil)
	root := m.GetByPath("a/")
	if m.Ancestors(root) != nil {
		t.Error("Ancestors of root should be nil")
	}

	// Test Ancestors on nil
	if m.Ancestors(nil) != nil {
		t.Error("Ancestors of nil should be nil")
	}
}

func TestTreeNavigation_Descendants(t *testing.T) {
	m := NewBuilder().
		AddDir("root").
			AddFile("file1.txt").
			AddDir("sub").
				AddFile("file2.txt").
				AddFile("file3.txt").
			EndDir().
		End().
		MustBuild()

	root := m.GetByPath("root/")
	descendants := m.Descendants(root)

	if len(descendants) != 4 {
		t.Errorf("expected 4 descendants, got %d", len(descendants))
	}

	// Test Descendants on file (should be nil)
	file := m.GetByPath("file1.txt")
	if m.Descendants(file) != nil {
		t.Error("Descendants of file should be nil")
	}

	// Test Descendants on nil
	if m.Descendants(nil) != nil {
		t.Error("Descendants of nil should be nil")
	}
}

func TestTreeNavigation_Root(t *testing.T) {
	m := NewBuilder().
		AddFile("root1.txt").
		AddDir("dir").
			AddFile("nested.txt").
		End().
		AddFile("root2.txt").
		MustBuild()

	roots := m.Root()
	if len(roots) != 3 {
		t.Errorf("expected 3 root entries, got %d", len(roots))
	}

	names := make(map[string]bool)
	for _, r := range roots {
		names[r.Name] = true
	}
	if !names["root1.txt"] || !names["dir/"] || !names["root2.txt"] {
		t.Error("missing expected root entries")
	}
}

func TestTreeNavigation_IndexInvalidation(t *testing.T) {
	m := NewBuilder().
		AddFile("initial.txt").
		MustBuild()

	// Build index
	_ = m.Root()

	// Add entry should invalidate index
	m.AddEntry(&Entry{Name: "added.txt", Depth: 0})

	// Index should be rebuilt and include new entry
	if m.GetByPath("added.txt") == nil {
		t.Error("index should include newly added entry")
	}
}

func TestManifestBuilder_WithC4ID(t *testing.T) {
	// Use a known test ID
	testID := mustParseID("c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")

	m := NewBuilder().
		AddFile("test.txt", WithC4ID(testID)).
		MustBuild()

	entry := m.GetByPath("test.txt")
	if entry == nil {
		t.Fatal("expected to find test.txt")
	}
	if entry.C4ID != testID {
		t.Errorf("expected C4ID %s, got %s", testID, entry.C4ID)
	}
}

func TestManifestBuilder_WithAttrs(t *testing.T) {
	testID := mustParseID("c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")
	ts := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)

	m := NewBuilder().
		AddFile("test.txt", WithAttrs(testID, 999, 0755, ts)).
		MustBuild()

	entry := m.GetByPath("test.txt")
	if entry == nil {
		t.Fatal("expected to find test.txt")
	}
	if entry.C4ID != testID {
		t.Errorf("expected C4ID %s, got %s", testID, entry.C4ID)
	}
	if entry.Size != 999 {
		t.Errorf("expected size 999, got %d", entry.Size)
	}
	if entry.Mode != 0755 {
		t.Errorf("expected mode 0755, got %o", entry.Mode)
	}
	if !entry.Timestamp.Equal(ts) {
		t.Errorf("expected timestamp %v, got %v", ts, entry.Timestamp)
	}
}

func TestManifestBuilder_EndDirOnRootDir(t *testing.T) {
	// Test EndDir() when called on a root-level directory (no parent)
	m := NewBuilder().
		AddDir("root").
			AddFile("child.txt").
		EndDir(). // This calls EndDir() on root dir, should return self
		End().
		MustBuild()

	if len(m.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.Entries))
	}
}

func TestManifestBuilder_DirWithExistingSlash(t *testing.T) {
	// Test that AddDir handles a name already with trailing slash
	m := NewBuilder().
		AddDir("withslash/"). // Already has slash
			AddFile("child.txt").
		End().
		MustBuild()

	// Should find with single trailing slash (not double)
	if m.GetByPath("withslash/") == nil {
		t.Error("expected to find withslash/")
	}
	if m.GetByPath("withslash//") != nil {
		t.Error("should not find withslash//")
	}
}

func TestManifestBuilder_Roundtrip(t *testing.T) {
	// Build a manifest with builder
	ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewBuilder().
		AddFile("readme.txt", WithSize(100), WithMode(0644), WithTimestamp(ts)).
		AddDir("src", WithTimestamp(ts)).
			AddFile("main.go", WithSize(500), WithMode(0644), WithTimestamp(ts)).
		End().
		MustBuild()

	// Encode
	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Decode
	m2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// Verify entry count
	if len(m.Entries) != len(m2.Entries) {
		t.Errorf("entry count mismatch: %d vs %d", len(m.Entries), len(m2.Entries))
	}

	// Re-encode and compare (canonical form should be identical)
	data2, err := Marshal(m2)
	if err != nil {
		t.Fatalf("second Marshal error: %v", err)
	}

	if string(data) != string(data2) {
		t.Errorf("roundtrip data mismatch:\nOriginal:\n%s\nRoundtrip:\n%s", data, data2)
	}
}

// ----------------------------------------------------------------------------
// Remove and Layer Tests
// ----------------------------------------------------------------------------

func TestManifestBuilder_WithBase(t *testing.T) {
	// Create a base manifest
	base := NewBuilder().
		AddFile("file1.txt", WithSize(100)).
		AddFile("file2.txt", WithSize(200)).
		MustBuild()

	// Create manifest extending base
	m, err := NewBuilder().
		WithBase(base).
		AddFile("file3.txt", WithSize(300)).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check base ID was set
	if m.Base.IsNil() {
		t.Error("expected Base ID to be set")
	}

	// Check new file was added
	if m.GetByPath("file3.txt") == nil {
		t.Error("expected to find file3.txt")
	}
}

func TestManifestBuilder_WithBaseID(t *testing.T) {
	testID := mustParseID("c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")

	m, err := NewBuilder().
		WithBaseID(testID).
		AddFile("newfile.txt").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Base != testID {
		t.Errorf("expected Base ID %s, got %s", testID, m.Base)
	}
}

func TestManifestBuilder_Remove_WithValidation(t *testing.T) {
	// Create base manifest
	base := NewBuilder().
		AddFile("keep.txt", WithSize(100)).
		AddFile("remove.txt", WithSize(200)).
		MustBuild()

	// Remove a file that exists
	m, err := NewBuilder().
		WithBase(base).
		Remove("remove.txt").
		Build()

	if err != nil {
		t.Fatalf("unexpected error removing existing file: %v", err)
	}

	// Check remove layer was created
	if len(m.Layers) != 1 {
		t.Errorf("expected 1 layer, got %d", len(m.Layers))
	}
	if m.Layers[0].Type != LayerTypeRemove {
		t.Error("expected LayerTypeRemove")
	}
}

func TestManifestBuilder_Remove_NonExistent(t *testing.T) {
	// Create base manifest
	base := NewBuilder().
		AddFile("exists.txt", WithSize(100)).
		MustBuild()

	// Try to remove a file that doesn't exist
	m, err := NewBuilder().
		WithBase(base).
		Remove("notexists.txt").
		Build()

	// Should return error but manifest should still be valid
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if m == nil {
		t.Fatal("manifest should still be returned even with error")
	}

	// The removal should still be in the layer
	if len(m.Layers) != 1 {
		t.Errorf("expected 1 layer even with error, got %d", len(m.Layers))
	}
}

func TestManifestBuilder_Remove_NoBase(t *testing.T) {
	// Try to remove without base
	m, err := NewBuilder().
		Remove("somefile.txt").
		Build()

	// Should return error
	if err == nil {
		t.Error("expected error for removal without base")
	}
	if m == nil {
		t.Fatal("manifest should still be returned")
	}

	// Removal should still be in layer
	if len(m.Layers) != 1 {
		t.Errorf("expected 1 layer, got %d", len(m.Layers))
	}
}

func TestManifestBuilder_Remove_BaseIDOnly(t *testing.T) {
	// Generate a real C4 ID
	testID := c4.Identify(strings.NewReader("test content for base ID"))

	builder := NewBuilder().
		WithBaseID(testID).
		Remove("somefile.txt")

	m, err := builder.Build()

	// Should succeed (can't validate, but that's OK)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have a warning
	warnings := builder.Warnings()
	if len(warnings) == 0 {
		t.Error("expected warning about unvalidatable removals")
	}

	if m == nil {
		t.Fatal("manifest should be returned")
	}

	// Verify base ID was set
	if m.Base.IsNil() {
		t.Error("expected Base ID to be set")
	}
}

func TestManifestBuilder_RemoveDir(t *testing.T) {
	base := NewBuilder().
		AddDir("mydir").
			AddFile("nested.txt").
		End().
		MustBuild()

	m, err := NewBuilder().
		WithBase(base).
		RemoveDir("mydir").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that mydir/ was added to removals (with trailing slash)
	found := false
	for _, e := range m.Entries {
		if e.Name == "mydir/" && e.removeLayer {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected mydir/ in removals")
	}
}

func TestManifestBuilder_LayerMetadata(t *testing.T) {
	ts := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	base := NewBuilder().
		AddFile("old.txt").
		MustBuild()

	m, _ := NewBuilder().
		WithBase(base).
		By("testuser").
		Note("Test removal").
		At(ts).
		Remove("old.txt").
		Build()

	if len(m.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(m.Layers))
	}

	layer := m.Layers[0]
	if layer.By != "testuser" {
		t.Errorf("expected By 'testuser', got %q", layer.By)
	}
	if layer.Note != "Test removal" {
		t.Errorf("expected Note 'Test removal', got %q", layer.Note)
	}
	if !layer.Time.Equal(ts) {
		t.Errorf("expected Time %v, got %v", ts, layer.Time)
	}
}

func TestManifestBuilder_MustBuild_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild should panic on error")
		}
	}()

	// This should panic because we're removing without base
	NewBuilder().
		Remove("file.txt").
		MustBuild()
}
