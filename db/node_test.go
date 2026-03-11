package db

import (
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func testID(s string) c4.ID {
	return c4.Identify(strings.NewReader(s))
}

func TestNodePutResolve(t *testing.T) {
	root := defaultRoot()

	id := testID("hello")
	root2 := root.put([]string{"mnt", "project", "main"}, id)

	got, ok := root2.resolve([]string{"mnt", "project", "main"})
	if !ok {
		t.Fatal("expected to resolve")
	}
	if got != id {
		t.Fatalf("got %s, want %s", got, id)
	}

	// Original root unchanged (COW)
	if _, ok := root.resolve([]string{"mnt", "project", "main"}); ok {
		t.Fatal("original root should not have the new entry")
	}
}

func TestNodePutOverwrite(t *testing.T) {
	root := defaultRoot()
	id1 := testID("v1")
	id2 := testID("v2")

	root = root.put([]string{"mnt", "file"}, id1)
	root = root.put([]string{"mnt", "file"}, id2)

	got, ok := root.resolve([]string{"mnt", "file"})
	if !ok {
		t.Fatal("expected to resolve")
	}
	if got != id2 {
		t.Fatal("expected v2")
	}
}

func TestNodeDelete(t *testing.T) {
	root := defaultRoot()
	id := testID("data")

	root = root.put([]string{"mnt", "project", "file"}, id)
	if _, ok := root.resolve([]string{"mnt", "project", "file"}); !ok {
		t.Fatal("should exist before delete")
	}

	root = root.del([]string{"mnt", "project", "file"})
	if _, ok := root.resolve([]string{"mnt", "project", "file"}); ok {
		t.Fatal("should not exist after delete")
	}
}

func TestNodeDeleteNonexistent(t *testing.T) {
	root := defaultRoot()
	root2 := root.del([]string{"mnt", "doesnotexist"})
	if root2 != root {
		t.Fatal("deleting nonexistent should return same node")
	}
}

func TestNodeCOWIsolation(t *testing.T) {
	root := defaultRoot()
	id1 := testID("a")
	id2 := testID("b")

	v1 := root.put([]string{"mnt", "file"}, id1)
	v2 := root.put([]string{"mnt", "file"}, id2)

	got1, _ := v1.resolve([]string{"mnt", "file"})
	got2, _ := v2.resolve([]string{"mnt", "file"})

	if got1 != id1 {
		t.Fatal("v1 should see id1")
	}
	if got2 != id2 {
		t.Fatal("v2 should see id2")
	}
}

func TestNodeStructuralSharing(t *testing.T) {
	root := defaultRoot()
	id := testID("x")
	root2 := root.put([]string{"mnt", "project", "file"}, id)

	// The "bin" subtree should be shared (same pointer)
	if root.get([]string{"bin"}) != root2.get([]string{"bin"}) {
		t.Fatal("bin subtree should be shared between versions")
	}
	// The "mnt" subtree should differ (modified path)
	if root.get([]string{"mnt"}) == root2.get([]string{"mnt"}) {
		t.Fatal("mnt subtree should differ between versions")
	}
}

func TestNodeGet(t *testing.T) {
	root := defaultRoot()
	id := testID("data")
	root = root.put([]string{"mnt", "project", "main"}, id)

	// Get directory
	n := root.get([]string{"mnt", "project"})
	if n == nil {
		t.Fatal("directory should exist")
	}
	if !n.isDir() {
		t.Fatal("should be directory")
	}

	// Get leaf
	n = root.get([]string{"mnt", "project", "main"})
	if n == nil {
		t.Fatal("leaf should exist")
	}
	if !n.isLeaf() {
		t.Fatal("should be leaf")
	}

	// Get nonexistent
	n = root.get([]string{"mnt", "nope"})
	if n != nil {
		t.Fatal("should be nil for nonexistent")
	}
}

func TestNodeLeafCount(t *testing.T) {
	root := defaultRoot()
	if root.leafCount() != 0 {
		t.Fatal("empty root should have 0 leaves")
	}

	root = root.put([]string{"mnt", "a"}, testID("a"))
	root = root.put([]string{"mnt", "b"}, testID("b"))
	root = root.put([]string{"mnt", "sub", "c"}, testID("c"))

	if got := root.leafCount(); got != 3 {
		t.Fatalf("expected 3 leaves, got %d", got)
	}
}

func TestNodeWalkLeaves(t *testing.T) {
	root := defaultRoot()
	root = root.put([]string{"mnt", "a"}, testID("a"))
	root = root.put([]string{"mnt", "b"}, testID("b"))

	seen := make(map[string]c4.ID)
	root.walkLeaves(func(path string, id c4.ID) {
		seen[path] = id
	})

	if len(seen) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(seen))
	}
	if _, ok := seen["mnt/a"]; !ok {
		t.Fatal("missing mnt/a")
	}
	if _, ok := seen["mnt/b"]; !ok {
		t.Fatal("missing mnt/b")
	}
}

func TestMerklePersistLoadRoundTrip(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	root := defaultRoot()
	root = root.put([]string{"mnt", "project", "main"}, testID("main"))
	root = root.put([]string{"mnt", "project", "test"}, testID("test"))
	root = root.put([]string{"home", "user", "config"}, testID("cfg"))

	// Persist (all nodes dirty)
	rootID, err := db.persistNode(root)
	if err != nil {
		t.Fatalf("persistNode: %v", err)
	}
	if rootID.IsNil() {
		t.Fatal("root blobID is nil after persist")
	}

	// Load from store
	rebuilt, err := db.loadNode(rootID)
	if err != nil {
		t.Fatalf("loadNode: %v", err)
	}

	// Verify all leaves survived
	for _, tc := range []struct {
		path []string
		id   c4.ID
	}{
		{[]string{"mnt", "project", "main"}, testID("main")},
		{[]string{"mnt", "project", "test"}, testID("test")},
		{[]string{"home", "user", "config"}, testID("cfg")},
	} {
		got, ok := rebuilt.resolve(tc.path)
		if !ok {
			t.Fatalf("missing after round-trip: %v", tc.path)
		}
		if got != tc.id {
			t.Fatalf("wrong ID after round-trip at %v", tc.path)
		}
	}

	// Verify directories survived
	for _, dir := range []string{"bin", "etc", "home", "mnt", "tmp"} {
		n := rebuilt.get([]string{dir})
		if n == nil || !n.isDir() {
			t.Fatalf("directory %s missing after round-trip", dir)
		}
	}

	// Verify blobIDs are set (clean after load)
	if rebuilt.blobID.IsNil() {
		t.Fatal("rebuilt root should have blobID set")
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"/", nil},
		{"mnt", []string{"mnt"}},
		{"/mnt", []string{"mnt"}},
		{"mnt/project/main", []string{"mnt", "project", "main"}},
		{"/mnt/project/main/", []string{"mnt", "project", "main"}},
	}
	for _, tc := range tests {
		got := splitPath(tc.input)
		if len(got) != len(tc.want) {
			t.Fatalf("splitPath(%q) = %v, want %v", tc.input, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("splitPath(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}
