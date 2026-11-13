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
// description it returns, and a content projection keeps it: the range's
// C4 ID does not change, so neither does its list. Through 2026-11-13 the
// fold dropped the detector's lists, so a scan without a store left the
// frames' C4 IDs nowhere (design/range-data-through-operations.md).
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
	check("ProjectContent", ProjectContent(m))
}

// A -S scan sizes a directory holding a folded range from its listing as
// written, the folded one, as its ID already was, and the range's size
// counts its ID list (ruled 2026-11-13 by the maintainer: "The size is the
// size of every frame and the frame id list."). So the normaliser changes
// nothing in a -S scan. Through 2026-11-13 the scanner sized the directory
// from its unfolded children and the normaliser disagreed.
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
	for _, e := range m.Entries {
		if e.IsSequence && e.Size != 33+3*90 {
			t.Errorf("%s: size %d, want %d", e.Name, e.Size, 33+3*90)
		}
	}
	if ch := c4m.Normalize(m.Copy()); len(ch) != 0 {
		t.Errorf("the normaliser changes a -S scan: %v", ch)
	}
}
