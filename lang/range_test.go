package lang

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		Input    string
		Expected Ranger
		Error    error
	}{
		{"[1-10]", basic{1, 10, 1, false}, nil},
		{"[1-10/2]", basic{1, 10, 2, false}, nil},
		{"[1-10/2^]", basic{1, 10, 2, true}, nil},
		{"[-10--20]", basic{-10, -20, -1, false}, nil},
		{"[20-10]", basic{20, 10, -1, false}, nil},
		{"[5--6]", basic{5, -6, -1, false}, nil},
		{"[1, 2, 3]", list{[]int64{1, 2, 3}, false}, nil},
		{"[5, 10, 12^]", list{[]int64{5, 10, 12}, true}, nil},
	}
	for i, test := range tests {
		got, err := ParseRange(strings.NewReader(test.Input))
		if err != test.Error {
			t.Errorf("FAIL: test %d: %q got: %v expected: %v\n", i, test.Input, err, test.Error)
		}
		if !reflect.DeepEqual(got, test.Expected) {
			t.Errorf("FAIL: test %d: %q got: %v expected: %v\n", i, test.Input, got, test.Expected)
		}
	}
}

func TestRangeIterate(t *testing.T) {
	tests := []struct {
		Input    Ranger
		Expected []int64
	}{
		{basic{1, 10, 1, false}, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}},
		{basic{1, 10, 2, false}, []int64{1, 3, 5, 7, 9}},
		{basic{1, 10, 2, true}, []int64{1, 5, 9, 3, 7}},
		{basic{-10, -20, -1, false}, []int64{-10, -11, -12, -13, -14, -15, -16, -17, -18, -19, -20}},
		{basic{20, 10, -1, false}, []int64{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10}},
		{basic{5, -6, -1, false}, []int64{5, 4, 3, 2, 1, 0, -1, -2, -3, -4, -5, -6}},
		{list{[]int64{1, 2, 3}, false}, []int64{1, 2, 3}},
		{list{[]int64{5, 10, 12}, true}, []int64{5, 10, 12}},
	}
	for i, test := range tests {
		got := []int64{}
		for n := range test.Input.Iterate(nil) {
			got = append(got, n)
		}
		if !reflect.DeepEqual(got, test.Expected) {
			t.Errorf("FAIL: test %d: %v got: %v expected: %v\n", i, test.Input, got, test.Expected)
		}
	}
}
