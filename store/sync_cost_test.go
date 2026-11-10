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
}
