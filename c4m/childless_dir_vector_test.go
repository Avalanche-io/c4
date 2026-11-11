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
	PropagateMetadata(normalized.Entries)

	encode := func(m *Manifest) []byte {
		var b bytes.Buffer
		if err := NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	// The stated form is the same on every line.
	if got, want := read("stated.c4m"), encode(stated); !bytes.Equal(got, want) {
		t.Errorf("stated.c4m differs from what the library writes:\n--- file\n%s--- library\n%s", got, want)
	}

	// Sizes derive from the ID alone. collapsed/ (any ID but the empty
	// listing's) stays null, as on master. unread/ (no ID) is the known
	// 1.0.x difference: this line still gives it 0, where master keeps it
	// null, because a metadata-mode scan sizes an observed empty directory
	// before it has an ID.
	want := map[string]int64{"empty/": 0, "collapsed/": -1, "unread/": 0}
	for _, form := range []string{"stated.c4m", "normalized.c4m"} {
		m, err := Unmarshal(read(form))
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		PropagateMetadata(m.Entries)
		for _, e := range m.Entries {
			if e.Size != want[e.Name] {
				t.Errorf("%s: %s derives to size %d, want %d", form, e.Name, e.Size, want[e.Name])
			}
		}
	}

	// The committed normalised file is master's. It differs from this
	// line's only in unread/'s size: set that one value through the API and
	// the bytes must match what this line writes.
	master, err := Unmarshal(read("normalized.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range master.Entries {
		if e.Name == "unread/" {
			e.Size = 0
		}
	}
	lineNormalized := encode(normalized)
	if got := encode(master); !bytes.Equal(got, lineNormalized) {
		t.Errorf("master's normalized.c4m differs from this line's beyond unread/'s size:\n--- master, unread/ at 0\n%s--- this line\n%s", got, lineNormalized)
	}

	// The committed root ID is master's: the C4 ID of master's normalised
	// file. This line's ID differs by unread/'s size and agrees with itself:
	// the stated form, its normalised form and that form's own bytes.
	if got := c4.Identify(bytes.NewReader(read("normalized.c4m"))).String() + "\n"; got != string(read("root-id.txt")) {
		t.Errorf("normalized.c4m's own C4 ID is %s, want root-id.txt's %s", got, read("root-id.txt"))
	}
	lineID := stated.ComputeC4ID()
	if got := normalized.ComputeC4ID(); got != lineID {
		t.Errorf("the normalised form resolves to %s, the stated form to %s", got, lineID)
	}
	if got := c4.Identify(bytes.NewReader(lineNormalized)); got != lineID {
		t.Errorf("this line's normalised bytes hash to %s, want %s", got, lineID)
	}
	if lineID.String()+"\n" == string(read("root-id.txt")) {
		t.Errorf("this line's root ID equals master's; the unread/ difference is gone, so update this test")
	}
}
