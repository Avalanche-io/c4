package asset_test

import (
	// "bytes"

	"strings"
	"testing"
	"testing/iotest"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

func TestIdentify(t *testing.T) {
	is := is.New(t)

	id, err := asset.Identify(iotest.DataErrReader(strings.NewReader("foo")))
	is.NoErr(err)
	is.Equal(id.String(), "c45XyDwWmrPQwJPdULBhma6LGNaLghKtN7R9vLn2tFrepZJ9jJFSDzpCKei11EgA5r1veenBu3Q8qfvWeDuPc7fJK2")
}
