package store

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSyncCost times the barrier for a wide batch (many shard
// directories) with the directory syncs on, and with the per-directory
// entry sync stubbed out to approximate the barrier before this change
// (one extra device flush remains). It runs only with C4_SYNC_COST set;
// it is a measurement, not a check. Numbers go in the PR, with the load.
func TestSyncCost(t *testing.T) {
	if os.Getenv("C4_SYNC_COST") == "" {
		t.Skip("set C4_SYNC_COST=1 to measure")
	}
	const objects = 3000
	run := func(label string, entries func(string) error) {
		s, err := NewTreeStore(filepath.Join(t.TempDir(), "store"))
		if err != nil {
			t.Fatal(err)
		}
		s.SetSyncMode(SyncBatch)
		s.SetSplitThreshold(16)
		for i := 0; i < objects; i++ {
			if _, err := s.Put(bytes.NewReader([]byte(fmt.Sprintf("cost object %d\n", i)))); err != nil {
				t.Fatal(err)
			}
		}
		old := syncDirEntries
		syncDirEntries = entries
		defer func() { syncDirEntries = old }()
		dirs := 0
		start := time.Now()
		if err := s.Sync(); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		filepath.Walk(s.root, func(_ string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				dirs++
			}
			return nil
		})
		t.Logf("%s: %d objects, %d directories, Sync %v", label, objects, dirs, elapsed)
	}
	run("entry fsync per touched directory + final flush", fsyncDir)
	run("no entry fsync (final flush only)", func(string) error { return nil })

	// What holding every name a writer rests on costs (a claim never rests
	// on a name that isn't durable):
	// a batch whose objects are all present already (a re-snapshot), and
	// SyncEach Puts that publish or skip.
	fill := func(n int, mode SyncMode) (string, [][]byte) {
		root := filepath.Join(t.TempDir(), "store")
		s, err := NewTreeStore(root)
		if err != nil {
			t.Fatal(err)
		}
		s.SetSyncMode(mode)
		s.SetSplitThreshold(16)
		var data [][]byte
		for i := 0; i < n; i++ {
			b := []byte(fmt.Sprintf("hold object %d\n", i))
			data = append(data, b)
			if _, err := s.Put(bytes.NewReader(b)); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Sync(); err != nil {
			t.Fatal(err)
		}
		return root, data
	}
	reopen := func(root string, mode SyncMode) *TreeStore {
		s, err := NewTreeStore(root)
		if err != nil {
			t.Fatal(err)
		}
		s.SetSyncMode(mode)
		s.SetSplitThreshold(16)
		return s
	}
	root, data := fill(objects, SyncBatch)
	s := reopen(root, SyncBatch)
	start := time.Now()
	for _, b := range data {
		if _, err := s.Put(bytes.NewReader(b)); err != nil {
			t.Fatal(err)
		}
	}
	puts := time.Since(start)
	start = time.Now()
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	t.Logf("batch of %d objects all present: Puts %v, Sync %v", objects, puts, time.Since(start))

	const each = 200
	root, _ = fill(0, SyncEach)
	s = reopen(root, SyncEach)
	start = time.Now()
	for i := 0; i < each; i++ {
		if _, err := s.Put(bytes.NewReader([]byte(fmt.Sprintf("each object %d\n", i)))); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("SyncEach: %d Puts that publish, %v each", each, time.Since(start)/each)
	start = time.Now()
	for i := 0; i < each; i++ {
		if _, err := s.Put(bytes.NewReader([]byte(fmt.Sprintf("each object %d\n", i)))); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("SyncEach: %d Puts that skip, %v each", each, time.Since(start)/each)
}
