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
// C4 id validity is checked when `Close()` is called on the reader, or writer,
// or when an io.EOF is encountered while reading or writing.
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
	done   bool
}

func (v *validatingWriter) Write(b []byte) (int, error) {
	n, err := v.w.Write(b)
	v.h.Write(b[:n])
	if err == nil {
		return n, nil
	}
	if err == io.EOF {
		if !v.isValid() {
			v.discard()
			return n, ErrInvalidID
		}
	}
	return n, err
}

// discard gives the inner write up without naming anything where the
// inner writer can (Aborter); otherwise the inner Close names the
// object and the remove that follows is the best available, with the
// window between them that an Aborter closes.
func (v *validatingWriter) discard() {
	if a, ok := v.w.(Aborter); ok {
		a.Abort()
		return
	}
	v.w.Close()
	v.remove(v.id)
}

// Abort gives the write up; nothing is named. Close after it is a no-op.
func (v *validatingWriter) Abort() error {
	if v.done {
		return nil
	}
	v.done = true
	if a, ok := v.w.(Aborter); ok {
		return a.Abort()
	}
	err := v.w.Close()
	v.remove(v.id)
	return err
}

func (v *validatingWriter) ChecksID() bool { return true }

func (v *validatingWriter) isValid() bool {
	if bytes.Compare(v.id[:], v.h.Sum(nil)) == 0 {
		return true
	}
	return false
}

// Close checks the bytes against the ID before the inner writer closes,
// so an inner writer that names on Close never names wrong bytes
// (through 1.0.x the inner Close came first and the remove after it,
// found in the storage project's review, 2026-11-09).
func (v *validatingWriter) Close() error {
	if v.done {
		return nil
	}
	v.done = true
	if !v.isValid() {
		v.discard()
		return ErrInvalidID
	}
	return v.w.Close()
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

// Create opens a writer for the given C4 ID in the wrapped store. A
// writer that checks its own bytes (Checked) is returned as it is: the
// wrapped store already refuses wrong bytes before naming them, and
// hashing twice would buy nothing. Any other writer is wrapped so the
// check happens here, before the inner Close names the object.
func (v *Validating) Create(id c4.ID) (io.WriteCloser, error) {
	w, err := v.s.Create(id)
	if err != nil {
		return nil, err
	}
	if c, ok := w.(Checked); ok && c.ChecksID() {
		return w, nil
	}
	return &validatingWriter{h: sha512.New(), id: id, w: w, remove: v.s.Remove}, nil
}

func (v *Validating) Has(id c4.ID) bool { return v.s.Has(id) }

func (v *Validating) Put(r io.Reader) (c4.ID, error) { return v.s.Put(r) }

func (v *Validating) Remove(id c4.ID) error {
	return v.s.Remove(id)
}
