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

// The shared name-rule vector (c4m/testdata/name-rule-vector): a name is a
// bare entry name, never a path. Only "/" separates paths in c4m, and a
// name may end in "/" only as a directory's marker; ".", "..", "./", "../"
// and "/" are path components, not names (SPECIFICATION.md, "Invalid
// names", and "Parsers MUST … Reject path traversal attempts"). Through
// 1.0.26 the Go decoder, c4py, c4ts and libc4 cut a name at its first "/"
// and dropped the rest of the line, C4 ID included, so "sub/real.txt" on
// a file line read as a directory "sub/" with exit 0; c4m-swift refused.

// nameRuleBases are the library's own lines for a file and a directory:
// every case changes only the name, never anything else on the line.
func nameRuleBases() (file, dir string) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	enc := func(e *Entry) string {
		m := NewManifest()
		m.AddEntry(e)
		text, err := Marshal(m)
		if err != nil {
			panic(err)
		}
		return strings.TrimSuffix(string(text), "\n")
	}
	file = enc(&Entry{Mode: 0o644, Timestamp: ts, Size: 6, Name: "real.txt", C4ID: c4.Identify(strings.NewReader("hello\n"))})
	dir = enc(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: 0, Name: "empty/", C4ID: c4.Identify(bytes.NewReader(nil))})
	return file, dir
}

type nameRuleCase struct {
	file string // the case's file name in the vector
	dir  bool   // damage the directory line, else the file line
	name string // the name written in place of the base's
	why  string
}

func nameRuleCases() []nameRuleCase {
	return []nameRuleCase{
		{"file-sub", false, "sub/real.txt", "a file name holding /"},
		{"file-leading-slash", false, "/real.txt", "a file name starting with /"},
		{"file-trailing-path", false, "real.txt/x", "/ inside a name, more after it"},
		{"file-dotdot-inside", false, "a/../real.txt", "a traversal inside a name"},
		{"file-dot", false, ".", "the current directory as a file name"},
		{"file-dotdot", false, "..", "the parent directory as a file name"},
		{"dir-nested", true, "a/empty/", "a directory name holding / before its marker"},
		{"dir-dot", true, "./", "the current directory as a directory name"},
		{"dir-dotdot", true, "../", "the parent directory as a directory name"},
		{"dir-root", true, "/", "the root as a directory name"},
		{"dir-double-slash", true, "empty//", "a second / after the marker"},
	}
}

// nameRuleDamage writes c.name in place of the base line's own name.
func nameRuleDamage(fileBase, dirBase string, c nameRuleCase) string {
	if c.dir {
		return strings.Replace(dirBase, " empty/ ", " "+c.name+" ", 1)
	}
	return strings.Replace(fileBase, " real.txt ", " "+c.name+" ", 1)
}

func TestNameRuleVector(t *testing.T) {
	dir := filepath.Join("testdata", "name-rule-vector")
	fileBase, dirBase := nameRuleBases()
	files := map[string][]byte{"ok-file.c4m": []byte(fileBase + "\n"), "ok-dir.c4m": []byte(dirBase + "\n")}
	for _, c := range nameRuleCases() {
		d := nameRuleDamage(fileBase, dirBase, c)
		if d == fileBase || d == dirBase {
			t.Fatalf("%s: damage changed nothing", c.file)
		}
		files[c.file+".c4m"] = []byte(d + "\n")
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
			t.Errorf("%s differs from what the library and the damage write:\n--- file\n%s--- generated\n%s", name, got, want)
		}
	}

	for _, ok := range []string{"ok-file.c4m", "ok-dir.c4m"} {
		if _, err := Unmarshal(files[ok]); err != nil {
			t.Fatalf("%s must decode: %v", ok, err)
		}
	}
	for _, c := range nameRuleCases() {
		m, err := Unmarshal(files[c.file+".c4m"])
		if err == nil {
			e := m.Entries[0]
			t.Errorf("%s (%s) decoded instead of being refused: name %q id %v", c.file, c.why, e.Name, e.C4ID)
			continue
		}
		if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%s: the error must name the line, got %v", c.file, err)
		}
	}
}

// The encoder half: the library never writes a name the format forbids.
// Encode and EncodePatch refuse it with an error naming the entry, and
// write nothing.
func TestEncoderRefusesNamesThatArePaths(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, c := range nameRuleCases() {
		mode := os.FileMode(0o644)
		if c.dir {
			mode = os.ModeDir | 0o755
		}
		m := NewManifest()
		m.AddEntry(&Entry{Mode: mode, Timestamp: ts, Size: 1, Name: c.name})
		var b bytes.Buffer
		err := NewEncoder(&b).Encode(m)
		if err == nil {
			t.Errorf("Encode wrote %q (%s): %q", c.name, c.why, b.String())
			continue
		}
		if b.Len() != 0 {
			t.Errorf("Encode refused %q but wrote %q", c.name, b.String())
		}
		if !strings.Contains(err.Error(), c.name) {
			t.Errorf("Encode's error for %q does not name it: %v", c.name, err)
		}

		pr := &PatchResult{Patch: m}
		b.Reset()
		if err := NewEncoder(&b).EncodePatch(pr); err == nil {
			t.Errorf("EncodePatch wrote %q (%s): %q", c.name, c.why, b.String())
		}
	}
	// A directory's trailing "/" is its marker and is written.
	m := NewManifest()
	m.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: 0, Name: "empty/", C4ID: c4.Identify(bytes.NewReader(nil))})
	if _, err := Marshal(m); err != nil {
		t.Errorf("a directory name ending in / must be written: %v", err)
	}
}

// A manifest built in code with a name the format forbids has no C4 ID and
// no canonical text: no conforming c4m spells it. ComputeC4ID gives the nil
// ID (the format's "unavailable"), Canonical gives "", WriteCanonical
// writes nothing and errs, and Validate names the entry. A forbidden name
// nested under a directory takes the whole tree's ID with it.
func TestForbiddenNameHasNoC4ID(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	flat := NewManifest()
	flat.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 6, Name: "sub/real.txt", C4ID: c4.Identify(strings.NewReader("hello\n"))})
	nested := NewManifest()
	nested.AddEntry(&Entry{Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, Name: "d/"})
	nested.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 6, Name: "../real.txt", Depth: 1, C4ID: c4.Identify(strings.NewReader("hello\n"))})
	for label, m := range map[string]*Manifest{"flat": flat, "nested": nested} {
		if id := m.ComputeC4ID(); !id.IsNil() {
			t.Errorf("%s: ComputeC4ID = %s, want the nil ID", label, id)
		}
		if text := m.Canonical(); text != "" {
			t.Errorf("%s: Canonical = %q, want no text", label, text)
		}
		var b bytes.Buffer
		if err := m.WriteCanonical(&b); err == nil || b.Len() != 0 {
			t.Errorf("%s: WriteCanonical wrote %q, err %v; want nothing and an error", label, b.String(), err)
		}
		err := m.Validate()
		if err == nil || !strings.Contains(err.Error(), "real.txt") {
			t.Errorf("%s: Validate must name the entry, got %v", label, err)
		}
	}
	// The same tree with a sound name keeps its ID.
	ok := NewManifest()
	ok.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 6, Name: "real.txt", C4ID: c4.Identify(strings.NewReader("hello\n"))})
	if ok.ComputeC4ID().IsNil() || ok.Canonical() == "" {
		t.Error("a sound manifest must keep its ID and text")
	}
}
