package asset_test

import (
	// "bytes"

	"encoding/json"
	"testing"

	"github.com/cheekybits/is"
	"github.com/etcenter/c4/asset"
)

func TestMarshalJSON(t *testing.T) {
	is := is.New(t)

	type testType struct {
		Name string    `json:"name"`
		ID   *asset.ID `json:"id"`
	}

	for _, test := range []struct {
		In  testType
		Exp string
	}{
		{
			In:  testType{"Test", asset.NIL_ID},
			Exp: `{"name":"Test","id":"c459CSJESBh38BxDwwxNFKTXE4cC9HASGe3bhtN6z58GbwLqpCyRaKyZSvBAvTdF5NpSTPdUMH4hHRJ75geLsB1Sfs"}`,
		},
		{
			In:  testType{"Test", nil},
			Exp: `{"name":"Test","id":null}`,
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
		Name string    `json:"name"`
		ID   *asset.ID `json:"id"`
	}

	for _, test := range []struct {
		In  string
		Exp testType
	}{
		{
			In:  `{"name":"Test","id":"c459CSJESBh38BxDwwxNFKTXE4cC9HASGe3bhtN6z58GbwLqpCyRaKyZSvBAvTdF5NpSTPdUMH4hHRJ75geLsB1Sfs"}`,
			Exp: testType{"Test", asset.NIL_ID},
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
