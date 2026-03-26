package store

import (
	"fmt"
	"io"

	"github.com/Avalanche-io/c4"
)

// Source - is an interface that defines a source for data identified by c4 id.
type Source interface {
	Open(id c4.ID) (io.ReadCloser, error)
}

// Sink - is an interface that defines a destination for data identified by c4
// id.
type Sink interface {
	Create(id c4.ID) (io.WriteCloser, error)
}

// Store defines the interface for a content-addressed data store.
type Store interface {
	Source
	Sink

	// Has reports whether the store contains content for the given ID.
	Has(id c4.ID) bool

	// Put reads all content, computes its C4 ID, stores it, and returns the ID.
	Put(r io.Reader) (c4.ID, error)

	// Remove deletes content by ID. Implementation is optional — return
	// ErrNotImplemented or os.ErrPermission if unsupported.
	Remove(id c4.ID) error
}

// ErrNotImplemented is the error to return for unimplemented interface methods.
var ErrNotImplemented = fmt.Errorf("not implemented")
