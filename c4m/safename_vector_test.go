package c4m

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/Avalanche-io/c4"
)

// The shared SafeName vector (c4m/testdata/safename-vector). C4M-STANDARD
// §4.4.2: printable UTF-8 (where Go's unicode.IsPrint is true) passes
// through, except ¤ and \; NUL, tab, LF, CR and \ use backslash escapes;
// every other byte is braille between ¤. One name, one written form, one C4
// ID, in every implementation. Each case sits at a tier boundary; the names
// are raw bytes, so invalid UTF-8 is covered too.

type safeNameCase struct {
	label string
	raw   string // the payload, placed between "<label>-" and ".txt"
}

func safeNameCases() []safeNameCase {
	return []safeNameCase{
		{"ascii-soh", "\x01"},
		{"ascii-us", "\x1f"},
		{"del", "\x7f"},
		{"c1-pad", "\u0080"},
		{"c1-apc", "\u009f"},
		{"nbsp", "\u00a0"},
		{"currency-sign", "\u00a4"},
		{"soft-hyphen", "\u00ad"},
		{"en-quad", "\u2000"},
		{"ideographic-space", "\u3000"},
		{"line-separator", "\u2028"},
		{"paragraph-separator", "\u2029"},
		{"zero-width-space", "\u200b"},
		{"zero-width-joiner", "\u200d"},
		{"bom", "\ufeff"},
		{"combining-acute", "e\u0301"},
		{"variation-selector", "\u2764\ufe0f"},
		{"emoji-zwj-sequence", "\U0001F468\u200d\U0001F469\u200d\U0001F467"},
		{"private-use", "\ue000"},
		{"unassigned", "\u0378"},
		{"noncharacter", "\ufffe"},
		{"invalid-ff", "\xff"},
		{"lone-continuation", "\x80"},
		{"overlong-nul", "\xc0\x80"},
		{"surrogate", "\xed\xa0\x80"},
		{"tab", "\t"},
		{"newline", "\n"},
		{"carriage-return", "\r"},
		{"backslash", "\\"},
		{"printable-ascii", "plain"},
		{"cjk", "\u65e5\u672c"},
	}
}

func safeNameName(c safeNameCase) string { return c.label + "-" + c.raw + ".txt" }

func TestSafeNameVector(t *testing.T) {
	dir := filepath.Join("testdata", "safename-vector")
	m := NewManifest()
	var cases strings.Builder
	for _, c := range safeNameCases() {
		name := safeNameName(c)
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: NullTimestamp(), Size: 1, Name: name, C4ID: c4.Identify(strings.NewReader(c.label))})
		// The field as the library writes it: SafeName, then the c4m
		// field-boundary escapes.
		fmt.Fprintf(&cases, "%s %s %s\n", c.label, hex.EncodeToString([]byte(name)), formatName(name, false))
	}
	m.SortEntries()
	text, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"names.c4m":           text,
		"cases.txt":           []byte(cases.String()),
		"root-id.txt":         []byte(m.ComputeC4ID().String() + "\n"),
		"unicode-version.txt": []byte(unicode.Version + "\n"),
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
			t.Errorf("%s differs from what the library writes:\n--- file\n%q\n--- library\n%q", name, got, want)
		}
	}

	// A conforming reader decodes every name to its exact bytes; a
	// conforming writer encodes every raw name to the same field.
	got, err := Unmarshal(text)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]bool{}
	for _, e := range got.Entries {
		byName[e.Name] = true
	}
	for _, c := range safeNameCases() {
		if !byName[safeNameName(c)] {
			t.Errorf("%s: %q does not read back as itself", c.label, safeNameName(c))
		}
	}
	if got.ComputeC4ID() != m.ComputeC4ID() {
		t.Errorf("root ID %s, want %s", got.ComputeC4ID(), m.ComputeC4ID())
	}
}
