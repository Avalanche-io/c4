package id_test

import (
	// "bytes"
	"errors"
	"strings"
	"testing"
	"testing/iotest"

	c4 "github.com/Avalanche-io/c4/id"
	"github.com/cheekybits/is"
)

func TestIdentify(t *testing.T) {
	is := is.New(t)

	id := c4.Identify(iotest.DataErrReader(strings.NewReader("foo")))
	is.Equal(id.String(), "c45xZeXwMSpqXjpDumcHMA6mhoAmGHkUo7r9WmN2UgSEQzj9KjgseaQdkEJ11fGb5S1WEENcV3q8RFWwEeVpC7Fjk2")
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

	id := c4.Identify(errorReader(true))
	is.Nil(id)
}
