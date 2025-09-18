package c4m

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
)

// SequenceDetector identifies and collapses file sequences
type SequenceDetector struct {
	minSequenceLength int
}

// NewSequenceDetector creates a detector with minimum sequence length
func NewSequenceDetector(minLength int) *SequenceDetector {
	if minLength < 2 {
		minLength = 2
	}
	return &SequenceDetector{
		minSequenceLength: minLength,
	}
}

// fileGroup represents a group of files that might form a sequence
type fileGroup struct {
	prefix  string
	suffix  string
	entries map[int]*Entry // frame number -> entry
	padding int            // number of digits in frame numbers
}

// DetectSequences finds and collapses sequences in a manifest
func (sd *SequenceDetector) DetectSequences(manifest *Manifest) *Manifest {
	result := NewManifest()
	result.Version = manifest.Version

	// Group files by prefix/suffix pattern
	groups := make(map[string]*fileGroup)
	processedIndices := make(map[int]bool)

	// Pattern to match frame numbers in filenames
	framePattern := regexp.MustCompile(`^(.*?)(\d+)(.*)$`)

	for i, entry := range manifest.Entries {
		// Skip directories and already processed entries
		if entry.IsDir() || processedIndices[i] {
			continue
		}

		// Try to extract frame number from filename
		basename := path.Base(entry.Name)
		matches := framePattern.FindStringSubmatch(basename)
		if matches == nil {
			// Not a numbered file, add as-is
			result.AddEntry(entry)
			processedIndices[i] = true
			continue
		}

		prefix := matches[1]
		frameStr := matches[2]
		suffix := matches[3]
		frameNum, err := strconv.Atoi(frameStr)
		if err != nil {
			// Shouldn't happen given the regex, but handle it
			result.AddEntry(entry)
			processedIndices[i] = true
			continue
		}

		// Get directory path
		dir := path.Dir(entry.Name)
		if dir == "." {
			dir = ""
		} else {
			dir = dir + "/"
		}

		// Create group key
		groupKey := fmt.Sprintf("%s|%s|%s|%d", dir, prefix, suffix, len(frameStr))

		// Get or create group
		group, exists := groups[groupKey]
		if !exists {
			group = &fileGroup{
				prefix:  dir + prefix,
				suffix:  suffix,
				entries: make(map[int]*Entry),
				padding: len(frameStr),
			}
			groups[groupKey] = group
		}

		// Add entry to group
		group.entries[frameNum] = entry
	}

	// Process each group to detect sequences
	for _, group := range groups {
		if len(group.entries) < sd.minSequenceLength {
			// Not enough files for a sequence
			for _, entry := range group.entries {
				result.AddEntry(entry)
			}
			continue
		}

		// Extract and sort frame numbers
		frames := make([]int, 0, len(group.entries))
		for frame := range group.entries {
			frames = append(frames, frame)
		}
		sort.Ints(frames)

		// Find continuous ranges
		ranges := sd.findRanges(frames)

		// Create sequence notation for continuous ranges
		for _, r := range ranges {
			if r.count >= sd.minSequenceLength {
				// Create sequence entry
				var pattern string
				if r.start == r.end {
					// Single frame (shouldn't happen with minLength check)
					pattern = fmt.Sprintf("%s%0*d%s", group.prefix, group.padding, r.start, group.suffix)
				} else {
					pattern = fmt.Sprintf("%s[%0*d-%0*d]%s",
						group.prefix, group.padding, r.start,
						group.padding, r.end, group.suffix)
				}

				// Use properties from first entry in range
				firstEntry := group.entries[r.start]
				seqEntry := &Entry{
					Name:       pattern,
					Mode:       firstEntry.Mode,
					Timestamp:  firstEntry.Timestamp,
					Size:       firstEntry.Size * int64(r.count), // Total size
					C4ID:       firstEntry.C4ID,                  // Note: This is placeholder
					Depth:      firstEntry.Depth,
					IsSequence: true,
					Pattern:    pattern,
				}
				result.AddEntry(seqEntry)
			} else {
				// Add individual files for small ranges
				for i := r.start; i <= r.end; i++ {
					if entry, ok := group.entries[i]; ok {
						result.AddEntry(entry)
					}
				}
			}
		}
	}

	// Add any remaining unprocessed entries
	for i, entry := range manifest.Entries {
		if !processedIndices[i] && entry.IsDir() {
			result.AddEntry(entry)
		}
	}

	return result
}

// sequenceRange represents a continuous range of frame numbers
type sequenceRange struct {
	start int
	end   int
	count int
}

// findRanges identifies continuous ranges in sorted frame numbers
func (sd *SequenceDetector) findRanges(frames []int) []sequenceRange {
	if len(frames) == 0 {
		return nil
	}

	var ranges []sequenceRange
	start := frames[0]
	end := frames[0]
	count := 1

	for i := 1; i < len(frames); i++ {
		if frames[i] == end+1 {
			// Continuous sequence
			end = frames[i]
			count++
		} else {
			// Gap in sequence, save current range
			ranges = append(ranges, sequenceRange{
				start: start,
				end:   end,
				count: count,
			})
			// Start new range
			start = frames[i]
			end = frames[i]
			count = 1
		}
	}

	// Add final range
	ranges = append(ranges, sequenceRange{
		start: start,
		end:   end,
		count: count,
	})

	return ranges
}

// CollapseToSequences processes a manifest to detect and collapse sequences
func CollapseToSequences(manifest *Manifest, minLength int) *Manifest {
	detector := NewSequenceDetector(minLength)
	return detector.DetectSequences(manifest)
}