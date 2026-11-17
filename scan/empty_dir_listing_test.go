package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// listingID is the C4 ID of a directory's one-level canonical listing,
// derived from its children as this line's store writer derives it
// (cmd/c4 storeDirectoryC4m): the children in a manifest of their own,
// canonicalised by the library.
func listingID(children []*c4m.Entry) c4.ID {
	if len(children) == 0 {
		return c4.Identify(strings.NewReader(""))
	}
	sub := c4m.NewManifest()
	for _, c := range children {
		cp := *c
		cp.Depth = 0
		sub.AddEntry(&cp)
	}
	sub.Canonicalize()
	sub.SortEntries()
	return c4.Identify(strings.NewReader(sub.Canonical()))
}

// writeEmptyDirTree makes a tree with an empty directory at two depths
// (top-empty/ at the root, a/b/empty/ two levels down), a folded range
// beside the deeper one (a/b/f.[0001-0003].exr) and plain files.
func writeEmptyDirTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, d := range []string{"a/b/empty", "a/c", "top-empty"} {
		must(t, os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o755))
	}
	files := map[string]string{
		"a/b/two.txt":    "two",
		"a/b/f.0001.exr": "frame 1",
		"a/b/f.0002.exr": "frame 2",
		"a/b/f.0003.exr": "frame 3",
		"a/c/x.txt":      "x",
	}
	for p, body := range files {
		must(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(body), 0o644))
	}
	return dir
}

// TestEmptyDirectoryParentIDIsItsListing: a scan gives every directory the
// C4 ID of its own one-level listing, with sequence detection and without,
// when the tree holds empty directories; a full scan gives an empty
// directory size 0. Through 2026-11-16, under -S, a directory holding an
// empty directory was given an ID that was not its listing's: the empty
// directory's size was still null at its own close, and the parent, which
// propagates copies of its children to fold them, hashed the original with
// that null size, while the line printed after the whole-manifest pass said
// 0. `c4 id -S -s` then stored the listing under one ID and printed
// another. This line has no streamed refinement layer and no normaliser
// that re-derives aggregates (master's), so the scanner is the one place.
func TestEmptyDirectoryParentIDIsItsListing(t *testing.T) {
	dir := writeEmptyDirTree(t)
	for _, mode := range []ScanMode{ModeFull, ModeMetadata} {
		for _, seq := range []bool{false, true} {
			m, err := NewGeneratorWithOptions(WithMode(mode), WithSequenceDetection(seq)).GenerateFromPath(dir)
			if err != nil {
				t.Fatal(err)
			}
			if seq && mode == ModeFull && !hasRange(m) {
				t.Fatalf("fixture: -S folds no range")
			}
			dirs := 0
			for path, e := range c4m.EntryPaths(m.Entries) {
				if !e.IsDir() {
					continue
				}
				dirs++
				children := m.Children(e)
				if mode == ModeFull && len(children) == 0 && e.Size != 0 {
					t.Errorf("sequences=%v: the empty directory %s has size %d, want 0", seq, path, e.Size)
				}
				if e.C4ID.IsNil() {
					continue // a metadata scan hashes nothing
				}
				if listing := listingID(children); e.C4ID != listing {
					t.Errorf("mode %v, sequences=%v: %s is %s, its listing is %s", mode, seq, path, e.C4ID, listing)
				}
			}
			if dirs != 5 {
				t.Fatalf("fixture: %d directories, want 5", dirs)
			}
		}
	}
}

func hasRange(m *c4m.Manifest) bool {
	for _, e := range m.Entries {
		if e.IsSequence {
			return true
		}
	}
	return false
}
