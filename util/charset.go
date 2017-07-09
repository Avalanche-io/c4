// The c4/util package is only of use to a very small group who began using
// C4 prior to standardization. Most people will not need to use this package
// for anything.
//
// In late 2016 the C4 ID character set was changed.
//
// From: "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
// To: "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
//
// The included characters are still the same, but the capital letters are
// now placed before the lower case letters.
//
// This change was deemed necessary to avoid inconsistent sorting between
// the Digest and String forms of C4 IDs which is counterintuitive and likely
// to lead to coding errors as well as requiring additional computation to
// handle correctly.
//

package util

import (
	"errors"
	"strconv"
	"strings"

	c4 "github.com/Avalanche-io/c4/id"
)

const (
	OldCharset = "123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ"
	Charset    = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
)

var (
	oldnewlut map[byte]byte
	newoldlut map[byte]byte
)

func init() {
	oldnewlut = make(map[byte]byte)
	newoldlut = make(map[byte]byte)
	base := len(Charset)
	for i := 0; i < base; i++ {
		oldnewlut[OldCharset[i]] = Charset[i]
		newoldlut[Charset[i]] = OldCharset[i]
	}
}

// Returns true if all digits match between two IDs.
func sameNumbers(a *c4.ID, b *c4.ID) bool {
	if a == nil || b == nil {
		return false
	}
	x := a.String()
	y := b.String()
	for i, c := range x {
		_, err := strconv.ParseInt(string(c), 10, 8)
		if err != nil {
			continue
		}
		if rune(y[i]) != c {
			return false
		}
	}
	return true
}

// CheckCharacterSet, given the same underlying C4 ID encoded in the old and new
// character sets, will return the version that is correctly encoded.
// A nil ID and an error are returned if either ID is invalid or the two ids
// are not an encoding of the same ID.
func CheckCharacterSet(a *c4.ID, b *c4.ID) (*c4.ID, error) {
	if !sameNumbers(a, b) {
		return nil, errors.New("not the same id")
	}
	var x, y string
	remove_set := "123456789"
	x = a.String()[2:]
	y = b.String()[2:]
	for _, c := range remove_set {
		x = strings.Replace(x, string(c), "", -1)
		y = strings.Replace(y, string(c), "", -1)
	}
	newer := 0 // -1, 0, 1: a is newer, not the same, b is newer
	for i := range x {
		xc := x[i]
		yc := y[i]
		if oldnewlut[xc] == yc {
			if newer == -1 {
				return nil, errors.New("not the same id 2")
			}
			newer = 1
			continue
		}
		if newoldlut[xc] != yc || newer == 1 {
			return nil, errors.New("not the same id 2")
		}
		newer = -1
	}
	if newer == -1 {
		return a, nil
	}
	return b, nil
}

// OldCharsetIDToNew transforms IDs from the incorrect (old) character set to
// the correct, and current one.
//
// Be careful!  This function cannot detect wither an ID uses the correct character set or
// not so care must be taken not to apply this function to IDs that are correct.
func OldCharsetIDToNew(id *c4.ID) *c4.ID {
	if id == nil {
		return nil
	}
	id_str := id.String()
	newid := "c4"

	for i := 2; i < 90; i++ {
		newid = newid + string(oldnewlut[id_str[i]])
	}
	idout, _ := c4.Parse(newid)
	return idout
}

// NewCharsetIDToOld transforms IDs from the correct, current character set
// to the old character set. This is not very useful outside of testing, so
// be sure you know what you're doing before using this function.
func NewCharsetIDToOld(id *c4.ID) *c4.ID {
	if id == nil {
		return nil
	}
	id_str := id.String()
	oldid := "c4"

	for i := 2; i < 90; i++ {
		oldid = oldid + string(newoldlut[id_str[i]])
	}
	idout, _ := c4.Parse(oldid)
	return idout
}
