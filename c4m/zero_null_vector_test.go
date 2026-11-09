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

// zeroNullVectorManifest builds the shared conformance fixture for known
// zero against null, through the library only: a ten-dash file (known
// zero mode), a directory at 0000 with a child (d---------, known), a
// null-mode file (-), a size-0 empty file with the empty-content ID, an
// empty directory (size 0, empty-content ID) and an unreadable directory
// (d---------, null size, null ID, no children). Set C4M_UPDATE_VECTORS=1
// to rewrite testdata/zero-null-vector from it; the files are never
// written by hand.
func zeroNullVectorManifest() *Manifest {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	id := func(s string) c4.ID { return c4.Identify(strings.NewReader(s)) }
	empty := c4.Identify(bytes.NewReader(nil))
	m := NewManifest()
	m.AddEntry(&Entry{Mode: ZeroMode, Timestamp: ts, Size: 4, Name: "zero.txt", C4ID: id("zero")})
	m.AddEntry(&Entry{Mode: 0, Timestamp: ts, Size: 4, Name: "nullmode.txt", C4ID: id("null")})
	m.AddEntry(&Entry{Mode: 0644, Timestamp: ts, Size: 0, Name: "empty.txt", C4ID: empty})
	m.AddEntry(&Entry{Mode: 0755 | os.ModeDir, Timestamp: ts, Size: 0, Name: "emptydir/", C4ID: empty})
	m.AddEntry(&Entry{Mode: os.ModeDir, Timestamp: ts, Size: -1, Name: "locked/"})
	m.AddEntry(&Entry{Mode: 0644, Timestamp: ts, Size: 6, Name: "inside.txt", C4ID: id("inside"), Depth: 1})
	m.AddEntry(&Entry{Mode: os.ModeDir, Timestamp: ts, Size: -1, Name: "unread/"})
	// locked/ has a listed child: the library derives its size and ID.
	// unread/ has none and no ID: it stays null, nobody read it.
	Normalize(m)
	return m
}

// TestZeroNullVector pins testdata/zero-null-vector: the bytes are what
// the library writes, both forms decode to one manifest with the expected
// knowledge per entry, and the root ID matches root-id.txt.
func TestZeroNullVector(t *testing.T) {
	dir := filepath.Join("testdata", "zero-null-vector")
	m := zeroNullVectorManifest()

	// The pretty form writes local time; the fixture is pinned in UTC so
	// its bytes are the same on every machine.
	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = time.UTC

	var canon, pretty bytes.Buffer
	if err := NewEncoder(&canon).Encode(m); err != nil {
		t.Fatal(err)
	}
	pe := NewEncoder(&pretty)
	pe.SetPretty(true)
	if err := pe.Encode(m); err != nil {
		t.Fatal(err)
	}
	rootID := m.ComputeC4ID().String() + "\n"

	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		for name, data := range map[string][]byte{
			"canonical.c4m": canon.Bytes(), "pretty.c4m": pretty.Bytes(), "root-id.txt": []byte(rootID),
		} {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, want := range map[string][]byte{
		"canonical.c4m": canon.Bytes(), "pretty.c4m": pretty.Bytes(), "root-id.txt": []byte(rootID),
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from what the library writes:\n--- file\n%s--- library\n%s", name, got, want)
		}
	}

	// What a conforming decoder must see, in either form.
	type want struct {
		modeNull, modeZero, sizeNull, idNil bool
		size                                int64
	}
	expect := map[string]want{
		"zero.txt":     {modeZero: true, size: 4},
		"nullmode.txt": {modeNull: true, size: 4},
		"empty.txt":    {size: 0},
		"emptydir/":    {size: 0},
		"locked/":      {}, // mode d---------: known; size and ID derived from inside.txt
		"inside.txt":   {size: 6},
		"unread/":      {sizeNull: true, idNil: true}, // mode d---------: known
	}
	for _, form := range []string{"canonical.c4m", "pretty.c4m"} {
		data, err := os.ReadFile(filepath.Join(dir, form))
		if err != nil {
			t.Fatal(err)
		}
		dec, err := Unmarshal(data)
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got := dec.ComputeC4ID().String() + "\n"; got != rootID {
			t.Errorf("%s resolves to %s, want %s", form, got, rootID)
		}
		seen := 0
		for _, e := range dec.Entries {
			w, ok := expect[e.Name]
			if !ok {
				t.Errorf("%s: unexpected entry %s", form, e.Name)
				continue
			}
			seen++
			if (e.Mode == 0) != w.modeNull {
				t.Errorf("%s: %s mode null = %v, want %v", form, e.Name, e.Mode == 0, w.modeNull)
			}
			if (e.Mode == ZeroMode) != w.modeZero {
				t.Errorf("%s: %s mode zero = %v, want %v", form, e.Name, e.Mode == ZeroMode, w.modeZero)
			}
			if e.IsDir() && e.Mode.Perm() != 0 && (e.Name == "locked/" || e.Name == "unread/") {
				t.Errorf("%s: %s must be d--------- (known, zero permissions)", form, e.Name)
			}
			if (e.Size < 0) != w.sizeNull {
				t.Errorf("%s: %s size null = %v, want %v (size %d)", form, e.Name, e.Size < 0, w.sizeNull, e.Size)
			}
			if !w.sizeNull && e.Name != "locked/" && e.Size != w.size {
				t.Errorf("%s: %s size %d, want %d", form, e.Name, e.Size, w.size)
			}
			if e.C4ID.IsNil() != w.idNil {
				t.Errorf("%s: %s ID nil = %v, want %v", form, e.Name, e.C4ID.IsNil(), w.idNil)
			}
		}
		if seen != len(expect) {
			t.Errorf("%s: %d of %d entries seen", form, seen, len(expect))
		}
	}
}
