package c4m

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

var (
	// sequencePattern matches sequence notation: [0001-0100], [01-50,75-100], [001,005,010], etc.
	sequencePattern = regexp.MustCompile(`\[([0-9,\-:]+)\]`)
)

// unescapeSequenceNotation resolves all backslash escapes defined for
// unquoted sequence patterns: \ →space, \[→[, \]→], \\→\, \"→", \,→,, \-→-.
func unescapeSequenceNotation(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case ' ', '[', ']', '\\', '"', ',', '-':
				buf.WriteByte(s[i+1])
				i++
				continue
			}
		}
		buf.WriteByte(s[i])
	}
	return buf.String()
}

// escapeSequenceNotation escapes characters in a sequence prefix or suffix
// that require backslash escaping per the spec: space, brackets, backslash, quote.
// Commas and hyphens are not escaped outside range notation (they are unambiguous).
func escapeSequenceNotation(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			buf.WriteString(`\\`)
		case ' ':
			buf.WriteString(`\ `)
		case '"':
			buf.WriteString(`\"`)
		case '[':
			buf.WriteString(`\[`)
		case ']':
			buf.WriteString(`\]`)
		default:
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

// ----------------------------------------------------------------------------
// Sequence Types
// ----------------------------------------------------------------------------

// Sequence represents a file sequence pattern like "frame.[0001-0100].exr"
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

// SequenceExpansionMode defines how sequences should be expanded
type SequenceExpansionMode int

const (
	// SequenceEmbedded expands sequences as layers within the same manifest
	SequenceEmbedded SequenceExpansionMode = iota
	// SequenceStandalone expands sequences into a separate manifest file
	SequenceStandalone
)

// SequenceExpander handles expansion of sequence notation into individual entries
type SequenceExpander struct {
	mode SequenceExpansionMode
}

// SequenceDetector identifies and collapses file sequences
type SequenceDetector struct {
	minSequenceLength int
}

// fileGroup represents a group of files that might form a sequence
type fileGroup struct {
	prefix  string
	suffix  string
	entries map[int]*Entry // frame number -> entry
	padding int            // number of digits in frame numbers
}

// sequenceRange represents a continuous range of frame numbers
type sequenceRange struct {
	start int
	end   int
	count int
}

// ----------------------------------------------------------------------------
// Constructors
// ----------------------------------------------------------------------------

// NewSequenceExpander creates a new sequence expander with the given mode
func NewSequenceExpander(mode SequenceExpansionMode) *SequenceExpander {
	return &SequenceExpander{mode: mode}
}

// NewSequenceDetector creates a detector with minimum sequence length
func NewSequenceDetector(minLength int) *SequenceDetector {
	if minLength < 2 {
		minLength = 2
	}
	return &SequenceDetector{minSequenceLength: minLength}
}

// ----------------------------------------------------------------------------
// Sequence Parsing
// ----------------------------------------------------------------------------

// ParseSequence parses a sequence pattern like "frame.[0001-0100].exr"
func ParseSequence(pattern string) (*Sequence, error) {
	matches := sequencePattern.FindStringSubmatchIndex(pattern)
	if matches == nil {
		return nil, fmt.Errorf("no sequence pattern found")
	}

	seq := &Sequence{
		Prefix: unescapeSequenceNotation(pattern[:matches[0]]),
		Suffix: unescapeSequenceNotation(pattern[matches[1]:]),
		Ranges: make([]Range, 0),
	}

	// Extract the range specification
	rangeSpec := pattern[matches[2]:matches[3]]

	// Split by comma for multiple ranges
	parts := strings.Split(rangeSpec, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check for step notation (e.g., "0001-0100:2")
		step := 1
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

	seq.normalizeRanges()
	return seq, nil
}

// normalizeRanges reduces an arbitrary set of ranges to canonical form.
// Any combination of ranges, stepped ranges, and individual frames is expanded
// to frame numbers, then re-derived as:
//  1. Maximal contiguous (step=1) runs — always preferred
//  2. Stepped arithmetic progressions of 3+ remaining frames
//  3. Individual frames for anything left
//
// The result is deterministic, sorted by start value, and minimal.
func (s *Sequence) normalizeRanges() {
	if len(s.Ranges) <= 1 {
		return
	}

	// Fast path: all step-1 ranges can be merged without expansion.
	allStep1 := true
	for _, r := range s.Ranges {
		if r.Step != 1 {
			allStep1 = false
			break
		}
	}
	if allStep1 {
		s.mergeContiguousRanges()
		s.collapseSingles()
		return
	}

	// General case: expand to frame numbers and re-derive.
	frames := s.frames()
	s.Ranges = framesToRanges(frames)
}

// mergeContiguousRanges is the fast path for all-step-1 ranges:
// sort by start, merge overlapping or adjacent.
func (s *Sequence) mergeContiguousRanges() {
	sort.Slice(s.Ranges, func(i, j int) bool {
		return s.Ranges[i].Start < s.Ranges[j].Start
	})
	merged := s.Ranges[:1]
	for _, next := range s.Ranges[1:] {
		cur := &merged[len(merged)-1]
		if next.Start <= cur.End+1 {
			if next.End > cur.End {
				cur.End = next.End
			}
		} else {
			merged = append(merged, next)
		}
	}
	s.Ranges = merged
}

// collapseSingles finds stepped arithmetic progressions among single-frame
// ranges left over after contiguous merging. Requires at least 3 frames to
// form a stepped range.
func (s *Sequence) collapseSingles() {
	if len(s.Ranges) < 3 {
		return
	}

	var fixed []Range
	var singles []int
	for _, r := range s.Ranges {
		if r.Start == r.End {
			singles = append(singles, r.Start)
		} else {
			fixed = append(fixed, r)
		}
	}
	if len(singles) < 3 {
		return
	}

	// Greedy left-to-right stepped detection among sorted singles.
	var detected []Range
	i := 0
	for i < len(singles) {
		if i+2 < len(singles) {
			step := singles[i+1] - singles[i]
			j := i + 2
			for j < len(singles) && singles[j] == singles[j-1]+step {
				j++
			}
			if j-i >= 3 {
				detected = append(detected, Range{Start: singles[i], End: singles[j-1], Step: step})
				i = j
				continue
			}
		}
		detected = append(detected, Range{Start: singles[i], End: singles[i], Step: 1})
		i++
	}

	result := append(fixed, detected...)
	sort.Slice(result, func(a, b int) bool {
		return result[a].Start < result[b].Start
	})
	s.Ranges = result
}

// frames expands all ranges to a sorted, deduplicated slice of frame numbers.
func (s *Sequence) frames() []int {
	var out []int
	for _, r := range s.Ranges {
		step := r.Step
		if step < 1 {
			step = 1
		}
		for i := r.Start; i <= r.End; i += step {
			out = append(out, i)
		}
	}
	sort.Ints(out)
	// Deduplicate in-place.
	if len(out) > 1 {
		j := 1
		for i := 1; i < len(out); i++ {
			if out[i] != out[i-1] {
				out[j] = out[i]
				j++
			}
		}
		out = out[:j]
	}
	return out
}

// framesToRanges converts a sorted, deduplicated slice of frame numbers to
// canonical Range form.
func framesToRanges(frames []int) []Range {
	if len(frames) == 0 {
		return nil
	}

	// Phase 1: extract maximal contiguous (step=1) runs.
	var result []Range
	var singles []int

	i := 0
	for i < len(frames) {
		j := i + 1
		for j < len(frames) && frames[j] == frames[j-1]+1 {
			j++
		}
		if j-i >= 2 {
			result = append(result, Range{Start: frames[i], End: frames[j-1], Step: 1})
		} else {
			singles = append(singles, frames[i])
		}
		i = j
	}

	// Phase 2: among remaining singletons, find stepped progressions.
	// Greedy left-to-right: the step is implied by the first two elements.
	// A stepped range must have at least 3 frames to be worth encoding.
	i = 0
	for i < len(singles) {
		if i+2 < len(singles) {
			step := singles[i+1] - singles[i]
			j := i + 2
			for j < len(singles) && singles[j] == singles[j-1]+step {
				j++
			}
			if j-i >= 3 {
				result = append(result, Range{Start: singles[i], End: singles[j-1], Step: step})
				i = j
				continue
			}
		}
		result = append(result, Range{Start: singles[i], End: singles[i], Step: 1})
		i++
	}

	sort.Slice(result, func(a, b int) bool {
		return result[a].Start < result[b].Start
	})
	return result
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

// ----------------------------------------------------------------------------
// Sequence Methods
// ----------------------------------------------------------------------------

// Expand returns all filenames in the sequence
func (s *Sequence) Expand() []string {
	var files []string

	for _, r := range s.Ranges {
		for i := r.Start; i <= r.End; i += r.Step {
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

// ----------------------------------------------------------------------------
// Sequence Expansion
// ----------------------------------------------------------------------------

// ExpandManifest processes a manifest and expands all sequence entries
func (se *SequenceExpander) ExpandManifest(manifest *Manifest) (*Manifest, *Manifest, error) {
	expanded := NewManifest()
	expansions := NewManifest()

	for _, entry := range manifest.Entries {
		// Check if entry name contains sequence notation
		if sequencePattern.MatchString(entry.Name) {
			// Add the sequence notation entry to the main manifest
			expanded.AddEntry(entry)

			// Expand using ID list if available
			expandedEntries, err := expandSequenceEntryWithManifest(entry, manifest)
			if err != nil {
				// Not a valid sequence, keep as-is without expansion
				continue
			}

			for _, expandedEntry := range expandedEntries {
				if se.mode == SequenceEmbedded {
					expanded.AddEntry(expandedEntry)
				} else {
					expansions.AddEntry(expandedEntry)
				}
			}
		} else {
			// Regular entry, copy as-is
			expanded.AddEntry(entry)
		}
	}

	// Copy data blocks to expanded manifest (they may be needed for further operations)
	for _, block := range manifest.DataBlocks {
		expanded.AddDataBlock(block)
	}

	if se.mode == SequenceEmbedded {
		return expanded, nil, nil
	}
	return expanded, expansions, nil
}

// expandSequenceEntry expands a single sequence entry into individual file entries.
// The idList parameter provides individual C4 IDs for each file in order.
// If idList is nil, the entry's C4ID is used for all expanded files (legacy behavior).
func expandSequenceEntry(entry *Entry, idList *idList) ([]*Entry, error) {
	seq, err := ParseSequence(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sequence: %w", err)
	}

	var entries []*Entry
	expandedFiles := seq.Expand()

	// Validate ID list count if provided
	if idList != nil && idList.Count() != len(expandedFiles) {
		return nil, fmt.Errorf("ID list count (%d) doesn't match expanded file count (%d)",
			idList.Count(), len(expandedFiles))
	}

	for i, filename := range expandedFiles {
		// Get individual C4 ID from ID list, or fall back to entry's C4ID
		fileID := entry.C4ID
		if idList != nil {
			fileID = idList.Get(i)
		}

		// Create a new entry for each expanded file
		// Note: Size is unknown for individual files (use -1 for null)
		expandedEntry := &Entry{
			Name:      filename,
			Size:      -1,              // Individual size unknown from range
			Timestamp: entry.Timestamp, // Shared timestamp
			Mode:      entry.Mode,      // Shared mode
			C4ID:      fileID,
			Depth:     entry.Depth,
		}
		entries = append(entries, expandedEntry)
	}

	return entries, nil
}

// expandSequenceEntryWithManifest expands a sequence entry using embedded DataBlocks
func expandSequenceEntryWithManifest(entry *Entry, manifest *Manifest) ([]*Entry, error) {
	// Try to get the ID list from embedded data blocks.
	// Use dataBlockID first (new-style manifests where C4ID is manifest-based),
	// then fall back to C4ID (legacy manifests where C4ID == dataBlock.ID).
	var idList *idList
	lookupID := entry.dataBlockID
	if lookupID.IsNil() {
		lookupID = entry.C4ID
	}
	if !lookupID.IsNil() {
		list, err := manifest.getIDList(lookupID)
		if err == nil {
			idList = list
		}
	}

	return expandSequenceEntry(entry, idList)
}

// ----------------------------------------------------------------------------
// Sequence Detection
// ----------------------------------------------------------------------------

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

				// Aggregate metadata from all entries in range
				var totalSize int64
				var latestTime time.Time
				var mostRestrictiveMode os.FileMode = 0777 // Start with most permissive
				var depth int
				idList := newIDList()

				for i := r.start; i <= r.end; i++ {
					entry, ok := group.entries[i]
					if !ok {
						continue
					}

					totalSize += entry.Size

					if entry.Timestamp.After(latestTime) {
						latestTime = entry.Timestamp
					}

					// Most restrictive mode (lowest permission bits)
					entryPerms := entry.Mode.Perm()
					if entryPerms < mostRestrictiveMode.Perm() {
						mostRestrictiveMode = entryPerms
					}

					idList.Add(entry.C4ID)

					if i == r.start {
						depth = entry.Depth
					}
				}

				// Get file type from first entry
				firstEntry := group.entries[r.start]
				finalMode := (firstEntry.Mode & os.ModeType) | mostRestrictiveMode

				// Build a manifest from the member entries to compute canonical C4 ID.
				// Sequence C4 IDs are computed like directory C4 IDs: hash of
				// the canonical c4m representation of the member entries.
				seqManifest := NewManifest()
				for i := r.start; i <= r.end; i++ {
					memberEntry, ok := group.entries[i]
					if !ok {
						continue
					}
					entryCopy := *memberEntry
					entryCopy.Depth = 0
					seqManifest.AddEntry(&entryCopy)
				}
				seqC4ID := seqManifest.ComputeC4ID()

				// Create and embed the data block for the ID list (for round-tripping)
				dataBlock := createDataBlockFromIDList(idList)

				seqEntry := &Entry{
					Name:        pattern,
					Mode:        finalMode,
					Timestamp:   latestTime,
					Size:        totalSize,
					C4ID:        seqC4ID,
					Depth:       depth,
					IsSequence:  true,
					Pattern:     pattern,
					dataBlockID: dataBlock.ID,
				}
				result.AddEntry(seqEntry)
				result.AddDataBlock(dataBlock)
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

	// Add any remaining unprocessed entries (directories)
	for i, entry := range manifest.Entries {
		if !processedIndices[i] && entry.IsDir() {
			result.AddEntry(entry)
		}
	}

	return result
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

// DetectSequences is a convenience function using default minimum sequence length of 3
func DetectSequences(manifest *Manifest) *Manifest {
	return NewSequenceDetector(3).DetectSequences(manifest)
}

// ----------------------------------------------------------------------------
// ID Lists
// ----------------------------------------------------------------------------

// c4IDPattern matches a valid C4 ID (c4 followed by 88 base58 characters = 90 total)
var c4IDPattern = regexp.MustCompile(`^c4[1-9A-HJ-NP-Za-km-z]{88}$`)

// idList represents an ordered list of C4 IDs, typically used for range/sequence files.
// The canonical form is one C4 ID per line with trailing newlines.
type idList struct {
	ids []c4.ID
}

// newIDList creates a new empty ID list
func newIDList() *idList {
	return &idList{
		ids: make([]c4.ID, 0),
	}
}

// Add appends a C4 ID to the list
func (l *idList) Add(id c4.ID) {
	l.ids = append(l.ids, id)
}

// Count returns the number of IDs in the list
func (l *idList) Count() int {
	return len(l.ids)
}

// Get returns the ID at the given index, or nil ID if out of bounds
func (l *idList) Get(index int) c4.ID {
	if index < 0 || index >= len(l.ids) {
		return c4.ID{}
	}
	return l.ids[index]
}

// Canonical returns the canonical form of the ID list as a string.
// One C4 ID per line, trailing newline on each line.
func (l *idList) Canonical() string {
	var buf strings.Builder
	for _, id := range l.ids {
		buf.WriteString(id.String())
		buf.WriteByte('\n')
	}
	return buf.String()
}

// Bytes returns the canonical form as bytes
func (l *idList) Bytes() []byte {
	return []byte(l.Canonical())
}

// ComputeC4ID computes the C4 ID of the canonical form of this ID list
func (l *idList) ComputeC4ID() c4.ID {
	return c4.Identify(strings.NewReader(l.Canonical()))
}

// parseIDList parses an ID list from a reader.
// It is tolerant of whitespace variations but validates that each line is a valid C4 ID.
func parseIDList(r io.Reader) (*idList, error) {
	list := newIDList()
	scanner := bufio.NewScanner(r)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines (tolerance)
		if line == "" {
			continue
		}

		// Validate C4 ID format
		if !c4IDPattern.MatchString(line) {
			return nil, fmt.Errorf("line %d: invalid C4 ID format: %q", lineNum, line)
		}

		// Parse the ID
		id, err := c4.Parse(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: failed to parse C4 ID: %w", lineNum, err)
		}

		list.Add(id)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ID list: %w", err)
	}

	return list, nil
}

// parseIDListFromString parses an ID list from a string
func parseIDListFromString(s string) (*idList, error) {
	return parseIDList(strings.NewReader(s))
}

// IsIDListContent checks if content appears to be a plain C4 ID list.
// Returns true if every non-empty line matches the C4 ID pattern.
func IsIDListContent(content []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	hasContent := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		hasContent = true
		if !c4IDPattern.MatchString(line) {
			return false
		}
	}

	return hasContent
}

// ----------------------------------------------------------------------------
// Data Blocks
// ----------------------------------------------------------------------------

// DataBlock represents an embedded @data block in a manifest
type DataBlock struct {
	ID       c4.ID  // The C4 ID this block provides content for
	Content  []byte // The raw content (decoded if it was base64)
	IsIDList bool   // True if content is a plain ID list, false if base64 encoded
}

// ParseDataBlock parses the content of a @data block.
// It auto-detects whether content is a plain ID list or base64 encoded.
func ParseDataBlock(id c4.ID, content string) (*DataBlock, error) {
	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	contentBytes := []byte(content)

	block := &DataBlock{
		ID: id,
	}

	// Check if content is plain ID list
	if IsIDListContent(contentBytes) {
		block.IsIDList = true
		// Normalize to canonical form
		idList, err := parseIDListFromString(content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ID list: %w", err)
		}
		block.Content = idList.Bytes()
	} else {
		// Treat as base64 encoded content
		block.IsIDList = false
		// Remove whitespace from base64 content
		b64Content := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, content)

		decoded, err := base64.StdEncoding.DecodeString(b64Content)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 content: %w", err)
		}
		block.Content = decoded
	}

	// Validate that content hashes to declared ID
	computedID := c4.Identify(bytes.NewReader(block.Content))
	if computedID != id {
		return nil, fmt.Errorf("content hash mismatch: declared %s, computed %s", id, computedID)
	}

	return block, nil
}

// getIDList returns the content as an idList if it is one, otherwise error
func (db *DataBlock) getIDList() (*idList, error) {
	if !db.IsIDList {
		return nil, fmt.Errorf("data block is not an ID list")
	}
	return parseIDList(bytes.NewReader(db.Content))
}

// FormatDataBlock formats a DataBlock for output in a manifest.
// ID lists are written as plain text, other content is base64 encoded.
func FormatDataBlock(block *DataBlock) string {
	var buf strings.Builder

	buf.WriteString("@data ")
	buf.WriteString(block.ID.String())
	buf.WriteByte('\n')

	if block.IsIDList {
		// Write as plain ID list
		buf.Write(block.Content)
	} else {
		// Write as base64 with 76-char line limit
		encoded := base64.StdEncoding.EncodeToString(block.Content)
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			buf.WriteString(encoded[i:end])
			buf.WriteByte('\n')
		}
	}

	return buf.String()
}

// createDataBlockFromIDList creates a DataBlock from an idList
func createDataBlockFromIDList(idList *idList) *DataBlock {
	content := idList.Bytes()
	return &DataBlock{
		ID:       c4.Identify(bytes.NewReader(content)),
		Content:  content,
		IsIDList: true,
	}
}
