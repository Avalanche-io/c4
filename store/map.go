package store

import (
	"io"
	"os"

	"github.com/Avalanche-io/c4"
)

var _ Store = &MAP{}

// A MAP store is an implementation of the Store interface that stores all data
// in ram.
type MAP map[c4.ID]string

func NewMap(m map[c4.ID]string) MAP {
	return MAP(m)
}

// Open opens a file named the given c4.ID in read-only mode from ram. If
// the file does not exist an error of type `*os.PathError` is returned.
func (s MAP) Open(id c4.ID) (io.ReadCloser, error) {
	return os.Open(s[id])
}

// Create creates an io.WriteCloser interface to a ram buffer, if the data for
// `id` already exists in the MAP store then an error of type `*os.PathError` is
// returned.
func (s MAP) Create(id c4.ID) (io.WriteCloser, error) {
	return os.Create(s[id])
}

// Remove removes the c4 id and it's assoceated data from memory, an error is
// returned if the id does not exist.
func (s MAP) Remove(id c4.ID) error {
	return os.Remove(s[id])
}

func (m MAP) Delete(id c4.ID) {
	delete(m, id)
}

func (m MAP) Load(id c4.ID) (path string) {
	return m[id]
}

func (m MAP) LoadOrStore(id c4.ID, path string) (actual string, loaded bool) {
	var ok bool
	actual, ok = m[id]
	if !ok {
		m[id] = path
		return path, false
	}
	return actual, true
}

func (m MAP) Range(f func(id c4.ID, path string) bool) {
	for id, path := range m {
		if !f(id, path) {
			return
		}
	}
}
