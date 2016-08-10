package asset_test

import (
	// "bytes"

	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

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
	bigID := asset.ID(*bigBig)
	smallID := asset.ID(*bigSmall)

	var idSlice asset.IDSlice

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
	bigID := asset.ID(*bigBig)
	smallID := asset.ID(*bigSmall)

	encoder := asset.NewIDEncoder()
	is.OK(encoder)
	_, err := io.Copy(encoder, strings.NewReader(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111c467RPWkcUr5dga8jgywjSup7CMoA9FNqkNjEFgAkEpF9vNktFnx77e2Js11EDL3BNu9MaKFUbacZRt1HYym4b8RNp`))
	is.NoErr(err)
	id := encoder.ID()

	var idSlice asset.IDSlice
	idSlice.Push(&bigID)
	idSlice.Push(&smallID)
	sliceID, err := idSlice.ID()
	is.NoErr(err)

	is.Equal(sliceID.String(), id.String())
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

func TestSliceIDFile(t *testing.T) {
	is := is.New(t)

	id, err := asset.Identify(errorReader(true))
	is.Err(err)
	is.Nil(id)
	is.Equal(err.Error(), "errorReader triggered error.")
}
