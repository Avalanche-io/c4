package asset_test

import (
	// "bytes"

	"io"
	"strings"
	"testing"

	"github.com/cheekybits/is"
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
