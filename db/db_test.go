package db

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/hashlib"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func storeTestBlob(t *testing.T, db *DB, data string) c4.ID {
	t.Helper()
	id, err := db.StoreBytes([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// --- Basic CRUD ---

func TestOpenClose(t *testing.T) {
	db := openTestDB(t)
	// Default directories should exist
	err := db.View(func(s *Snapshot) error {
		entries, err := s.List("mnt")
		if err != nil {
			return err
		}
		_ = entries // mnt is empty, that's fine
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPutResolve(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "hello world")

	if err := db.Update(func(tx *Tx) error {
		return tx.Put("mnt/project/main", id)
	}); err != nil {
		t.Fatal(err)
	}

	err := db.View(func(s *Snapshot) error {
		got, err := s.Resolve("mnt/project/main")
		if err != nil {
			return err
		}
		if got != id {
			return fmt.Errorf("got %s, want %s", got, id)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPutOverwrite(t *testing.T) {
	db := openTestDB(t)
	id1 := storeTestBlob(t, db, "v1")
	id2 := storeTestBlob(t, db, "v2")

	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id1) })
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id2) })

	db.View(func(s *Snapshot) error {
		got, _ := s.Resolve("mnt/file")
		if got != id2 {
			t.Fatalf("expected v2, got %s", got)
		}
		return nil
	})
}

func TestDelete(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "data")

	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })

	db.Update(func(tx *Tx) error { return tx.Delete("mnt/file") })

	db.View(func(s *Snapshot) error {
		if _, err := s.Resolve("mnt/file"); err == nil {
			t.Fatal("should be deleted")
		}
		return nil
	})
}

func TestDeleteNotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.Update(func(tx *Tx) error {
		return tx.Delete("mnt/nonexistent")
	})
	if err == nil {
		t.Fatal("expected error deleting nonexistent path")
	}
}

func TestList(t *testing.T) {
	db := openTestDB(t)
	id1 := storeTestBlob(t, db, "a")
	id2 := storeTestBlob(t, db, "b")

	db.Update(func(tx *Tx) error {
		tx.Put("mnt/project/alpha", id1)
		return tx.Put("mnt/project/beta", id2)
	})

	db.View(func(s *Snapshot) error {
		entries, err := s.List("mnt/project")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Name != "alpha" || entries[1].Name != "beta" {
			t.Fatalf("unexpected entries: %v", entries)
		}
		return nil
	})
}

func TestBlobStorage(t *testing.T) {
	db := openTestDB(t)
	data := []byte("test content")
	id, err := db.StoreBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	if !db.Has(id) {
		t.Fatal("blob should exist")
	}

	rc, err := db.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(rc)
	rc.Close()

	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatal("content mismatch")
	}
}

// --- Transaction semantics ---

func TestTransactionRollback(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "data")

	// Transaction that errors should not commit
	err := db.Update(func(tx *Tx) error {
		tx.Put("mnt/file", id)
		return errors.New("abort")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	db.View(func(s *Snapshot) error {
		if _, err := s.Resolve("mnt/file"); err == nil {
			t.Fatal("aborted transaction should not be visible")
		}
		return nil
	})
}

func TestTransactionSeesOwnWrites(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "data")

	db.Update(func(tx *Tx) error {
		tx.Put("mnt/file", id)
		// Should see our own write
		got, err := tx.Resolve("mnt/file")
		if err != nil {
			t.Fatal("tx should see its own write")
		}
		if got != id {
			t.Fatal("wrong ID in tx read")
		}
		return nil
	})
}

func TestMultipleWritesInTransaction(t *testing.T) {
	db := openTestDB(t)

	db.Update(func(tx *Tx) error {
		for i := 0; i < 100; i++ {
			id := testID(fmt.Sprintf("blob-%d", i))
			tx.Put(fmt.Sprintf("mnt/dir/file-%d", i), id)
		}
		return nil
	})

	db.View(func(s *Snapshot) error {
		entries, err := s.List("mnt/dir")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 100 {
			t.Fatalf("expected 100 entries, got %d", len(entries))
		}
		return nil
	})
}

// --- Snapshot isolation ---

func TestSnapshotIsolation(t *testing.T) {
	db := openTestDB(t)
	id1 := storeTestBlob(t, db, "v1")
	id2 := storeTestBlob(t, db, "v2")

	// Write v1
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id1) })

	// Take snapshot
	snap := db.Snapshot()
	defer snap.Release()

	// Write v2
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id2) })

	// Snapshot should still see v1
	got, err := snap.Resolve("mnt/file")
	if err != nil {
		t.Fatal(err)
	}
	if got != id1 {
		t.Fatal("snapshot should see v1, not v2")
	}

	// Current view should see v2
	db.View(func(s *Snapshot) error {
		got, _ := s.Resolve("mnt/file")
		if got != id2 {
			t.Fatal("current view should see v2")
		}
		return nil
	})
}

// --- Persistence ---

func TestPersistAndReopen(t *testing.T) {
	dir := t.TempDir()
	id := testID("persist-test")

	// Write
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	db.StoreBytes([]byte("persist-test"))
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })
	db.Close()

	// Reopen
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.View(func(s *Snapshot) error {
		got, err := s.Resolve("mnt/file")
		if err != nil {
			t.Fatal("entry should survive reopen")
		}
		if got != id {
			t.Fatal("ID should match after reopen")
		}
		return nil
	})
}

func TestPersistMultipleVersions(t *testing.T) {
	dir := t.TempDir()

	// Write multiple versions
	db, _ := Open(dir)
	for i := 0; i < 10; i++ {
		id := testID(fmt.Sprintf("v%d", i))
		db.Update(func(tx *Tx) error {
			return tx.Put("mnt/file", id)
		})
	}
	db.Close()

	// Reopen — should see last version
	db, _ = Open(dir)
	defer db.Close()

	db.View(func(s *Snapshot) error {
		got, _ := s.Resolve("mnt/file")
		if got != testID("v9") {
			t.Fatal("should see last version after reopen")
		}
		return nil
	})
}

// --- GC ---

func TestGCCollectsUnreachable(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "gc-target")

	// Put then delete
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })
	db.Update(func(tx *Tx) error { return tx.Delete("mnt/file") })

	if !db.Has(id) {
		t.Fatal("blob should exist before GC")
	}

	result := db.GC()
	if result.Collected == 0 {
		t.Fatal("GC should have collected something")
	}

	if db.Has(id) {
		t.Fatal("unreachable blob should be gone after GC")
	}
}

func TestGCPreservesLive(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "live-blob")

	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })

	db.GC()

	if !db.Has(id) {
		t.Fatal("live blob should survive GC")
	}
}

func TestGCPreservesSnapshotBlobs(t *testing.T) {
	db := openTestDB(t)
	id1 := storeTestBlob(t, db, "snap-blob")
	id2 := storeTestBlob(t, db, "new-blob")

	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id1) })

	// Hold snapshot referencing id1
	snap := db.Snapshot()
	defer snap.Release()

	// Overwrite with id2 — id1 is unreachable from current root
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id2) })

	db.GC()
	// Verify id2 survives
	if !db.Has(id2) {
		t.Fatal("current blob should survive GC")
	}
}

func TestGCRecoversFromFilterOverflow(t *testing.T) {
	db := openTestDB(t)

	// Build a c4m blob with many entries — more than the initial filter
	// estimate allows.
	m := c4m.NewManifest()
	for i := 0; i < 2000; i++ {
		entryID := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		m.AddEntry(&c4m.Entry{
			Name: fmt.Sprintf("file%d.txt", i),
			C4ID: entryID,
			Mode: 0644,
			Size: 100,
		})
	}
	data, err := c4m.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	// Store the c4m blob and register it as the single leaf.
	leafID, err := db.StoreBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *Tx) error { return tx.Put("mnt/bigmanifest", leafID) })
	if err != nil {
		t.Fatal(err)
	}

	// Store an unreachable blob — GC should collect it after recovery.
	orphanID := storeTestBlob(t, db, "should-be-collected")

	result := db.GC()

	// GC must complete (via retry or map fallback) — never abort.
	if result.MarkRetries > 0 {
		t.Logf("GC recovered via filter retry (attempt %d)", result.MarkRetries)
	}
	if result.UsedFallback {
		t.Log("GC used map fallback")
	}

	// The orphan should be collected.
	if db.Has(orphanID) {
		t.Fatal("orphan blob should have been collected after successful GC")
	}

	// The leaf blob must survive.
	if !db.Has(leafID) {
		t.Fatal("live blob should survive GC")
	}
}

func TestGCMapFallback(t *testing.T) {
	db := openTestDB(t)

	// Force map fallback by setting maxFilterMemory to 1 byte.
	db.maxFilterMemory = 1

	id := storeTestBlob(t, db, "fallback-live")
	orphan := storeTestBlob(t, db, "fallback-orphan")
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })

	result := db.GC()
	if !result.UsedFallback {
		t.Fatal("expected map fallback with 1-byte filter ceiling")
	}

	if !db.Has(id) {
		t.Fatal("live blob should survive map-fallback GC")
	}
	if db.Has(orphan) {
		t.Fatal("orphan should be collected by map-fallback GC")
	}
}

func TestGCFillLiveFilterOverflow(t *testing.T) {
	// Unit test: verify fillLiveFilter returns false on a too-small filter.
	db := openTestDB(t)

	m := c4m.NewManifest()
	for i := 0; i < 100; i++ {
		entryID := c4.Identify(strings.NewReader(fmt.Sprintf("fill-test-%d", i)))
		m.AddEntry(&c4m.Entry{
			Name: fmt.Sprintf("f%d.txt", i),
			C4ID: entryID,
			Mode: 0644,
			Size: 10,
		})
	}
	data, err := c4m.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	leafID, err := db.StoreBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Update(func(tx *Tx) error { return tx.Put("mnt/leaf", leafID) })
	if err != nil {
		t.Fatal(err)
	}

	root := db.root.Load()

	// Tiny filter — guaranteed to overflow.
	tiny := hashlib.NewCuckooFilter(4)
	if db.fillLiveFilter(root, tiny) {
		t.Fatal("fillLiveFilter should return false on undersized filter")
	}

	// Large filter — should succeed.
	big := hashlib.NewCuckooFilter(10000)
	if !db.fillLiveFilter(root, big) {
		t.Fatal("fillLiveFilter should succeed on adequately sized filter")
	}
}

// --- Concurrency ---

func TestConcurrentReaders(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "concurrent")
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })

	var wg sync.WaitGroup
	var errs atomic.Int64
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				db.View(func(s *Snapshot) error {
					got, err := s.Resolve("mnt/file")
					if err != nil || got != id {
						errs.Add(1)
					}
					return nil
				})
			}
		}()
	}
	wg.Wait()
	if e := errs.Load(); e > 0 {
		t.Fatalf("%d read errors", e)
	}
}

func TestConcurrentReadersAndWriter(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "base")
	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id) })

	var wg sync.WaitGroup

	// 16 readers
	var readErrors atomic.Int64
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				db.View(func(s *Snapshot) error {
					_, err := s.Resolve("mnt/file")
					if err != nil {
						readErrors.Add(1)
					}
					return nil
				})
			}
		}()
	}

	// 1 writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			newID := testID(fmt.Sprintf("w%d", j))
			db.Update(func(tx *Tx) error {
				return tx.Put("mnt/file", newID)
			})
		}
	}()

	wg.Wait()
	if e := readErrors.Load(); e > 0 {
		t.Fatalf("%d read errors during concurrent writes", e)
	}
}

func TestConcurrentWriters(t *testing.T) {
	db := openTestDB(t)

	var wg sync.WaitGroup
	var errs atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				id := testID(fmt.Sprintf("w%d-j%d", workerID, j))
				path := fmt.Sprintf("mnt/worker-%d/file-%d", workerID, j)
				err := db.Update(func(tx *Tx) error {
					return tx.Put(path, id)
				})
				if err != nil {
					errs.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	if e := errs.Load(); e > 0 {
		t.Fatalf("%d write errors", e)
	}

	// Verify all writes landed
	db.View(func(s *Snapshot) error {
		for i := 0; i < 8; i++ {
			for j := 0; j < 50; j++ {
				path := fmt.Sprintf("mnt/worker-%d/file-%d", i, j)
				if _, err := s.Resolve(path); err != nil {
					t.Errorf("missing: %s", path)
				}
			}
		}
		return nil
	})
}

// --- Property test: random operations ---

func TestPropertyRandomOps(t *testing.T) {
	db := openTestDB(t)
	rng := rand.New(rand.NewSource(42))

	// Track expected state
	expected := make(map[string]c4.ID)
	paths := []string{
		"mnt/a/1", "mnt/a/2", "mnt/a/3",
		"mnt/b/1", "mnt/b/2",
		"mnt/c/x/1", "mnt/c/x/2",
	}

	for i := 0; i < 500; i++ {
		path := paths[rng.Intn(len(paths))]
		if rng.Intn(3) == 0 && len(expected) > 0 {
			// Delete
			if _, ok := expected[path]; ok {
				db.Update(func(tx *Tx) error { return tx.Delete(path) })
				delete(expected, path)
			}
		} else {
			// Put
			id := testID(fmt.Sprintf("op-%d", i))
			db.Update(func(tx *Tx) error { return tx.Put(path, id) })
			expected[path] = id
		}
	}

	// Verify
	db.View(func(s *Snapshot) error {
		for path, wantID := range expected {
			got, err := s.Resolve(path)
			if err != nil {
				t.Errorf("missing expected path %s", path)
				continue
			}
			if got != wantID {
				t.Errorf("wrong ID at %s", path)
			}
		}
		// Verify deleted paths are gone
		for _, path := range paths {
			if _, ok := expected[path]; ok {
				continue
			}
			if _, err := s.Resolve(path); err == nil {
				t.Errorf("path %s should be deleted", path)
			}
		}
		return nil
	})
}

// --- Invariant: snapshot isolation under stress ---

func TestPropertySnapshotIsolation(t *testing.T) {
	db := openTestDB(t)

	// Write initial state
	for i := 0; i < 10; i++ {
		id := testID(fmt.Sprintf("init-%d", i))
		db.Update(func(tx *Tx) error {
			return tx.Put(fmt.Sprintf("mnt/file-%d", i), id)
		})
	}

	// Take snapshot
	snap := db.Snapshot()
	defer snap.Release()

	// Capture snapshot state
	snapState := make(map[string]c4.ID)
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("mnt/file-%d", i)
		id, _ := snap.Resolve(path)
		snapState[path] = id
	}

	// Hammer the DB with writes
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				id := testID(fmt.Sprintf("w%d-%d", w, j))
				path := fmt.Sprintf("mnt/file-%d", j%10)
				db.Update(func(tx *Tx) error {
					return tx.Put(path, id)
				})
			}
		}(i)
	}
	wg.Wait()

	// Snapshot must still see its original state
	for path, wantID := range snapState {
		got, err := snap.Resolve(path)
		if err != nil {
			t.Errorf("snapshot lost %s", path)
			continue
		}
		if got != wantID {
			t.Errorf("snapshot %s changed: got %s, want %s", path, got, wantID)
		}
	}
}

// --- Holds ---

func TestHoldProtectsFromGC(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "held-blob")

	// Hold without namespace reference — GC should not collect
	release := db.Hold([]c4.ID{id})

	db.GC()
	if !db.Has(id) {
		t.Fatal("held blob should survive GC")
	}

	// Release and GC again — now it should be collected
	release()
	db.GC()
	if db.Has(id) {
		t.Fatal("released blob should be collected by GC")
	}
}

func TestHoldDoubleRelease(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "double")

	release := db.Hold([]c4.ID{id})
	release()
	release() // should not panic

	if db.IsHeld(id) {
		t.Fatal("should not be held after release")
	}
}

func TestHoldRefCounting(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "refcount")

	r1 := db.Hold([]c4.ID{id})
	r2 := db.Hold([]c4.ID{id})

	if !db.IsHeld(id) {
		t.Fatal("should be held")
	}

	r1()
	if !db.IsHeld(id) {
		t.Fatal("should still be held (second hold active)")
	}

	r2()
	if db.IsHeld(id) {
		t.Fatal("should not be held after both released")
	}
}

func TestHoldConcurrentWithGC(t *testing.T) {
	db := openTestDB(t)

	// Store blobs, hold some, run GC concurrently
	var held []c4.ID
	var unheld []c4.ID
	for i := 0; i < 20; i++ {
		id := storeTestBlob(t, db, fmt.Sprintf("blob-%d", i))
		if i%2 == 0 {
			held = append(held, id)
		} else {
			unheld = append(unheld, id)
		}
	}

	release := db.Hold(held)

	var wg sync.WaitGroup
	// Run GC multiple times concurrently
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.GC()
		}()
	}
	wg.Wait()

	// Held blobs survive
	for _, id := range held {
		if !db.Has(id) {
			t.Fatalf("held blob %s should survive GC", id)
		}
	}
	// Unheld blobs collected
	for _, id := range unheld {
		if db.Has(id) {
			t.Fatalf("unheld blob %s should be collected", id)
		}
	}

	release()
}

// --- Watchers ---

func TestWatchNotifiesOnCommit(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "watch-me")

	ch := db.Watch()

	go func() {
		db.Update(func(tx *Tx) error {
			return tx.Put("mnt/file", id)
		})
	}()

	select {
	case <-ch:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("watcher not notified")
	}
}

func TestWatchNotNotifiedOnNoChange(t *testing.T) {
	db := openTestDB(t)

	ch := db.Watch()

	// Update that changes nothing (root == base)
	db.Update(func(tx *Tx) error {
		return nil
	})

	select {
	case <-ch:
		t.Fatal("watcher should not fire on no-op transaction")
	case <-time.After(50 * time.Millisecond):
		// good — no notification
	}
}

func TestWatchMultipleWatchers(t *testing.T) {
	db := openTestDB(t)
	id := storeTestBlob(t, db, "multi")

	const n = 10
	chs := make([]<-chan struct{}, n)
	for i := range chs {
		chs[i] = db.Watch()
	}

	db.Update(func(tx *Tx) error {
		return tx.Put("mnt/file", id)
	})

	for i, ch := range chs {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("watcher %d not notified", i)
		}
	}
}

func TestWatchResubscribe(t *testing.T) {
	db := openTestDB(t)

	// First commit
	ch1 := db.Watch()
	db.Update(func(tx *Tx) error {
		return tx.Put("mnt/a", testID("a"))
	})
	<-ch1

	// Second commit — must re-watch
	ch2 := db.Watch()
	db.Update(func(tx *Tx) error {
		return tx.Put("mnt/b", testID("b"))
	})
	select {
	case <-ch2:
	case <-time.After(2 * time.Second):
		t.Fatal("second watcher not notified")
	}
}

// --- CompareAndPut ---

func TestCompareAndPutSuccess(t *testing.T) {
	db := openTestDB(t)
	id1 := testID("v1")
	id2 := testID("v2")

	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id1) })

	err := db.Update(func(tx *Tx) error {
		return tx.CompareAndPut("mnt/file", id1, id2)
	})
	if err != nil {
		t.Fatalf("CAS should succeed: %v", err)
	}

	db.View(func(s *Snapshot) error {
		got, _ := s.Resolve("mnt/file")
		if got != id2 {
			t.Fatal("should see v2")
		}
		return nil
	})
}

func TestCompareAndPutConflict(t *testing.T) {
	db := openTestDB(t)
	id1 := testID("v1")
	id2 := testID("v2")
	wrong := testID("wrong")

	db.Update(func(tx *Tx) error { return tx.Put("mnt/file", id1) })

	err := db.Update(func(tx *Tx) error {
		return tx.CompareAndPut("mnt/file", wrong, id2)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

	// Original value unchanged
	db.View(func(s *Snapshot) error {
		got, _ := s.Resolve("mnt/file")
		if got != id1 {
			t.Fatal("should still see v1")
		}
		return nil
	})
}

func TestCompareAndPutCreateOnly(t *testing.T) {
	db := openTestDB(t)
	id := testID("new")

	// Create: expected is nil (path doesn't exist)
	err := db.Update(func(tx *Tx) error {
		return tx.CompareAndPut("mnt/file", c4.ID{}, id)
	})
	if err != nil {
		t.Fatalf("create should succeed: %v", err)
	}

	// Try again — should conflict (path already exists)
	err = db.Update(func(tx *Tx) error {
		return tx.CompareAndPut("mnt/file", c4.ID{}, testID("other"))
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict on create-if-not-exists, got %v", err)
	}
}

// --- WalkLeaves ---

func TestSnapshotWalkLeaves(t *testing.T) {
	db := openTestDB(t)

	db.Update(func(tx *Tx) error {
		tx.Put("mnt/a", testID("a"))
		tx.Put("mnt/b", testID("b"))
		tx.Put("mnt/sub/c", testID("c"))
		return nil
	})

	seen := make(map[string]c4.ID)
	db.View(func(s *Snapshot) error {
		s.WalkLeaves(func(path string, id c4.ID) {
			seen[path] = id
		})
		return nil
	})

	if len(seen) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(seen))
	}
	for _, path := range []string{"mnt/a", "mnt/b", "mnt/sub/c"} {
		if _, ok := seen[path]; !ok {
			t.Fatalf("missing %s", path)
		}
	}
}

func TestSnapshotWalkLeavesIsolation(t *testing.T) {
	db := openTestDB(t)
	db.Update(func(tx *Tx) error {
		return tx.Put("mnt/file", testID("v1"))
	})

	snap := db.Snapshot()
	defer snap.Release()

	// Mutate after snapshot
	db.Update(func(tx *Tx) error {
		tx.Put("mnt/file", testID("v2"))
		tx.Put("mnt/new", testID("new"))
		return nil
	})

	seen := make(map[string]c4.ID)
	snap.WalkLeaves(func(path string, id c4.ID) {
		seen[path] = id
	})

	if len(seen) != 1 {
		t.Fatalf("snapshot should have 1 leaf, got %d", len(seen))
	}
	if seen["mnt/file"] != testID("v1") {
		t.Fatal("snapshot should see v1")
	}
}
