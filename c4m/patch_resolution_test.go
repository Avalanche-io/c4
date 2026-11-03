package c4m

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// Reproduction of the report: PatchDiff writes a null-ID directory's
// entry unchanged when only its children changed, and ApplyPatch reads
// it as removing the directory. Manifests are built from real trees
// through the library, with the directory aggregates nulled the way a
// stream's base section carries them.
func TestPatchDiffOfNullIDDirectoryWithChangedChild(t *testing.T) {
	scan := func(x string) *Manifest {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "d"), 0o755)
		os.WriteFile(filepath.Join(dir, "d", "x.txt"), []byte(x), 0o644)
		os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0o644)
		m := NewManifest()
		for _, p := range []string{"a.txt", "d/", "d/x.txt"} {
			full := filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(p, "/")))
			info, err := os.Lstat(full)
			if err != nil {
				t.Fatal(err)
			}
			e := &Entry{Mode: KnownMode(info.Mode()), Timestamp: info.ModTime().UTC(), Size: info.Size(), Name: filepath.Base(p), Depth: strings.Count(p, "/")}
			if strings.HasSuffix(p, "/") {
				e.Name += "/"
				e.Size = -1
				e.Timestamp = NullTimestamp() // a base section: aggregates null
				e.Depth = 0
			} else {
				f, _ := os.Open(full)
				e.C4ID = c4.Identify(f)
				f.Close()
			}
			m.AddEntry(e)
		}
		return m
	}
	old := scan("one")
	new := scan("two!") // different bytes and size

	pr := PatchDiff(old, new)
	text, err := Marshal(pr.Patch)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("patch:\n%s", text)
	// a.txt is the same bytes in both trees; the two scans differ only in
	// sub-second timestamps, which a c4m line cannot carry, so it is not
	// a change
	if strings.Contains(string(text), "a.txt") {
		t.Fatalf("an unchanged file was written to the patch (sub-second timestamps compared):\n%s", text)
	}
	decoded, err := Unmarshal(text)
	if err != nil {
		t.Fatal(err)
	}
	// the base as a reader would hold it: decoded from text, at second
	// resolution
	oldText, _ := Marshal(old)
	oldDecoded, err := Unmarshal(oldText)
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []*Manifest{old, oldDecoded} {
		for _, patch := range []*Manifest{pr.Patch, decoded} {
			got := ApplyPatch(base, patch)
			paths := EntryPaths(got.Entries)
			if paths["d/"] == nil {
				t.Fatalf("the directory was removed by the patch:\n%s", text)
			}
			if paths["a.txt"] == nil {
				t.Fatalf("an unchanged file was removed by the patch:\n%s", text)
			}
			x := paths["d/x.txt"]
			if x == nil || x.Size != 4 {
				t.Fatalf("the child was not updated: %+v\n%s", x, text)
			}
		}
	}
}
