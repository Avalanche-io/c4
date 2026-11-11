package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// collidingPairs are names some folders take for one: case, Unicode forms
// (NFC and NFD), ß and ss, final and medial sigma, the Ångström sign and Å;
// plus a control pair no folder folds.
var collidingPairs = [][2]string{
	{"README.md", "readme.md"},
	{"é.txt", "é.txt"},
	{"straße", "strasse"},
	{"σ.txt", "ς.txt"},
	{"Å", "Å"},
	{"a.txt", "b.txt"},
}

// pairFolds reports whether dir's volume takes a and b for one name.
func pairFolds(t *testing.T, dir, a, b string) bool {
	t.Helper()
	d, err := os.MkdirTemp(dir, "fold-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)
	if err := os.WriteFile(filepath.Join(d, a), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ia, _ := os.Lstat(filepath.Join(d, a))
	ib, err := os.Lstat(filepath.Join(d, b))
	return err == nil && os.SameFile(ia, ib)
}

// Every pair a folder takes for one is caught by measuring the folder, with
// no rule for how it folds: the last name in byte order is written whole
// (its own bytes), the other is refused and never written. A pair the
// folder tells apart is written as two files. The expectation for each pair
// is measured on this test's own volume first, so the test holds on every
// platform.
func TestPlanMeasuresEveryFoldTheFolderMakes(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	src := store.NewRAM()
	target := c4m.NewManifest()
	content := map[string]string{}
	for _, p := range collidingPairs {
		for _, n := range p {
			c := fmt.Sprintf("bytes of %q\n", n)
			content[n] = c
			id, err := src.Put(strings.NewReader(c))
			if err != nil {
				t.Fatal(err)
			}
			target.AddEntry(&c4m.Entry{Name: n, Mode: 0o644, Timestamp: ts, Size: int64(len(c)), C4ID: id})
		}
	}
	target.SortEntries()

	dest := t.TempDir()
	var wantRefused []string
	written := map[string]bool{}
	for _, p := range collidingPairs {
		lo, hi := p[0], p[1]
		if hi < lo {
			lo, hi = hi, lo
		}
		if pairFolds(t, dest, lo, hi) {
			wantRefused = append(wantRefused, lo)
			written[hi] = true
		} else {
			written[lo], written[hi] = true, true
		}
	}
	sort.Strings(wantRefused)

	// A dry run measures nothing and writes nothing.
	if _, err := New(WithSource(src), WithDryRun(true)).Plan(target, dest); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dest); len(entries) != 0 {
		t.Errorf("a dry run wrote into the destination: %v", entries)
	}

	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range plan.Refused {
		got = append(got, x.Path)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantRefused) {
		t.Fatalf("refused %q, want %q", got, wantRefused)
	}
	if _, err := r.Apply(plan, dest); err != nil {
		t.Fatal(err)
	}
	for n := range written {
		b, err := os.ReadFile(filepath.Join(dest, n))
		if err != nil || string(b) != content[n] {
			t.Errorf("%q = %q (%v), want its own bytes %q", n, b, err, content[n])
		}
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != len(written) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("destination lists %q, want exactly %d entries (a probe left behind?)", names, len(written))
	}
}

// Directories that fold together: the byte-order last is written with its
// own subtree, and the other's subtree is never written into it.
func TestPlanRefusesAFoldedDirectoryWithItsSubtree(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dest := t.TempDir()
	if !pairFolds(t, dest, "Docs", "docs") {
		t.Skip("this volume tells Docs and docs apart")
	}
	src := store.NewRAM()
	put := func(c string) *c4m.Entry {
		id, err := src.Put(strings.NewReader(c))
		if err != nil {
			t.Fatal(err)
		}
		return &c4m.Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(c)), C4ID: id, Depth: 1}
	}
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "Docs/", Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1})
	a := put("A in Docs\n")
	a.Name = "a"
	target.AddEntry(a)
	target.AddEntry(&c4m.Entry{Name: "docs/", Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1})
	b := put("B in docs\n")
	b.Name = "b"
	target.AddEntry(b)
	target.SortEntries()

	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Refused) != 1 || plan.Refused[0].Path != "Docs/" || plan.Refused[0].Entries != 2 {
		t.Fatalf("refused %+v, want Docs/ with 2 entries", plan.Refused)
	}
	if _, err := r.Apply(plan, dest); err != nil {
		t.Fatal(err)
	}
	if bb, err := os.ReadFile(filepath.Join(dest, "docs", "b")); err != nil || string(bb) != "B in docs\n" {
		t.Errorf("docs/b = %q (%v)", bb, err)
	}
	if _, err := os.Lstat(filepath.Join(dest, "docs", "a")); err == nil {
		t.Errorf("Docs/a was written into docs/")
	}
}

// A folder that already holds a file under a losing spelling (a leftover
// of an earlier run, say) lists the winner under exactly its own spelling
// afterwards: writing through a folding lookup kept the old spelling, so
// README.md held readme.md's bytes.
func TestApplyRespellsTheWinnerOverALeftoverSpelling(t *testing.T) {
	dest := t.TempDir()
	if !pairFolds(t, dest, "README.md", "readme.md") {
		t.Skip("this volume tells README.md and readme.md apart")
	}
	if err := os.WriteFile(filepath.Join(dest, "README.md"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	src := store.NewRAM()
	target := c4m.NewManifest()
	for _, n := range []string{"README.md", "readme.md"} {
		c := "bytes of " + n + "\n"
		id, _ := src.Put(strings.NewReader(c))
		target.AddEntry(&c4m.Entry{Name: n, Mode: 0o644, Timestamp: ts, Size: int64(len(c)), C4ID: id})
	}
	target.SortEntries()
	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Apply(plan, dest); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 1 || entries[0].Name() != "readme.md" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("folder lists %q, want exactly readme.md", names)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "readme.md")); string(b) != "bytes of readme.md\n" {
		t.Errorf("readme.md = %q", b)
	}
}
