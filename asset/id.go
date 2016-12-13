// Package asset provides c4id functions.
package asset

import (
	"bytes"
	"io"
	"math/big"
)

// using the flickr character set which removes:
// ['=', '+', '_', '0', 'O', 'I', 'l'] from base64
// to reduce transcription errors, and make friendlier URLs
const (
	charset = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	base    = 58
)

var (
	lut     [256]byte
	lowbyte = []byte("1")
	prefix  = []byte{'c', '4'}
	idlen   = 90
)

func init() {
	for i := 0; i < len(lut); i++ {
		lut[i] = 0xFF
	}
	for i := 0; i < len(charset); i++ {
		lut[charset[i]] = byte(i)
	}
}

// ID represents a C4 Asset ID.
type ID big.Int

// Sum returns the ID of two IDs in sorted order.
func (i *ID) Sum(j *ID) (*ID, error) {
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
	_, err := io.Copy(e, bytes.NewReader(append(ids[l].Bytes(), ids[r].Bytes()...)))
	return e.ID(), err
}

// ParseID parses a C4 ID string into an ID.
func ParseID(src string) (*ID, error) {
	return ParseBytesID([]byte(src))
}

// ParseBytesID parses a C4 ID as []byte into an ID.
func ParseBytesID(src []byte) (*ID, error) {
	if len(src) != 90 {
		return nil, errBadLength(len(src))
	}
	bigNum := new(big.Int)
	bigBase := big.NewInt(base)
	for i := 2; i < len(src); i++ {
		b := lut[src[i]]
		if b == 0xFF {
			return nil, errBadChar(i)
		}
		bigNum.Mul(bigNum, bigBase)
		bigNum.Add(bigNum, big.NewInt(int64(b)))
	}
	id := ID(*bigNum)
	return &id, nil
}

// String returns the standard string representation of a C4 id.
func (id *ID) String() (s string) {
	return string(id.Bytes())
}

/*
 * Cmp compares to c4ids.
 * There are 3 possible return values.
 * -1 : Argument id is less than calling id.
 * 0: Argument id and calling id are identical.
 * +1: Argument id is greater than calling id.
 * Comparison is done on the actual numerical value of the ids.
 * Not the string representation.
 */
func (x *ID) Cmp(y *ID) int {
	if y == nil {
		return -1
	}
	bigX := big.Int(*x)
	bigY := big.Int(*y)
	return bigX.Cmp(&bigY)
}

// Bytes encodes the written bytes to C4 ID format.
func (id *ID) Bytes() []byte {
	var bigNum big.Int
	bigID := big.Int(*id)
	bigNum.Set(&bigID)
	bigBase := big.NewInt(base)
	bigZero := big.NewInt(0)
	bigMod := new(big.Int)
	var encoded []byte
	for bigNum.Cmp(bigZero) > 0 {
		bigNum.DivMod(&bigNum, bigBase, bigMod)
		encoded = append([]byte{charset[bigMod.Int64()]}, encoded...)
	}
	// padding
	diff := idlen - 2 - len(encoded)
	encoded = append(bytes.Repeat(lowbyte, diff), encoded...)
	// c4... prefix
	encoded = append(prefix, encoded...)
	return encoded
}

// Returns true if B less than A in: A.Less(B)
func (id *ID) Less(idArg *ID) bool {
	return id.Cmp(idArg) < 0
}
