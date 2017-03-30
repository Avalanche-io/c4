package id

import (
	"bytes"
	"crypto/sha512"
	"errors"
	"math/big"
)

// Digest represents a 64 byte "C4 Digest", which is the SHA-512 hash. Amongst other
// things Digest enforces padding to insure alignment with the original  64 byte hash.
//
// A digest is simply a slice of bytes and can be use wherever the raw SHA hash
// might be needed.
type Digest []byte

// NewDigest creates a Digest and initializes it with the argument, enforcing
// byte alignment by padding with 0 (zero). NewDigest will not create a Digest and will
// return nil if the argument is larger then 64 bytes.
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

// Sum returns the digest of the reviver and argument combined. Insuring
// proper sorting order.
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

// Digest supports the io.Reader interface specifically for the purpose of
// reading the 64 digest bytes without the need to create a new reader.
func (d Digest) Read(p []byte) (n int, err error) {
	if len(p) < 64 {
		return 0, errors.New("argument to read must accommodate size of digest (64 bytes)")
	}
	copy(p, []byte(d))
	return 64, nil
}
