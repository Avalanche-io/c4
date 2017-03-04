package os

import (
	"os"

	c4 "github.com/Avalanche-io/c4/id"
)

// Attributes is an interface to an abstract key/value type data store in which
// keys are represented as a slice of bytes, and values are go data of any type.
type Attributes interface {
	Set(key []byte, v interface{}) error
	Get(key []byte, v interface{}) error
	// ID returns the ID of the asset object.
	ID() *c4.ID
	Delete(key []byte) (interface{}, bool)
	Stat() (os.FileInfo, error)
	ForEach(prefix string, f func(key []byte, v interface{}) error) error
}

type AttributeReader interface {
	Attributes() Attributes
}

type ReadOnlyAttributes interface {
	Get(key []byte, v interface{}) error
	ID() *c4.ID
	Stat() (os.FileInfo, error)
	ForEach(prefix string, f func(key []byte, v interface{}) error) error
}
