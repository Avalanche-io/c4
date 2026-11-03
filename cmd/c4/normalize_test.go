package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// normalizeFixture scans a two-file tree with c4 id (the library-made
// reference), flattens it with c4 id FILE, and derives the hand-edit
// variants by decoding the flat form, mutating the d/ entry through the
// c4m package, and encoding again. No c4m text is written by hand.
type normalizeFixture struct {
	bin    string
	env    map[string]string
	dir    string
	treeID string
	flat   string // normalised flat form of the reference, as c4 id prints it
}

func newNormalizeFixture(t *testing.T) *normalizeFixture {
	t.Helper()
	f := &normalizeFixture{bin: buildC4(t), dir: t.TempDir()}
	f.env = map[string]string{"C4_STORE": filepath.Join(f.dir, "store"), "HOME": filepath.Join(f.dir, "home")}
	tree := filepath.Join(f.dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "alpha!", "d/b.txt": "beta!"})

	stdout, stderr, code := runC4WithEnv(t, f.bin, f.env, "id", tree)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	ref := filepath.Join(f.dir, "n.c4m")
	if err := os.WriteFile(ref, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _, code = runC4WithEnv(t, f.bin, f.env, "id", "-q", tree)
	if code != 0 {
		t.Fatal("c4 id -q on the tree failed")
	}
	f.treeID = strings.TrimSpace(stdout)

	stdout, stderr, code = runC4WithEnv(t, f.bin, f.env, "id", ref)
	if code != 0 {
		t.Fatalf("c4 id on the reference: exit %d: %s", code, stderr)
	}
	f.flat = stdout
	if err := os.WriteFile(filepath.Join(f.dir, "flat.c4m"), []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// variant decodes the flat form, lets mutate change it, and writes it in
// canonical form under name.
func (f *normalizeFixture) variant(t *testing.T, name string, mutate func(m *c4m.Manifest)) string {
	t.Helper()
	m, err := c4m.Unmarshal([]byte(f.flat))
	if err != nil {
		t.Fatalf("decode flat: %v", err)
	}
	mutate(m)
	text, err := c4m.Marshal(m)
	if err != nil {
		t.Fatalf("encode %s: %v", name, err)
	}
	p := filepath.Join(f.dir, name)
	if err := os.WriteFile(p, text, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func dirEntry(t *testing.T, m *c4m.Manifest, name string) *c4m.Entry {
	t.Helper()
	for _, e := range m.Entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no entry %s", name)
	return nil
}

func TestNormalizeStaleAndNullAggregatesToTheLibraryID(t *testing.T) {
	f := newNormalizeFixture(t)
	other := c4.Identify(strings.NewReader("not the record"))
	stale := f.variant(t, "stale.c4m", func(m *c4m.Manifest) {
		d := dirEntry(t, m, "d/")
		d.Size = 999
		d.C4ID = other
	})
	nulled := f.variant(t, "derive.c4m", func(m *c4m.Manifest) {
		d := dirEntry(t, m, "d/")
		d.Size = -1
		d.C4ID = c4.ID{}
	})

	for _, p := range []string{stale, nulled} {
		stdout, stderr, code := runC4WithEnv(t, f.bin, f.env, "id", "-q", p)
		if code != 0 {
			t.Fatalf("c4 id -q %s: exit %d: %s", p, code, stderr)
		}
		if got := strings.TrimSpace(stdout); got != f.treeID {
			t.Errorf("%s normalises to %s, the library's scan is %s", filepath.Base(p), got, f.treeID)
		}
		stdout, stderr, code = runC4WithEnv(t, f.bin, f.env, "id", p)
		if code != 0 {
			t.Fatalf("c4 id %s: exit %d: %s", p, code, stderr)
		}
		if stdout != f.flat {
			t.Errorf("%s normalised text differs from the reference's:\n%s\nwant:\n%s", filepath.Base(p), stdout, f.flat)
		}
		if p == stale && !strings.Contains(stderr, "normalized: "+p+": d/: size 999 -> ") {
			t.Errorf("the stale size should be reported on stderr, got: %q", stderr)
		}
		if p == nulled && strings.Contains(stderr, "normalized:") {
			t.Errorf("filling nulls is not a change to report, got: %q", stderr)
		}
	}
}

func TestNormalizeListAndDiff(t *testing.T) {
	f := newNormalizeFixture(t)
	stale := f.variant(t, "stale.c4m", func(m *c4m.Manifest) {
		dirEntry(t, m, "d/").Size = 999
	})
	flat := filepath.Join(f.dir, "flat.c4m")

	stdout, _, code := runC4WithEnv(t, f.bin, f.env, "id", "-l", flat)
	if code != 0 || stdout != "" {
		t.Errorf("-l on a normalised file: exit %d, stdout %q; want 0 and nothing", code, stdout)
	}
	stdout, _, code = runC4WithEnv(t, f.bin, f.env, "id", "-l", flat, stale)
	if code != 1 || strings.TrimSpace(stdout) != stale {
		t.Errorf("-l on a stale file: exit %d, stdout %q; want 1 and the stale name", code, stdout)
	}
	stdout, _, code = runC4WithEnv(t, f.bin, f.env, "id", "-d", stale)
	if code != 1 || !strings.Contains(stdout, "--- "+stale) || !strings.Contains(stdout, "\n-") || !strings.Contains(stdout, "\n+") {
		t.Errorf("-d on a stale file: exit %d, stdout:\n%s", code, stdout)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("-l/-d must never write: %v", err)
	}
}

func TestNormalizeCollapsedDirectoryKeepsStatedValues(t *testing.T) {
	f := newNormalizeFixture(t)
	collapsed := f.variant(t, "collapsed.c4m", func(m *c4m.Manifest) {
		// drop d/'s children: d/ keeps its stated size and ID
		var keep []*c4m.Entry
		for _, e := range m.Entries {
			if e.Depth == 0 {
				keep = append(keep, e)
			}
		}
		m.Entries = keep
	})
	m, err := c4m.Unmarshal(mustRead(t, collapsed))
	if err != nil {
		t.Fatal(err)
	}
	want := m.ComputeC4ID().String()
	stdout, stderr, code := runC4WithEnv(t, f.bin, f.env, "id", "-q", collapsed)
	if code != 0 {
		t.Fatalf("c4 id -q: exit %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != want {
		t.Errorf("a collapsed directory was re-derived: %s, want %s", got, want)
	}
	if strings.Contains(stderr, "normalized:") {
		t.Errorf("nothing should be reported for a collapsed directory: %q", stderr)
	}
	_, _, code = runC4WithEnv(t, f.bin, f.env, "id", "-l", collapsed)
	if code != 0 {
		t.Errorf("-l should accept the collapsed file as normalised, exit %d", code)
	}
}

func TestNormalizeDropsAValueTheChildrenCannotSupport(t *testing.T) {
	f := newNormalizeFixture(t)
	edge := f.variant(t, "edge.c4m", func(m *c4m.Manifest) {
		dirEntry(t, m, "b.txt").Size = -1 // a structure-level child
		// d/ keeps the stated size the flat form gave it
	})
	stdout, stderr, code := runC4WithEnv(t, f.bin, f.env, "id", edge)
	if code != 0 && code != 2 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	for _, line := range strings.Split(stdout, "\n") {
		if strings.Contains(line, " d/ ") && !strings.Contains(line, " - d/ ") {
			t.Errorf("d/ should derive to a null size above a null child: %q", line)
		}
	}
	if !strings.Contains(stderr, "normalized: "+edge+": d/: size ") || !strings.Contains(stderr, " -> -") {
		t.Errorf("the dropped size should be reported: %q", stderr)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
