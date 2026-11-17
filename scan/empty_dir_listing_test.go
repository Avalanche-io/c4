package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// TestEmptyDirectoryParentIDIsItsListing: a scan gives every directory the
// C4 ID of its own one-level listing, with sequence detection and without,
// when the tree holds empty directories. Through 2026-11-16, under -S, a
// directory holding an empty directory was given an ID that was not its
// listing's: the empty directory's size was still null at its own close,
// and the parent, which propagates copies of its children to fold them,
// hashed the original with that null size, while the line printed after the
// whole-manifest pass said 0. `c4 id -S -s` then stored the listing under
// one ID and printed another.
func TestEmptyDirectoryParentIDIsItsListing(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"a/b/empty", "a/c", "top-empty", "seq"} {
		must(t, os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o755))
	}
	files := map[string]string{
		"a/b/two.txt":    "two",
		"a/c/x.txt":      "x",
		"seq/f.0001.exr": "frame 1",
		"seq/f.0002.exr": "frame 2",
		"seq/f.0003.exr": "frame 3",
	}
	for p, body := range files {
		must(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(body), 0o644))
	}
	for _, seq := range []bool{false, true} {
		m, err := NewGeneratorWithOptions(WithMode(ModeFull), WithSequenceDetection(seq)).GenerateFromPath(dir)
		if err != nil {
			t.Fatal(err)
		}
		dirs := 0
		for path, e := range c4m.EntryPaths(m.Entries) {
			if !e.IsDir() {
				continue
			}
			dirs++
			_, listing := c4m.ListingRecord(m.Children(e))
			if e.C4ID != listing {
				t.Errorf("sequences=%v: %s is %s, its listing is %s", seq, path, e.C4ID, listing)
			}
			if len(m.Children(e)) == 0 && e.Size != 0 {
				t.Errorf("sequences=%v: the empty directory %s has size %d, want 0", seq, path, e.Size)
			}
		}
		if dirs != 6 {
			t.Fatalf("fixture: %d directories, want 6", dirs)
		}
	}
}
