package asset_test

import (
	// "bytes"

	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

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
