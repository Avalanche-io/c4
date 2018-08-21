package id_test

import (
	// "bytes"

	"io"
	"strconv"
	"strings"
	"testing"

	c4 "github.com/Avalanche-io/c4/id"
)

func TestEncoding(t *testing.T) {

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
		if actual.String() != test.Exp {
			t.Errorf("IDs don't match, got %q expected %q", actual.String(), test.Exp)
		}
	}
}

func TestIDEncoder(t *testing.T) {
	e := c4.NewEncoder()
	if e == nil {
		t.Errorf("Bad value returned from NewEncoder")
	}
	_, err := io.Copy(e, strings.NewReader(`This is a pretend asset file, for testing asset id generation.
`))
	if err != nil {
		t.Errorf("unexpected error %q", err)
	}

	id := e.ID()
	if id == nil {
		t.Errorf("unexpected nil")
	}
	if id.String() != `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D` {
		t.Errorf("IDs don't match, got %q expected %q", id.String(), `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`)
	}
	// Added test for mutability bug. Calling String() should not alter id!
	if id.String() != `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D` {
		t.Errorf("IDs don't match, got %q expected %q", id.String(), `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`)
	}
}

func TestIDEncoderReset(t *testing.T) {
	e := c4.NewEncoder()
	if e == nil {
		t.Errorf("unexpected nil")
	}
	for i := 0; i < 10; i++ {
		s := strconv.Itoa(i)
		e2 := c4.NewEncoder()
		_, err := io.Copy(e, strings.NewReader(s))
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}
		_, err = io.Copy(e2, strings.NewReader(s))
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}

		id1 := e.ID()
		id2 := e2.ID()
		if id1.String() != id2.String() {
			t.Errorf("IDs don't match, got %q expected %q", id1.String(), id2.String())
		}
		e.Reset()
	}
}
