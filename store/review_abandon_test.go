package store_test

// Two cases from the storage project's review, 2026-11-09, kept as they
// arrived: both ran red against master 5f313db (a partial write and the
// wrong bytes were named under the chosen ID) and state behaviour only.

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// A writer from Create whose source fails part way has no way to give up:
// its only method besides Write is Close, and Close gives the bytes written
// so far the object's name. A caller must choose between publishing a
// partial object under a C4 ID and leaking the writer. (The operator's tree
// shard chose Close, and so names a partial file when a copy fails.)
//
// The test takes any of the usual names for giving up if the writer has
// one, and otherwise does what a caller can do today.
func TestReviewAWriteThatStopsPartWayNamesNothing(t *testing.T) {
	for _, mode := range []struct {
		name string
		mode store.SyncMode
	}{{"sync each", store.SyncEach}, {"batch", store.SyncBatch}} {
		t.Run(mode.name, func(t *testing.T) {
			ts, err := store.NewTreeStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			ts.SetSyncMode(mode.mode)
			whole := bytes.Repeat([]byte("the whole object "), 1000)
			id := c4.Identify(bytes.NewReader(whole))
			w, err := ts.Create(id)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write(whole[:100]); err != nil {
				t.Fatal(err)
			}
			// The source failed here. Give the write up.
			switch a := w.(type) {
			case interface{ Abort() error }:
				a.Abort()
			case interface{ Discard() error }:
				a.Discard()
			case interface{ Cancel() error }:
				a.Cancel()
			default:
				w.Close()
			}
			ts.Sync() // the batch barrier, which publishes what is pending
			if !ts.Has(id) {
				return
			}
			rc, err := ts.Open(id)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(rc)
			rc.Close()
			t.Fatalf("a write that stopped at 100 of %d bytes left %d bytes in the store under the object's C4 ID", len(whole), len(got))
		})
	}
}

// noRemove is a store whose Remove fails, as a read-only or append-only
// backend's does (the Store interface allows it: "return ErrNotImplemented
// or os.ErrPermission if unsupported").
type noRemove struct{ store.Store }

func (noRemove) Remove(c4.ID) error { return store.ErrNotImplemented }

// The Validating store checks a written object against its C4 ID only
// after the store under it has closed the writer, which is when that store
// gives the bytes their name. It then removes the object. So bytes that do
// not match are in the store under the C4 ID from the close until the
// remove, and for good if the remove fails or the process stops between
// the two: the name comes before the check.
func TestReviewValidatingNamesBeforeItChecks(t *testing.T) {
	ts, err := store.NewTreeStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v := store.NewValidating(noRemove{ts})
	right := []byte("the bytes the C4 ID names")
	id := c4.Identify(bytes.NewReader(right))
	w, err := v.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("other bytes entirely")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); !errors.Is(err, store.ErrInvalidID) {
		t.Fatalf("closing a writer of the wrong bytes returned %v, want ErrInvalidID", err)
	}
	if ts.Has(id) {
		rc, _ := ts.Open(id)
		got, _ := io.ReadAll(rc)
		rc.Close()
		t.Fatalf("the write was refused as not matching its C4 ID, and the store holds %q under that ID", got)
	}
}
