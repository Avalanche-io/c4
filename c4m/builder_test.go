package c4m

import (
	"os"
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

	entry := m.GetEntry("readme.txt")
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
	src := m.GetEntry("src/")
	if src == nil {
		t.Fatal("expected to find src/")
	}
	if src.Depth != 0 {
		t.Errorf("src/ expected depth 0, got %d", src.Depth)
	}

	// Check main.go inside src (full path includes parent)
	mainGo := m.GetEntry("src/main.go")
	if mainGo == nil {
		t.Fatal("expected to find src/main.go")
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

	// Check depths (GetEntry uses full paths)
	tests := []struct {
		path  string
		depth int
	}{
		{"src/", 0},
		{"src/main.go", 1},
		{"src/internal/", 1},
		{"src/internal/helper.go", 2},
		{"src/internal/deep/", 2},
		{"src/internal/deep/nested.go", 3},
		{"src/util.go", 1},
	}

	for _, tt := range tests {
		entry := m.GetEntry(tt.path)
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

	entry := m.GetEntry("test.txt")
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

	entry := m.GetEntry("link")
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
	if m.GetEntry("nodash/") == nil {
		t.Error("expected to find nodash/")
	}
	// Should NOT find without trailing slash
	if m.GetEntry("nodash") != nil {
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

	if m.GetEntry("existing.txt") == nil {
		t.Error("existing.txt should still exist")
	}
	if m.GetEntry("new.txt") == nil {
		t.Error("new.txt should exist")
	}
	if m.GetEntry("newdir/") == nil {
		t.Error("newdir/ should exist")
	}
	if m.GetEntry("newdir/nested.txt") == nil {
		t.Error("newdir/nested.txt should exist")
	}
}

// ----------------------------------------------------------------------------
// Tree Navigation Tests
// ----------------------------------------------------------------------------

func TestTreeNavigation_GetEntry(t *testing.T) {
	m := NewBuilder().
		AddFile("root.txt").
		AddDir("dir").
			AddFile("nested.txt").
		End().
		MustBuild()

	if m.GetEntry("root.txt") == nil {
		t.Error("GetEntry failed for root.txt")
	}
	if m.GetEntry("dir/") == nil {
		t.Error("GetEntry failed for dir/")
	}
	if m.GetEntry("dir/nested.txt") == nil {
		t.Error("GetEntry failed for dir/nested.txt")
	}
	if m.GetEntry("nonexistent") != nil {
		t.Error("GetEntry should return nil for nonexistent")
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

	parent := m.GetEntry("parent/")
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
	file := m.GetEntry("parent/child1.txt")
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

	deep := m.GetEntry("level1/level2/deep.txt")
	parent := m.Parent(deep)
	if parent == nil || parent.Name != "level2/" {
		t.Error("Parent of deep.txt should be level2/")
	}

	grandparent := m.Parent(parent)
	if grandparent == nil || grandparent.Name != "level1/" {
		t.Error("Parent of level2/ should be level1/")
	}

	// Root level has no parent
	root := m.GetEntry("level1/")
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
	file1 := m.GetEntry("file1.txt")
	siblings := m.Siblings(file1)
	if len(siblings) != 2 {
		t.Errorf("expected 2 root siblings, got %d", len(siblings))
	}

	// Test nested siblings
	nested1 := m.GetEntry("dir/nested1.txt")
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

	deep := m.GetEntry("a/b/c/deep.txt")
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
	root := m.GetEntry("a/")
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

	root := m.GetEntry("root/")
	descendants := m.Descendants(root)

	if len(descendants) != 4 {
		t.Errorf("expected 4 descendants, got %d", len(descendants))
	}

	// Test Descendants on file (should be nil)
	file := m.GetEntry("root/file1.txt")
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
	if m.GetEntry("added.txt") == nil {
		t.Error("index should include newly added entry")
	}
}

func TestManifestBuilder_WithC4ID(t *testing.T) {
	// Use a known test ID
	testID := mustParseID("c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")

	m := NewBuilder().
		AddFile("test.txt", WithC4ID(testID)).
		MustBuild()

	entry := m.GetEntry("test.txt")
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

	entry := m.GetEntry("test.txt")
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
	if m.GetEntry("withslash/") == nil {
		t.Error("expected to find withslash/")
	}
	if m.GetEntry("withslash//") != nil {
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

