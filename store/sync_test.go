package store

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Avalanche-io/c4"
)

// TestTreeStoreSyncBatchComplete verifies the batch-barrier path: a
// batch of Puts (large enough to force leaf splits) followed by one
// Sync produces a complete, validating store.
func TestTreeStoreSyncBatchComplete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(8)
	s.SetSyncMode(SyncBatch)

	const n = 100
	contents := make(map[c4.ID][]byte, n)
	for i := 0; i < n; i++ {
		content := []byte(fmt.Sprintf("batch object %d", i))
		id, err := s.Put(bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		expected := c4.Identify(bytes.NewReader(content))
		if id != expected {
			t.Fatalf("ID mismatch: got %s, want %s", id, expected)
		}
		contents[id] = content
	}

	if err := s.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// Second Sync with nothing new written is a no-op.
	if err := s.Sync(); err != nil {
		t.Fatalf("second Sync: %v", err)
	}

	// Every object must be present and round-trip its content.
	for id, want := range contents {
		if !s.Has(id) {
			t.Fatalf("missing object %s", id)
		}
		rc, err := s.Open(id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("content mismatch for %s", id)
		}
	}

	// Walk must enumerate exactly the batch — no extras, no temp files.
	seen := 0
	err = s.Walk(func(id c4.ID, size int64) error {
		if _, ok := contents[id]; !ok {
			return fmt.Errorf("unexpected object %s", id)
		}
		seen++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != n {
		t.Fatalf("Walk saw %d objects, want %d", seen, n)
	}
	assertNoTempFiles(t, dir)
}

// TestTreeStoreSyncNoopModes verifies Sync is a safe no-op under
// SyncEach and SyncNone.
func TestTreeStoreSyncNoopModes(t *testing.T) {
	for _, mode := range []SyncMode{SyncEach, SyncNone} {
		dir := t.TempDir()
		s, err := NewTreeStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		s.SetSyncMode(mode)
		id, err := s.Put(strings.NewReader("noop mode content"))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Sync(); err != nil {
			t.Fatalf("mode %d Sync: %v", mode, err)
		}
		if !s.Has(id) {
			t.Fatalf("mode %d: missing object after Put", mode)
		}
	}
}

// TestTreeStoreConcurrentPut verifies Put is safe for concurrent use,
// including duplicate content and leaf splits under contention.
func TestTreeStoreConcurrentPut(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSplitThreshold(8)
	s.SetSyncMode(SyncBatch)

	const workers = 8
	const perWorker = 40
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				// Half the objects are duplicates across workers.
				content := fmt.Sprintf("concurrent object %d", (w%2)*perWorker+i)
				if _, err := s.Put(strings.NewReader(content)); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}

	// 2*perWorker unique contents; each must be present exactly once.
	count := 0
	if err := s.Walk(func(id c4.ID, size int64) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != 2*perWorker {
		t.Fatalf("store holds %d objects, want %d", count, 2*perWorker)
	}
	for i := 0; i < 2*perWorker; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("concurrent object %d", i)))
		if !s.Has(id) {
			t.Fatalf("missing object %d after concurrent Put", i)
		}
	}
	assertNoTempFiles(t, dir)
}

// TestTreeStoreCreateFollowsSyncMode verifies writers from Create
// honor the store's sync mode and land complete objects.
func TestTreeStoreCreateFollowsSyncMode(t *testing.T) {
	for _, mode := range []SyncMode{SyncEach, SyncBatch, SyncNone} {
		dir := t.TempDir()
		s, err := NewTreeStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		s.SetSyncMode(mode)

		content := []byte("created content")
		id := c4.Identify(bytes.NewReader(content))
		w, err := s.Create(id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := s.Sync(); err != nil {
			t.Fatal(err)
		}
		rc, err := s.Open(id)
		if err != nil {
			t.Fatalf("mode %d: %v", mode, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(got, content) {
			t.Fatalf("mode %d: content mismatch", mode)
		}
	}
}

// TestMultiStoreSyncForwarding verifies MultiStore forwards SetSyncMode
// and Sync to member stores that support them.
func TestMultiStoreSyncForwarding(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := NewMultiStore(ts, NewRAM())
	m.SetSyncMode(SyncBatch)
	if ts.syncMode != SyncBatch {
		t.Fatal("SetSyncMode not forwarded to TreeStore member")
	}
	id, err := m.Put(strings.NewReader("multi store batch content"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !ts.Has(id) {
		t.Fatal("object missing from write store")
	}
}

// assertNoTempFiles fails if any ingest temp files remain under root.
func assertNoTempFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fmt.Errorf("leftover temp file: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
