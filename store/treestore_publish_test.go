package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

// refuseReplace renames as Windows can: it refuses to replace a name that
// exists. MoveFileEx does this ("Access is denied") while another writer's
// rename or a reader holds the destination; os.Rename on POSIX never does.
func refuseReplace(oldpath, newpath string) error {
	if _, err := os.Lstat(newpath); err == nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("access is denied (simulated)")}
	}
	return os.Rename(oldpath, newpath)
}

// storeFiles lists the regular files under dir: temp files and objects.
func storeFiles(t *testing.T, dir string) (temps []string, copies map[c4.ID]int) {
	t.Helper()
	copies = map[c4.ID]int{}
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if isTemp(d.Name()) {
			temps = append(temps, publishRelTo(dir, p))
		} else if id, perr := c4.Parse(d.Name()); perr == nil {
			copies[id]++
		}
		return nil
	})
	return temps, copies
}

// Two writers publish the same bytes: the second one's look misses, the
// first publishes, then the second's rename meets the name. On Windows that
// rename can fail; through 2026-11-16 Put, Sync and Create's writer
// returned the failure for an object that was stored (measured on
// release/1.0.27 before the change: 3% of concurrent same-ID Puts across
// eight TreeStore values on every Windows leg, c4 #181; master #177).
// The store is content-addressed, so the name it met is the same bytes:
// each publish is a success, leaves no temp file, and the object reads
// back. A refused rename with the
// object still absent is a failure, as before.
func TestTreeStoreSameIDPublishers(t *testing.T) {
	data := []byte("published by two writers at once")
	want := c4.Identify(bytes.NewReader(data))
	for _, how := range []string{"Put", "Sync", "Create"} {
		t.Run(how, func(t *testing.T) {
			dir := t.TempDir()
			a := publishTestStore(t, dir, 1<<20)
			b := publishTestStore(t, dir, 1<<20)
			// a publishes between b's look and b's rename.
			b.rename = func(oldpath, newpath string) error {
				if _, err := a.Put(bytes.NewReader(data)); err != nil {
					t.Fatal(err)
				}
				return refuseReplace(oldpath, newpath)
			}
			var err error
			switch how {
			case "Put":
				var id c4.ID
				id, err = b.Put(bytes.NewReader(data))
				if err == nil && id != want {
					t.Fatalf("Put returned %s, want %s", id, want)
				}
			case "Sync":
				b.SetSyncMode(SyncBatch)
				if _, err := b.Put(bytes.NewReader(data)); err != nil {
					t.Fatal(err)
				}
				err = b.Sync()
				if len(b.pending) != 0 {
					t.Fatalf("Sync left %d pending", len(b.pending))
				}
			case "Create":
				w, cerr := b.Create(want)
				if cerr != nil {
					t.Fatal(cerr)
				}
				if _, werr := w.Write(data); werr != nil {
					t.Fatal(werr)
				}
				err = w.Close()
			}
			if err != nil {
				t.Fatalf("%s with the object published by another writer meanwhile: %v", how, err)
			}
			temps, copies := storeFiles(t, dir)
			if len(temps) > 0 {
				t.Fatalf("%s left temp files: %v", how, temps)
			}
			if copies[want] != 1 {
				t.Fatalf("%d copies of the object, want 1", copies[want])
			}
			f, err := b.Open(want)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(f)
			f.Close()
			if !bytes.Equal(got, data) {
				t.Fatal("object reads back wrong bytes")
			}
		})
	}

	// The control: a rename that fails with nothing at the name is a
	// failure, and the temp file goes.
	for _, how := range []string{"Put", "Sync", "Create"} {
		t.Run(how+"Absent", func(t *testing.T) {
			dir := t.TempDir()
			b := publishTestStore(t, dir, 1<<20)
			b.rename = func(oldpath, newpath string) error {
				return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errors.New("refused (simulated)")}
			}
			var err error
			switch how {
			case "Put":
				_, err = b.Put(bytes.NewReader(data))
			case "Sync":
				b.SetSyncMode(SyncBatch)
				if _, perr := b.Put(bytes.NewReader(data)); perr != nil {
					t.Fatal(perr)
				}
				err = b.Sync()
				os.Remove(b.pending[want]) // what a failed Sync leaves pending is the caller's to retry
			case "Create":
				w, cerr := b.Create(want)
				if cerr != nil {
					t.Fatal(cerr)
				}
				w.Write(data)
				err = w.Close()
			}
			if err == nil {
				t.Fatalf("%s reported success for an object that is not stored", how)
			}
			if b.Has(want) && how != "Sync" {
				t.Fatalf("%s: Has reports an object that was never published", how)
			}
			if temps, _ := storeFiles(t, dir); len(temps) > 0 && how != "Sync" {
				t.Fatalf("%s left temp files: %v", how, temps)
			}
		})
	}
}

// The same with real concurrency and no seam: eight TreeStore values Put
// the same bytes at once, with no split. Every Put succeeds and every ID
// reads back. On POSIX this passed before the change too; on Windows
// release/1.0.27 failed it (c4 #181), as master did (#177).
func TestTreeStoreConcurrentSameIDPuts(t *testing.T) {
	const rounds, writers = 20, 8
	shared := make([][]byte, 12)
	for i := range shared {
		shared[i] = []byte(fmt.Sprintf("same-ID publishers %d", i))
	}
	var fails int64
	var first atomic.Value
	for r := 0; r < rounds; r++ {
		dir := t.TempDir()
		var wg sync.WaitGroup
		for k := 0; k < writers; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				s, err := NewTreeStore(dir)
				if err != nil {
					t.Error(err)
					return
				}
				s.SetSplitThreshold(1 << 20)
				s.SetSyncMode(SyncNone)
				for i := range shared {
					if _, err := s.Put(bytes.NewReader(shared[(i+k)%len(shared)])); err != nil {
						if atomic.AddInt64(&fails, 1) == 1 {
							first.Store(err.Error())
						}
					}
				}
			}(k)
		}
		wg.Wait()
		s := publishTestStore(t, dir, 1<<20)
		for _, b := range shared {
			if !s.Has(c4.Identify(bytes.NewReader(b))) {
				t.Fatalf("round %d: an ID is missing", r)
			}
		}
		if temps, _ := storeFiles(t, dir); len(temps) > 0 {
			t.Fatalf("round %d: temp files left: %v", r, temps)
		}
	}
	if fails > 0 {
		t.Fatalf("%d of %d same-ID Puts failed; first: %v", fails, rounds*writers*len(shared), first.Load())
	}
}

func publishTestStore(t *testing.T, dir string, threshold int) *TreeStore {
	t.Helper()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(threshold)
	s.SetSyncMode(SyncNone)
	return s
}

func publishRelTo(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}
