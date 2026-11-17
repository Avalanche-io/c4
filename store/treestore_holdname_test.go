package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
)

// syncedDirs records every directory the two directory syncs touch.
func syncedDirs(t *testing.T) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	oldFull, oldEntries := syncDirFull, syncDirEntries
	syncDirFull = func(d string) error { got[filepath.Clean(d)] = true; return oldFull(d) }
	syncDirEntries = func(d string) error { got[filepath.Clean(d)] = true; return oldEntries(d) }
	t.Cleanup(func() { syncDirFull, syncDirEntries = oldFull, oldEntries })
	return got
}

// Every way a writer comes to rely on an object's name, by publishing it
// or by skipping because it is there, leaves that name's directory and
// its ancestors synced by the time the writer's claim can be made: when
// Put or Create's Close returns under SyncEach, when Sync returns in a
// batch (GC-DESIGN-R4 S13; through 2026-11-16 no skip synced anything).
func TestTreeStoreHoldsEveryNameItRestsOn(t *testing.T) {
	data := []byte("an object some writer rests on")
	id := c4.Identify(bytes.NewReader(data))
	// other stores the object first, in a split store, and syncs nothing.
	setup := func(t *testing.T, stored bool) (root string) {
		root = t.TempDir()
		other := splitTestStore(t, root, 4)
		for i := 0; i < 12; i++ {
			other.Put(bytes.NewReader([]byte(fmt.Sprint("filler ", i))))
		}
		// The object's leaf two levels down, as a split leaves it, so its
		// chain has a directory between the leaf and the root.
		str := id.String()
		if err := os.MkdirAll(filepath.Join(root, str[0:2], str[2:4]), 0o755); err != nil {
			t.Fatal(err)
		}
		if stored {
			if _, err := other.Put(bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}
	held := func(t *testing.T, root string, got map[string]bool, how string) {
		t.Helper()
		s := splitTestStore(t, root, 4)
		p, ok := s.ContentPath(id)
		if !ok {
			t.Fatalf("%s: the object is not stored", how)
		}
		for _, d := range dirsWithinRoot(root, filepath.Dir(p)) {
			if depth := len(dirsWithinRoot(root, filepath.Dir(p))); depth < 3 {
				t.Fatalf("%s: fixture: the object is only %d levels down", how, depth)
			}
			if !got[filepath.Clean(d)] {
				t.Fatalf("%s: %s was not synced before the claim could be made (synced %d dirs)", how, relTo(root, d), len(got))
			}
		}
	}
	put := func(s *TreeStore) error { _, err := s.Put(bytes.NewReader(data)); return err }
	create := func(s *TreeStore) error {
		w, err := s.Create(id)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		return w.Close()
	}
	cases := []struct {
		name   string
		stored bool                            // another writer has stored it first
		mode   SyncMode                        // this writer's mode
		act    func(*TreeStore) error          // what this writer does
		race   func(t *testing.T, root string) // another writer publishes it mid-act
		refuse bool                            // the rename refuses to replace (Windows)
	}{
		{name: "SyncEach Put publishes", mode: SyncEach, act: put},
		{name: "SyncEach Put skips", stored: true, mode: SyncEach, act: put},
		{name: "SyncEach Put meets another's publish", mode: SyncEach, act: put, refuse: true},
		{name: "SyncEach Create publishes", mode: SyncEach, act: create},
		{name: "batch Put skips", stored: true, mode: SyncBatch, act: put},
		{name: "batch Sync skips", mode: SyncBatch, act: put, race: func(t *testing.T, root string) {
			o := splitTestStore(t, root, 4)
			if _, err := o.Put(bytes.NewReader(data)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "batch Sync meets another's publish", mode: SyncBatch, act: put, refuse: true},
		{name: "batch Create skips at Close", mode: SyncBatch, act: func(s *TreeStore) error {
			w, err := s.Create(id)
			if err != nil {
				return err
			}
			w.Write(data)
			o := splitTestStore(t, s.root, 4)
			if _, err := o.Put(bytes.NewReader(data)); err != nil {
				return err
			}
			return w.Close()
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := setup(t, c.stored)
			got := syncedDirs(t)
			s := splitTestStore(t, root, 4)
			s.SetSyncMode(c.mode)
			if c.refuse {
				s.rename = func(oldpath, newpath string) error {
					o := splitTestStore(t, root, 4) // SyncNone: syncs nothing
					if _, err := o.Put(bytes.NewReader(data)); err != nil {
						t.Fatal(err)
					}
					return refuseReplace(oldpath, newpath)
				}
			}
			if err := c.act(s); err != nil {
				t.Fatal(err)
			}
			if c.race != nil {
				c.race(t, root)
			}
			if c.mode == SyncBatch {
				for k := range got {
					delete(got, k) // only what the barrier syncs counts
				}
				if err := s.Sync(); err != nil {
					t.Fatal(err)
				}
			}
			held(t, root, got, c.name)
			f, err := s.Open(id)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(f)
			f.Close()
			if !bytes.Equal(b, data) {
				t.Fatal("wrong bytes")
			}
		})
	}
}
