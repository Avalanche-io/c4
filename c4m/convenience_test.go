package c4m

import (
	"os"
	"testing"
)

// TestGetEntry_FullPath verifies that GetEntry indexes by full path.
func TestGetEntry_FullPath(t *testing.T) {
	m := NewBuilder().
		AddFile("root.txt").
		AddDir("src").
			AddFile("main.go").
			AddDir("internal").
				AddFile("helper.go").
			EndDir().
		End().
		MustBuild()

	tests := []struct {
		path     string
		wantName string
		wantNil  bool
	}{
		{"root.txt", "root.txt", false},
		{"src/", "src/", false},
		{"src/main.go", "main.go", false},
		{"src/internal/", "internal/", false},
		{"src/internal/helper.go", "helper.go", false},
		// Bare names for nested entries should NOT work
		{"main.go", "", true},
		{"helper.go", "", true},
		{"internal/", "", true},
		// Nonexistent
		{"nonexistent.txt", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			e := m.GetEntry(tt.path)
			if tt.wantNil {
				if e != nil {
					t.Errorf("GetEntry(%q) = %v, want nil", tt.path, e.Name)
				}
				return
			}
			if e == nil {
				t.Fatalf("GetEntry(%q) = nil, want entry with name %q", tt.path, tt.wantName)
			}
			if e.Name != tt.wantName {
				t.Errorf("GetEntry(%q).Name = %q, want %q", tt.path, e.Name, tt.wantName)
			}
		})
	}
}

// TestGetEntryByName verifies bare-name lookup.
func TestGetEntryByName(t *testing.T) {
	m := NewBuilder().
		AddFile("root.txt").
		AddDir("src").
			AddFile("main.go").
		End().
		MustBuild()

	// Bare names work for all entries regardless of depth
	if e := m.GetEntryByName("root.txt"); e == nil {
		t.Error("GetEntryByName(\"root.txt\") = nil")
	}
	if e := m.GetEntryByName("main.go"); e == nil {
		t.Error("GetEntryByName(\"main.go\") = nil")
	}
	if e := m.GetEntryByName("src/"); e == nil {
		t.Error("GetEntryByName(\"src/\") = nil")
	}
	if e := m.GetEntryByName("nonexistent"); e != nil {
		t.Error("GetEntryByName(\"nonexistent\") should be nil")
	}

	// When multiple entries share a bare name, last one wins
	m2 := NewBuilder().
		AddDir("a").
			AddFile("data.txt").
		End().
		AddDir("b").
			AddFile("data.txt").
		End().
		MustBuild()

	// GetEntryByName returns one of them (last indexed)
	e := m2.GetEntryByName("data.txt")
	if e == nil {
		t.Fatal("GetEntryByName(\"data.txt\") = nil, want non-nil")
	}

	// GetEntry with full path returns each one unambiguously
	ea := m2.GetEntry("a/data.txt")
	eb := m2.GetEntry("b/data.txt")
	if ea == nil {
		t.Error("GetEntry(\"a/data.txt\") = nil")
	}
	if eb == nil {
		t.Error("GetEntry(\"b/data.txt\") = nil")
	}
	if ea == eb {
		t.Error("GetEntry should return different entries for different full paths")
	}
}

// TestEntryPath verifies full path reconstruction.
func TestEntryPath(t *testing.T) {
	m := NewBuilder().
		AddFile("root.txt").
		AddDir("src").
			AddFile("main.go").
			AddDir("internal").
				AddFile("helper.go").
				AddDir("deep").
					AddFile("nested.go").
				EndDir().
			EndDir().
		End().
		MustBuild()

	tests := []struct {
		fullPath string
		want     string
	}{
		{"root.txt", "root.txt"},
		{"src/", "src/"},
		{"src/main.go", "src/main.go"},
		{"src/internal/", "src/internal/"},
		{"src/internal/helper.go", "src/internal/helper.go"},
		{"src/internal/deep/", "src/internal/deep/"},
		{"src/internal/deep/nested.go", "src/internal/deep/nested.go"},
	}

	for _, tt := range tests {
		t.Run(tt.fullPath, func(t *testing.T) {
			e := m.GetEntry(tt.fullPath)
			if e == nil {
				t.Fatalf("GetEntry(%q) = nil", tt.fullPath)
			}
			got := m.EntryPath(e)
			if got != tt.want {
				t.Errorf("EntryPath() = %q, want %q", got, tt.want)
			}
		})
	}

	// EntryPath on nil returns ""
	if got := m.EntryPath(nil); got != "" {
		t.Errorf("EntryPath(nil) = %q, want \"\"", got)
	}

	// EntryPath on unindexed entry returns ""
	orphan := &Entry{Name: "orphan.txt"}
	if got := m.EntryPath(orphan); got != "" {
		t.Errorf("EntryPath(orphan) = %q, want \"\"", got)
	}
}

// TestEntryPath_MatchesEntryTreePath verifies that EntryPath and
// EntryTreePath return the same value for all entries.
func TestEntryPath_MatchesEntryTreePath(t *testing.T) {
	m := NewBuilder().
		AddDir("project").
			AddDir("src").
				AddDir("shared").
					AddFile("file.txt").
				EndDir().
			EndDir().
		End().
		MustBuild()

	for _, e := range m.Entries {
		ep := m.EntryPath(e)
		etp := m.EntryTreePath(e)
		if ep != etp {
			t.Errorf("entry %q: EntryPath=%q, EntryTreePath=%q (should match)",
				e.Name, ep, etp)
		}
	}
}

// TestMoveEntry_SimpleRename renames a root-level file.
func TestMoveEntry_SimpleRename(t *testing.T) {
	m := NewBuilder().
		AddFile("old.txt", WithSize(100)).
		AddFile("other.txt", WithSize(200)).
		MustBuild()

	e := m.GetEntry("old.txt")
	if e == nil {
		t.Fatal("old.txt not found")
	}

	m.MoveEntry(e, nil, "new.txt")

	if m.GetEntry("new.txt") == nil {
		t.Error("new.txt not found after rename")
	}
	if m.GetEntry("old.txt") != nil {
		t.Error("old.txt should not exist after rename")
	}
	if e.Name != "new.txt" {
		t.Errorf("entry Name = %q, want \"new.txt\"", e.Name)
	}
	if e.Depth != 0 {
		t.Errorf("entry Depth = %d, want 0", e.Depth)
	}
}

// TestMoveEntry_IntoDir moves a root file into a directory.
func TestMoveEntry_IntoDir(t *testing.T) {
	m := NewBuilder().
		AddFile("loose.txt", WithSize(100)).
		AddDir("dest").
		End().
		MustBuild()

	loose := m.GetEntry("loose.txt")
	dest := m.GetEntry("dest/")
	if loose == nil || dest == nil {
		t.Fatal("setup failed")
	}

	m.MoveEntry(loose, dest, "loose.txt")

	if m.GetEntry("dest/loose.txt") == nil {
		t.Error("dest/loose.txt not found after move")
	}
	if m.GetEntry("loose.txt") != nil {
		t.Error("loose.txt at root should not exist after move")
	}
	if loose.Depth != 1 {
		t.Errorf("entry Depth = %d, want 1", loose.Depth)
	}
}

// TestMoveEntry_DirWithDescendants moves a directory with children.
func TestMoveEntry_DirWithDescendants(t *testing.T) {
	m := NewBuilder().
		AddDir("src").
			AddFile("main.go").
			AddDir("internal").
				AddFile("helper.go").
			EndDir().
		End().
		AddDir("dest").
		End().
		MustBuild()

	src := m.GetEntry("src/")
	dest := m.GetEntry("dest/")
	if src == nil || dest == nil {
		t.Fatal("setup failed")
	}

	m.MoveEntry(src, dest, "src/")

	// src/ should now be at dest/src/
	if m.GetEntry("dest/src/") == nil {
		t.Error("dest/src/ not found after move")
	}
	if m.GetEntry("dest/src/main.go") == nil {
		t.Error("dest/src/main.go not found after move")
	}
	if m.GetEntry("dest/src/internal/") == nil {
		t.Error("dest/src/internal/ not found after move")
	}
	if m.GetEntry("dest/src/internal/helper.go") == nil {
		t.Error("dest/src/internal/helper.go not found after move")
	}
	// Old paths should be gone
	if m.GetEntry("src/") != nil {
		t.Error("src/ at root should not exist after move")
	}
}

// TestMoveEntry_OutOfDir moves a file from a directory to root.
func TestMoveEntry_OutOfDir(t *testing.T) {
	m := NewBuilder().
		AddDir("src").
			AddFile("main.go").
		End().
		MustBuild()

	mainGo := m.GetEntry("src/main.go")
	if mainGo == nil {
		t.Fatal("src/main.go not found")
	}

	m.MoveEntry(mainGo, nil, "main.go")

	if m.GetEntry("main.go") == nil {
		t.Error("main.go not found at root after move")
	}
	if mainGo.Depth != 0 {
		t.Errorf("entry Depth = %d, want 0", mainGo.Depth)
	}
}

// TestMoveEntry_WithRename moves and renames simultaneously.
func TestMoveEntry_WithRename(t *testing.T) {
	m := NewBuilder().
		AddFile("old.txt").
		AddDir("dest").
		End().
		MustBuild()

	old := m.GetEntry("old.txt")
	dest := m.GetEntry("dest/")
	if old == nil || dest == nil {
		t.Fatal("setup failed")
	}

	m.MoveEntry(old, dest, "renamed.txt")

	if m.GetEntry("dest/renamed.txt") == nil {
		t.Error("dest/renamed.txt not found after move+rename")
	}
	if old.Name != "renamed.txt" {
		t.Errorf("entry Name = %q, want \"renamed.txt\"", old.Name)
	}
}

// TestMoveEntry_PreservesEntryCount ensures no entries are lost or duplicated.
func TestMoveEntry_PreservesEntryCount(t *testing.T) {
	m := NewBuilder().
		AddFile("a.txt").
		AddDir("d1").
			AddFile("b.txt").
			AddDir("d2").
				AddFile("c.txt").
			EndDir().
		End().
		MustBuild()

	before := len(m.Entries)

	d2 := m.GetEntry("d1/d2/")
	if d2 == nil {
		t.Fatal("d1/d2/ not found")
	}

	// Move d2 to root
	m.MoveEntry(d2, nil, "d2/")

	after := len(m.Entries)
	if after != before {
		t.Errorf("entry count changed: before=%d, after=%d", before, after)
	}
}

// TestAncestors_Ordering documents that Ancestors returns inner-to-outer.
func TestAncestors_Ordering(t *testing.T) {
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
	if deep == nil {
		t.Fatal("a/b/c/deep.txt not found")
	}

	ancestors := m.Ancestors(deep)
	if len(ancestors) != 3 {
		t.Fatalf("expected 3 ancestors, got %d", len(ancestors))
	}

	// Ordering: [parent, grandparent, great-grandparent]
	wantNames := []string{"c/", "b/", "a/"}
	for i, a := range ancestors {
		if a.Name != wantNames[i] {
			t.Errorf("ancestor[%d] = %q, want %q", i, a.Name, wantNames[i])
		}
	}

	// EntryPath is the better alternative to manual path reconstruction
	if path := m.EntryPath(deep); path != "a/b/c/deep.txt" {
		t.Errorf("EntryPath = %q, want \"a/b/c/deep.txt\"", path)
	}
}

// TestGetEntry_DirectoryPaths verifies directory trailing slash behavior.
func TestGetEntry_DirectoryPaths(t *testing.T) {
	m := NewBuilder().
		AddDir("src").
			AddDir("internal").
			EndDir().
		End().
		MustBuild()

	// Directories are stored with trailing slash
	if m.GetEntry("src/") == nil {
		t.Error("GetEntry(\"src/\") should find root dir")
	}
	if m.GetEntry("src/internal/") == nil {
		t.Error("GetEntry(\"src/internal/\") should find nested dir")
	}
	// Without trailing slash, directories are not found
	if m.GetEntry("src") != nil {
		t.Error("GetEntry(\"src\") should not find directory (missing trailing slash)")
	}
}

// TestMoveEntry_Directory_DepthUpdates verifies all descendants have correct depths.
func TestMoveEntry_Directory_DepthUpdates(t *testing.T) {
	m := NewBuilder().
		AddDir("top").
			AddFile("f1.txt").
			AddDir("mid").
				AddFile("f2.txt").
				AddDir("bot").
					AddFile("f3.txt").
				EndDir().
			EndDir().
		End().
		AddDir("target").
			AddDir("nested").
			EndDir().
		End().
		MustBuild()

	top := m.GetEntry("top/")
	nested := m.GetEntry("target/nested/")
	if top == nil || nested == nil {
		t.Fatal("setup failed")
	}

	// Move top/ (depth 0) into target/nested/ (depth 1), so top/ becomes depth 2
	m.MoveEntry(top, nested, "top/")

	wantDepths := map[string]int{
		"target/nested/top/":               2,
		"target/nested/top/f1.txt":         3,
		"target/nested/top/mid/":           3,
		"target/nested/top/mid/f2.txt":     4,
		"target/nested/top/mid/bot/":       4,
		"target/nested/top/mid/bot/f3.txt": 5,
	}

	for path, wantDepth := range wantDepths {
		e := m.GetEntry(path)
		if e == nil {
			t.Errorf("GetEntry(%q) = nil after move", path)
			continue
		}
		if e.Depth != wantDepth {
			t.Errorf("GetEntry(%q).Depth = %d, want %d", path, e.Depth, wantDepth)
		}
	}
}

// TestGetEntry_EmptyManifest verifies behavior on empty manifest.
func TestGetEntry_EmptyManifest(t *testing.T) {
	m := NewManifest()
	if m.GetEntry("anything") != nil {
		t.Error("GetEntry on empty manifest should return nil")
	}
	if m.GetEntryByName("anything") != nil {
		t.Error("GetEntryByName on empty manifest should return nil")
	}
}

// TestEntryPath_AfterSort verifies index is correct after re-sorting.
func TestEntryPath_AfterSort(t *testing.T) {
	m := NewManifest()
	m.AddEntry(&Entry{Name: "dir/", Depth: 0, Mode: os.ModeDir})
	m.AddEntry(&Entry{Name: "b.txt", Depth: 1})
	m.AddEntry(&Entry{Name: "a.txt", Depth: 1})

	m.SortEntries()

	// After sort, a.txt should come before b.txt
	ea := m.GetEntry("dir/a.txt")
	eb := m.GetEntry("dir/b.txt")
	if ea == nil || eb == nil {
		t.Fatal("entries not found after sort")
	}
	if m.EntryPath(ea) != "dir/a.txt" {
		t.Errorf("EntryPath(a.txt) = %q, want \"dir/a.txt\"", m.EntryPath(ea))
	}
	if m.EntryPath(eb) != "dir/b.txt" {
		t.Errorf("EntryPath(b.txt) = %q, want \"dir/b.txt\"", m.EntryPath(eb))
	}
}
