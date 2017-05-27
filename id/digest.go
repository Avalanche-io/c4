package id

import (
	"bytes"
	"crypto/sha512"
	"io"
	"math/big"
)

// Digest represents a 64 byte "C4 Digest", which is the SHA-512 hash. Amongst other
// things Digest enforces padding to insure alignment with the original 64 byte hash.
//
// A digest is simply a slice of bytes and can be use wherever the raw SHA hash
// might be needed.
type Digest []byte

// NewDigest creates a Digest and initializes it with the argument, enforcing
// byte alignment by padding with 0 (zero) if needed. NewDigest will return nil
// if the argument is larger then 64 bytes.
func NewDigest(b []byte) Digest {
	if len(b) > 64 {
		return nil
	}
	// If we don't need padding we're done.
	if len(b) == 64 {
		return Digest(b)
	}
	out := make([]byte, 64)
	// leading values are zero, so we shift the copy by the padding amount
	copy(out[64-len(b):], b)
	return Digest(out)
}

// Sum returns the digest of the receiver and argument combined. Insuring
// proper order. C4 Digests of C4 Digests are always identified by concatenating
// the bytes of the larger digest after the bytes of the lesser digest to form a
// block of 128 bytes which are then IDed.
func (l Digest) Sum(r Digest) Digest {
	switch bytes.Compare(l, r) {
	case 1:
		l, r = r, l
	case 0:
		return l
	}
	h := sha512.New()
	h.Write([]byte(l))
	h.Write([]byte(r))
	return NewDigest(h.Sum(nil))
}

// ID returns the C4 ID representation of the digest by directly translating the byte
// slice to the standard C4 ID string format (the bytes are not (re)hashed).
func (d Digest) ID() *ID {
	i := new(big.Int)
	i.SetBytes([]byte(d))
	return (*ID)(i)
}

// Read implements the io.Reader interface. It reads exactly 64 bytes into p.
// Read only returns an error if p is less than 64 bytes, in which case it
// returns io.EOF, without reading any bytes.
func (d Digest) Read(p []byte) (n int, err error) {
	if len(p) < 64 {
		return 0, io.EOF
	}
	copy(p, []byte(d))
	return 64, nil
}

// Write implements the io.Writer interface. It writes exactly 64 bytes
// replacing the value of the digest. The bytes must be a valid c4 Digest
// (i.e. sha-512 hash), any other value and the behavior of Write is undefined.
// Write only return an error if less than 64 bytes of input are available,
// in which case it returns io.EOF, without writing any bytes.
func (d Digest) Write(p []byte) (n int, err error) {
	if len(p) < 64 {
		return 0, io.EOF
	}
	copy([]byte(d), p)
	return 64, nil
}
