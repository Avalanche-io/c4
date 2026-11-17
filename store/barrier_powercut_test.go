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

// cutRecorder orders the events a power cut depends on: each object name
// created by a rename, and each directory sync. A name survives the cut
// only if every directory from its own up to the store root was synced
// after it appeared.
type cutRecorder struct {
	seq     int
	created map[string]int // object path -> when its name appeared
	synced  map[string]int // directory -> last sync
}

func newCutRecorder(t *testing.T) *cutRecorder {
	t.Helper()
	r := &cutRecorder{created: map[string]int{}, synced: map[string]int{}}
	oldFull, oldEntries := syncDirFull, syncDirEntries
	syncDirFull = func(d string) error {
		err := oldFull(d)
		r.seq++
		r.synced[filepath.Clean(d)] = r.seq
		return err
	}
	syncDirEntries = func(d string) error {
		err := oldEntries(d)
		r.seq++
		r.synced[filepath.Clean(d)] = r.seq
		return err
	}
	t.Cleanup(func() { syncDirFull, syncDirEntries = oldFull, oldEntries })
	return r
}

// renames wraps a store's publishing renames so the recorder sees each
// name appear.
func (r *cutRecorder) renames(s *TreeStore) {
	s.rename = func(oldpath, newpath string) error {
		err := os.Rename(oldpath, newpath)
		if err == nil {
			r.seq++
			r.created[filepath.Clean(newpath)] = r.seq
		}
		return err
	}
}

// cut removes every recorded name not covered by later syncs of its
// whole directory chain, and reports how many were lost.
func (r *cutRecorder) cut(t *testing.T, root string) (lost int) {
	t.Helper()
	for p, at := range r.created {
		for _, d := range dirsWithinRoot(root, filepath.Dir(p)) {
			if r.synced[filepath.Clean(d)] <= at {
				if err := os.Remove(p); err == nil || !os.IsNotExist(err) {
					lost++
				}
				break
			}
		}
	}
	return lost
}

// c4 id -s prints a snapshot's ID once the batch barrier returns. Here a
// batch store Puts new objects, the barrier returns, and then the power
// cut loses every directory entry not synced since it appeared; c4 cat
// of each must still find it. Through 2026-11-16 on this line the
// barrier flushed the device before its renames and synced no directory
// after them, so every name the batch published could be lost
// (master's 12b8fc9, 2026-11-09, never reached 1.0.x).
func TestBatchPublishSurvivesAPowerCut(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%v", split), func(t *testing.T) {
			root := t.TempDir()
			rec := newCutRecorder(t)
			s, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			s.SetSyncMode(SyncBatch)
			if split {
				s.SetSplitThreshold(4)
			}
			rec.renames(s)
			var data [][]byte
			for i := 0; i < 12; i++ {
				b := []byte(fmt.Sprint("published in this batch ", i))
				if _, err := s.Put(bytes.NewReader(b)); err != nil {
					t.Fatal(err)
				}
				data = append(data, b)
			}
			if err := s.Sync(); err != nil {
				t.Fatal(err)
			}
			lost := rec.cut(t, root)

			old, had := os.LookupEnv("C4_STORE")
			os.Setenv("C4_STORE", root)
			defer func() {
				if had {
					os.Setenv("C4_STORE", old)
				} else {
					os.Unsetenv("C4_STORE")
				}
			}()
			cs, err := OpenStore() // c4 cat's resolution
			if err != nil || cs == nil {
				t.Fatalf("OpenStore: %v, %v", cs, err)
			}
			for i, b := range data {
				id := c4.Identify(bytes.NewReader(b))
				f, err := cs.Open(id)
				if err != nil {
					t.Fatalf("c4 cat of object %d after the barrier and a power cut: %v (%d names lost)", i, err, lost)
				}
				got, _ := io.ReadAll(f)
				f.Close()
				if !bytes.Equal(got, b) {
					t.Fatalf("object %d reads back wrong bytes", i)
				}
			}
		})
	}
}

// The same cut after a batch that only skips. Another writer stored X and
// its name never reached the disk (a SyncNone writer, or one that died
// before its barrier). Then c4 id -s's path Puts the same bytes, skips
// because X is there, runs the barrier, and prints the ID. Through
// 2026-11-16 a batch of only skips made the barrier do nothing, so the
// printed ID could name content a power cut had taken (a claim never
// rests on a name that isn't durable; master's #196).
func TestSkippedObjectSurvivesAPowerCut(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%v", split), func(t *testing.T) {
			root := t.TempDir()
			rec := newCutRecorder(t)
			a, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			a.SetSyncMode(SyncNone)
			if split {
				a.SetSplitThreshold(4)
			}
			rec.renames(a)
			for i := 0; split && i < 12; i++ {
				a.Put(bytes.NewReader([]byte(fmt.Sprint("other ", i))))
			}
			data := []byte("bytes another writer stored and never synced")
			id, err := a.Put(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			b, err := NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			b.SetSyncMode(SyncBatch)
			if got, err := b.Put(bytes.NewReader(data)); err != nil || got != id {
				t.Fatalf("Put: %v, %v", got, err)
			}
			if err := b.Sync(); err != nil {
				t.Fatal(err)
			}
			lost := rec.cut(t, root)
			old, had := os.LookupEnv("C4_STORE")
			os.Setenv("C4_STORE", root)
			defer func() {
				if had {
					os.Setenv("C4_STORE", old)
				} else {
					os.Unsetenv("C4_STORE")
				}
			}()
			cs, err := OpenStore()
			if err != nil || cs == nil {
				t.Fatalf("OpenStore: %v, %v", cs, err)
			}
			f, err := cs.Open(id)
			if err != nil {
				t.Fatalf("c4 cat of the printed ID after a power cut: %v (%d names lost)", err, lost)
			}
			got, _ := io.ReadAll(f)
			f.Close()
			if !bytes.Equal(got, data) {
				t.Fatal("c4 cat read the wrong bytes")
			}
		})
	}
}
