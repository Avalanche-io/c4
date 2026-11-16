package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// The shared fixture for the store split race (testdata/split-race-store,
// design/store-split-race.md): a tree store the library split, with one
// object one level above its proper place, where a write that raced
// another process's split leaves it. The ports read it to check their own
// store readers. `C4M_UPDATE_VECTORS=1 go test ./store -run
// TestSplitRaceStoreFixture` rewrites it from the library.

const splitFixtureDir = "testdata/split-race-store"

// buildSplitFixture writes the fixture's store under root and returns the
// misplaced object's ID. The store is the library's: Put at split
// threshold 4 of contents "object N", taken in order of N, skipping any
// whose first shard segment equals another's under case folding, so the
// tree is the same on a case-insensitive filesystem; at least twelve, and
// until two share a shard directory. Then the first
// object by ID whose shard directory also holds another object is moved
// up one level, the inverse of the move Settle makes. (Its directory must
// hold another object: an empty one would not survive in git, and without
// it the object's proper place is where it already is.)
func buildSplitFixture(t *testing.T, root string) c4.ID {
	t.Helper()
	s := splitTestStore(t, root, 4)
	var ids []c4.ID
	segs := map[string]string{}
	shared := false
	bySeg := map[string]int{}
	for i := 1; len(ids) < 12 || !shared; i++ {
		body := fmt.Sprintf("object %d", i)
		id := c4.Identify(strings.NewReader(body))
		seg := id.String()[2:4]
		if o, ok := segs[strings.ToLower(seg)]; ok && o != seg {
			continue
		}
		segs[strings.ToLower(seg)] = seg
		if _, err := s.Put(strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		p, _ := s.ContentPath(id)
		if d := filepath.Dir(p); d != filepath.Join(root, "c4") && d != root {
			bySeg[d]++
			shared = shared || bySeg[d] > 1
		}
		if i > 1000 {
			t.Fatal("fixture: no contents found")
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	perDir := map[string]int{}
	for _, id := range ids {
		p, _ := s.ContentPath(id)
		// In c4/ itself (no subdirectory for its segment yet, which is
		// its proper place), or under its own segment spelled exactly.
		if d := filepath.Dir(p); d != filepath.Join(root, "c4") && d != filepath.Join(root, "c4", id.String()[2:4]) {
			t.Fatalf("fixture: %s is at %s", id.String()[:12], p)
		}
		perDir[filepath.Dir(p)]++
	}
	for _, id := range ids {
		p, _ := s.ContentPath(id)
		if perDir[filepath.Dir(p)] < 2 {
			continue
		}
		if err := os.Rename(p, filepath.Join(filepath.Dir(filepath.Dir(p)), id.String())); err != nil {
			t.Fatal(err)
		}
		return id
	}
	t.Fatal("fixture: no shard directory holds two objects")
	return c4.ID{}
}

// fixtureFiles lists every file under root, by slash-separated path, with
// its bytes.
func fixtureFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		b, err := os.ReadFile(p)
		out[filepath.ToSlash(rel)] = b
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSplitRaceStoreFixture(t *testing.T) {
	built := t.TempDir()
	up := buildSplitFixture(t, built)
	want := fixtureFiles(t, built)

	store := filepath.Join(splitFixtureDir, "store")
	if os.Getenv("C4M_UPDATE_VECTORS") != "" {
		if err := os.RemoveAll(store); err != nil {
			t.Fatal(err)
		}
		for p, b := range want {
			dst := filepath.Join(store, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var list bytes.Buffer
		for _, p := range sortedKeys(want) {
			fmt.Fprintf(&list, "%s %s\n", filepath.Base(p), want[p])
		}
		if err := os.WriteFile(filepath.Join(splitFixtureDir, "objects.txt"), list.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(splitFixtureDir, "misplaced.txt"), []byte(up.String()+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := fixtureFiles(t, store)
	if len(got) != len(want) {
		t.Errorf("the fixture holds %d files, the library writes %d", len(got), len(want))
	}
	for p, b := range want {
		if !bytes.Equal(got[p], b) {
			t.Errorf("fixture file %s differs from what the library writes", p)
		}
	}
	mp, err := os.ReadFile(filepath.Join(splitFixtureDir, "misplaced.txt"))
	if err != nil || strings.TrimSpace(string(mp)) != up.String() {
		t.Errorf("misplaced.txt does not name %s", up)
	}

	// What a conforming reader must do with the committed fixture, on a
	// copy so nothing is written into testdata.
	work := t.TempDir()
	for p, b := range got {
		dst := filepath.Join(work, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(dst), 0o755)
		os.WriteFile(dst, b, 0o644)
	}
	r := splitTestStore(t, work, 4)
	list, err := os.ReadFile(filepath.Join(splitFixtureDir, "objects.txt"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		f := strings.SplitN(line, " ", 2)
		id, err := c4.Parse(f[0])
		if err != nil {
			t.Fatalf("objects.txt: %v", err)
		}
		n++
		rc, err := r.Open(id)
		if err != nil {
			t.Errorf("a conforming reader must find %s: %v", f[0][:12], err)
			continue
		}
		var b bytes.Buffer
		b.ReadFrom(rc)
		rc.Close()
		if b.String() != f[1] || c4.Identify(&b) != id {
			t.Errorf("%s reads back %q, want %q", f[0][:12], b.String(), f[1])
		}
	}
	if n != len(got)-0 || n < 12 {
		t.Errorf("objects.txt lists %d objects for %d files", n, len(got))
	}
	// A reader that only looks at the deepest level misses exactly the
	// misplaced object; Settle moves it, and then it does not.
	for _, line := range strings.Split(strings.TrimSpace(string(list)), "\n") {
		id, _ := c4.Parse(strings.SplitN(line, " ", 2)[0])
		_, err := os.Stat(r.path(id))
		if missed := err != nil; missed != (id == up) {
			t.Errorf("an exact-path reader: %s missed=%v, want missed only for the misplaced object", id.String()[:12], missed)
		}
	}
	if m, err := r.Settle(true); err != nil || len(m) != 1 || m[0].ID != up || !m[0].Moved {
		t.Errorf("Settle(true) on the fixture: %v, %v; want the one misplaced object moved", m, err)
	}
}

func sortedKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
