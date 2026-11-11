package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// The shared conformance fixture for the empty tree (testdata/empty-tree-vector;
// README there). An empty listing is known to be empty, not unknown: its C4
// ID is the C4 ID of its canonical text, which is empty, and a directory
// whose listing is empty carries that ID. Every entry is built through the
// package's API; C4M_UPDATE_VECTORS=1 rewrites the files.

// emptyDirVectorManifest is a tree whose only child is an empty directory.
func emptyDirVectorManifest() *Manifest {
	m := NewManifest()
	m.AddEntry(&Entry{
		Name:      "empty/",
		Mode:      os.ModeDir | 0o755,
		Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Size:      0, // as the scanner writes an empty directory
		C4ID:      NewManifest().ComputeC4ID(),
	})
	Normalize(m)
	return m
}

func TestEmptyTreeVector(t *testing.T) {
	dir := filepath.Join("testdata", "empty-tree-vector")

	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = time.UTC

	empty := NewManifest()
	emptyID := empty.ComputeC4ID()
	if want := c4.Identify(bytes.NewReader(nil)); emptyID != want {
		t.Fatalf("the empty manifest's C4 ID is %s, want the C4 ID of the empty text %s", emptyID, want)
	}
	m := emptyDirVectorManifest()

	encode := func(m *Manifest, pretty bool) []byte {
		var b bytes.Buffer
		e := NewEncoder(&b)
		e.SetPretty(pretty)
		if err := e.Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	var patch bytes.Buffer
	if err := NewEncoder(&patch).EncodePatch(&PatchResult{OldID: emptyID, Patch: m}); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"empty.c4m":             encode(empty, false),
		"empty-id.txt":          []byte(emptyID.String() + "\n"),
		"empty-dir.c4m":         encode(m, false),
		"empty-dir-pretty.c4m":  encode(m, true),
		"empty-dir-patch.c4m":   patch.Bytes(),
		"empty-dir-root-id.txt": []byte(m.ComputeC4ID().String() + "\n"),
	}
	if len(files["empty.c4m"]) != 0 {
		t.Fatalf("the empty manifest encodes as %q, want no bytes", files["empty.c4m"])
	}
	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, data := range files {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from what the library writes:\n--- file\n%s--- library\n%s", name, got, want)
		}
	}

	// Reading back: the empty text is the empty manifest, and both forms of
	// the one-directory tree land on its root ID with the directory's
	// listing ID and size intact.
	dec, err := Unmarshal(files["empty.c4m"])
	if err != nil {
		t.Fatalf("empty.c4m: %v", err)
	}
	if got := dec.ComputeC4ID(); got != emptyID {
		t.Errorf("empty.c4m resolves to %s, want %s", got, emptyID)
	}
	for _, form := range []string{"empty-dir.c4m", "empty-dir-pretty.c4m"} {
		dec, err := Unmarshal(files[form])
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got := dec.ComputeC4ID(); got != m.ComputeC4ID() {
			t.Errorf("%s resolves to %s, want %s", form, got, m.ComputeC4ID())
		}
		if len(dec.Entries) != 1 || dec.Entries[0].C4ID != emptyID || dec.Entries[0].Size != 0 {
			t.Errorf("%s: want one directory with the empty listing's ID and size 0, got %+v", form, dec.Entries)
		}
	}
	sections, err := DecodePatchChain(bytes.NewReader(files["empty-dir-patch.c4m"]))
	if err != nil {
		t.Fatalf("empty-dir-patch.c4m: %v", err)
	}
	if len(sections) != 1 || sections[0].BaseID != emptyID || len(sections[0].Entries) != 1 {
		t.Errorf("empty-dir-patch.c4m: want one section of one entry over %s, got %+v", emptyID, sections)
	}
}
