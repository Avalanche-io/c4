package c4

import (
	"crypto/sha512"
	"math/big"

	"hash"
)

// using the flickr character set which removes:
// ['=', '+', '_', '0', 'O', 'I', 'l'] from base64
// to reduce transcription errors, and make friendlier URLs
const (
	charset = "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	base    = 58
)

var lut [256]byte

func init() {
	for i := 0; i < len(lut); i++ {
		lut[i] = 0xFF
	}
	for i := 0; i < len(charset); i++ {
		lut[charset[i]] = byte(i)
	}
}

type errBadChar int64

func (e errBadChar) Error() string {
	return "non c4 character at position " + string(e)
}

// ID represents a C4 Asset ID.
type ID big.Int

// ParseID parses a C4 ID string into an ID.
func ParseID(src string) (*ID, error) {
	return ParseBytesID([]byte(src))
}

// ParseBytesID parses a C4 ID as []byte into an ID.
func ParseBytesID(src []byte) (*ID, error) {
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

func (id *ID) String() string {
	return string(id.Bytes())
}

// Bytes encodes the written bytes to C4 ID format.
func (id *ID) Bytes() []byte {
	bigNum := big.Int(*id)
	bigBase := big.NewInt(base)
	bigZero := big.NewInt(0)
	bigMod := new(big.Int)
	var encoded []byte
	for bigNum.Cmp(bigZero) > 0 {
		bigNum.DivMod(&bigNum, bigBase, bigMod)
		encoded = append([]byte{charset[bigMod.Int64()]}, encoded...)
	}
	encoded = append([]byte{'c', '4'}, encoded...)
	return encoded
}

// IDEncoder generates C4 Asset IDs.
type IDEncoder struct {
	h hash.Hash
}

// NewIDEncoder makes a new IDEncoder.
func NewIDEncoder() *IDEncoder {
	return &IDEncoder{
		h: sha512.New(),
	}
}

// Write writes bytes to the hash that makes up the ID.
func (e *IDEncoder) Write(b []byte) (int, error) {
	return e.h.Write(b)
}

// ID gets the ID for the written bytes.
func (e *IDEncoder) ID() *ID {
	b := new(big.Int)
	b.SetBytes(e.h.Sum(nil))
	id := ID(*b)
	return &id
}
