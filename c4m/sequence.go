package c4m

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// Pattern to match sequence notation: [0001-0100], [01-50,75-100], [001,005,010], etc.
	sequencePattern = regexp.MustCompile(`\[([0-9,\-:]+)\]`)
)

// Sequence represents a file sequence pattern
type Sequence struct {
	Prefix  string
	Suffix  string
	Ranges  []Range
	Padding int // Number of digits for padding
}

// Range represents a numeric range in a sequence
type Range struct {
	Start int
	End   int
	Step  int
}

// ParseSequence parses a sequence pattern like "frame.[0001-0100].exr"
func ParseSequence(pattern string) (*Sequence, error) {
	matches := sequencePattern.FindStringSubmatchIndex(pattern)
	if matches == nil {
		return nil, fmt.Errorf("no sequence pattern found")
	}
	
	seq := &Sequence{
		Prefix: pattern[:matches[0]],
		Suffix: pattern[matches[1]:],
		Ranges: make([]Range, 0),
	}
	
	// Handle backslash-space in prefix (for filenames with spaces)
	seq.Prefix = strings.ReplaceAll(seq.Prefix, `\ `, " ")
	
	// Extract the range specification
	rangeSpec := pattern[matches[2]:matches[3]]
	
	// Split by comma for multiple ranges
	parts := strings.Split(rangeSpec, ",")
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		
		// Check for step notation (e.g., "0001-0100:2")
		var step = 1
		if idx := strings.Index(part, ":"); idx > 0 {
			stepStr := part[idx+1:]
			part = part[:idx]
			s, err := strconv.Atoi(stepStr)
			if err != nil {
				return nil, fmt.Errorf("invalid step value: %s", stepStr)
			}
			step = s
		}
		
		// Check for range (e.g., "0001-0100")
		if idx := strings.Index(part, "-"); idx > 0 {
			startStr := part[:idx]
			endStr := part[idx+1:]
			
			// Detect padding from the start value
			if seq.Padding == 0 {
				seq.Padding = len(startStr)
			}
			
			start, err := strconv.Atoi(startStr)
			if err != nil {
				return nil, fmt.Errorf("invalid start value: %s", startStr)
			}
			
			end, err := strconv.Atoi(endStr)
			if err != nil {
				return nil, fmt.Errorf("invalid end value: %s", endStr)
			}
			
			if start > end {
				return nil, fmt.Errorf("start %d > end %d", start, end)
			}
			
			seq.Ranges = append(seq.Ranges, Range{
				Start: start,
				End:   end,
				Step:  step,
			})
		} else {
			// Single frame
			if seq.Padding == 0 {
				seq.Padding = len(part)
			}
			
			frame, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid frame number: %s", part)
			}
			
			seq.Ranges = append(seq.Ranges, Range{
				Start: frame,
				End:   frame,
				Step:  1,
			})
		}
	}
	
	return seq, nil
}

// Expand returns all filenames in the sequence
func (s *Sequence) Expand() []string {
	var files []string
	
	for _, r := range s.Ranges {
		for i := r.Start; i <= r.End; i += r.Step {
			// Format with padding
			numStr := fmt.Sprintf("%0*d", s.Padding, i)
			filename := s.Prefix + numStr + s.Suffix
			files = append(files, filename)
		}
	}
	
	return files
}

// Count returns the total number of files in the sequence
func (s *Sequence) Count() int {
	count := 0
	for _, r := range s.Ranges {
		count += (r.End-r.Start)/r.Step + 1
	}
	return count
}

// Contains checks if a frame number is in the sequence
func (s *Sequence) Contains(frame int) bool {
	for _, r := range s.Ranges {
		if frame >= r.Start && frame <= r.End {
			if (frame-r.Start)%r.Step == 0 {
				return true
			}
		}
	}
	return false
}

// IsSequence checks if a filename pattern contains sequence notation
func IsSequence(pattern string) bool {
	return sequencePattern.MatchString(pattern)
}

// ExpandSequencePattern is a convenience function to expand a pattern
func ExpandSequencePattern(pattern string) ([]string, error) {
	seq, err := ParseSequence(pattern)
	if err != nil {
		return nil, err
	}
	return seq.Expand(), nil
}