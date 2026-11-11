package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// A scan's C4 ID is the C4 ID of its own written canonical text: for a flat
// tree, the whole text; for a nested one, the root's one-level listing (each
// subdirectory one line carrying its own ID). An ID that hashes values the
// text does not state (a size filled in only while hashing, say) fails here.
// Checked in every scan mode, on a tree holding an empty directory.
func TestScanIDIsTheIDOfItsText(t *testing.T) {
	encode := func(t *testing.T, m *c4m.Manifest) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := c4m.NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	rootListing := func(m *c4m.Manifest) *c4m.Manifest {
		top := c4m.NewManifest()
		for _, e := range m.Entries {
			if e.Depth == 0 {
				cp := *e
				top.AddEntry(&cp)
			}
		}
		return top
	}

	flat := t.TempDir()
	must(t, os.WriteFile(filepath.Join(flat, "a.txt"), []byte("a"), 0o644))
	must(t, os.MkdirAll(filepath.Join(flat, "empty"), 0o755))

	nested := t.TempDir()
	must(t, os.WriteFile(filepath.Join(nested, "z"), []byte("z"), 0o644))
	must(t, os.MkdirAll(filepath.Join(nested, "a", "b", "empty"), 0o755))
	must(t, os.WriteFile(filepath.Join(nested, "a", "f"), []byte("f"), 0o644))

	for _, mode := range []ScanMode{ModeStructure, ModeMetadata, ModeFull} {
		m, err := Dir(flat, WithMode(mode))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := c4.Identify(bytes.NewReader(encode(t, m))), m.ComputeC4ID(); got != want {
			t.Errorf("mode %d, flat: the text's C4 ID is %s, the scan's %s\n%s", mode, got, want, encode(t, m))
		}

		m, err = Dir(nested, WithMode(mode))
		if err != nil {
			t.Fatal(err)
		}
		top := rootListing(m)
		if got, want := c4.Identify(bytes.NewReader(encode(t, top))), m.ComputeC4ID(); got != want {
			t.Errorf("mode %d, nested: the root listing's C4 ID is %s, the scan's %s\n%s", mode, got, want, encode(t, m))
		}
	}
}
