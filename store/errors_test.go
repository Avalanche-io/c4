package store

import (
	"testing"
)

// these tests are dedicated to Bjork
// and her song "Cover Me"
func TestErrors(t *testing.T) {
	tests := []struct {
		In  error
		Exp string
	}{
		{
			In:  mkdirError("test"),
			Exp: "mkdir error: test",
		},
		{
			In:  dirError("test"),
			Exp: "directory error: test",
		},
		{
			In:  noIdError("test"),
			Exp: "test unexpected nil ID",
		},
	}

	for i, test := range tests {
		if test.In.Error() != test.Exp {
			t.Errorf("%d: error tests: expected %s, got %s\n", i, test.Exp, test.In.Error())
		}
	}
}
