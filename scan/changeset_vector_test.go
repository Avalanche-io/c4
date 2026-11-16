package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// changesetTreeEntry is one path of a changeset-vector state: a name ending
// in "/" is a directory.
type changesetTreeEntry struct {
	path string
	body string
	ts   time.Time
}

// writeChangesetTree makes the tree on disk with explicit modes and times,
// each directory's time set after its children are written, deepest first.
func writeChangesetTree(t *testing.T, root string, entries []changesetTreeEntry) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var dirs []changesetTreeEntry
	for _, e := range entries {
		p := filepath.Join(root, filepath.FromSlash(e.path))
		if e.path[len(e.path)-1] == '/' {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			dirs = append(dirs, e)
			continue
		}
		if err := os.WriteFile(p, []byte(e.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, e.ts, e.ts); err != nil {
			t.Fatal(err)
		}
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		p := filepath.Join(root, filepath.FromSlash(dirs[i].path))
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, dirs[i].ts, dirs[i].ts); err != nil {
			t.Fatal(err)
		}
	}
}

// TestChangesetVectorStates pins base.c4m and v2.c4m of
// c4m/testdata/changeset-vector as this line's scanner writes them (`c4 id`
// on each state). On the 1.0.x line the library derives a directory's C4
// ID only by scanning; master builds the same files through the API.
// `C4M_UPDATE_VECTORS=1 go test ./scan -run TestChangesetVectorStates`
// rewrites them.
func TestChangesetVectorStates(t *testing.T) {
	// Windows reports different permission bits, so the files differ there
	// for reasons unrelated to the changeset.
	if runtime.GOOS == "windows" {
		return
	}
	t1 := time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)
	t2 := time.Date(2026, 1, 3, 10, 5, 6, 0, time.UTC)
	states := map[string][]changesetTreeEntry{
		"base.c4m": {
			{"top.txt", "top", t1},
			{"sub/", "", t1},
			{"sub/gone.txt", "gone", t1},
			{"sub/mid.txt", "mid", t1},
			{"sub/deep/", "", t1},
			{"sub/deep/leaf.txt", "leaf", t1},
		},
		"v2.c4m": {
			{"top.txt", "top", t1},
			{"sub/", "", t2},
			{"sub/mid.txt", "mid", t1},
			{"sub/new.txt", "new", t2},
			{"sub/deep/", "", t2},
			{"sub/deep/leaf.txt", "leaf2", t2},
		},
	}
	vec := filepath.Join("..", "c4m", "testdata", "changeset-vector")
	for name, entries := range states {
		root := filepath.Join(t.TempDir(), "tree")
		writeChangesetTree(t, root, entries)
		m, err := NewGenerator().GenerateFromPath(root)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := c4m.NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("C4M_UPDATE_VECTORS") != "" {
			if err := os.WriteFile(filepath.Join(vec, name), b.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(filepath.Join(vec, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b.Bytes(), want) {
			t.Errorf("%s differs from what the scanner writes:\n--- file\n%s--- scanner\n%s", name, want, b.Bytes())
		}
	}
}
