package os

import (
	"os"

	c4 "github.com/Avalanche-io/c4/id"
)

type AttributeReader interface {
	Get(key []byte, v interface{}) error
	ID() *c4.ID
	Stat() (os.FileInfo, error)
	ForEach(prefix string, f func(key []byte, v interface{}) error) error
}

type Attributes interface {
	Set(key []byte, v interface{}) error
	Get(key []byte, v interface{}) error
	ID() *c4.ID
	Delete(key []byte) (interface{}, bool)
	Stat() (os.FileInfo, error)
	ForEach(prefix string, f func(key []byte, v interface{}) error) error
}
