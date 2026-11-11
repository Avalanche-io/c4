package reconcile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// folds reports whether dir's volume takes "A" and "a" for one name.
func folds(t *testing.T, dir string) bool {
	t.Helper()
	p := filepath.Join(dir, "probe-A")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(p)
	_, err := os.Lstat(filepath.Join(dir, "probe-a"))
	return err == nil
}

// Names a destination folder takes for one (README.md and readme.md;
// Docs/ and docs/) used to be written over each other: one name with the
// other's bytes, or two directories merged. On a folding volume the last
// in byte order is written whole and the others are refused, a directory
// with its subtree; on a volume that tells them apart nothing is refused.
// The test measures its own volume first, so it holds on every platform.
func TestPlanRefusesNamesTheFolderFoldsTogether(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	src := store.NewRAM()
	file := func(name, content string, depth int) *c4m.Entry {
		id, err := src.Put(strings.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		return &c4m.Entry{Name: name, Depth: depth, Mode: 0o644, Timestamp: ts, Size: int64(len(content)), C4ID: id}
	}
	dir := func(name string) *c4m.Entry {
		return &c4m.Entry{Name: name, Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1, C4ID: c4.ID{}}
	}
	target := c4m.NewManifest()
	target.AddEntry(file("README.md", "UPPER readme\n", 0))
	target.AddEntry(file("readme.md", "lower readme\n", 0))
	target.AddEntry(dir("Docs/"))
	target.AddEntry(file("a", "A in Docs\n", 1))
	target.AddEntry(dir("docs/"))
	target.AddEntry(file("b", "B in docs\n", 1))
	target.SortEntries()

	dest := t.TempDir()
	folding := folds(t, dest)
	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range plan.Refused {
		got = append(got, x.Path)
	}
	var want []string
	if folding {
		want = []string{"Docs/", "README.md"}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("folding=%v: refused %v, want %v", folding, got, want)
	}
	if _, err := r.Apply(plan, dest); err != nil {
		t.Fatal(err)
	}
	check := func(rel, want string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil || string(b) != want {
			t.Errorf("%s = %q (%v), want %q", rel, b, err, want)
		}
	}
	// The byte-order last is written whole on any volume.
	check("readme.md", "lower readme\n")
	check("docs/b", "B in docs\n")
	if !folding {
		check("README.md", "UPPER readme\n")
		check("Docs/a", "A in Docs\n")
	}
	// The probe leaves nothing behind.
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".c4-fold-probe-") {
			t.Errorf("probe left behind: %s", e.Name())
		}
	}
}
