package asset_test

import (
	// "bytes"

	"fmt"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

var _ io.Writer = (*asset.IDEncoder)(nil)
var _ fmt.Stringer = (*asset.ID)(nil)

func encode(src io.Reader) *asset.ID {
	e := asset.NewIDEncoder()
	_, err := io.Copy(e, src)
	if err != nil {
		panic(err)
	}
	return e.ID()
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

func TestIDLess(t *testing.T) {
	is := is.New(t)
	id1 := encode(strings.NewReader(`1`)) // c42yrSHMvUcscrQBssLhrRE28YpGUv9Gf95uH8KnwTiBv4odDbVqNnCYFs3xpsLrgVZfHebSaQQsvxgDGmw5CX1fVy
	id2 := encode(strings.NewReader(`2`)) // c42i2hTBA9Ej4nqEo9iUy3pJRRE53KAH9RwwMSWjmfaQN7LxCymVz1zL9hEjqeFYzxtxXz2wRK7CBtt71AFkRfHodu

	is.Equal(id1.Less(id2), false)
}

func TestIDCmp(t *testing.T) {
	is := is.New(t)
	id1 := encode(strings.NewReader(`1`)) // c42yrSHMvUcscrQBssLhrRE28YpGUv9Gf95uH8KnwTiBv4odDbVqNnCYFs3xpsLrgVZfHebSaQQsvxgDGmw5CX1fVy
	id2 := encode(strings.NewReader(`2`)) // c42i2hTBA9Ej4nqEo9iUy3pJRRE53KAH9RwwMSWjmfaQN7LxCymVz1zL9hEjqeFYzxtxXz2wRK7CBtt71AFkRfHodu

	is.Equal(id1.Cmp(id2), 1)
	is.Equal(id2.Cmp(id1), -1)
	is.Equal(id1.Cmp(id1), 0)

}

func TestCompareIDs(t *testing.T) {
	is := is.New(t)

	for _, test := range []struct {
		Id_A *asset.ID
		Id_B *asset.ID
		Exp  int
	}{
		{

			Id_A: encode(strings.NewReader("Test string")),
			Id_B: encode(strings.NewReader("Test string")),
			Exp:  0,
		},
		{
			Id_A: encode(strings.NewReader("Test string A")),
			Id_B: encode(strings.NewReader("Test string B")),
			Exp:  -1,
		},
		{
			Id_A: encode(strings.NewReader("Test string B")),
			Id_B: encode(strings.NewReader("Test string A")),
			Exp:  1,
		},
		{
			Id_A: encode(strings.NewReader("Test string")),
			Id_B: nil,
			Exp:  -1,
		},
	} {
		is.Equal(test.Id_A.Cmp(test.Id_B), test.Exp)
	}

}

func TestBytesToID(t *testing.T) {
	is := is.New(t)

	b := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58}
	id := asset.BytesToID(b)
	is.Equal(id.String(), "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121")
}

func TestSum(t *testing.T) {
	is := is.New(t)

	id1, err := asset.Identify(strings.NewReader("foo"))
	is.NoErr(err)
	id2, err := asset.Identify(strings.NewReader("bar"))
	is.NoErr(err)

	is.True(id2.Less(id1))

	sr := strings.NewReader(id2.String() + id1.String())
	expectedSum, err := asset.Identify(sr)
	is.NoErr(err)

	testSum, err := id1.Sum(id2)
	is.NoErr(err)

	is.Equal(expectedSum, testSum)
}

func TestNILID(t *testing.T) {
	is := is.New(t)

	// ID of nothing constant
	nilid := asset.NIL_ID
	is.Equal(nilid.String(), "c459CSJESBh38BxDwwxNFKTXE4cC9HASGe3bhtN6z58GbwLqpCyRaKyZSvBAvTdF5NpSTPdUMH4hHRJ75geLsB1Sfs")
}
