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

// Store - Defines an interface to a data sources that can Open readonly or
// Create write only data accessed via c4 id. The interface includes a Remove
// fucntion, but implementation is optional. Implementations should return an
// appropreate error if Remove is diregarded, such as ErrNotImplemented,
// os.ErrPermission, etc.
type Store interface {
	Source
	Sink

	Remove(id c4.ID) error
}

// ErrNotImplemented is the error to return for unimplemented interface mathods.
var ErrNotImplemented = fmt.Errorf("not implemented")
