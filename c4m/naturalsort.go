package c4m

import (
	"strings"
)

// NaturalLess compares two strings using natural sorting
// Natural sorting handles numeric sequences intelligently:
// - "file2.txt" comes before "file10.txt"
// - "render.1.exr" comes before "render.01.exr" (equal value, shorter first)
//
// Digit runs compare by integer value of any length; nothing is parsed
// into a machine integer, so a 20-digit run cannot wrap and two
// different runs can never compare equal.
func NaturalLess(a, b string) bool {
	segsA := segmentString(a)
	segsB := segmentString(b)

	minLen := len(segsA)
	if len(segsB) < minLen {
		minLen = len(segsB)
	}

	for i := 0; i < minLen; i++ {
		segA := segsA[i]
		segB := segsB[i]

		switch {
		case segA.isNumeric && segB.isNumeric:
			if c := compareDigitRuns(segA.text, segB.text); c != 0 {
				return c < 0
			}
		case segA.isNumeric != segB.isNumeric:
			// Mixed types - text sorts before numeric (per spec)
			return !segA.isNumeric
		default:
			// Both text - UTF-8 comparison
			if segA.text != segB.text {
				return segA.text < segB.text
			}
		}
	}

	// All segments equal - shorter string first
	return len(segsA) < len(segsB)
}

// compareDigitRuns orders two runs of ASCII digits as integers of any
// length: leading zeros are dropped, a shorter remainder is the smaller
// value, equal lengths compare digit by digit, and equal values put the
// shorter spelling first ("1" < "01"). Returns -1, 0 or 1.
func compareDigitRuns(a, b string) int {
	ta := strings.TrimLeft(a, "0")
	tb := strings.TrimLeft(b, "0")
	switch {
	case len(ta) != len(tb):
		return lessInt(len(ta), len(tb))
	case ta != tb:
		if ta < tb {
			return -1
		}
		return 1
	case len(a) != len(b):
		return lessInt(len(a), len(b))
	}
	return 0
}

func lessInt(a, b int) int {
	if a < b {
		return -1
	}
	return 1
}

// segment represents a text or numeric segment of a string
type segment struct {
	text      string
	isNumeric bool
}

// segmentString splits a string into alternating text/numeric segments
func segmentString(s string) []segment {
	if s == "" {
		return nil
	}

	var segments []segment
	var current strings.Builder
	var isNumeric bool
	var firstChar = true

	for _, r := range s {
		isDigit := r >= '0' && r <= '9'

		if firstChar {
			firstChar = false
			isNumeric = isDigit
			current.WriteRune(r)
		} else if isDigit != isNumeric {
			// Transition between text and numeric
			segments = append(segments, segment{text: current.String(), isNumeric: isNumeric})

			// Start new segment
			current.Reset()
			current.WriteRune(r)
			isNumeric = isDigit
		} else {
			current.WriteRune(r)
		}
	}

	// Add final segment
	if current.Len() > 0 {
		segments = append(segments, segment{text: current.String(), isNumeric: isNumeric})
	}

	return segments
}
