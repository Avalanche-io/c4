package c4m

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// Decoder reads and decodes C4M manifests from an input stream.
type Decoder struct {
	reader      *bufio.Reader
	lineNum     int
	indentWidth int // detected indent width
}

// NewDecoder creates a new Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		reader:      bufio.NewReader(r),
		indentWidth: -1, // will be detected
	}
}

// Decode reads and decodes a manifest from the input stream.
// The format is entry-only: no @c4m header, no directives.
//
// A bare C4 ID on its own line acts as a patch boundary:
//   - First line: references an external base manifest (set on Manifest.Base)
//   - Subsequent lines: must match the canonical C4 ID of all content above.
//     Entries after the boundary are applied as a patch (add/modify/delete).
func (d *Decoder) Decode() (*Manifest, error) {
	m := &Manifest{
		Version: "1.0",
		Entries: make([]*Entry, 0),
	}

	// Current section accumulates entries between patch boundaries.
	var section []*Entry
	firstLine := true
	patchMode := false

	for {
		line, err := d.readLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		// Skip blank lines (do not clear firstLine — spec says "first non-blank line").
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check for bare C4 ID line (exactly 90 chars starting with "c4").
		if isBareC4ID(trimmed) {
			id, parseErr := c4.Parse(trimmed)
			if parseErr != nil {
				return nil, fmt.Errorf("line %d: invalid C4 ID: %w", d.lineNum, parseErr)
			}

			if firstLine && len(section) == 0 {
				// First line of file: external base reference.
				m.Base = id
			} else {
				// Reject empty patch sections.
				if patchMode && len(section) == 0 {
					return nil, fmt.Errorf("%w (line %d)", ErrEmptyPatch, d.lineNum)
				}

				// Subsequent bare ID: must match canonical ID of accumulated state.
				// Flush current section into the manifest.
				if !patchMode {
					m.Entries = append(m.Entries, section...)
				} else {
					patch := &Manifest{Version: "1.0", Entries: section}
					m = ApplyPatch(m, patch)
				}
				section = nil

				expected := m.ComputeC4ID()
				if id != expected {
					return nil, fmt.Errorf("%w: line %d: got %s, want %s",
						ErrPatchIDMismatch, d.lineNum, id, expected)
				}
				patchMode = true
			}
			firstLine = false
			continue
		}

		// Reject directive lines.
		if strings.HasPrefix(trimmed, "@") {
			return nil, fmt.Errorf("%w: directives not supported (line %d): %s", ErrInvalidEntry, d.lineNum, line)
		}

		// Parse as a normal entry.
		entry, parseErr := d.parseEntryFromLine(line)
		if parseErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidEntry, parseErr)
		}
		if entry != nil {
			section = append(section, entry)
		}
		firstLine = false
	}

	// Flush remaining section.
	if !patchMode {
		m.Entries = append(m.Entries, section...)
	} else if len(section) > 0 {
		patch := &Manifest{Version: "1.0", Entries: section}
		m = ApplyPatch(m, patch)
	} else if patchMode {
		// Patch mode was entered but no entries followed — empty patch.
		return nil, fmt.Errorf("%w (at end of input)", ErrEmptyPatch)
	}

	return m, nil
}

// isBareC4ID returns true if the line is exactly a C4 ID (90 chars, starts with "c4").
func isBareC4ID(s string) bool {
	return len(s) == 90 && s[0] == 'c' && s[1] == '4'
}

// parseEntry reads a line and parses a single manifest entry.
func (d *Decoder) parseEntry() (*Entry, error) {
	line, err := d.readLine()
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(line) == "" {
		return nil, nil
	}

	if strings.HasPrefix(strings.TrimSpace(line), "@") {
		return nil, fmt.Errorf("directives not supported (line %d): %s", d.lineNum, line)
	}

	return d.parseEntryFromLine(line)
}

// parseEntryFromLine parses a manifest entry from a pre-read line.
func (d *Decoder) parseEntryFromLine(line string) (*Entry, error) {
	// Detect indentation
	indent := 0
	for i, c := range line {
		if c != ' ' {
			indent = i
			break
		}
	}

	// Detect indent width from first indented line
	if d.indentWidth == -1 && indent > 0 {
		d.indentWidth = indent
	}

	depth := 0
	if d.indentWidth > 0 {
		depth = indent / d.indentWidth
	}

	// Trim indentation
	line = strings.TrimLeft(line, " ")

	// Smart field parsing for ergonomic forms
	// Mode could be single "-" or full 10 characters
	var modeStr string
	if strings.HasPrefix(line, "- ") {
		// Single dash null mode
		modeStr = "-"
		line = line[2:] // Skip "-" and space
	} else if len(line) >= 11 {
		// Normal 10-character mode
		modeStr = line[:10]
		line = line[11:] // Skip mode and space
	} else {
		return nil, fmt.Errorf("%w: line %d: line too short", ErrInvalidEntry, d.lineNum)
	}

	// Parse mode (handle null value "-")
	var mode os.FileMode
	if modeStr == "-" || modeStr == "----------" {
		// Null mode - treat as unspecified (zero value)
		mode = 0
	} else {
		var err error
		mode, err = parseMode(modeStr)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid mode %q: %w", d.lineNum, modeStr, err)
		}
	}

	entry := &Entry{
		Depth: depth,
		Mode:  mode,
	}

	// Try to extract timestamp - could be canonical or pretty format
	var timestampStr string
	var remainingLine string

	// Check for null timestamp first
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "0 ") {
		// Null timestamp - use zero value (Unix epoch)
		timestampStr = "-"
		remainingLine = line[2:]
	} else if len(line) >= 20 && line[4] == '-' && line[10] == 'T' {
		// Canonical/RFC3339 timestamp format
		// Could be:
		// - 2006-01-02T15:04:05Z (20 chars)
		// - 2006-01-02T15:04:05-07:00 (25 chars)
		// - 2006-01-02T15:04:05+07:00 (25 chars)
		endIdx := 20
		if len(line) >= 25 && (line[19] == '-' || line[19] == '+') {
			// Has timezone offset
			endIdx = 25
		}
		timestampStr = line[:endIdx]
		if len(line) > endIdx {
			remainingLine = line[endIdx+1:] // Skip timestamp and space
		} else {
			remainingLine = ""
		}
	} else {
		// Try pretty format (e.g., "Sep  1 00:36:18 2025 CDT")
		parts := strings.Fields(line)
		if len(parts) >= 5 {
			// Timestamp is first 5 fields
			timestampStr = strings.Join(parts[:5], " ")
			remainingLine = strings.Join(parts[5:], " ")
		} else {
			return nil, fmt.Errorf("line %d: cannot parse timestamp from %q", d.lineNum, line)
		}
	}

	// Parse timestamp (handle null value)
	var timestamp time.Time
	if timestampStr == "-" || timestampStr == "0" {
		// Null timestamp - use Unix epoch (zero value)
		timestamp = time.Unix(0, 0).UTC()
	} else {
		var err error
		timestamp, err = parseTimestamp(timestampStr)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid timestamp %q: %w", d.lineNum, timestampStr, err)
		}
	}
	entry.Timestamp = timestamp

	// Parse remaining fields using character-level parsing
	size, name, rawName, target, id, hardLink, flowDir, flowTarget, err := d.parseEntryFields(remainingLine, mode)
	if err != nil {
		return nil, fmt.Errorf("line %d: %w", d.lineNum, err)
	}

	entry.Size = size
	entry.Name = UnsafeName(name)
	entry.Target = UnsafeName(target)
	entry.C4ID = id
	entry.HardLink = hardLink
	entry.FlowDirection = flowDir
	entry.FlowTarget = flowTarget

	// Check for sequence notation in the RAW name (before unescaping).
	// Escaped brackets (\[, \]) are excluded; only unescaped brackets
	// forming a valid range pattern trigger sequence detection.
	if hasUnescapedSequenceNotation(rawName) {
		entry.IsSequence = true
		entry.Pattern = entry.Name
	}

	return entry, nil
}

// parseEntryFields parses the remaining line after timestamp using character-level
// scanning. Names and targets use backslash escaping for spaces and
// double-quotes. No quoting mechanism is supported.
//
// The expected format is: SIZE NAME [LINK_OP TARGET] [C4ID]
// Where LINK_OP is ->, <-, or <>, and NAME/TARGET use backslash escaping.
//
// The mode parameter is used to disambiguate the overloaded -> operator:
// when mode indicates a symlink, -> is always parsed as a symlink target.
func (d *Decoder) parseEntryFields(line string, mode os.FileMode) (size int64, name, rawName, target string, id c4.ID, hardLink int, flowDir FlowDirection, flowTarget string, err error) {
	pos := 0
	n := len(line)

	// Skip leading whitespace
	for pos < n && line[pos] == ' ' {
		pos++
	}
	if pos >= n {
		err = fmt.Errorf("insufficient fields after timestamp")
		return
	}

	// 1. Parse size token (digits, commas, or "-")
	sizeStart := pos
	if line[pos] == '-' {
		// Null size
		size = -1
		pos++
	} else {
		for pos < n && (line[pos] >= '0' && line[pos] <= '9' || line[pos] == ',') {
			pos++
		}
		if pos == sizeStart {
			err = fmt.Errorf("invalid size at position %d", pos)
			return
		}
		sizeStr := strings.ReplaceAll(line[sizeStart:pos], ",", "")
		size, err = strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			err = fmt.Errorf("invalid size %q: %w", line[sizeStart:pos], err)
			return
		}
	}

	// Skip whitespace between size and name
	for pos < n && line[pos] == ' ' {
		pos++
	}
	if pos >= n {
		err = fmt.Errorf("missing name after size")
		return
	}

	// 2. Parse name
	nameStart := pos
	name, pos, _, err = d.parseNameOrTarget(line, pos)
	rawName = line[nameStart:pos]
	if err != nil {
		err = fmt.Errorf("parsing name: %w", err)
		return
	}

	// Skip whitespace
	for pos < n && line[pos] == ' ' {
		pos++
	}

	// 3. Check for link operator: ->, <-, or <>
	isSymlink := mode&os.ModeSymlink != 0
	if pos+1 < n && line[pos] == '-' && line[pos+1] == '>' {
		pos += 2

		if isSymlink {
			// Symlink mode: -> is always a symlink target. No ambiguity.
			for pos < n && line[pos] == ' ' {
				pos++
			}
			if pos < n {
				target, pos, err = d.parseTarget(line, pos)
				if err != nil {
					err = fmt.Errorf("parsing symlink target: %w", err)
					return
				}
				for pos < n && line[pos] == ' ' {
					pos++
				}
			}
		} else if pos < n && line[pos] >= '1' && line[pos] <= '9' {
			// Hard link group number: ->N (digit immediately after ->)
			groupStart := pos
			for pos < n && line[pos] >= '0' && line[pos] <= '9' {
				pos++
			}
			groupNum, _ := strconv.Atoi(line[groupStart:pos])
			hardLink = groupNum
			for pos < n && line[pos] == ' ' {
				pos++
			}
		} else {
			// Skip whitespace after ->
			for pos < n && line[pos] == ' ' {
				pos++
			}

			// Determine type by examining token after ->
			// Check flow target BEFORE c4 prefix to avoid misclassifying
			// location names starting with "c4" (e.g., "c4studio:inbox/").
			if pos < n && isFlowTarget(line[pos:]) {
				// Flow target (matches location:... pattern)
				flowDir = FlowOutbound
				flowTarget, pos, err = d.parseFlowTarget(line, pos)
				if err != nil {
					return
				}
				for pos < n && line[pos] == ' ' {
					pos++
				}
			} else if remaining := strings.TrimSpace(line[pos:]); remaining == "-" || strings.HasPrefix(remaining, "c4") {
				// Hard link (ungrouped) — remaining is null C4 ID or a C4 ID
				hardLink = -1
			} else if pos < n {
				// Fallback: treat as symlink target
				target, pos, err = d.parseTarget(line, pos)
				if err != nil {
					err = fmt.Errorf("parsing symlink target: %w", err)
					return
				}
				for pos < n && line[pos] == ' ' {
					pos++
				}
			}
		}
	} else if pos+1 < n && line[pos] == '<' && line[pos+1] == '-' {
		// <- operator: always inbound flow
		pos += 2
		for pos < n && line[pos] == ' ' {
			pos++
		}
		flowDir = FlowInbound
		flowTarget, pos, err = d.parseFlowTarget(line, pos)
		if err != nil {
			return
		}
		for pos < n && line[pos] == ' ' {
			pos++
		}
	} else if pos+1 < n && line[pos] == '<' && line[pos+1] == '>' {
		// <> operator: always bidirectional flow
		pos += 2
		for pos < n && line[pos] == ' ' {
			pos++
		}
		flowDir = FlowBidirectional
		flowTarget, pos, err = d.parseFlowTarget(line, pos)
		if err != nil {
			return
		}
		for pos < n && line[pos] == ' ' {
			pos++
		}
	}

	// 4. Check for C4 ID or null ("-")
	if pos < n {
		remaining := strings.TrimSpace(line[pos:])
		if remaining == "-" {
			// Null C4 ID
			id = c4.ID{}
		} else if strings.HasPrefix(remaining, "c4") {
			id, err = c4.Parse(remaining)
			if err != nil {
				err = fmt.Errorf("invalid C4 ID %q: %w", remaining, err)
				return
			}
		}
	}

	return
}

// parseNameOrTarget parses a backslash-escaped name starting at pos.
// Returns the parsed string, new position, whether unescaped brackets were found
// (indicating potential sequence notation), and error.
//
// c4m field-boundary escapes: \<space>→space, \"→", \[→[, \]→]
// All other backslash sequences (SafeName tier-2 escapes like \\, \t, \n, \r, \0,
// and tier-3 braille sequences) pass through for UnsafeName to decode.
//
// Boundary detection:
//   - Directory names end at / (inclusive)
//   - File names end at space followed by ->, <-, <>, c4 prefix, or -
func (d *Decoder) parseNameOrTarget(line string, pos int) (string, int, bool, error) {
	n := len(line)
	if pos >= n {
		return "", pos, false, fmt.Errorf("unexpected end of line")
	}

	// Scan for boundary with backslash-escape handling.
	// c4m field-boundary escapes are consumed here; SafeName escapes pass through.
	var buf strings.Builder
	hasUnescapedBrackets := false
	for pos < n {
		ch := line[pos]

		// c4m field-boundary escapes.
		if ch == '\\' && pos+1 < n {
			next := line[pos+1]
			if next == ' ' || next == '"' || next == '[' || next == ']' {
				buf.WriteByte(next)
				pos += 2
				continue
			}
		}

		if ch == '[' || ch == ']' {
			hasUnescapedBrackets = true
		}

		// Directory name ends at / (inclusive)
		if ch == '/' {
			buf.WriteByte('/')
			pos++
			return buf.String(), pos, hasUnescapedBrackets, nil
		}

		// Check for boundary: space followed by link operator, c4 prefix, or -
		if ch == ' ' {
			rest := line[pos:]
			if strings.HasPrefix(rest, " -> ") ||
				strings.HasPrefix(rest, " <- ") ||
				strings.HasPrefix(rest, " <> ") {
				return buf.String(), pos, hasUnescapedBrackets, nil
			}
			// Hard link group marker: " ->N" where N is a digit 1-9
			if len(rest) >= 4 && rest[1] == '-' && rest[2] == '>' && rest[3] >= '1' && rest[3] <= '9' {
				return buf.String(), pos, hasUnescapedBrackets, nil
			}
			if len(rest) > 1 && rest[1] == 'c' && len(rest) > 2 && rest[2] == '4' {
				return buf.String(), pos, hasUnescapedBrackets, nil
			}
			if len(rest) >= 2 && rest[1] == '-' && (len(rest) == 2 || rest[2] == ' ') {
				return buf.String(), pos, hasUnescapedBrackets, nil
			}
		}
		buf.WriteByte(ch)
		pos++
	}

	return buf.String(), pos, hasUnescapedBrackets, nil
}

// hasUnescapedSequenceNotation checks if raw text contains sequence notation
// [digits] where the brackets are NOT preceded by a backslash escape.
// This correctly handles mixed cases like file\[test\].[001-010].dat where
// some brackets are literal (escaped) and others form range notation.
func hasUnescapedSequenceNotation(raw string) bool {
	// Replace all escape sequences with neutral characters so that
	// escaped brackets don't match the sequence pattern regex.
	var buf strings.Builder
	buf.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			buf.WriteByte('_')
			buf.WriteByte('_')
			i++
			continue
		}
		buf.WriteByte(raw[i])
	}
	return sequencePattern.MatchString(buf.String())
}

// parseTarget parses a symlink target starting at pos.
// Unlike parseNameOrTarget, this does NOT treat / as a boundary because
// symlink targets can be absolute paths or contain path separators.
// Targets do not get bracket escaping since brackets in paths are literal.
//
// c4m field-boundary escapes: \<space>→space, \"→"
// All other backslash sequences pass through for UnsafeName.
// Boundary: space followed by c4 prefix, null marker (-), or end-of-line.
func (d *Decoder) parseTarget(line string, pos int) (string, int, error) {
	n := len(line)
	if pos >= n {
		return "", pos, fmt.Errorf("unexpected end of line")
	}

	// Scan with backslash-escape handling.
	// \<space> and \" are consumed as literal characters (not boundaries).
	var buf strings.Builder
	for pos < n {
		ch := line[pos]

		// Backslash escapes: consume space and quote escapes.
		if ch == '\\' && pos+1 < n {
			next := line[pos+1]
			if next == ' ' || next == '"' {
				buf.WriteByte(next)
				pos += 2
				continue
			}
		}

		if ch == ' ' {
			rest := line[pos:]
			if len(rest) > 1 && rest[1] == 'c' && len(rest) > 2 && rest[2] == '4' {
				return buf.String(), pos, nil
			}
			if len(rest) >= 2 && rest[1] == '-' && (len(rest) == 2 || rest[2] == ' ') {
				return buf.String(), pos, nil
			}
		}
		buf.WriteByte(ch)
		pos++
	}

	return buf.String(), pos, nil
}

// isFlowTarget returns true if the text at the current position matches
// the flow target pattern: a location label ([a-zA-Z][a-zA-Z0-9_-]*) followed by ":".
func isFlowTarget(s string) bool {
	if len(s) == 0 {
		return false
	}
	ch := s[0]
	if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == ':' {
			return true
		}
		if c == ' ' {
			return false
		}
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return false
}

// parseFlowTarget reads a flow target (location:path) starting at pos.
// The flow target ends at the next space followed by a C4 prefix or "-", or at end-of-line.
func (d *Decoder) parseFlowTarget(line string, pos int) (string, int, error) {
	n := len(line)
	if pos >= n {
		return "", pos, fmt.Errorf("expected flow target")
	}
	start := pos
	for pos < n {
		ch := line[pos]
		if ch == ' ' {
			rest := line[pos:]
			if len(rest) > 1 && rest[1] == 'c' && len(rest) > 2 && rest[2] == '4' {
				return line[start:pos], pos, nil
			}
			if len(rest) >= 2 && rest[1] == '-' && (len(rest) == 2 || rest[2] == ' ') {
				return line[start:pos], pos, nil
			}
		}
		pos++
	}
	return line[start:pos], pos, nil
}

// readLine reads a line from the input
func (d *Decoder) readLine() (string, error) {
	line, err := d.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}

	d.lineNum++

	// Trim line ending — LF only, reject CR
	line = strings.TrimSuffix(line, "\n")
	if strings.ContainsRune(line, '\r') {
		return "", fmt.Errorf("line %d: CR (0x0D) not allowed — c4m requires LF-only line endings", d.lineNum)
	}

	if err == io.EOF && line == "" {
		return "", io.EOF
	}

	return line, nil
}

// parseTimestamp tries multiple timestamp formats.
// The canonical format requires UTC-only timestamps (2006-01-02T15:04:05Z),
// but the decoder accepts various ergonomic formats and converts them to UTC.
func parseTimestamp(s string) (time.Time, error) {
	// Try canonical format first (2006-01-02T15:04:05Z) - strict UTC subset of RFC3339
	if t, err := time.Parse(TimestampFormat, s); err == nil {
		return t, nil
	}

	// Try full RFC3339 with timezone offset (e.g., "2006-01-02T15:04:05-07:00")
	// This is an ergonomic variation - will be converted to UTC
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}

	// Try Unix format (e.g., "Mon Jan  2 15:04:05 MST 2006")
	if t, err := time.Parse(time.UnixDate, s); err == nil {
		return t.UTC(), nil
	}

	// Try pretty format with timezone (e.g., "Jan  2 15:04:05 2006 MST")
	// Need to handle both single and double-space after month
	if t, err := time.Parse("Jan _2 15:04:05 2006 MST", s); err == nil {
		return t.UTC(), nil
	}

	// Try without the space padding
	if t, err := time.Parse("Jan 2 15:04:05 2006 MST", s); err == nil {
		return t.UTC(), nil
	}

	// Try with numeric timezone offset
	if t, err := time.Parse("Jan _2 15:04:05 2006 -0700", s); err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("cannot parse timestamp %q", s)
}

// parseMode converts a mode string to os.FileMode
func parseMode(s string) (os.FileMode, error) {
	if len(s) != 10 {
		return 0, fmt.Errorf("mode must be 10 characters")
	}

	var mode os.FileMode

	// File type
	switch s[0] {
	case '-':
		// regular file
	case 'd':
		mode |= os.ModeDir
	case 'l':
		mode |= os.ModeSymlink
	case 'p':
		mode |= os.ModeNamedPipe
	case 's':
		mode |= os.ModeSocket
	case 'b':
		mode |= os.ModeDevice
	case 'c':
		mode |= os.ModeCharDevice
	default:
		return 0, fmt.Errorf("unknown file type: %c", s[0])
	}

	// Permission bits (standard rwx)
	perms := uint32(0)
	permChars := s[1:]

	// User permissions
	if permChars[0] == 'r' {
		perms |= 0400
	}
	if permChars[1] == 'w' {
		perms |= 0200
	}
	if permChars[2] == 'x' || permChars[2] == 's' {
		perms |= 0100
	}

	// Group permissions
	if permChars[3] == 'r' {
		perms |= 0040
	}
	if permChars[4] == 'w' {
		perms |= 0020
	}
	if permChars[5] == 'x' || permChars[5] == 's' {
		perms |= 0010
	}

	// Other permissions
	if permChars[6] == 'r' {
		perms |= 0004
	}
	if permChars[7] == 'w' {
		perms |= 0002
	}
	if permChars[8] == 'x' || permChars[8] == 't' {
		perms |= 0001
	}

	// Special bits
	if permChars[2] == 's' || permChars[2] == 'S' {
		mode |= os.ModeSetuid
	}
	if permChars[5] == 's' || permChars[5] == 'S' {
		mode |= os.ModeSetgid
	}
	if permChars[8] == 't' || permChars[8] == 'T' {
		mode |= os.ModeSticky
	}

	mode |= os.FileMode(perms)

	return mode, nil
}
