package main

import (
	"errors"
	"io"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// failingSyncer is a store whose batch barrier fails.
type failingSyncer struct{ store.Store }

func (failingSyncer) Sync() error { return errors.New("device flush failed") }

type plainStore struct{}

func (plainStore) Has(c4.ID) bool                       { return false }
func (plainStore) Open(c4.ID) (io.ReadCloser, error)    { return nil, errors.New("none") }
func (plainStore) Create(c4.ID) (io.WriteCloser, error) { return nil, errors.New("none") }
func (plainStore) Put(io.Reader) (c4.ID, error)         { return c4.ID{}, errors.New("none") }
func (plainStore) Remove(c4.ID) error                   { return nil }
func (plainStore) Walk(func(c4.ID, int64) error) error  { return nil }
func (plainStore) ContentPath(c4.ID) (string, bool)     { return "", false }

// The barrier's error reaches the caller, which is fatal before any claim
// is journaled or printed (c4d review, 2026-11-09: it used to warn and
// claim anyway). A nil store and a store without batch semantics pass.
func TestStoreBarrierReportsFailure(t *testing.T) {
	if err := storeBarrier(nil); err != nil {
		t.Fatalf("nil store: %v", err)
	}
	if err := storeBarrier(plainStore{}); err != nil {
		t.Fatalf("store without Syncer: %v", err)
	}
	err := storeBarrier(failingSyncer{plainStore{}})
	if err == nil || err.Error() != "device flush failed" {
		t.Fatalf("a failed barrier must be reported, got %v", err)
	}
}
