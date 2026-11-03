package c4m

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// A known zero mode ("----------") and a null mode ("-") are different
// values and each survives an encode/decode round trip, in canonical and
// pretty form. The text is made by the encoder, never written by hand.
func TestZeroModeAndNullModeRoundTrip(t *testing.T) {
	when := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	m.Entries = append(m.Entries,
		&Entry{Mode: ZeroMode, Timestamp: when, Size: 1, Name: "zero.txt"},
		&Entry{Mode: 0, Timestamp: when, Size: 1, Name: "null.txt"},
		&Entry{Mode: 0644, Timestamp: when, Size: 1, Name: "plain.txt"},
	)

	for _, form := range []string{"canonical", "pretty"} {
		var buf bytes.Buffer
		enc := NewEncoder(&buf)
		if form == "pretty" {
			enc.SetPretty(true)
		}
		if err := enc.Encode(m); err != nil {
			t.Fatalf("%s encode: %v", form, err)
		}
		text := buf.String()
		// the encoder orders entries by name, so find lines by name
		line := func(name string) string {
			for _, l := range strings.Split(text, "\n") {
				if strings.Contains(l, " "+name+" ") {
					return strings.TrimSpace(l)
				}
			}
			t.Fatalf("%s: no line for %s in:\n%s", form, name, text)
			return ""
		}
		if !strings.HasPrefix(line("zero.txt"), "---------- ") {
			t.Errorf("%s: the zero mode must encode as ten dashes: %q", form, line("zero.txt"))
		}
		if !strings.HasPrefix(line("null.txt"), "- ") {
			t.Errorf("%s: the null mode must encode as one dash: %q", form, line("null.txt"))
		}

		got, err := NewDecoder(strings.NewReader(text)).Decode()
		if err != nil {
			t.Fatalf("%s decode: %v", form, err)
		}
		byName := map[string]*Entry{}
		for _, e := range got.Entries {
			byName[e.Name] = e
		}
		if byName["zero.txt"].Mode != ZeroMode {
			t.Errorf("%s: ten dashes decoded as %v, want ZeroMode", form, byName["zero.txt"].Mode)
		}
		if byName["null.txt"].Mode != 0 {
			t.Errorf("%s: one dash decoded as %v, want null (0)", form, byName["null.txt"].Mode)
		}
		if byName["plain.txt"].Mode != 0644 {
			t.Errorf("%s: -rw-r--r-- decoded as %v", form, byName["plain.txt"].Mode)
		}
		if byName["zero.txt"].HasNullValues() {
			t.Errorf("%s: the zero-mode entry reports null values: %v", form, byName["zero.txt"].GetNullFields())
		}
		if !byName["null.txt"].HasNullValues() {
			t.Errorf("%s: the null-mode entry reports no null values", form)
		}
	}
}

// ZeroMode behaves as mode 0 wherever a mode is applied.
func TestZeroModeActsAsZero(t *testing.T) {
	if ZeroMode.Perm() != 0 {
		t.Errorf("ZeroMode.Perm() = %o, want 0", ZeroMode.Perm())
	}
	if ZeroMode&os.ModeType != 0 {
		t.Errorf("ZeroMode carries a type bit")
	}
	if ZeroMode.IsDir() {
		t.Errorf("ZeroMode is a directory")
	}
	if formatMode(ZeroMode) != "----------" {
		t.Errorf("formatMode(ZeroMode) = %q", formatMode(ZeroMode))
	}
	if KnownMode(0) != ZeroMode || KnownMode(0644) != 0644 || KnownMode(os.ModeDir|0755) != os.ModeDir|0755 {
		t.Errorf("KnownMode maps only all-zero to ZeroMode")
	}
	if !IsNullMode(0) || IsNullMode(ZeroMode) {
		t.Errorf("IsNullMode: 0 is null, ZeroMode is not")
	}
}

// The canonical ID of a zero-mode entry differs from the null-mode one:
// they are different descriptions.
func TestZeroModeAndNullModeHaveDifferentIDs(t *testing.T) {
	when := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	zero := NewManifest()
	zero.Entries = append(zero.Entries, &Entry{Mode: ZeroMode, Timestamp: when, Size: 1, Name: "a.txt"})
	null := NewManifest()
	null.Entries = append(null.Entries, &Entry{Mode: 0, Timestamp: when, Size: 1, Name: "a.txt"})
	if zero.ComputeC4ID() == null.ComputeC4ID() {
		t.Errorf("zero and null modes must not share an ID")
	}
}
