package id

import (
	"crypto/sha512"
	"hash"
	"math/big"
)

// Encoder generates an ID for a contiguous bock of data.
type Encoder struct {
	err error
	h   hash.Hash
}

// NewIDEncoder makes a new Encoder.
func NewEncoder() *Encoder {
	return &Encoder{
		h: sha512.New(),
	}
}

// Write writes bytes to the hash that makes up the ID.
func (e *Encoder) Write(b []byte) (int, error) {
	return e.h.Write(b)
}

// ID returns the ID for the bytes written so far.
func (e *Encoder) ID() *ID {
	b := new(big.Int)
	b.SetBytes(e.h.Sum(nil))
	id := ID(*b)
	return &id
}

// Digest get the Digest for the bytes written so far.
func (e *Encoder) Digest() Digest {
	return NewDigest(e.h.Sum(nil))
}

// Reset the encoder so it can identify a new block of data.
func (e *Encoder) Reset() {
	e.h.Reset()
}
