package asset_test

import (
	// "bytes"
	"bytes"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

func TestRawBytes(t *testing.T) {
	is := is.New(t)
	// var b []byte
	// for i := 0; i < 64; i++ {
	//   b = append(b, 0xFF)
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
	//   is.Equal(bb, tb2[i])
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
