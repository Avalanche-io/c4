package os

import (
	"bytes"
	"math/rand"
	"os"
	"sort"
)

type Sorting uint8

const (
	Unsorted Sorting = iota
	NaturalSort
	LexSort
	ValueSort
)

type naturalFileInfoList []os.FileInfo

func (d naturalFileInfoList) Len() int             { return len(d) }
func (d naturalFileInfoList) Swap(i, j int)        { d[i], d[j] = d[j], d[i] }
func (d naturalFileInfoList) At(i int) os.FileInfo { return d[i] }

func (n naturalFileInfoList) Less(i, j int) bool {
	// The following is a slightly improved version of "vbom.ml/util/sortorder"
	left, right := n[i].Name(), n[j].Name()
	l, r := 0, 0
	lend := len(left) - 1
	rend := len(right) - 1
	for l <= lend && r <= rend {
		if left[l] > '9' {
			if right[r] > '9' {
				if left[l] == right[r] {
					l++
					r++
					continue
				}
				return left[l] < right[r]
			}
			return false
		}
		if right[r] > '9' {
			return true
		}
		if left[l] < '0' {
			if right[r] < '0' {
				if left[l] == right[r] {
					l++
					r++
					continue
				}
				return left[l] < right[r]
			}
			return false
		}
		if right[r] < '0' {
			return true
		}

		// leading '0'
		for ; l < lend && left[l] == '0'; l++ {
		}
		for ; r < rend && right[r] == '0'; r++ {
		}
		// number range
		zl, zr := l, r
		for ; l <= lend && (left[l] <= '9' && left[l] >= '0'); l++ {
		}
		for ; r <= rend && (right[r] <= '9' && right[r] >= '0'); r++ {
		}

		// the longer number is larger
		if ll, lr := l-zl, r-zr; ll != lr {
			return ll < lr
		}
		// string compare equal length numbers
		if nr1, nr2 := left[zl:l], right[zr:r]; nr1 != nr2 {
			return nr1 < nr2
		}
		// same number but different leading zeros size
		if zl != zr {
			return zl < zr
		}
		// meh they are the same number, loop around and try again.
	}
	return lend < rend
}

type lexFileInfoList []os.FileInfo

func (d lexFileInfoList) Len() int             { return len(d) }
func (d lexFileInfoList) Swap(i, j int)        { d[i], d[j] = d[j], d[i] }
func (d lexFileInfoList) Less(i, j int) bool   { return d[i].Name() < d[j].Name() }
func (d lexFileInfoList) At(i int) os.FileInfo { return d[i] }

type valueFileInfoList []os.FileInfo

func (d valueFileInfoList) Len() int      { return len(d) }
func (d valueFileInfoList) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
func (d valueFileInfoList) Less(i, j int) bool {
	return bytes.Compare([]byte(d[i].Name()), []byte(d[j].Name())) == -1
}
func (d valueFileInfoList) At(i int) os.FileInfo { return d[i] }

type randFileInfoList []os.FileInfo

func (d randFileInfoList) Len() int             { return len(d) }
func (d randFileInfoList) Swap(i, j int)        { d[i], d[j] = d[j], d[i] }
func (d randFileInfoList) Less(i, j int) bool   { return rand.NormFloat64() < 0 }
func (d randFileInfoList) At(i int) os.FileInfo { return d[i] }

type nilSort []os.FileInfo

func (d nilSort) Len() int             { return len(d) }
func (d nilSort) Swap(i, j int)        { return }
func (d nilSort) Less(i, j int) bool   { return true }
func (d nilSort) At(i int) os.FileInfo { return d[i] }

type Sortable interface {
	sort.Interface
	At(int) os.FileInfo
}
