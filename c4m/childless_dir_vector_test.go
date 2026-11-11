package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// The shared conformance fixture for a directory with no listed children
// and a null size (testdata/childless-dir-vector; README there). What the
// size derives to depends on the directory's C4 ID alone:
//
//	empty/      the empty listing's ID  -> 0: known to hold nothing
//	collapsed/  any other ID           -> null: its listing lives elsewhere
//	unread/     no ID                  -> null: nobody listed it
//
// Every entry is built through the package's API; C4M_UPDATE_VECTORS=1
// rewrites the files.

func childlessDirVectorManifest() *Manifest {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewManifest()
	for _, d := range []struct {
		name string
		id   c4.ID
	}{
		{"collapsed/", c4.Identify(strings.NewReader("a listing stored elsewhere"))},
		{"empty/", NewManifest().ComputeC4ID()},
		{"unread/", c4.ID{}},
	} {
		m.AddEntry(&Entry{Name: d.name, Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, C4ID: d.id})
	}
	m.SortEntries()
	return m
}

func TestChildlessDirVector(t *testing.T) {
	dir := filepath.Join("testdata", "childless-dir-vector")

	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = time.UTC

	stated := childlessDirVectorManifest()
	normalized := childlessDirVectorManifest()
	Normalize(normalized)

	encode := func(m *Manifest) []byte {
		var b bytes.Buffer
		if err := NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	files := map[string][]byte{
		"stated.c4m":     encode(stated),
		"normalized.c4m": encode(normalized),
		"root-id.txt":    []byte(stated.ComputeC4ID().String() + "\n"),
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

	want := map[string]int64{"empty/": 0, "collapsed/": -1, "unread/": -1}
	for _, form := range []string{"stated.c4m", "normalized.c4m"} {
		m, err := Unmarshal(files[form])
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		// The ID hashes the derived sizes: the stated file and its
		// normalised form are the same description.
		if got := m.ComputeC4ID().String() + "\n"; got != string(files["root-id.txt"]) {
			t.Errorf("%s resolves to %s, want %s", form, got, files["root-id.txt"])
		}
		Normalize(m)
		for _, e := range m.Entries {
			if e.Size != want[e.Name] {
				t.Errorf("%s: %s normalises to size %d, want %d", form, e.Name, e.Size, want[e.Name])
			}
		}
	}
	// The normalised file is canonical: its bytes are the description, so
	// its own C4 ID is the root ID.
	if got := c4.Identify(bytes.NewReader(files["normalized.c4m"])).String() + "\n"; got != string(files["root-id.txt"]) {
		t.Errorf("normalized.c4m's own C4 ID is %s, want the root ID %s", got, files["root-id.txt"])
	}
}
