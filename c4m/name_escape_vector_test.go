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

// The shared name-escape vector (c4m/testdata/name-escape-vector). In a
// written name a space is "\ ", a double quote is "\"", and a control byte
// is encoded by SafeName (\t, \n, \r, \0, or braille between ¤); no
// conforming writer leaves one raw (SPECIFICATION.md "Universal Filename
// Encoding" and "c4m field-boundary escaping"). A raw one is damage: an
// unescaped space most often means a damaged C4 ID field, which used to
// join the name ("real.txt junk" read as one name, its ID dropped). Every
// decoder refuses such a line, naming it. Indentation is not damage: any
// consistent unit reads, since the library's own APIs can write one.

type nameEscapeCase struct {
	file string
	dir  bool
	name string // written raw in place of the base line's name
	why  string
}

func nameEscapeCases() []nameEscapeCase {
	return []nameEscapeCase{
		{"file-unescaped-space", false, "real txt", "a space in a name, unescaped"},
		{"file-damaged-id-field", false, "real.txt junk", "a damaged field before the C4 ID joins the name"},
		{"file-raw-quote", false, `re"al.txt`, "a raw double quote"},
		{"file-raw-tab", false, "re\tal.txt", "a raw tab"},
		{"file-raw-control", false, "re\x01al.txt", "a raw control byte"},
		{"dir-unescaped-space", true, "my dir/", "a directory name with an unescaped space"},
	}
}

// TestNameEscapeVector reads the shared vector as it was written on master
// (its generator lives there): the ok files decode, the escaped name to
// exactly itself and the 3-space indent to depth 1, and every damaged file
// is refused naming line 1.
func TestNameEscapeVector(t *testing.T) {
	dir := filepath.Join("testdata", "name-escape-vector")
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, ok := range []string{"ok-file.c4m", "ok-dir.c4m"} {
		if _, err := Unmarshal(read(ok)); err != nil {
			t.Fatalf("%s must decode: %v", ok, err)
		}
	}
	m, err := Unmarshal(read("ok-escaped.c4m"))
	if want := "my file \"q\"\t.txt"; err != nil || len(m.Entries) != 1 || m.Entries[0].Name != want {
		t.Fatalf("ok-escaped.c4m must decode to %q: %v %v", want, m, err)
	}
	m, err = Unmarshal(read("ok-indent-3.c4m"))
	if err != nil || len(m.Entries) != 2 || m.Entries[1].Depth != 1 {
		t.Fatalf("ok-indent-3.c4m must decode, x.txt at depth 1: %v %v", m, err)
	}
	for _, c := range nameEscapeCases() {
		m, err := Unmarshal(read(c.file + ".c4m"))
		if err == nil {
			t.Errorf("%s (%s) decoded instead of being refused: name %q", c.file, c.why, m.Entries[0].Name)
			continue
		}
		if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%s: the error must name the line, got %v", c.file, err)
		}
	}
}

// IsSequence set on a name with no range notation: the formatter used to
// return the raw name, the one writer path that could leave a space, a
// quote or a control byte unescaped. It is now an ordinary name, escaped,
// and reads back as itself.
func TestSequenceFlagWithoutRangeIsEscaped(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	name := "bad seq \"x\"\x01[a]"
	m := NewManifest()
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 1, Name: name, IsSequence: true, C4ID: c4.Identify(strings.NewReader("x"))})
	text, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	rawQuote := false
	for i, b := range text {
		if b == '"' && (i == 0 || text[i-1] != '\\') {
			rawQuote = true
		}
	}
	if rawQuote || bytes.ContainsRune(text, '\x01') || bytes.Contains(text, []byte("bad seq")) {
		t.Errorf("a raw space, quote or control byte was written: %q", text)
	}
	back, err := Unmarshal(text)
	if err != nil || back.Entries[0].Name != name {
		t.Fatalf("does not read back as %q: %v %v", name, back, err)
	}
}
