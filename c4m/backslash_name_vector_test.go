package c4m

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The shared conformance fixture for names and link targets that hold a
// backslash (testdata/backslash-name-vector; README there), byte-identical
// to the files made by the Go library on master. A backslash is a literal
// in a c4m name (only "/" separates paths), written "\\" by SafeName. The
// decoder must consume every backslash escape as a two-byte unit: through
// 1.0.25 a name ending in a backslash read back with a space added.
//
// This line's library derives directory C4 IDs differently from master's,
// so it does not regenerate the files; it reads them, which is what the
// fix is about.
func TestBackslashNameVector(t *testing.T) {
	dir := filepath.Join("testdata", "backslash-name-vector")
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	rootID := string(bytes.TrimSpace(read("root-id.txt")))
	want := map[string]string{ // name -> symlink target
		"plain.txt": "",
		`trail\`:    "",
		`end\\`:     "",
		`a\ b`:      "",
		"link":      `target\`,
		`dir\x/`:    "",
		"in.txt":    "",
	}
	check := func(form string, entries []*Entry) {
		t.Helper()
		got := map[string]string{}
		for _, e := range entries {
			got[e.Name] = e.Target
		}
		if len(got) != len(want) {
			t.Errorf("%s: read back %d names, want %d: %q", form, len(got), len(want), got)
		}
		for name, target := range want {
			g, ok := got[name]
			if !ok {
				t.Errorf("%s: name %q not read back; got %q", form, name, got)
				continue
			}
			if g != target {
				t.Errorf("%s: %q target read back as %q, want %q", form, name, g, target)
			}
		}
	}
	for _, form := range []string{"canonical.c4m", "pretty.c4m"} {
		m, err := Unmarshal(read(form))
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got := m.ComputeC4ID().String(); got != rootID {
			t.Errorf("%s resolves to %s, want %s", form, got, rootID)
		}
		check(form, m.Entries)
	}
	canon := read("canonical.c4m")
	m, _ := Unmarshal(canon)
	if again, _ := Marshal(m); !bytes.Equal(again, canon) {
		t.Errorf("canonical.c4m written again differs:\n%s", again)
	}
	sections, err := DecodePatchChain(bytes.NewReader(read("patch.c4m")))
	if err != nil {
		t.Fatalf("patch.c4m: %v", err)
	}
	check("patch.c4m", sections[0].Entries)
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Only "/" separates paths in c4m (Joshua, 2026-11-10): a backslash or a
// colon is a literal character in a name. Manifest.Validate refused a name
// holding a backslash as "path traversal", and the line Validator refused
// it in the raw field and in the decoded name, so the library rejected
// names a scan writes for real files. Both now accept them.
func TestBackslashAndColonNamesAreValid(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "backslash-name-vector", "canonical.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	m.AddEntry(&Entry{Name: "c:d.txt", Mode: 0o644, Size: 0, Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), C4ID: NewManifest().ComputeC4ID()})
	m.SortEntries()
	if err := m.Validate(); err != nil {
		t.Errorf("Manifest.Validate: %v", err)
	}
	for _, form := range []string{"canonical.c4m", "pretty.c4m"} {
		f, err := os.Open(filepath.Join("testdata", "backslash-name-vector", form))
		if err != nil {
			t.Fatal(err)
		}
		v := NewValidator(false)
		err = v.ValidateManifest(f)
		f.Close()
		if err != nil {
			t.Errorf("Validator %s: %v", form, err)
		}
	}
	// "/" is still refused inside a name.
	bad := NewManifest()
	bad.AddEntry(&Entry{Name: "a/b.txt", Mode: 0o644, Size: 0, Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	if err := bad.Validate(); err == nil {
		t.Error(`Manifest.Validate accepted "a/b.txt"`)
	}
}
