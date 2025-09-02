package c4m

import (
	"strings"
	"unicode"
)

// NaturalLess compares two strings using natural sorting
// Natural sorting handles numeric sequences intelligently:
// - "file2.txt" comes before "file10.txt"
// - "render.1.exr" comes before "render.01.exr" (equal value, shorter first)
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
		
		// Compare segments
		if segA.isNumeric && segB.isNumeric {
			// Both numeric - compare as integers
			if segA.numValue != segB.numValue {
				return segA.numValue < segB.numValue
			}
			// Equal values - shorter representation first
			if len(segA.text) != len(segB.text) {
				return len(segA.text) < len(segB.text)
			}
		} else if segA.isNumeric != segB.isNumeric {
			// Mixed types - compare as strings
			return segA.text < segB.text
		} else {
			// Both text - UTF-8 comparison
			if segA.text != segB.text {
				return segA.text < segB.text
			}
		}
	}
	
	// All segments equal - shorter string first
	return len(segsA) < len(segsB)
}

// segment represents a text or numeric segment of a string
type segment struct {
	text      string
	isNumeric bool
	numValue  int64
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
		isDigit := unicode.IsDigit(r)
		
		if firstChar {
			firstChar = false
			isNumeric = isDigit
			current.WriteRune(r)
		} else if isDigit != isNumeric {
			// Transition between text and numeric
			seg := segment{
				text:      current.String(),
				isNumeric: isNumeric,
			}
			if isNumeric {
				seg.numValue = parseNumber(seg.text)
			}
			segments = append(segments, seg)
			
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
		seg := segment{
			text:      current.String(),
			isNumeric: isNumeric,
		}
		if isNumeric {
			seg.numValue = parseNumber(seg.text)
		}
		segments = append(segments, seg)
	}
	
	return segments
}

// parseNumber converts a numeric string to int64
func parseNumber(s string) int64 {
	var result int64
	for _, r := range s {
		if unicode.IsDigit(r) {
			result = result*10 + int64(r-'0')
		}
	}
	return result
}