package store_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// loseUnsyncedEntries models a power cut: an object file survives only if
// its directory and every ancestor up to the store root were synced after
// its name appeared. synced holds the directories synced since then.
func loseUnsyncedEntries(t *testing.T, root string, synced map[string]bool) (lost int) {
	t.Helper()
	var victims []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if _, perr := c4.Parse(info.Name()); perr != nil {
			return nil // not an object (the journal is checked on its own)
		}
		for _, d := range store.DirsWithinRoot(root, filepath.Dir(p)) {
			if !synced[filepath.Clean(d)] {
				victims = append(victims, p)
				break
			}
		}
		return nil
	})
	for _, p := range victims {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	return len(victims)
}

// A claim must name only content that survives a power cut. Here another
// writer stored an object and its directory entry never reached the disk
// (a SyncNone writer, or one that died before its barrier). Then c4 id -s's
// path runs: a batch store Puts the same bytes, which it skips because the
// object is there, runs the barrier, and the claim is journaled. Through
// 2026-11-16 the barrier of a batch with only skips returned at once and
// synced nothing, so a power cut could leave a journaled claim whose
// content no store can find. A claim never rests on a name that isn't
// durable.
func TestSkippedObjectSurvivesAPowerCut(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprintf("split=%v", split), func(t *testing.T) {
			root := t.TempDir()
			a, err := store.NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			a.SetSyncMode(store.SyncNone)
			if split {
				a.SetSplitThreshold(4)
				for i := 0; i < 12; i++ {
					a.Put(bytes.NewReader([]byte(fmt.Sprint("other ", i))))
				}
			}
			data := []byte("bytes another writer stored and never synced")
			id, err := a.Put(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}

			synced := map[string]bool{}
			restore := store.ObserveDirSyncs(func(d string) { synced[filepath.Clean(d)] = true })
			b, err := store.NewTreeStore(root)
			if err != nil {
				t.Fatal(err)
			}
			b.SetSyncMode(store.SyncBatch)
			if got, err := b.Put(bytes.NewReader(data)); err != nil || got != id {
				t.Fatalf("Put: %v, %v", got, err)
			}
			if err := b.Sync(); err != nil {
				t.Fatal(err)
			}
			if err := c4m.OpenJournal(root).Append(c4m.Claim{ScanStart: time.Now().UTC(), ID: id}); err != nil {
				t.Fatal(err)
			}
			restore()

			lost := loseUnsyncedEntries(t, root, synced)
			claims, err := c4m.OpenJournal(root).Claims()
			if err != nil || len(claims) != 1 || claims[0].ID != id {
				t.Fatalf("journal after the cut: %v, %v", claims, err)
			}
			// c4 cat's resolution: the configured store, then Open.
			old, had := os.LookupEnv("C4_STORE")
			os.Setenv("C4_STORE", root)
			defer func() {
				if had {
					os.Setenv("C4_STORE", old)
				} else {
					os.Unsetenv("C4_STORE")
				}
			}()
			s, err := store.OpenStore()
			if err != nil || s == nil {
				t.Fatalf("OpenStore: %v, %v", s, err)
			}
			f, err := s.Open(id)
			if err != nil {
				t.Fatalf("c4 cat of the journaled claim after a power cut: %v (%d object entries lost, synced: %d dirs)", err, lost, len(synced))
			}
			got, _ := io.ReadAll(f)
			f.Close()
			if !bytes.Equal(got, data) {
				t.Fatal("c4 cat read the wrong bytes")
			}
		})
	}
}
