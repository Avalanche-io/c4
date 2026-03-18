package store

import (
	"fmt"
	"io"

	"github.com/Avalanche-io/c4"
)

// MultiStore combines multiple stores into one. Writes go to the first
// store. Reads check all stores in order, returning the first hit.
// Has returns true if any store has the content.
type MultiStore struct {
	stores []Store
}

// NewMultiStore creates a store that writes to the first store and reads
// from all stores in order.
func NewMultiStore(stores ...Store) *MultiStore {
	return &MultiStore{stores: stores}
}

func (m *MultiStore) Has(id c4.ID) bool {
	for _, s := range m.stores {
		if s.Has(id) {
			return true
		}
	}
	return false
}

func (m *MultiStore) Open(id c4.ID) (io.ReadCloser, error) {
	var lastErr error
	for _, s := range m.stores {
		if s.Has(id) {
			rc, err := s.Open(id)
			if err == nil {
				return rc, nil
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("c4 id %s not found in any store", id)
}

func (m *MultiStore) Create(id c4.ID) (io.WriteCloser, error) {
	if len(m.stores) == 0 {
		return nil, ErrNotImplemented
	}
	return m.stores[0].Create(id)
}

func (m *MultiStore) Put(r io.Reader) (c4.ID, error) {
	if len(m.stores) == 0 {
		return c4.ID{}, ErrNotImplemented
	}
	return m.stores[0].Put(r)
}

func (m *MultiStore) Remove(id c4.ID) error {
	// Remove from all stores that have it.
	var lastErr error
	for _, s := range m.stores {
		if s.Has(id) {
			if err := s.Remove(id); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}
