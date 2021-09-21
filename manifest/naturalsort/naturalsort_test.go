package naturalsort_test

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"

	natural "github.com/Avalanche-io/c4/manifest/naturalsort"
)

// updated, delete me

func TestBasics(t *testing.T) {

	size := 22
	format := "%04d"
	list := shuffle(format, size)
	olist := natural.Strings(list)
	j := 0
	for i := range list {
		j = i + 1
		if j == size {
			break
		}
	}
	sort.Sort(olist)
	for i := range list {
		if olist[i] != fmt.Sprintf(format, i) {
			t.Fatalf("error expected %q to equal %q", olist[i], fmt.Sprintf(format, i))
		}
	}
}

// tests from "vbom.ml/util/sortorder"
func TestLess(t *testing.T) {

	tests := []struct {
		s1, s2 string
		less   bool
	}{
		{"0", "00", true},
		{"00", "0", false},
		{"aa", "ab", true},
		{"ab", "abc", true},
		{"abc", "ad", true},
		{"ab1", "ab2", true},
		{"ab1c", "ab1c", false},
		{"ab12", "abc", true},
		{"ab2a", "ab10", true},
		{"a0001", "a0000001", true},
		{"a10", "abcdefgh2", true},
		{"аб2аб", "аб10аб", true},
		{"2аб", "3аб", true},
		//
		{"a1b", "a01b", true},
		{"a01b", "a1b", false},
		{"ab01b", "ab010b", true},
		{"ab010b", "ab01b", false},
		{"a01b001", "a001b01", true},
		{"a001b01", "a01b001", false},
		{"a1", "a1x", true},
		{"1ax", "1b", true},
		{"1b", "1ax", false},
		//
		{"082", "83", true},
		//
		{"083a", "9a", false},
		{"9a", "083a", true},
	}
	for _, test := range tests {
		list := natural.Strings([]string{test.s1, test.s2})
		if list.Less(0, 1) != test.less {
			if test.less {
				t.Fatalf("error expected %q < %q but got >=", test.s1, test.s2)
			} else {
				t.Fatalf("error expected %q >= %q but got <", test.s1, test.s2)
			}
		}
	}
}

func TestNaturalSort(t *testing.T) {
	size := 100
	format := "foo_%06d_bar.baz"
	list := natural.Strings(shuffle(format, size))
	sort.Sort(list)

	for i, name := range list {
		if name != fmt.Sprintf(format, i) {
			t.Fatalf("error expected values to be equal %q %q", name, fmt.Sprintf(format, i))
		}
	}
}

func TestStrings(t *testing.T) {

	tests := []struct {
		f string
		c int
		s bool
		r int
	}{
		{"%d", 23, false, 1},
		{"filename.%d.ext", 23, false, 1},
		{"filename.%04d.ext", 23, true, 1},
		{"filename%d.ext", 23, false, 1},
		{"filename %d", 23, false, 1},
		{"filename %d .3 .ext", 23, false, 1},
		{"filename-3123_%d.ext", 23, false, 1},
		{"filename-%d_%d.ext", 23, false, 2},
	}
	for _, test := range tests {
		max := test.c
		set_range := max
		if test.r > 1 {
			max *= 3
		}
		lexOrdered := make([]string, max)
		natOrdered := make([]string, max)
		testList := make([]string, max)
		switch test.r {
		case 1:
			for i := 0; i < set_range; i++ {
				natOrdered[i] = fmt.Sprintf(test.f, i)
				lexOrdered[i] = natOrdered[i]
			}
		case 2:
			k := 0
			for j := 0; j < 3; j++ {
				for i := 0; i < set_range; i++ {
					natOrdered[k] = fmt.Sprintf(test.f, j, i)
					lexOrdered[k] = natOrdered[k]
					k++
				}
			}

		}

		sort.Strings(lexOrdered)
		different := false
		for i, name := range lexOrdered {
			if natOrdered[i] != name {
				different = true
			}
			testList[i] = name
		}
		if test.s != !different {
			t.Fatalf("expected values to match %T %T", test.s, !different)
		}
		sort.Sort(natural.Strings(testList))

		for i, name := range natOrdered {
			if testList[i] != name {
				t.Fatalf("expected values to match %q, %q", testList[i], name)
			}
		}
	}
}

func TestShuffle(t *testing.T) {
	size := 1000
	list := shuffle("%d", size)
	same_true := 0
	for i, name := range list {
		d, err := strconv.Atoi(name)
		if err != nil {
			t.Fatalf("unexpected error %s", err)
		}
		if d < 0 {
			t.Fatalf("expected value to be greater than zero %d", d)
		}
		if d == i {
			same_true++
		}
	}
	if same_true >= (size / 100) {
		t.Fatalf("expected value to be less %d >= %d", same_true, size/100)
	}
}

func shuffle(format string, size int) []string {
	list := make([]string, size)
	for j, i := 0, 0; i < size; i++ {
		if i > 0 {
			j = int(rand.Int31n(int32(i)))
		}
		if j != i {
			list[i] = list[j]
		}
		list[j] = fmt.Sprintf(format, i)
	}
	return list
}

// BenchmarkSort-8                 	 3000000	       646 ns/op
// BenchmarkNaturalSort-8         	 1000000	      1327 ns/op

func BenchmarkSort(b *testing.B) {
	b.StopTimer()
	list := shuffle("name.%d.ext", b.N)
	b.StartTimer()

	sort.Strings(list)
}

func BenchmarkNaturalSort(b *testing.B) {
	b.StopTimer()
	list := natural.Strings(shuffle("name.%d.ext", b.N))
	b.StartTimer()

	sort.Sort(list)
}

