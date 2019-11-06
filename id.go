package c4

import (
	"bytes"
	"crypto/sha512"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/xtgo/set"
)

const (
	charset = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	base    = 58
)

var (
	lut     [256]byte
	lowbyte = byte('1')
	prefix  = []byte{'c', '4'}
	idlen   = 90

	// Id of empty string
	nilID = Identify(bytes.NewReader([]byte{}))

	// Id with all bytes set to 0.
	voidID ID

	// Id with all bytes set to 255.
	maxID ID
)

type errBadChar int

func (e errBadChar) Error() string {
	return "non c4 id character at position " + strconv.Itoa(int(e))
}

type errBadLength int

func (e errBadLength) Error() string {
	return "c4 ids must be 90 characters long, input length " + strconv.Itoa(int(e))
}

type errNil struct{}

func (e errNil) Error() string {
	return "unexpected nil id"
}

type errInvalidTree struct{}

func (e errInvalidTree) Error() string {
	return "invalid tree data"
}

func init() {
	for i := range lut {
		lut[i] = 0xFF
	}
	for i, c := range charset {
		lut[c] = byte(i)
	}
	for i := range maxID[:] {
		maxID[i] = 0xFF
	}
}

// Generate an id from an io.Reader
func Identify(src io.Reader) (id ID) {
	h := sha512.New()
	_, err := io.Copy(h, src)
	if err != nil && err != io.EOF {
		return id
	}
	copy(id[:], h.Sum(nil))
	return id
}

/*
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
func (e *Encoder) ID() (id ID) {
	copy(id[:], e.h.Sum(nil))
	return id
}

// Reset the encoder so it can identify a new block of data.
func (e *Encoder) Reset() {
	e.h.Reset()
}
*/
// ID represents a C4 ID.
type ID [64]byte

// Identifiable is an interface that requires an ID() method that returns
// the c4 ID of the of the object.
type Identifiable interface {
	ID() ID
}

func (id ID) IsNil() bool {
	for _, b := range id[:] {
		if b != 0 {
			return false
		}
	}
	return true
}

// Parse parses a C4 ID string into an ID.
func Parse(source string) (ID, error) {
	var id ID
	if len(source) == 0 {
		return voidID, errBadLength(len(source))
	}
	src := []byte(source)
	if len(src) != 90 {
		return id, errBadLength(len(src))
	}
	bigNum := new(big.Int)
	bigBase := big.NewInt(base)
	for i := 2; i < len(src); i++ {
		b := lut[src[i]]
		if b == 0xFF {
			return id, errBadChar(i)
		}
		bigNum.Mul(bigNum, bigBase)
		bigNum.Add(bigNum, big.NewInt(int64(b)))
	}
	data := bigNum.Bytes()
	if len(data) > 64 {
		data = data[:64]
	}
	shift := 64 - len(data)
	for i := 0; i < shift; i++ {
		data = append(data, 0)
	}

	if shift > 0 {
		copy(data[shift:], data)
		for i := 0; i < shift; i++ {
			data[i] = 0
		}
	}

	copy(id[:], data)
	return id, nil
}

// Digest returns the C4 Digest of the ID.
func (id ID) Digest() []byte {
	return id[:]
}

// Cmp compares two IDs.
// There are 3 possible return values.
//
// -1 : Argument id is less than calling id.
//  0 : Argument id and calling id are identical.
// +1 : Argument id is greater than calling id.
//
// Comparison is done on the actual numerical value of the ids.
// Not the string representation.
func (l ID) Cmp(r ID) int {
	if r.IsNil() {
		return -1
	}
	return bytes.Compare(l[:], r[:])
}

// String returns the standard string representation of a C4 id.
func (id ID) String() string {

	var bigID, bigNum big.Int
	bigID.SetBytes(id[:])
	bigNum.Set(&bigID)
	bigBase := big.NewInt(base)
	bigZero := big.NewInt(0)
	bigMod := new(big.Int)

	encoded := make([]byte, 90)
	for i := range encoded {
		encoded[i] = lowbyte
	}
	encoded[0] = 'c'
	encoded[1] = '4'

	for i := 89; i > 1 && bigNum.Cmp(bigZero) > 0; i-- {
		bigNum.DivMod(&bigNum, bigBase, bigMod)
		encoded[i] = charset[bigMod.Int64()]
	}

	return string(encoded)
}

// Returns true if B less than A in: A.Less(B)
func (id ID) Less(idArg ID) bool {
	return id.Cmp(idArg) < 0
}

func (l ID) Sum(r ID) ID {
	switch bytes.Compare(l[:], r[:]) {
	case -1:
		// If the left side is larger then they are already in order, do nothing
	case 1: // If the right side is larger swap them
		l, r = r, l
	case 0: // If they are identical return no sum needed, so just return one.
		return l
	}

	h := sha512.New()
	h.Write(l[:])
	h.Write(r[:])
	var id ID
	copy(id[:], h.Sum(nil))
	return id
}

func (id *ID) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"' \t`)
	if len(s) == 0 {
		return nil
	}
	i, err := Parse(s)
	if err != nil {
		return err
	}
	copy(id[:], i[:])
	return nil
}

func (id ID) MarshalJSON() ([]byte, error) {
	if id.IsNil() {
		return []byte(`""`), nil
	}
	return []byte(`"` + id.String() + `"`), nil
}

type IDs []ID

func (d IDs) Len() int           { return len(d) }
func (d IDs) Less(i, j int) bool { return bytes.Compare(d[i][:], d[j][:]) < 0 }
func (d IDs) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }

// Provides a computed C4 Tree for the slice of digests
func (d IDs) Tree() Tree {
	if !sort.IsSorted(d) {
		sort.Sort(d)
	}
	n := set.Uniq(d)
	d = d[:n]
	t := NewTree(d)
	t.compute()
	return t
}

func (d IDs) ID() ID {
	t := d.Tree()
	return t.ID()
}
