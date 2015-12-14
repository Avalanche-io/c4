package c4_test

import (
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4go"
)

var _ io.Writer = (*c4.IDEncoder)(nil)
var _ hash.Hash = (*c4.IDEncoder)(nil)
var _ fmt.Stringer = (*c4.ID)(nil)

func encode(src io.Reader) *c4.ID {
	e := c4.NewIDEncoder()
	_, err := io.Copy(e, src)
	if err != nil {
		panic(err)
	}
	return e.ID()
}

func TestEncoding(t *testing.T) {
	is := is.New(t)

	for _, test := range []struct {
		In  io.Reader
		Exp string
	}{
		{
			In:  strings.NewReader(``),
			Exp: "c459CSJESBh38BxDwwxNFKTXE4cC9HASGe3bhtN6z58GbwLqpCyRaKyZSvBAvTdF5NpSTPdUMH4hHRJ75geLsB1Sfs",
		},
	} {

		actual := encode(test.In)
		is.Equal(actual.String(), test.Exp)

	}

}

func TestAllFFFF(t *testing.T) {
	is := is.New(t)
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0xFF)
	}
	bignum := big.NewInt(0)
	bignum = bignum.SetBytes(b)
	id := c4.ID(*bignum)
	is.Equal(id.String(), `c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)

	id2, err := c4.ParseID(`c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)
	is.NoErr(err)
	bignum2 := big.Int(*id2)
	b = (&bignum2).Bytes()
	for _, bb := range b {
		is.Equal(bb, 0xFF)
	}
}

func TestAll0000(t *testing.T) {
	is := is.New(t)
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0x00)
	}
	bignum := big.NewInt(0)
	bignum = bignum.SetBytes(b)
	id := c4.ID(*bignum)
	is.Equal(id.String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)

	id2, err := c4.ParseID(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	is.NoErr(err)
	bignum2 := big.Int(*id2)
	b = (&bignum2).Bytes()
	// This loop is unnecessary, bignum zero has only 1 byte.
	for _, bb := range b {
		is.Equal(bb, 0x00)
	}
}

func TestIDSliceSort(t *testing.T) {
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
	bigID := c4.ID(*bigBig)
	smallID := c4.ID(*bigSmall)

	var idSlice c4.IDSlice

	idSlice.Push(&bigID)
	idSlice.Push(&smallID)
	is.Equal(idSlice[0].String(), `c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)
	is.Equal(idSlice[1].String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	idSlice.Sort()
	is.Equal(idSlice[0].String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	is.Equal(idSlice[1].String(), `c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)
}

func TestIDofIDSlice(t *testing.T) {
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
	bigID := c4.ID(*bigBig)
	smallID := c4.ID(*bigSmall)

	encoder := c4.NewIDEncoder()
	is.OK(encoder)
	_, err := io.Copy(encoder, strings.NewReader(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`))
	is.NoErr(err)
	id := encoder.ID()

	var idSlice c4.IDSlice
	idSlice.Push(&bigID)
	idSlice.Push(&smallID)
	is.Equal(idSlice.ID().String(), id.String())
}

func TestAppendOrder(t *testing.T) {
	is := is.New(t)
	byteData := [4][]byte{
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x0d, 0x24},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0xfa, 0x28},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xac, 0xad, 0x10},
	}
	expectedIDs := [4]string{
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111211`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111112111`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111121111`,
	}
	for k := 0; k < 4; k++ {
		b := byteData[k]
		bignum := big.NewInt(0)
		bignum = bignum.SetBytes(b)
		id := c4.ID(*bignum)
		is.Equal(id.String(), expectedIDs[k])

		id2, err := c4.ParseID(expectedIDs[k])
		is.NoErr(err)
		bignum2 := big.Int(*id2)
		b = (&bignum2).Bytes()
		size := len(b)
		for size < 64 {
			b = append([]byte{0}, b...)
			size++
		}
		for i, bb := range b {
			is.Equal(bb, byteData[k][i])
		}
	}
}

func TestIDEncoder(t *testing.T) {
	is := is.New(t)
	e := c4.NewIDEncoder()
	is.OK(e)
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	is.NoErr(err)

	id := e.ID()
	is.OK(id)
	is.Equal(id.String(), `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)
	// Added test for mutability bug. Calling String() should not alter id!
	is.Equal(id.String(), `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)

	is.Equal(e.BlockSize(), sha512.BlockSize)
	is.Equal(e.Size(), sha512.Size)
	is.Equal(e.Sum(nil), e.ID().Bytes())
	old := e.ID().Bytes()
	e.Reset()
	is.NotEqual(e.Sum(nil), old)

}

func TestParseBytesID(t *testing.T) {
	is := is.New(t)
	e := c4.NewIDEncoder()
	is.OK(e)
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	is.NoErr(err)

	id, err := c4.ParseBytesID([]byte(`c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`))
	is.NoErr(err)
	is.Equal(id, e.ID())

	id2, err := c4.ParseID(`c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)
	is.NoErr(err)
	is.Equal(id2, e.ID())
}
