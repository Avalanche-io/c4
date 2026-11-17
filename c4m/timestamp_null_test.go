package c4m

import (
	"strings"
	"testing"
	"time"
)

// A time is null at the format's resolution: the second it falls in is the
// epoch. An entry at 0.5 s is written "-", and a manifest holding it has
// the description ID of its own text.
func TestTimestampIsNullAtTheSecond(t *testing.T) {
	cases := []struct {
		ns   int64
		null bool
	}{
		{0, true},
		{1, true},
		{500_000_000, true},
		{999_999_999, true},
		{1_000_000_000, false},
		{-1, false},
		{-500_000_000, false},
		{1_750_000_000_000_000_000, false},
	}
	for _, c := range cases {
		e := &Entry{Mode: 0o644, Timestamp: time.Unix(0, c.ns).UTC(), Size: 1, Name: "f.txt"}
		if got := e.TimestampIsNull(); got != c.null {
			t.Errorf("%d ns: TimestampIsNull %v, want %v", c.ns, got, c.null)
		}
		m := NewManifest()
		m.AddEntry(e)
		text, err := Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		field := strings.Fields(string(text))[1]
		if (field == "-") != c.null {
			t.Errorf("%d ns: written %q, null %v", c.ns, field, c.null)
		}
		back, err := Unmarshal(text)
		if err != nil {
			t.Fatal(err)
		}
		if a, b := m.ComputeC4ID(), back.ComputeC4ID(); a != b {
			t.Errorf("%d ns: description ID %s, its own c4m's %s", c.ns, a, b)
		}
	}
}
