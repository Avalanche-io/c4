package asset_test

import (
	// "bytes"
	"errors"
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

// returns error on read for testing the negative case
type errorReader bool

func (e errorReader) Read(p []byte) (int, error) {
	if e == true {
		return 0, errors.New("errorReader triggered error.")
	}
	return 0, nil
}

func TestIOFailure(t *testing.T) {
	is := is.New(t)

	id, err := asset.Identify(errorReader(true))
	is.Err(err)
	is.Nil(id)
	is.Equal(err.Error(), "errorReader triggered error.")
}
