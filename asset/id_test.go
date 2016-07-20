package asset_test

import (
	// "bytes"
	"bytes"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Avalanche-io/c4/asset"
	"github.com/cheekybits/is"
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
}

func TestIDEncoderReset(t *testing.T) {
	is := is.New(t)
	e := asset.NewIDEncoder()
	is.OK(e)
	for i := 0; i < 10; i++ {
		s := strconv.Itoa(i)
		e2 := asset.NewIDEncoder()
		_, err := io.Copy(e, strings.NewReader(s))
		is.NoErr(err)
		_, err2 := io.Copy(e2, strings.NewReader(s))
		is.NoErr(err2)
		id1 := e.ID()
		id2 := e2.ID()
		is.Equal(id1, id2)
		e.Reset()
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

func TestRawBytes(t *testing.T) {
	is := is.New(t)
	// var b []byte
	// for i := 0; i < 64; i++ {
	// 	b = append(b, 0xFF)
	// }
	// id := asset.BytesToID(b)
	// is.Equal(id.String(), `c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`)

	// id2, err := asset.ParseID(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`)
	// tb2 := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58}
	// is.NoErr(err)
	// b2 := id2.RawBytes()
	// fmt.Println("tb2: ", tb2)
	// fmt.Println("b2: ", b2)
	// for i, bb := range b2 {
	// 	is.Equal(bb, tb2[i])
	// }

	for _, test := range []struct {
		Bytes []byte
		IdStr string
	}{
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`,
		},
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x0d, 0x24},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111211`,
		},
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0xfa, 0x28},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111112111`,
		},
		{
			Bytes: []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xac, 0xad, 0x10},
			IdStr: `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111121111`,
		},
	} {
		id, err := asset.ParseID(test.IdStr)
		is.NoErr(err)
		for i, bb := range id.RawBytes() {
			is.Equal(bb, test.Bytes[i])
		}
	}

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

func TestIdentify(t *testing.T) {
	is := is.New(t)

	id, err := asset.Identify(iotest.DataErrReader(strings.NewReader("foo")))
	is.NoErr(err)
	is.Equal(id.String(), "c45XyDwWmrPQwJPdULBhma6LGNaLghKtN7R9vLn2tFrepZJ9jJFSDzpCKei11EgA5r1veenBu3Q8qfvWeDuPc7fJK2")
}

func TestBytesToID(t *testing.T) {
	is := is.New(t)

	b := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58}
	id := asset.BytesToID(b)
	is.Equal(id.String(), "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121")
}

func TestRawSum(t *testing.T) {
	is := is.New(t)

	id1, err := asset.Identify(strings.NewReader("foo"))
	is.NoErr(err)
	id2, err := asset.Identify(strings.NewReader("bar"))
	is.NoErr(err)

	is.True(id2.Less(id1))

	b := id2.RawBytes()
	b = append(b, id1.RawBytes()...)
	expectedSum, err := asset.Identify(bytes.NewReader(b))
	is.NoErr(err)

	testSum, err := id1.RawSum(id2)
	is.NoErr(err)

	is.Equal(expectedSum, testSum)
}

func TestIDSliceString(t *testing.T) {
	is := is.New(t)

	var ids asset.IDSlice
	id1, err := asset.Identify(strings.NewReader("foo"))
	is.NoErr(err)
	id2, err := asset.Identify(strings.NewReader("bar"))
	is.NoErr(err)

	ids.Push(id1)
	ids.Push(id2)

	is.Equal(ids.String(), id1.String()+id2.String())
}

func TestIDSliceSearchIDs(t *testing.T) {
	is := is.New(t)

	var ids asset.IDSlice
	id1, err := asset.Identify(strings.NewReader("foo"))
	is.NoErr(err)
	id2, err := asset.Identify(strings.NewReader("bar"))
	is.NoErr(err)
	id3, err := asset.Identify(strings.NewReader("baz"))
	is.NoErr(err)

	ids.Push(id1)
	ids.Push(id2)
	ids.Push(id3)
	ids.Sort()

	is.True(id2.Less(id1))
	is.True(id3.Less(id2))

	is.Equal(asset.SearchIDs(ids, id1), 2)
	is.Equal(asset.SearchIDs(ids, id2), 1)
	is.Equal(asset.SearchIDs(ids, id3), 0)
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
