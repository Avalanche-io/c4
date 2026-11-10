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

// The shared conformance fixture for link targets that look like a C4 ID
// (testdata/link-target-vector; README there). Every entry is built
// through the package's API; C4M_UPDATE_VECTORS=1 rewrites the files.
type linkTargetCase struct {
	name, target string
	hardLink     int
	nullFields   bool  // mode, timestamp and size null, as views carry links
	nullTime     bool  // a symlink mode and size, but a null timestamp: what c4 id -e writes for a link whose mtime is the epoch
	id           c4.ID // the entry's own C4 ID, nil for none
}

func linkTargetCases() []linkTargetCase {
	idText := c4.Identify(strings.NewReader("some content")).String()
	content := c4.Identify(strings.NewReader("what the link points at"))
	return []linkTargetCase{
		{name: "id-null", target: idText, nullFields: true},
		{name: "id-set", target: idText, id: content},
		{name: "id-exr-null", target: idText + ".exr", nullFields: true},
		{name: "id-exr-set", target: idText + ".exr", id: content},
		{name: "c4-prefix-null", target: "c4studio", nullFields: true},
		{name: "objects-null", target: "../objects/" + idText, nullFields: true}, // control: read back before the fix
		{name: "hard-null-id", hardLink: -1, nullFields: true, id: content},
		{name: "hard-null-nil", hardLink: -1, nullFields: true},
		{name: "epoch-link-nil", target: "missing.txt", nullTime: true},
		{name: "epoch-link-id", target: "real.txt", nullTime: true, id: content},
	}
}

func linkTargetVectorManifest() *Manifest {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewManifest()
	for _, c := range linkTargetCases() {
		e := &Entry{Name: c.name, Target: c.target, HardLink: c.hardLink, C4ID: c.id}
		switch {
		case c.nullFields:
			e.Size, e.Timestamp = -1, NullTimestamp()
		case c.nullTime:
			e.Mode, e.Size, e.Timestamp = os.ModeSymlink|0o755, int64(len(c.target)), NullTimestamp()
		case c.target != "":
			e.Mode, e.Size, e.Timestamp = os.ModeSymlink|0o777, int64(len(c.target)), ts
		}
		m.AddEntry(e)
	}
	m.SortEntries()
	return m
}

func TestLinkTargetVector(t *testing.T) {
	dir := filepath.Join("testdata", "link-target-vector")
	m := linkTargetVectorManifest()

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
	base := NewManifest() // the empty tree: the patch adds every entry
	pr := &PatchResult{OldID: base.ComputeC4ID(), Patch: m}
	if err := NewEncoder(&patch).EncodePatch(pr); err != nil {
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
		byName := map[string]*Entry{}
		for _, e := range entries {
			byName[e.Name] = e
		}
		for _, c := range linkTargetCases() {
			e := byName[c.name]
			if e == nil {
				t.Errorf("%s: %s missing", form, c.name)
				continue
			}
			if e.Target != c.target || e.HardLink != c.hardLink || e.C4ID != c.id {
				t.Errorf("%s: %s read back as target %q hard link %d C4 ID %v; want %q %d %v",
					form, c.name, e.Target, e.HardLink, e.C4ID, c.target, c.hardLink, c.id)
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
	}
	sections, err := DecodePatchChain(bytes.NewReader(files["patch.c4m"]))
	if err != nil {
		t.Fatalf("patch.c4m: %v", err)
	}
	if len(sections) != 1 || sections[0].BaseID != base.ComputeC4ID() {
		t.Fatalf("patch.c4m must be one section over the empty tree, got %d", len(sections))
	}
	check("patch.c4m", sections[0].Entries)
}
