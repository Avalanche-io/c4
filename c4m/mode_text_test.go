package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// A known-zero mode on a directory name is written d---------, in every
// form, and decodes back as a known directory mode with zero permissions;
// it must never round-trip to null through the legacy ten-dash rule.
func TestZeroModeOnDirectoryNameEncodesAsDirectory(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewManifest()
	m.AddEntry(&Entry{Mode: ZeroMode, Timestamp: ts, Size: -1, Name: "locked/"})
	m.AddEntry(&Entry{Mode: os.ModeDir, Timestamp: ts, Size: -1, Name: "alsolocked/"})
	m.AddEntry(&Entry{Mode: ZeroMode, Timestamp: ts, Size: 1, Name: "zero.txt"})
	m.AddEntry(&Entry{Mode: 0, Timestamp: ts, Size: -1, Name: "nulldir/"})

	for _, pretty := range []bool{false, true} {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		enc.SetPretty(pretty)
		if err := enc.Encode(m); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(buf.String(), "\n") {
			f := strings.Fields(line)
			if len(f) < 4 {
				continue
			}
			name := f[len(f)-2]
			switch name {
			case "locked/", "alsolocked/":
				if f[0] != "d---------" {
					t.Errorf("pretty=%v: %s mode %q, want d---------", pretty, name, f[0])
				}
			case "zero.txt":
				if f[0] != "----------" {
					t.Errorf("pretty=%v: %s mode %q, want ----------", pretty, name, f[0])
				}
			case "nulldir/":
				if f[0] != "-" {
					t.Errorf("pretty=%v: %s mode %q, want -", pretty, name, f[0])
				}
			}
		}
		dec, err := Unmarshal(buf.Bytes())
		if err != nil {
			t.Fatalf("pretty=%v: %v", pretty, err)
		}
		for _, e := range dec.Entries {
			switch e.Name {
			case "locked/", "alsolocked/":
				if e.Mode == 0 || !e.Mode.IsDir() || e.Mode.Perm() != 0 {
					t.Errorf("pretty=%v: %s decoded %v, want a known directory at 0000", pretty, e.Name, e.Mode)
				}
			case "nulldir/":
				if e.Mode != 0 {
					t.Errorf("pretty=%v: nulldir/ decoded %v, want null", pretty, e.Mode)
				}
			}
		}
	}
	if m.Entries[0].Canonical()[:10] != "d---------" && !strings.HasPrefix(m.Entries[0].Canonical(), "d---------") {
		t.Errorf("Canonical must write d--------- for a known-zero directory, got %q", m.Entries[0].Canonical())
	}
}
