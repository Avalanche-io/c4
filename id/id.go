package id

import (
	"bytes"
	"math/big"
)

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

// ID represents a C4 ID.
type ID big.Int

// Parse parses a C4 ID string into an ID.
func Parse(src string) (*ID, error) {
	return parseBytesID([]byte(src))
}

// String returns the standard string representation of a C4 id.
func (id *ID) String() (s string) {
	return string(id.bytes())
}

// Digest returns the C4 Digest of the ID.
func (id *ID) Digest() Digest {
	return NewDigest((*big.Int)(id).Bytes())
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
func (l *ID) Cmp(r *ID) int {
	if r == nil {
		return -1
	}
	bigL := (*big.Int)(l)
	bigR := (*big.Int)(r)
	return bigL.Cmp(bigR)
}

// ParseBytesID parses a C4 ID as []byte into an ID.
// This method is no longer exported to avoid confusion, reduce the API
// surface area, and to conform with standards. Use Parse() instead.
func parseBytesID(src []byte) (*ID, error) {
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

// This method is no longer exported to avoid confusion, reduce the API
// surface area, and to conform with standards. Use String() instead.
func (id *ID) bytes() []byte {
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
