package asset

import (
	"bytes"
	"io"
	"math/big"
)

// Return the 64 raw bytes of a c4 id.  I.e. the actual sha512.
func (id *ID) RawBytes() (b []byte) {
	bignum := big.Int(*id)
	b_raw := (&bignum).Bytes()
	bytes64 := make([]byte, 64)

	padding := 64 - len(b_raw)
	// Can't use copy!
	// It doesn't properly handle leading zeros
	// copy(bytes64, b_raw)
	for i, bb := range b_raw {
		bytes64[padding+i] = bb
	}
	return bytes64[:]
}

// Create an ID from SHA-512 bytes.
func BytesToID(b []byte) *ID {
	bignum := big.NewInt(0)
	bignum = bignum.SetBytes(b)
	id := ID(*bignum)
	return &id
}

// Generate a c4 id from the raw bytes of two sorted c4 ids.
// As apposed to the .Sum(*ID) function that uses encoded ids.
func (i *ID) RawSum(j *ID) (*ID, error) {
	var ids [2]*ID
	ids[0] = i
	ids[1] = j
	l := 0
	r := 1

	if ids[r].Cmp(ids[l]) < 0 {
		r = 0
		l = 1
	}
	e := NewIDEncoder()
	_, err := io.Copy(e, bytes.NewReader(append(ids[l].RawBytes(), ids[r].RawBytes()...)))
	return e.ID(), err
}
