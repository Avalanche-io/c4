package id_test

import (
	"encoding/json"
	"math/big"
	"testing"

	c4 "github.com/avalanche-io/c4/id"
	"github.com/cheekybits/is"
)

func TestMarshalJSON(t *testing.T) {
	is := is.New(t)

	type testType struct {
		Name string `json:"name"`
		ID   *c4.ID `json:"id"`
	}

	big_empty := big.NewInt(0)
	for _, test := range []struct {
		In  testType
		Exp string
	}{
		{
			In:  testType{"Test", c4.NIL_ID},
			Exp: `{"name":"Test","id":"c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT"}`,
		},
		{
			In:  testType{"Test", nil},
			Exp: `{"name":"Test","id":null}`,
		},
		{
			In:  testType{"Test", (*c4.ID)(big_empty)},
			Exp: `{"name":"Test","id":""}`,
		},
	} {
		actual, err := json.Marshal(test.In)
		is.NoErr(err)
		is.Equal(string(actual), test.Exp)
	}
}

func TestUnarshalJSON(t *testing.T) {
	is := is.New(t)

	type testType struct {
		Name string `json:"name"`
		ID   *c4.ID `json:"id"`
	}

	for _, test := range []struct {
		In  string
		Exp testType
	}{
		{
			In:  `{"name":"Test","id":"c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT"}`,
			Exp: testType{"Test", c4.NIL_ID},
		},
		{
			In:  `{"name":"Test","id":null}`,
			Exp: testType{"Test", nil},
		},
	} {

		testObject := testType{}
		err := json.Unmarshal([]byte(test.In), &testObject)
		is.NoErr(err)

		is.Equal(testObject, test.Exp)
	}
}
