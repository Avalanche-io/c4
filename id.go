package c4

import (
	"bytes"
	"crypto/sha512"
	"hash"
	"io"
	"math/big"
	"sort"
	"strconv"
)

// using the flickr character set which removes:
// ['=', '+', '_', '0', 'O', 'I', 'l'] from base64
// to reduce transcription errors, and make friendlier URLs
const (
	charset = "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
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

type errBadChar int

func (e errBadChar) Error() string {
	return "non c4 character at position " + strconv.Itoa(int(e))
}

// ID represents a C4 Asset ID.
type ID big.Int

// IDSlice represents a slice of IDs.
type IDSlice []*ID

func (s IDSlice) Len() int           { return len(s) }
func (s IDSlice) Less(i, j int) bool { return s[i].Cmp(s[j]) < 0 }
func (s IDSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Sort is a convenience method.
func (s IDSlice) Sort() {
	sort.Sort(s)
}

// Push adds the item to the IDSlice.
func (s *IDSlice) Push(id *ID) {
	*s = append(*s, id)
}

func (s *IDSlice) String() string {
	result := "[ "
	for _, bigID := range *s {
		result += ((*ID)(bigID)).String() + " "
	}
	return result + "]"
}

// SearchIDs searches for x in a sorted slice of *ID and returns the index
// as specified by sort.Search. The slice must be sorted in ascending order.
func SearchIDs(a IDSlice, x *ID) int {
	return sort.Search(len(a), func(i int) bool { return a[i].Cmp(x) >= 0 })
}

// ID gets the ID from the IDSlice.
func (s IDSlice) ID() *ID {
	s.Sort()
	encoder := NewIDEncoder()
	for _, bigID := range s {
		if _, err := io.Copy(encoder, bytes.NewReader(bigID.Bytes())); err != nil {
			panic(err)
		}
	}
	return encoder.ID()
}

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

// Cmp compares two IDs.
func (id *ID) Cmp(y *ID) int {
	bigX := big.Int(*id)
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

// BlockSize gets the block size of the underlying
// sha512 hash object.
func (e *IDEncoder) BlockSize() int {
	return e.h.BlockSize()
}

// Reset resets the underlying
// sha512 hash object.
func (e *IDEncoder) Reset() {
	e.h.Reset()
}

// Size gets the size of the underlying
// sha512 hash object.
func (e *IDEncoder) Size() int {
	return e.h.Size()
}

// Sum gets the C4ID sum of the underlying
// sha512 hash object.
// Recommended to use ID() instead.
func (e *IDEncoder) Sum(b []byte) []byte {
	return e.h.Sum(b)
}
