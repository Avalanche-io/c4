package id_test

import (
	"math/big"
	"strings"
	"testing"

	c4 "github.com/avalanche-io/c4/id"
	"github.com/cheekybits/is"
)

func TestSliceSort(t *testing.T) {
	is := is.New(t)
	var b, s []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0xFF)
		s = append(s, 0x00)
	}
	bigBig := big.NewInt(0)
	bigSmall := big.NewInt(0)
	bigBig = bigBig.SetBytes(b)
	bigSmall = bigSmall.SetBytes(s)
	bigID := (*c4.ID)(bigBig)
	smallID := (*c4.ID)(bigSmall)

	var slice c4.Slice

	slice.Insert(bigID)
	slice.Insert(smallID)
	is.Equal(slice[0].String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	is.Equal(slice[1].String(), `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)
}

func TestSliceString(t *testing.T) {
	is := is.New(t)

	var ids c4.Slice
	id1 := c4.Identify(strings.NewReader("foo"))
	id2 := c4.Identify(strings.NewReader("bar"))
	ids.Insert(id1)
	ids.Insert(id2)
	is.Equal(ids.String(), id2.String()+id1.String())
}

func TestSliceSearchIDs(t *testing.T) {
	is := is.New(t)

	var ids c4.Slice
	id1 := c4.Identify(strings.NewReader("foo"))
	id2 := c4.Identify(strings.NewReader("bar"))
	id3 := c4.Identify(strings.NewReader("baz"))

	ids.Insert(id1)
	ids.Insert(id2)
	ids.Insert(id3)
	is.Equal(ids.Index(id1), 2)
	is.Equal(ids.Index(id2), 1)
	is.Equal(ids.Index(id3), 0)
}
