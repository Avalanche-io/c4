package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// TestBatchPutPublishesOnlyAfterBarrier pins the barrier-before-rename
// ordering: in SyncBatch mode an object must NOT appear at its hash
// name before Sync (a rename visible after a power cut must always
// imply durable bytes), while Has/Open/ContentPath still serve it from
// the pending set. After Sync it must be at its hash name.
func TestBatchPutPublishesOnlyAfterBarrier(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSyncMode(SyncBatch)

	content := []byte("barrier ordering test content\n")
	id, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	// Not yet at its hash name.
	if _, err := os.Stat(s.path(id)); err == nil {
		t.Fatal("object visible at hash name before the barrier")
	}
	// But fully served from the pending set.
	if !s.Has(id) {
		t.Fatal("Has must see pending objects")
	}
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got := c4.Identify(rc)
	rc.Close()
	if got != id {
		t.Fatalf("pending Open returned wrong bytes: %s != %s", got, id)
	}
	if p, ok := s.ContentPath(id); !ok || p == "" {
		t.Fatal("ContentPath must serve pending objects")
	}

	// Same-content Put within the batch dedupes.
	id2, err := s.Put(bytes.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("dedupe: %s != %s", id2, id)
	}

	// After the barrier the object is at its hash name and no temp
	// files remain.
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(id)); err != nil {
		t.Fatalf("object missing at hash name after Sync: %v", err)
	}
	assertNoIngestTemps(t, dir)

	// Idempotent re-Sync.
	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
}

// TestBatchCreatePublishesOnlyAfterBarrier pins the same contract for
// the Create path.
func TestBatchCreatePublishesOnlyAfterBarrier(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSyncMode(SyncBatch)

	content := []byte("create-path barrier test\n")
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

	if _, err := os.Stat(s.path(id)); err == nil {
		t.Fatal("Create object visible at hash name before the barrier")
	}
	if !s.Has(id) {
		t.Fatal("Has must see pending Create objects")
	}

	// Second Create for the same ID must refuse while pending.
	if _, err := s.Create(id); err == nil {
		t.Fatal("Create must refuse an ID that is already pending")
	}

	if err := s.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(id)); err != nil {
		t.Fatalf("object missing at hash name after Sync: %v", err)
	}
	rc, err := s.Open(id)
	if err != nil {
		t.Fatal(err)
	}
	got := c4.Identify(rc)
	rc.Close()
	if got != id {
		t.Fatalf("wrong bytes after publish: %s != %s", got, id)
	}
	assertNoIngestTemps(t, dir)
}

// TestSyncEachStillPublishesImmediately pins that per-object durable
// mode keeps its immediate-publication behavior (each object is
// durable before its rename, so immediate visibility is sound).
func TestSyncEachStillPublishesImmediately(t *testing.T) {
	dir := t.TempDir()
	s, err := NewTreeStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Default mode is SyncEach.
	id, err := s.Put(bytes.NewReader([]byte("each mode content\n")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path(id)); err != nil {
		t.Fatalf("SyncEach object must be at its hash name immediately: %v", err)
	}
}

func assertNoIngestTemps(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(filepath.Base(path), ".ingest.") {
			t.Fatalf("stale ingest temp after Sync: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
