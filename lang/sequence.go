package lang

import "sort"

type Ordered []string

func (n Ordered) Len() int      { return len(n) }
func (n Ordered) Swap(i, j int) { n[i], n[j] = n[j], n[i] }

// This 'natural' sort is about 20% faster than "vbom.ml/util/sortorder", but still
// 3x the time of the typical 'lexicographical' sort.
// BenchmarkSort-8           	   10000	    209676 ns/op
// BenchmarkNaturalOrder-8   	    2000	    624033 ns/op
// BenchmarkVbomOrder-8      	    2000	    759521 ns/op
func (n Ordered) Less(i, j int) bool {
	// The following is a slightly improved version of "vbom.ml/util/sortorder"
	left, right := n[i], n[j]
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

func NaturalSort(list []string) {
	sort.Sort(Ordered(list))
}

// NaturalCompare functions similarly to the strings.Compare function but for natural
// sorting order.   The result will be 0 if a==b, -1 if a < b, and +1 if a > b.
func NaturalCompare(a, b string) int {
	// The following is a slightly improved version of "vbom.ml/util/sortorder"
	left, right := a, b
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
				if left[l] < right[r] {
					return -1
				}
				return 1
			}
			return 1
		}
		if right[r] > '9' {
			return -1
		}
		if left[l] < '0' {
			if right[r] < '0' {
				if left[l] == right[r] {
					l++
					r++
					continue
				}
				if left[l] < right[r] {
					return -1
				}
				return 1
			}
			return 1
		}
		if right[r] < '0' {
			return -1
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
			if ll < lr {
				return -1
			}
			return 1
		}
		// string compare equal length numbers
		if nr1, nr2 := left[zl:l], right[zr:r]; nr1 != nr2 {
			if nr1 < nr2 {
				return -1
			}
			return 1
		}
		// same number but different leading zeros size
		if zl != zr {
			if zl < zr {
				return -1
			}
			return 1
		}
		// meh they are the same number, loop around and try again.
	}
	switch {
	case lend < rend:
		return -1
	case lend > rend:
		return 1
	}
	return 0
}
