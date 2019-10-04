package store

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"

	"github.com/Avalanche-io/c4"
)

var _ Store = &Validating{}

// The Validating store wrapps another c4 store and validates all c4 ids with
// the data that is read or written. If the data does not match the id, then
// ErrInvalidC4ID will be returned.
// C4 id validity is checked when `Close()` is called on the reader, or writter,
// or when an io.EOF is encountered while reading or writting.
type Validating struct {
	s Store
}

func NewValidating(s Store) *Validating {
	return &Validating{s}
}

var ErrInvalidID = fmt.Errorf("c4 id does not match data")

type validatingReader struct {
	h  hash.Hash
	id c4.ID
	r  io.ReadCloser
}

func (v *validatingReader) Read(b []byte) (int, error) {
	n, err := v.r.Read(b)
	v.h.Write(b[:n])
	if err == nil {
		return n, nil
	}
	if err == io.EOF {
		if !v.isValid() {
			return n, ErrInvalidID
		}
	}
	return n, err
}

func (v *validatingReader) isValid() bool {
	if bytes.Compare(v.id[:], v.h.Sum(nil)) == 0 {
		return true
	}
	return false
}

func (v *validatingReader) Close() error {
	err := v.r.Close()
	if !v.isValid() {
		return ErrInvalidID
	}
	return err
}

type validatingWriter struct {
	h      hash.Hash
	id     c4.ID
	w      io.WriteCloser
	remove func(id c4.ID) error
}

func (v *validatingWriter) Write(b []byte) (int, error) {
	n, err := v.w.Write(b)
	v.h.Write(b[:n])
	if err == nil {
		return n, nil
	}
	if err == io.EOF {
		if !v.isValid() {
			v.w.Close()
			v.remove(v.id)
			return n, ErrInvalidID
		}
	}
	return n, err
}

func (v *validatingWriter) isValid() bool {
	if bytes.Compare(v.id[:], v.h.Sum(nil)) == 0 {
		return true
	}
	return false
}

func (v *validatingWriter) Close() error {
	err := v.w.Close()
	if !v.isValid() {
		v.remove(v.id)
		return ErrInvalidID
	}
	return err
}

// Open opens a file named the given c4.ID in read-only mode from the folder. If
// the file does not exist an error is returned.
func (v *Validating) Open(id c4.ID) (io.ReadCloser, error) {
	r, err := v.s.Open(id)
	if err != nil {
		return nil, err
	}
	return &validatingReader{sha512.New(), id, r}, nil
}

// Create creates and opens for writting a file with the given c4 id as it's
// name if the file does not already exist. If it cannot open the file or the
// file already exists is returns an error.
func (v *Validating) Create(id c4.ID) (io.WriteCloser, error) {
	w, err := v.s.Create(id)
	if err != nil {
		return nil, err
	}
	return &validatingWriter{sha512.New(), id, w, v.s.Remove}, nil
}

func (v *Validating) Remove(id c4.ID) error {
	return v.s.Remove(id)
}
