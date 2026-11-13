package c4m

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

var deepenTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func deepenFile(name, content string) *Entry {
	return &Entry{
		Mode: 0o644, Timestamp: deepenTime, Size: int64(len(content)),
		Name: name, C4ID: c4.Identify(strings.NewReader(content)), Depth: 1,
	}
}

// deepenTree is the tree "d/ holding files", built through the API with its
// directory aggregates derived by the library.
func deepenTree(t *testing.T, files ...*Entry) *Manifest {
	t.Helper()
	m := NewManifest()
	m.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: deepenTime, Size: -1, Name: "d/"})
	for _, f := range files {
		cp := *f
		m.AddEntry(&cp)
	}
	Normalize(m)
	return m
}

// storeOneLevel stores the tree the way a store does: the root's one-level
// listing (d/ as one line carrying its ID) and d/'s own listing, separately.
func storeOneLevel(t *testing.T, src *blockSource, m *Manifest, withDir bool) c4.ID {
	t.Helper()
	dir := *m.Entries[0]
	dir.Depth = 0
	rootText, rootID := ListingRecord([]*Entry{&dir})
	src.put(rootText)
	if withDir {
		var kids []*Entry
		for _, e := range m.Entries[1:] {
			k := *e
			k.Depth = 0
			kids = append(kids, &k)
		}
		text, id := ListingRecord(kids)
		if id != dir.C4ID {
			t.Fatalf("d/'s listing ID %s, its line says %s", id, dir.C4ID)
		}
		src.put(text)
	}
	if rootID != m.ComputeC4ID() {
		t.Fatalf("root listing %s, tree %s", rootID, m.ComputeC4ID())
	}
	return rootID
}

func encodeBlock(t *testing.T, old, new *Manifest, base c4.ID) string {
	t.Helper()
	pr := PatchDiff(old, new)
	pr.OldID = base
	var buf bytes.Buffer
	if err := NewEncoder(&buf).EncodePatch(pr); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// A shape-B block chain over a one-level base: block 1 adds d/z.txt, block
// 2 (naming block 1's byte ID) removes d/x.txt. The base's d/ is one
// collapsed line; its children live in d/'s own listing. Before the fix the
// fold treated the collapsed d/ as empty, so block 2's removal of x.txt
// read as an addition: the result held x and z under a d/ line whose ID
// names y and z (found 2026-11-12 by an independent review).
func TestResolveBaseDeepensCollapsedDirectory(t *testing.T) {
	x, y, z := deepenFile("x.txt", "x"), deepenFile("y.txt", "y"), deepenFile("z.txt", "z")
	v0 := deepenTree(t, x, y)
	v1 := deepenTree(t, x, y, z)
	v2 := deepenTree(t, y, z)

	for _, withDir := range []bool{true, false} {
		src := newBlockSource()
		base := storeOneLevel(t, src, v0, withDir)
		b1 := src.put(encodeBlock(t, v0, v1, base))
		b2 := src.put(encodeBlock(t, v1, v2, b1))

		got, err := ResolveBaseID(b2, src)
		if !withDir {
			if !errors.Is(err, ErrUnresolvedBase) || !strings.Contains(err.Error(), v0.Entries[0].C4ID.String()) {
				t.Fatalf("without d/'s listing the fold must refuse, naming it: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.ComputeC4ID() != v2.ComputeC4ID() {
			t.Errorf("resolved ID %s, want %s", got.ComputeC4ID(), v2.ComputeC4ID())
		}
		names := map[string]bool{}
		for _, e := range got.Entries {
			names[e.Name] = true
		}
		if names["x.txt"] || !names["y.txt"] || !names["z.txt"] {
			t.Errorf("d/ must hold y.txt and z.txt only, got %v", names)
		}
		// The description agrees with its own IDs: re-deriving changes nothing.
		if ch := Normalize(got); len(ch) != 0 {
			t.Errorf("resolved description disagrees with its own directory IDs: %v", ch)
		}
	}
}

// A self-contained chain whose first section is a one-level listing (d/ as
// one collapsed line) and whose next section deepens d/. The decoder used
// to apply the section without d/'s listing, so the normaliser re-derived
// d/ from the partial listing and named a tree that was never there.
// Decided 2026-11-13 by the release lead, under the maintainer's standing instruction to
// decide; reversible: with a source the listing is read first; without
// one the chain is refused, naming the listing.
func TestDecodeDeepensCollapsedDirectoryInAChain(t *testing.T) {
	x, y, z := deepenFile("x.txt", "x"), deepenFile("y.txt", "y"), deepenFile("z.txt", "z")
	v0 := deepenTree(t, x, y)
	v1 := deepenTree(t, x, y, z)

	src := newBlockSource()
	base := storeOneLevel(t, src, v0, true)
	dir := *v0.Entries[0]
	dir.Depth = 0
	rootText, _ := ListingRecord([]*Entry{&dir})
	chain := rootText + encodeBlock(t, v0, v1, base)

	got, err := DecodeResolved(strings.NewReader(chain), src)
	if err != nil {
		t.Fatalf("with a source: %v", err)
	}
	if got.ComputeC4ID() != v1.ComputeC4ID() {
		t.Errorf("with a source: %s, want %s", got.ComputeC4ID(), v1.ComputeC4ID())
	}
	// The root ID alone can come out right from the restated d/ line while
	// the entries under it are wrong: check the entries, and that the
	// description agrees with its own directory IDs.
	names := map[string]bool{}
	for _, e := range got.Entries {
		names[e.Name] = true
	}
	if !names["x.txt"] || !names["y.txt"] || !names["z.txt"] {
		t.Errorf("with a source, d/ must hold x, y and z, got %v", names)
	}
	if ch := Normalize(got); len(ch) != 0 {
		t.Errorf("with a source, the description disagrees with its own directory IDs: %v", ch)
	}

	_, err = Unmarshal([]byte(chain))
	if !errors.Is(err, ErrUnresolvedBase) || !strings.Contains(err.Error(), v0.Entries[0].C4ID.String()) {
		t.Errorf("without a source the chain must be refused, naming d/'s listing: %v", err)
	}
}
