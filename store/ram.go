package store

import (
	"io"
	"os"
	"sync"

	"github.com/Avalanche-io/c4"
)

var _ Store = &RAM{}

// A RAM store is an implementation of the Store interface that stores all data
// in ram. It is safe for concurrent use.
type RAM struct {
	mu   sync.RWMutex
	data map[c4.ID][]byte
}

func NewRAM() *RAM {
	return &RAM{data: make(map[c4.ID][]byte)}
}

type ramfile struct {
	ram      *RAM
	id       c4.ID
	readonly bool
	buf      []byte
}

func (f *ramfile) Read(b []byte) (int, error) {
	if len(f.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(b, f.buf)
	f.buf = f.buf[n:]
	return n, nil
}

func (f *ramfile) Write(b []byte) (int, error) {
	if f.readonly {
		return 0, os.ErrPermission
	}
	f.buf = append(f.buf, b...)
	return len(b), nil
}

func (f *ramfile) Close() error {
	if f.readonly {
		return nil
	}
	f.ram.mu.Lock()
	f.ram.data[f.id] = f.buf
	f.ram.mu.Unlock()
	return nil
}

// Open opens a file named the given c4.ID in read-only mode from ram.
func (s *RAM) Open(id c4.ID) (io.ReadCloser, error) {
	s.mu.RLock()
	data, ok := s.data[id]
	s.mu.RUnlock()
	if !ok {
		return nil, &os.PathError{Op: "open", Path: id.String(), Err: os.ErrNotExist}
	}
	// Copy so reads don't alias the stored slice
	cp := make([]byte, len(data))
	copy(cp, data)
	return &ramfile{s, id, true, cp}, nil
}

// Create creates an io.WriteCloser interface to a ram buffer.
func (s *RAM) Create(id c4.ID) (io.WriteCloser, error) {
	s.mu.RLock()
	_, ok := s.data[id]
	s.mu.RUnlock()
	if ok {
		return nil, &os.PathError{Op: "create", Path: id.String(), Err: os.ErrExist}
	}
	return &ramfile{s, id, false, []byte{}}, nil
}

// Remove removes the c4 id and its associated data from memory.
func (s *RAM) Remove(id c4.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[id]; !ok {
		return &os.PathError{Op: "remove", Path: id.String(), Err: os.ErrNotExist}
	}
	delete(s.data, id)
	return nil
}
