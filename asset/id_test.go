package asset_test

import (
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

var _ io.Writer = (*asset.IDEncoder)(nil)
var _ hash.Hash = (*asset.IDEncoder)(nil)
var _ fmt.Stringer = (*asset.ID)(nil)

func encode(src io.Reader) *asset.ID {
	e := asset.NewIDEncoder()
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
	id := asset.ID(*bignum)
	is.Equal(id.String(), `c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)

	id2, err := asset.ParseID(`c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)
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
	id := asset.ID(*bignum)
	is.Equal(id.String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)

	id2, err := asset.ParseID(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	is.NoErr(err)
	bignum2 := big.Int(*id2)
	b = (&bignum2).Bytes()
	// This loop is unnecessary, bignum zero has only 1 byte.
	for _, bb := range b {
		is.Equal(bb, 0x00)
	}
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
		id := asset.ID(*bignum)
		is.Equal(id.String(), expectedIDs[k])

		id2, err := asset.ParseID(expectedIDs[k])
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
	e := asset.NewIDEncoder()
	is.OK(e)
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	is.NoErr(err)

	id := e.ID()
	is.OK(id)
	is.Equal(id.String(), `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)
	// Added test for mutability bug. Calling String() should not alter id!
	is.Equal(id.String(), `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`)

	// ensure it implements hash.Hash
	is.Equal(e.BlockSize(), sha512.BlockSize)
	is.Equal(e.Size(), sha512.Size)

}

func TestParseBytesID(t *testing.T) {
	is := is.New(t)

	for _, test := range []struct {
		In  string
		Err string
		Exp string
	}{
		{
			In:  `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`,
			Err: ``,
			Exp: "This is a pretend asset file, for testing asset id generation.\n",
		},
		{
			In:  `c430BJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9d`,
			Err: `non c4 id character at position 3`,
			Exp: "",
		},
		{
			In:  ``,
			Err: `c4 ids must be 90 characters long, input length 0`,
			Exp: "",
		},
		{
			In:  `c43UBJqUTjQyrcRv43pgt1UWqysgNud7a7Kohjp1Z4w1gD8LGv4p1FK48kC8ufPPRpbEtc8inVhxuFQ453GcfRFE9`,
			Err: `c4 ids must be 90 characters long, input length 89`,
			Exp: "",
		},
	} {
		id, err := asset.ParseBytesID([]byte(test.In))
		if len(test.Err) != 0 {
			is.Err(err)
			is.Equal(err.Error(), test.Err)
		} else {
			expectedID, err := asset.Identify(strings.NewReader(test.Exp))
			is.NoErr(err)
			is.Equal(expectedID.Cmp(id), 0)
		}
	}
}
