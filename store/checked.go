package store

import (
	"bytes"
	"crypto/sha512"
	"hash"
	"io"

	"github.com/Avalanche-io/c4"
)

// Checked is implemented by a Create writer that itself refuses, on
// Close, bytes that do not hash to the ID it was created for, naming
// nothing (ErrInvalidID). Every Sink in this package hands out such a
// writer; Validating passes one through rather than hashing a second
// time. A wrapper reports what it wraps.
type Checked interface {
	ChecksID() bool
}

// abortable is what a checked writer wraps: a writer that can be given
// up without naming anything.
type abortable interface {
	io.WriteCloser
	Aborter
}

// checkedWriter hashes what is written and checks it against the ID on
// Close: a mismatch aborts the inner writer and returns ErrInvalidID,
// so the inner writer, which names on Close, never names wrong bytes.
// It does not forward ReadFrom: every byte must pass the hash.
type checkedWriter struct {
	w    abortable
	id   c4.ID
	h    hash.Hash
	done bool
}

func newChecked(w abortable, id c4.ID) *checkedWriter {
	return &checkedWriter{w: w, id: id, h: sha512.New()}
}

func (w *checkedWriter) Write(b []byte) (int, error) {
	n, err := w.w.Write(b)
	w.h.Write(b[:n])
	return n, err
}

func (w *checkedWriter) ChecksID() bool { return true }

func (w *checkedWriter) Abort() error {
	if w.done {
		return nil
	}
	w.done = true
	return w.w.Abort()
}

func (w *checkedWriter) Close() error {
	if w.done {
		return nil
	}
	w.done = true
	if !bytes.Equal(w.h.Sum(nil), w.id[:]) {
		w.w.Abort()
		return ErrInvalidID
	}
	return w.w.Close()
}
