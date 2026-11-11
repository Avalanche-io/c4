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

// planApply plans and applies target onto dest with every content in src.
func planApply(t *testing.T, src *store.RAM, target *c4m.Manifest, dest string) *Plan {
	t.Helper()
	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Apply(plan, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("apply errors: %v", res.Errors)
	}
	return plan
}

// listing returns dir's names, with "/" after a directory, recursively.
func listing(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			rel += "/"
		}
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out
}

// A description that only changes a file's case: on a folding folder the
// existing Readme.md is the target's readme.md (the same entry), so it is
// kept, given the target's bytes and respelled. Through 1.0.25 it counted
// as unlisted and was removed after the target's bytes were written into
// it, leaving the folder empty with exit 0.
func TestCaseOnlyRenameKeepsTheFile(t *testing.T) {
	dest := t.TempDir()
	if !pairFolds(t, dest, "Readme.md", "readme.md") {
		t.Skip("this volume tells Readme.md and readme.md apart")
	}
	os.WriteFile(filepath.Join(dest, "Readme.md"), []byte("old\n"), 0o644)
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	src := store.NewRAM()
	id, _ := src.Put(strings.NewReader("new\n"))
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "readme.md", Mode: 0o644, Timestamp: ts, Size: 4, C4ID: id})
	planApply(t, src, target, dest)
	if got := listing(t, dest); !reflect.DeepEqual(got, []string{"readme.md"}) {
		t.Fatalf("folder lists %q, want exactly readme.md", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "readme.md")); string(b) != "new\n" {
		t.Errorf("readme.md = %q, want the target's bytes", b)
	}
}

// Both spellings in the target, over an existing third: the byte-order
// last is written whole and respelled, the other refused; nothing is lost.
func TestBothSpellingsOverAThirdKeepTheWinner(t *testing.T) {
	dest := t.TempDir()
	if !pairFolds(t, dest, "Readme.md", "readme.md") {
		t.Skip("this volume tells case apart")
	}
	os.WriteFile(filepath.Join(dest, "Readme.md"), []byte("old\n"), 0o644)
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	src := store.NewRAM()
	target := c4m.NewManifest()
	for _, n := range []string{"README.md", "readme.md"} {
		c := "bytes of " + n + "\n"
		id, _ := src.Put(strings.NewReader(c))
		target.AddEntry(&c4m.Entry{Name: n, Mode: 0o644, Timestamp: ts, Size: int64(len(c)), C4ID: id})
	}
	target.SortEntries()
	plan := planApply(t, src, target, dest)
	if len(plan.Refused) != 1 || plan.Refused[0].Path != "README.md" {
		t.Errorf("refused %+v, want README.md", plan.Refused)
	}
	if got := listing(t, dest); !reflect.DeepEqual(got, []string{"readme.md"}) {
		t.Fatalf("folder lists %q, want exactly readme.md", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "readme.md")); string(b) != "bytes of readme.md\n" {
		t.Errorf("readme.md = %q", b)
	}
}

// A case-only rename of a directory that holds files: every file survives,
// under the target's exact spellings, and a file the target does not list
// under any spelling is still removed.
func TestCaseOnlyRenameOfADirectoryKeepsItsFiles(t *testing.T) {
	dest := t.TempDir()
	if !pairFolds(t, dest, "Docs", "docs") {
		t.Skip("this volume tells case apart")
	}
	os.MkdirAll(filepath.Join(dest, "Docs"), 0o755)
	for n, c := range map[string]string{"A.txt": "a\n", "b.txt": "b\n", "gone.txt": "gone\n"} {
		os.WriteFile(filepath.Join(dest, "Docs", n), []byte(c), 0o644)
	}
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	src := store.NewRAM()
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "docs/", Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1})
	for _, n := range []string{"a.txt", "b.txt"} {
		c := strings.ToLower(n[:1]) + "\n"
		id, _ := src.Put(strings.NewReader(c))
		target.AddEntry(&c4m.Entry{Name: n, Depth: 1, Mode: 0o644, Timestamp: ts, Size: int64(len(c)), C4ID: id})
	}
	target.SortEntries()
	planApply(t, src, target, dest)
	if got, want := listing(t, dest), []string{"docs/", "docs/a.txt", "docs/b.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("folder lists %q, want %q", got, want)
	}
	for n, c := range map[string]string{"a.txt": "a\n", "b.txt": "b\n"} {
		if b, _ := os.ReadFile(filepath.Join(dest, "docs", n)); string(b) != c {
			t.Errorf("docs/%s = %q, want %q", n, b, c)
		}
	}
}
