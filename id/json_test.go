package id_test

import (
	"encoding/json"
	"math/big"
	"testing"

	c4 "github.com/Avalanche-io/c4/id"
)

func TestMarshalJSON(t *testing.T) {

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
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}
		if string(actual) != test.Exp {
			t.Errorf("results do not match got %q, expected %q", string(actual), test.Exp)
		}
	}
}

func TestUnarshalJSON(t *testing.T) {

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
		if err != nil {
			t.Errorf("unexpected error %q", err)
		}

		if testObject.ID == nil {
			if test.Exp.ID != nil {
				t.Errorf("results do not match got %v, expected %v", testObject, test.Exp)
			}
		} else if testObject.Name != test.Exp.Name || testObject.ID.String() != test.Exp.ID.String() {
			t.Errorf("results do not match got %v, expected %v", testObject, test.Exp)
		}
	}
}
