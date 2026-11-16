package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// A scan that folds sequences keeps each range's ID list on the
// description it returns. Through 2026-11-13 the fold dropped the
// detector's lists, so a scan without a store left the frames' C4 IDs
// nowhere (design/range-data-through-operations.md). 1.0.x has no
// ProjectContent, so master's projection check is not here.
func TestSequenceScanKeepsIDLists(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"1", "2", "3"} {
		if err := os.WriteFile(filepath.Join(dir, "sub", "f.00"+f+".exr"), []byte("frame "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewGeneratorWithOptions(WithSequenceDetection(true)).GenerateFromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(what string, m *c4m.Manifest) {
		t.Helper()
		var rng *c4m.Entry
		for _, e := range m.Entries {
			if e.IsSequence {
				rng = e
			}
		}
		if rng == nil {
			t.Fatalf("%s: no folded range", what)
		}
		list, ok := m.RangeData[rng.C4ID]
		if !ok {
			t.Fatalf("%s: %s has no ID list", what, rng.Name)
		}
		if c4.Identify(strings.NewReader(list)) != rng.C4ID || len(list) != 3*90 {
			t.Errorf("%s: the list does not name %s's three frames", what, rng.Name)
		}
	}
	check("scan -S", m)
}

// A -S scan sizes a directory holding a folded range from its listing as
// written, the folded one, as its ID already was, and the range's size
// counts its ID list (ruled 2026-11-13 by the maintainer: "The size is the
// size of every frame and the frame id list."). Through 2026-11-13 the
// scanner sized the directory from its unfolded children. 1.0.x has no
// Normalize, so the directory's size is checked against its listing
// directly: the range's size plus the listing's bytes.
func TestSequenceScanIsNormal(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"1", "2", "3"} {
		if err := os.WriteFile(filepath.Join(dir, "sub", "f.00"+f+".exr"), []byte("frame "+f+" abc"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, err := NewGeneratorWithOptions(WithSequenceDetection(true)).GenerateFromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	var rng, sub *c4m.Entry
	for _, e := range m.Entries {
		switch {
		case e.IsSequence:
			rng = e
		case e.Name == "sub/":
			sub = e
		}
	}
	if rng == nil || sub == nil {
		t.Fatal("the scan lacks the range or sub/")
	}
	if rng.Size != 33+3*90 {
		t.Errorf("%s: size %d, want %d", rng.Name, rng.Size, 33+3*90)
	}
	listing, err := m.ExtractSubtree("sub/")
	if err != nil {
		t.Fatal(err)
	}
	if want := rng.Size + int64(len(listing.Canonical())); sub.Size != want {
		t.Errorf("sub/: size %d, want %d (the range plus its folded listing)", sub.Size, want)
	}
	if id := listing.ComputeC4ID(); sub.C4ID != id {
		t.Errorf("sub/: C4 ID %s, want its folded listing's %s", sub.C4ID, id)
	}
}
