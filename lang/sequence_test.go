package lang_test

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"

	"github.com/cheekybits/is"

	"github.com/Avalanche-io/c4/lang"
	"vbom.ml/util/sortorder"
)

// updated, delete me

func TestNaturalOrderBasics(t *testing.T) {
	is := is.New(t)
	size := 22
	format := "%04d"
	list := shuffle(format, size)
	olist := lang.Ordered(list)
	j := 0
	for i := range list {
		j = i + 1
		if j == size {
			break
		}
	}
	sort.Sort(olist)
	for i := range list {
		is.Equal(olist[i], fmt.Sprintf(format, i))
	}
}

// tests from "vbom.ml/util/sortorder"
func TestNaturalLess(t *testing.T) {
	is := is.New(t)
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
		list := lang.Ordered([]string{test.s1, test.s2})
		is.Equal(list.Less(0, 1), test.less)
	}
}

func TestNaturalSort(t *testing.T) {
	is := is.New(t)
	size := 100
	format := "foo_%06d_bar.baz"
	list := shuffle(format, size)
	lang.NaturalSort(list)
	for i, name := range list {
		is.Equal(name, fmt.Sprintf(format, i))
	}
}

func TestNaturalOrder(t *testing.T) {
	is := is.New(t)

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
		is.Equal(test.s, !different)
		sort.Sort(lang.Ordered(testList))

		for i, name := range natOrdered {
			is.Equal(testList[i], name)
		}
	}
}

func TestShuffle(t *testing.T) {
	is := is.New(t)
	size := 1000
	list := shuffle("%d", size)
	same_true := 0
	for i, name := range list {
		d, err := strconv.Atoi(name)
		is.NoErr(err)
		is.True(d >= 0)
		if d == i {
			same_true++
		}
	}
	is.True(same_true < (size / 100))
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

// BenchmarkSort-8           	   10000	    209676 ns/op
// BenchmarkNaturalOrder-8   	    2000	    624033 ns/op
// BenchmarkVbomOrder-8      	    2000	    759521 ns/op

func BenchmarkSort(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		list := shuffle("name.%d.ext", 1000)
		b.StartTimer()
		sort.Strings(list)
	}
}

func BenchmarkNaturalOrder(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		list := lang.Ordered(shuffle("name.%d.ext", 1000))
		b.StartTimer()
		sort.Sort(list)
	}
}

func BenchmarkVbomOrder(b *testing.B) {
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		list := sortorder.Natural(shuffle("name.%d.ext", 1000))
		b.StartTimer()
		sort.Sort(list)
	}
}
