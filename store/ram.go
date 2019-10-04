package store

import (
	"io"
	"os"

	"github.com/Avalanche-io/c4"
)

var _ Store = &RAM{}

// A RAM store is an implementation of the Store interface that stores all data
// in ram.
type RAM map[c4.ID][]byte

func NewRAM() *RAM {
	r := RAM(make(map[c4.ID][]byte))
	return &r
}

type ramfile struct {
	ram      *RAM
	id       c4.ID
	readonly bool
	data     []byte
}

func (f *ramfile) Read(b []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, io.EOF
	}

	n := copy(b, f.data)
	f.data = f.data[n:]
	return n, nil
}

func (f *ramfile) Write(b []byte) (int, error) {
	if f.readonly {
		return 0, os.ErrPermission
	}
	f.data = append(f.data, b...)
	return len(b), nil
}

func (f *ramfile) Close() error {
	if f.readonly {
		return nil
	}
	(*f.ram)[f.id] = f.data
	return nil
}

// Open opens a file named the given c4.ID in read-only mode from ram. If
// the file does not exist an error of type `*os.PathError` is returned.
func (s *RAM) Open(id c4.ID) (io.ReadCloser, error) {
	data, ok := (*s)[id]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: id.String(), Err: os.ErrNotExist}
	}
	return &ramfile{s, id, true, data}, nil
}

// Create creates an io.WriteCloser interface to a ram buffer, if the data for
// `id` already exists in the RAM store then an error of type `*os.PathError` is
// returned.
func (s *RAM) Create(id c4.ID) (io.WriteCloser, error) {
	_, ok := (*s)[id]
	if ok {
		return nil, &os.PathError{Op: "create", Path: id.String(), Err: os.ErrExist}
	}

	return &ramfile{s, id, false, []byte{}}, nil
}

// Remove removes the c4 id and it's assoceated data from memory, an error is
// returned if the id does not exist.
func (s *RAM) Remove(id c4.ID) error {
	_, ok := (*s)[id]
	if !ok {
		return &os.PathError{Op: "remove", Path: id.String(), Err: os.ErrNotExist}
	}
	delete((*s), id)
	return nil
}
