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
//
// Create's writer names the object on Close, and the contract is that
// it never names bytes that do not hash to id: because the caller chose
// the ID, the bytes are checked, never trusted. Close of wrong bytes,
// or of a write that stopped part way, returns ErrInvalidID and leaves
// nothing named and nothing behind. The writer implements Aborter, so a
// caller whose source fails gives the write up and names nothing, and
// Checked, so Validating does not hash it a second time. Every Sink in
// this package holds this contract (TestSinkContract runs each through
// it); a Sink from elsewhere that cannot is wrapped in Validating,
// which then holds it on the store's behalf.
type Sink interface {
	Create(id c4.ID) (io.WriteCloser, error)
}

// Aborter is implemented by Create writers that can give up: Abort
// closes the writer, discards what was written, and names nothing. It
// is safe to call more than once, and Close after Abort is a no-op.
type Aborter interface {
	Abort() error
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
