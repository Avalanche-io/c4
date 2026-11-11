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

// The shared conformance fixture for names and link targets that hold a
// backslash (testdata/backslash-name-vector; README there). A backslash is
// a literal in a c4m name (only "/" separates paths), written "\\" by
// SafeName. The decoder must consume every backslash escape as a two-byte
// unit: through 1.0.25 a name ending in a backslash read back with a space
// added. Every entry is built through the package's API;
// C4M_UPDATE_VECTORS=1 rewrites the files.
type backslashCase struct {
	name   string // the entry's name as it is on disk
	target string // a symlink target, or ""
	depth  int
}

func backslashCases() []backslashCase {
	return []backslashCase{
		{name: "plain.txt"},
		{name: `trail\`},
		{name: `end\\`},
		{name: `a\ b`},
		{name: `link`, target: `target\`},
		{name: `dir\x/`},
		{name: "in.txt", depth: 1},
	}
}

func backslashVectorManifest() *Manifest {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewManifest()
	for _, c := range backslashCases() {
		e := &Entry{Name: c.name, Depth: c.depth, Timestamp: ts}
		switch {
		case strings.HasSuffix(c.name, "/"):
			e.Mode, e.Size = os.ModeDir|0o755, -1
		case c.target != "":
			e.Mode, e.Size, e.Target = os.ModeSymlink|0o777, int64(len(c.target)), c.target
		default:
			content := "content of " + c.name
			e.Mode, e.Size, e.C4ID = 0o644, int64(len(content)), c4.Identify(strings.NewReader(content))
		}
		m.AddEntry(e)
	}
	Normalize(m)
	return m
}

func TestBackslashNameVector(t *testing.T) {
	dir := filepath.Join("testdata", "backslash-name-vector")
	m := backslashVectorManifest()

	defer func(l *time.Location) { time.Local = l }(time.Local)
	time.Local = time.UTC

	var canon, pretty, patch bytes.Buffer
	if err := NewEncoder(&canon).Encode(m); err != nil {
		t.Fatal(err)
	}
	pe := NewEncoder(&pretty)
	pe.SetPretty(true)
	if err := pe.Encode(m); err != nil {
		t.Fatal(err)
	}
	base := NewManifest()
	if err := NewEncoder(&patch).EncodePatch(&PatchResult{OldID: base.ComputeC4ID(), Patch: m}); err != nil {
		t.Fatal(err)
	}
	rootID := m.ComputeC4ID().String() + "\n"
	files := map[string][]byte{
		"canonical.c4m": canon.Bytes(), "pretty.c4m": pretty.Bytes(),
		"patch.c4m": patch.Bytes(), "root-id.txt": []byte(rootID),
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

	check := func(form string, entries []*Entry) {
		t.Helper()
		got := map[string]string{}
		for _, e := range entries {
			got[e.Name] = e.Target
		}
		if len(got) != len(backslashCases()) {
			t.Errorf("%s: read back %d names, want %d: %q", form, len(got), len(backslashCases()), keys(got))
		}
		for _, c := range backslashCases() {
			target, ok := got[c.name]
			if !ok {
				t.Errorf("%s: name %q not read back; got %q", form, c.name, keys(got))
				continue
			}
			if target != c.target {
				t.Errorf("%s: %q target read back as %q, want %q", form, c.name, target, c.target)
			}
		}
	}
	for _, form := range []string{"canonical.c4m", "pretty.c4m"} {
		dec, err := Unmarshal(files[form])
		if err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		if got := dec.ComputeC4ID().String() + "\n"; got != rootID {
			t.Errorf("%s resolves to %s, want %s", form, got, rootID)
		}
		check(form, dec.Entries)
		if again, _ := Marshal(dec); form == "canonical.c4m" && !bytes.Equal(again, files[form]) {
			t.Errorf("canonical.c4m written again differs:\n%s", again)
		}
	}
	sections, err := DecodePatchChain(bytes.NewReader(files["patch.c4m"]))
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
	m := backslashVectorManifest()
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
