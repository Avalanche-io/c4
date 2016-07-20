package asset_test

import (
	// "bytes"
	"io"
	"math/big"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/asset"
	"github.com/cheekybits/is"
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
