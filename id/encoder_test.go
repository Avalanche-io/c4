package id_test

import (
	// "bytes"

	"io"
	"strconv"
	"strings"
	"testing"

	c4 "github.com/Avalanche-io/c4/id"
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
			Exp: "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT",
		},
	} {
		actual := encode(test.In)
		is.Equal(actual.String(), test.Exp)
	}
}

func TestIDEncoder(t *testing.T) {
	is := is.New(t)
	e := c4.NewEncoder()
	is.OK(e)
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	is.NoErr(err)

	id := e.ID()
	is.OK(id)
	is.Equal(id.String(), `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`)
	// Added test for mutability bug. Calling String() should not alter id!
	is.Equal(id.String(), `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`)
}

func TestIDEncoderReset(t *testing.T) {
	is := is.New(t)
	e := c4.NewEncoder()
	is.OK(e)
	for i := 0; i < 10; i++ {
		s := strconv.Itoa(i)
		e2 := c4.NewEncoder()
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
