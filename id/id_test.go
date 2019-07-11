package id_test

import (
	"io"
	"math/big"
	"strings"
	"testing"

	c4 "github.com/Avalanche-io/c4/id"
)

func encode(src io.Reader) *c4.ID {
	e := c4.NewEncoder()
	_, err := io.Copy(e, src)
	if err != nil {
		panic(err)
	}
	return e.ID()
}

func TestAllFFFF(t *testing.T) {
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0xFF)
	}
	bignum := big.NewInt(0)
	bignum = bignum.SetBytes(b)

	id := c4.ID(*bignum)
	if id.String() != `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ` {
		t.Errorf("IDs don't match, got %q, expcted %q", id.String(), `c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)
	}

	id2, err := c4.Parse(`c467rpwLCuS5DGA8KGZXKsVQ7dnPb9goRLoKfgGbLfQg9WoLUgNY77E2jT11fem3coV9nAkguBACzrU1iyZM4B8roQ`)
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	bignum2 := big.Int(*id2)
	b = (&bignum2).Bytes()
	for _, bb := range b {
		if bb != 0xFF {
			t.Errorf("incorrect Parse results")
		}
	}
}

func TestAll0000(t *testing.T) {
	var b []byte
	for i := 0; i < 64; i++ {
		b = append(b, 0x00)
	}
	bignum := big.NewInt(0)
	bignum = bignum.SetBytes(b)
	id := c4.ID(*bignum)

	if id.String() != `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111` {
		t.Errorf("IDs don't match, got %q, expcted %q", id.String(), `c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	}

	id2, err := c4.Parse(`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111111`)
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	bignum2 := big.Int(*id2)
	b = (&bignum2).Bytes()
	// This loop is unnecessary, bignum zero has only 1 byte.
	for _, bb := range b {
		if bb != 0x00 {
			t.Errorf("incorrect Parse results")
		}
	}
}

func TestAppendOrder(t *testing.T) {
	byteData := [4][]byte{
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x0d, 0x24},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0xfa, 0x28},
		{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xac, 0xad, 0x10},
	}
	expectedIDs := [4]string{
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111211`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111112111`,
		`c41111111111111111111111111111111111111111111111111111111111111111111111111111111111121111`,
	}
	for k := 0; k < 4; k++ {
		b := byteData[k]
		bignum := big.NewInt(0)
		bignum = bignum.SetBytes(b)
		id := c4.ID(*bignum)

		// is.Equal(id.String(), expectedIDs[k])
		if id.String() != expectedIDs[k] {
			t.Errorf("IDs don't match, got %q, expcted %q", id.String(), expectedIDs[k])
		}
		id2, err := c4.Parse(expectedIDs[k])
		if err != nil {
			t.Errorf("Unexpected error %q", err)
		}

		bignum2 := big.Int(*id2)
		b = (&bignum2).Bytes()
		size := len(b)
		for size < 64 {
			b = append([]byte{0}, b...)
			size++
		}
		for i, bb := range b {
			if bb != byteData[k][i] {
				t.Errorf("incorrect Parse results")
			}

		}
	}
}

func TestParseBytesID(t *testing.T) {
	for _, test := range []struct {
		In  string
		Err string
		Exp string
	}{
		{
			In:  `c43ucjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`,
			Err: ``,
			Exp: "This is a pretend asset file, for testing asset id generation.\n",
		},
		{
			In:  `c430cjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9D`,
			Err: `non c4 id character at position 3`,
			Exp: "",
		},
		{
			In:  ``,
			Err: `c4 ids must be 90 characters long, input length 0`,
			Exp: "",
		},
		{
			In:  `c430cjRutKqZSCrW43QGU1uwRZTGoVD7A7kPHKQ1z4X1Ge8mhW4Q1gk48Ld8VFpprQBfUC8JNvHYVgq453hCFrgf9`,
			Err: `c4 ids must be 90 characters long, input length 89`,
			Exp: "",
		},
	} {
		id, err := c4.Parse(test.In)
		if len(test.Err) != 0 {
			// is.Err(err)
			if err == nil {
				t.Errorf("Expected error but got none")
			}

			if err.Error() != test.Err {
				t.Errorf("incorrect error got %q, expected %q", err.Error(), test.Err)
			}

		} else {
			expectedID := c4.Identify(strings.NewReader(test.Exp))
			if err != nil {
				t.Errorf("Unexpected error %q", err)
			}

			if expectedID.Cmp(id) != 0 {
				t.Errorf("IDs don't match, got %q, expcted %q", id, expectedID)
			}
		}
	}
}

func TestIDLess(t *testing.T) {
	id1 := encode(strings.NewReader(`1`)) // c42yrSHMvUcscrQBssLhrRE28YpGUv9Gf95uH8KnwTiBv4odDbVqNnCYFs3xpsLrgVZfHebSaQQsvxgDGmw5CX1fVy
	id2 := encode(strings.NewReader(`2`)) // c42i2hTBA9Ej4nqEo9iUy3pJRRE53KAH9RwwMSWjmfaQN7LxCymVz1zL9hEjqeFYzxtxXz2wRK7CBtt71AFkRfHodu

	if id1.Less(id2) != false {
		t.Errorf("expected %q to be less than %q", id2, id1)
	}
}

func TestIDCmp(t *testing.T) {
	id1 := encode(strings.NewReader(`1`)) // c42yrSHMvUcscrQBssLhrRE28YpGUv9Gf95uH8KnwTiBv4odDbVqNnCYFs3xpsLrgVZfHebSaQQsvxgDGmw5CX1fVy
	id2 := encode(strings.NewReader(`2`)) // c42i2hTBA9Ej4nqEo9iUy3pJRRE53KAH9RwwMSWjmfaQN7LxCymVz1zL9hEjqeFYzxtxXz2wRK7CBtt71AFkRfHodu

	// is.Equal(id1.Cmp(id2), 1)
	if id1.Cmp(id2) != 1 {
		t.Errorf("Incorrect comparison between %q, %q", id1, id2)
	}

	if id2.Cmp(id1) != -1 {
		t.Errorf("Incorrect comparison between %q, %q", id2, id1)
	}

	if id1.Cmp(id1) != 0 {
		t.Errorf("Incorrect comparison between %q, %q", id1, id1)
	}

}

func TestCompareIDs(t *testing.T) {

	for _, test := range []struct {
		Id_A *c4.ID
		Id_B *c4.ID
		Exp  int
	}{
		{

			Id_A: encode(strings.NewReader("Test string")),
			Id_B: encode(strings.NewReader("Test string")),
			Exp:  0,
		},
		{
			Id_A: encode(strings.NewReader("Test string A")),
			Id_B: encode(strings.NewReader("Test string B")),
			Exp:  -1,
		},
		{
			Id_A: encode(strings.NewReader("Test string B")),
			Id_B: encode(strings.NewReader("Test string A")),
			Exp:  1,
		},
		{
			Id_A: encode(strings.NewReader("Test string")),
			Id_B: nil,
			Exp:  -1,
		},
	} {
		if test.Id_A.Cmp(test.Id_B) != test.Exp {
			t.Errorf("Incorrect comparison between %q, %q", test.Id_A, test.Id_B)
		}
	}

}

func TestBytesToID(t *testing.T) {

	d := c4.Digest([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 58})
	id := d.ID()
	if id.String() != "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121" {
		t.Errorf("IDs don't match, got %q, expcted %q", id.String(), "c41111111111111111111111111111111111111111111111111111111111111111111111111111111111111121")
	}

}

// func TestSum(t *testing.T) {
// 	is := is.New(t)

// 	id1 := c4.Identify(strings.NewReader("foo"))
// 	id2 := c4.Identify(strings.NewReader("bar"))

// 	is.True(id2.Less(id1))

// 	bts := append(id2.Digest(), id1.Digest()...)
// 	expectedSum := c4.Identify(bts)

// 	testSum := id1.Digest().Sum(id2.Digest())
// 	is.Equal(expectedSum, testSum)
// }

func TestNILID(t *testing.T) {

	// ID of nothing constant
	nilid := c4.NIL_ID

	if nilid.String() != "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT" {
		t.Errorf("IDs don't match, got %q, expcted %q", nilid.String(), "c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT")
	}
}
