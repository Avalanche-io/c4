package naturalsort

// updated, delete me

type Strings []string

func (n Strings) Len() int      { return len(n) }
func (n Strings) Swap(i, j int) { n[i], n[j] = n[j], n[i] }

// This 'natural' sort is about 20% faster than "vbom.ml/util/sortorder", but still
// 3x the time of the typical 'lexicographical' sort.

// BenchmarkSort-8           	   10000	    209676 ns/op
// BenchmarkNaturalOrder-8   	    2000	    624033 ns/op
// BenchmarkVbomOrder-8      	    2000	    759521 ns/op

func (n Strings) Less(i, j int) bool {
	// The following is a slightly improved version of "vbom.ml/util/sortorder"
	left, right := n[i], n[j]
	l, r := 0, 0

	for l < len(left) && r < len(right) {

		// If the next character between left and right is above the set of numbers
		// then compare them directly

		// if left is greater then the set of numbers
		if left[l] > '9' {
			// .. and if right is greater then the set of numbers
			if right[r] > '9' {

				// .. then if they are equal move to the next character
				if left[l] == right[r] {
					l++
					r++
					continue
				}

				// .. otherwise return a direct comparison
				return left[l] < right[r]
			}
			// If right is lesser than equal to '9' than exit early, left is not
			// less than right
			return false
		} else if right[r] > '9' {
			return true
		}

		// If the next character between left and right is below the set of numbers
		// then compare them directly

		// if left is lesser then the set of numbers
		if left[l] < '0' {
			// if right is also lesser then the set of numbers
			if right[r] < '0' {
				// .. then continue if they are identical
				if left[l] == right[r] {
					l++
					r++
					continue
				}
				// .. otherwise return the direct comparison
				return left[l] < right[r]
			}
			return false
		} else if right[r] < '0' {
			return true
		}

		// lend := len(left) - 1
		// rend := len(right) - 1

		// consume leading zeros, for the left side
		for ; l < len(left) && left[l] == '0'; l++ {
		}

		// consume leading zeros, for the right side
		for ; r < len(right) && right[r] == '0'; r++ {
		}

		// after consuming leading zeros, find the end of any numbers found.
		zl, zr := l, r

		// find end of string of digits
		for ; l < len(left) && (left[l] <= '9' && left[l] >= '0'); l++ {
		}
		for ; r < len(right) && (right[r] <= '9' && right[r] >= '0'); r++ {
		}

		// exit early if one value is longer than the other
		if ll, lr := l-zl, r-zr; ll != lr {
			return ll < lr
		}

		// string compare equal length numbers
		if left[zl:l] != right[zr:r] {
			return left[zl:l] < right[zr:r]
		}
		// numbers are equal

		// same number but different leading zeros size causes character
		// pointers to be at different counts
		if l != r {
			return l < r
		}

		// meh they are the same number, loop around and try again.

	}

	return len(left) < len(right)
}
