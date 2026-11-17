package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

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
// C4 ID of its own one-level listing, in every mode that records IDs, with
// sequence detection and without, when the tree holds empty directories,
// and the normaliser finds nothing to correct. Through 2026-11-16, under
// -S, a directory holding an empty directory was given an ID that was not
// its listing's: the empty directory's size was still null at its own
// close, and the parent, which propagates copies of its children to fold
// them, hashed the original with that null size, while the line printed
// after the whole-manifest pass said 0. `c4 id -S -s` then stored the
// listing under one ID and printed another.
func TestEmptyDirectoryParentIDIsItsListing(t *testing.T) {
	dir := writeEmptyDirTree(t)
	for _, mode := range []ScanMode{ModeFull, ModeContent, ModeMetadata} {
		for _, seq := range []bool{false, true} {
			m, err := NewGeneratorWithOptions(WithMode(mode), WithSequenceDetection(seq)).GenerateFromPath(dir)
			if err != nil {
				t.Fatal(err)
			}
			hashes := mode == ModeFull || mode == ModeContent
			if seq && hashes && !hasRange(m) {
				t.Fatalf("fixture: mode %v -S folds no range", mode)
			}
			dirs, empties := 0, 0
			for path, e := range c4m.EntryPaths(m.Entries) {
				if !e.IsDir() {
					continue
				}
				dirs++
				children := m.Children(e)
				if len(children) == 0 {
					empties++
					if e.Size != 0 || e.C4ID != emptyListingID {
						t.Errorf("mode %v, sequences=%v: the empty directory %s is size %d, ID %s; want 0 and the empty listing's", mode, seq, path, e.Size, e.C4ID)
					}
					continue
				}
				if e.C4ID.IsNil() {
					continue // a metadata scan hashes nothing
				}
				if _, listing := c4m.ListingRecord(children); e.C4ID != listing {
					t.Errorf("mode %v, sequences=%v: %s is %s, its listing is %s", mode, seq, path, e.C4ID, listing)
				}
			}
			if dirs != 5 || empties != 2 {
				t.Fatalf("fixture: %d directories, %d empty; want 5 and 2", dirs, empties)
			}
			// The normaliser re-derives every aggregate from the listings:
			// a scan it would correct was wrong. (A metadata scan hashes
			// nothing, and the normaliser derives directory IDs from its
			// null-ID listings; that is a separate question, not checked
			// here.)
			if !hashes {
				continue
			}
			for _, c := range c4m.Normalize(m.Copy()) {
				t.Errorf("mode %v, sequences=%v: the normaliser corrects the scan's %s of %s: %s to %s", mode, seq, c.Field, c.Path, c.Old, c.New)
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
