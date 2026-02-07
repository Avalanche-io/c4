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
	version     string
	indentWidth int // detected indent width
}

// NewDecoder creates a new Decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		reader:      bufio.NewReader(r),
		indentWidth: -1, // will be detected
	}
}

// Version returns the parsed manifest version.
// This is only valid after Decode() has been called.
func (d *Decoder) Version() string {
	return d.version
}

// Decode reads and decodes a manifest from the input stream.
func (d *Decoder) Decode() (*Manifest, error) {
	if err := d.parseHeader(); err != nil {
		return nil, err
	}

	m := &Manifest{
		Version: d.version,
		Entries: make([]*Entry, 0),
	}

	for {
		entry, err := d.parseEntry()
		if err != nil {
			if err == io.EOF {
				break
			}
			if dirErr, ok := err.(*directiveError); ok {
				directive := dirErr.directive
				// Check for @data which requires special multi-line handling
				if strings.HasPrefix(directive, "@data ") {
					if err := d.handleDataBlock(m, directive); err != nil {
						return nil, err
					}
					continue
				}
				// Handle other directives
				if err := d.handleDirective(m, directive); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}

		m.Entries = append(m.Entries, entry)
	}

	return m, nil
}

// parseHeader reads and validates the version header
func (d *Decoder) parseHeader() error {
	line, err := d.readLine()
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	if !strings.HasPrefix(line, "@c4m ") {
		return fmt.Errorf("invalid header: expected '@c4m X.Y', got %q", line)
	}

	d.version = strings.TrimPrefix(line, "@c4m ")
	if d.version == "" {
		return fmt.Errorf("missing version number")
	}

	// Currently only support version 1.x
	if !strings.HasPrefix(d.version, "1.") {
		return fmt.Errorf("unsupported version: %s", d.version)
	}

	return nil
}

// parseEntry parses a single manifest entry
func (d *Decoder) parseEntry() (*Entry, error) {
	line, err := d.readLine()
	if err != nil {
		return nil, err
	}

	// Handle directives
	if strings.HasPrefix(line, "@") {
		return nil, &directiveError{directive: line}
	}

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
		return nil, fmt.Errorf("line %d: line too short", d.lineNum)
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
		if strings.HasPrefix(line, "- ") {
			remainingLine = line[2:]
		} else {
			remainingLine = line[2:]
		}
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
	size, name, target, id, err := d.parseEntryFields(remainingLine)
	if err != nil {
		return nil, fmt.Errorf("line %d: %w", d.lineNum, err)
	}

	entry.Size = size
	entry.Name = name
	entry.Target = target
	entry.C4ID = id

	// Check for sequence notation
	if strings.Contains(entry.Name, "[") && strings.Contains(entry.Name, "]") {
		entry.IsSequence = true
		entry.Pattern = entry.Name
	}

	return entry, nil
}

// parseEntryFields parses the remaining line after timestamp using character-level
// scanning. This correctly handles quoted names with escapes, symlink targets
// with spaces, and filenames with backslashes or leading spaces.
//
// The expected format is: SIZE NAME [-> TARGET] [C4ID]
// Where NAME and TARGET may be quoted with backslash escapes.
func (d *Decoder) parseEntryFields(line string) (size int64, name, target string, id c4.ID, err error) {
	pos := 0
	n := len(line)

	// Skip leading whitespace
	for pos < n && line[pos] == ' ' {
		pos++
	}
	if pos >= n {
		return 0, "", "", c4.ID{}, fmt.Errorf("insufficient fields after timestamp")
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
			return 0, "", "", c4.ID{}, fmt.Errorf("invalid size at position %d", pos)
		}
		sizeStr := strings.ReplaceAll(line[sizeStart:pos], ",", "")
		size, err = strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return 0, "", "", c4.ID{}, fmt.Errorf("invalid size %q: %w", line[sizeStart:pos], err)
		}
	}

	// Skip whitespace between size and name
	for pos < n && line[pos] == ' ' {
		pos++
	}
	if pos >= n {
		return 0, "", "", c4.ID{}, fmt.Errorf("missing name after size")
	}

	// 2. Parse name (quoted or unquoted)
	name, pos, err = d.parseNameOrTarget(line, pos)
	if err != nil {
		return 0, "", "", c4.ID{}, fmt.Errorf("parsing name: %w", err)
	}

	// Skip whitespace
	for pos < n && line[pos] == ' ' {
		pos++
	}

	// 3. Check for symlink target ("->")
	if pos+1 < n && line[pos] == '-' && line[pos+1] == '>' {
		pos += 2
		// Skip whitespace after ->
		for pos < n && line[pos] == ' ' {
			pos++
		}
		if pos >= n {
			return 0, "", "", c4.ID{}, fmt.Errorf("missing symlink target after ->")
		}
		target, pos, err = d.parseTarget(line, pos)
		if err != nil {
			return 0, "", "", c4.ID{}, fmt.Errorf("parsing symlink target: %w", err)
		}
		// Skip whitespace
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
				return 0, "", "", c4.ID{}, fmt.Errorf("invalid C4 ID %q: %w", remaining, err)
			}
		}
	}

	return size, name, target, id, nil
}

// parseNameOrTarget parses a quoted or unquoted name/target starting at pos.
// Returns the parsed string and the new position after consuming it.
//
// Quoted names: "..." with \\→\, \"→", \n→newline
// Unquoted names: read until the boundary is detected:
//   - For directory names (trailing /): read until / then stop
//   - For file names: read until space followed by ->, c4 prefix, or end-of-line
func (d *Decoder) parseNameOrTarget(line string, pos int) (string, int, error) {
	n := len(line)
	if pos >= n {
		return "", pos, fmt.Errorf("unexpected end of line")
	}

	if line[pos] == '"' {
		// Quoted name: process escape sequences
		pos++ // skip opening quote
		var buf strings.Builder
		for pos < n {
			ch := line[pos]
			if ch == '\\' && pos+1 < n {
				next := line[pos+1]
				switch next {
				case '\\':
					buf.WriteByte('\\')
				case '"':
					buf.WriteByte('"')
				case 'n':
					buf.WriteByte('\n')
				default:
					buf.WriteByte('\\')
					buf.WriteByte(next)
				}
				pos += 2
			} else if ch == '"' {
				pos++ // skip closing quote
				return buf.String(), pos, nil
			} else {
				buf.WriteByte(ch)
				pos++
			}
		}
		return "", pos, fmt.Errorf("unterminated quoted name")
	}

	// Unquoted name: scan for boundary
	start := pos
	for pos < n {
		ch := line[pos]

		// Directory name ends at / (inclusive)
		if ch == '/' {
			pos++ // include the slash
			return line[start:pos], pos, nil
		}

		// Check for boundary: space followed by -> or c4 prefix or -
		if ch == ' ' {
			rest := line[pos:]
			if strings.HasPrefix(rest, " -> ") {
				return line[start:pos], pos, nil
			}
			if len(rest) > 1 && rest[1] == 'c' && len(rest) > 2 && rest[2] == '4' {
				return line[start:pos], pos, nil
			}
			// Space followed by "-" and then end-of-line or space (null C4 ID)
			if len(rest) >= 2 && rest[1] == '-' && (len(rest) == 2 || rest[2] == ' ') {
				return line[start:pos], pos, nil
			}
		}
		pos++
	}

	// End of line — the whole remainder is the name
	return line[start:pos], pos, nil
}

// parseTarget parses a symlink target starting at pos.
// Unlike parseNameOrTarget, this does NOT treat / as a boundary because
// symlink targets can be absolute paths or contain path separators.
// For unquoted targets, the boundary is a space followed by a c4 prefix or end-of-line.
func (d *Decoder) parseTarget(line string, pos int) (string, int, error) {
	n := len(line)
	if pos >= n {
		return "", pos, fmt.Errorf("unexpected end of line")
	}

	if line[pos] == '"' {
		// Quoted target: same logic as quoted name
		pos++ // skip opening quote
		var buf strings.Builder
		for pos < n {
			ch := line[pos]
			if ch == '\\' && pos+1 < n {
				next := line[pos+1]
				switch next {
				case '\\':
					buf.WriteByte('\\')
				case '"':
					buf.WriteByte('"')
				case 'n':
					buf.WriteByte('\n')
				default:
					buf.WriteByte('\\')
					buf.WriteByte(next)
				}
				pos += 2
			} else if ch == '"' {
				pos++ // skip closing quote
				return buf.String(), pos, nil
			} else {
				buf.WriteByte(ch)
				pos++
			}
		}
		return "", pos, fmt.Errorf("unterminated quoted target")
	}

	// Unquoted target: scan until c4 prefix or end-of-line
	start := pos
	for pos < n {
		ch := line[pos]
		if ch == ' ' {
			rest := line[pos:]
			// Space followed by c4 prefix
			if len(rest) > 1 && rest[1] == 'c' && len(rest) > 2 && rest[2] == '4' {
				return line[start:pos], pos, nil
			}
			// Space followed by "-" and then end-of-line or space (null C4 ID)
			if len(rest) >= 2 && rest[1] == '-' && (len(rest) == 2 || rest[2] == ' ') {
				return line[start:pos], pos, nil
			}
		}
		pos++
	}

	return line[start:pos], pos, nil
}

// handleDataBlock reads and parses a @data block
func (d *Decoder) handleDataBlock(m *Manifest, directive string) error {
	// Parse the C4 ID from the directive
	parts := strings.Fields(directive)
	if len(parts) < 2 {
		return fmt.Errorf("@data requires C4 ID")
	}

	id, err := c4.Parse(parts[1])
	if err != nil {
		return fmt.Errorf("invalid @data C4 ID: %w", err)
	}

	// Read content until next @ directive or EOF
	var content strings.Builder
	for {
		line, err := d.readLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Check for next directive (but not lines that start with @ inside content)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@") && len(trimmed) > 1 {
			// This is a new directive
			if strings.HasPrefix(trimmed, "@data ") {
				// Parse the accumulated content first
				block, parseErr := ParseDataBlock(id, content.String())
				if parseErr != nil {
					return fmt.Errorf("failed to parse @data block: %w", parseErr)
				}
				m.AddDataBlock(block)
				// Handle the new @data directive
				return d.handleDataBlock(m, trimmed)
			}
			// Parse the accumulated content
			block, parseErr := ParseDataBlock(id, content.String())
			if parseErr != nil {
				return fmt.Errorf("failed to parse @data block: %w", parseErr)
			}
			m.AddDataBlock(block)
			// Handle the other directive
			return d.handleDirective(m, trimmed)
		}

		content.WriteString(line)
		content.WriteByte('\n')
	}

	// Parse the accumulated content
	block, err := ParseDataBlock(id, content.String())
	if err != nil {
		return fmt.Errorf("failed to parse @data block: %w", err)
	}
	m.AddDataBlock(block)

	return nil
}

// readLine reads a line from the input
func (d *Decoder) readLine() (string, error) {
	line, err := d.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}

	d.lineNum++

	// Trim line ending
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r") // handle CRLF

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

// handleDirective processes @ directives
func (d *Decoder) handleDirective(m *Manifest, directive string) error {
	parts := strings.Fields(directive)
	if len(parts) == 0 {
		return nil
	}

	switch parts[0] {
	case "@base":
		if len(parts) < 2 {
			return fmt.Errorf("@base requires C4 ID")
		}
		id, err := c4.Parse(parts[1])
		if err != nil {
			return fmt.Errorf("invalid base C4 ID: %w", err)
		}
		m.Base = id

	case "@layer":
		m.currentLayer = &Layer{Type: LayerTypeAdd}
		m.Layers = append(m.Layers, m.currentLayer)

	case "@remove":
		m.currentLayer = &Layer{Type: LayerTypeRemove}
		m.Layers = append(m.Layers, m.currentLayer)

	case "@expand":
		return fmt.Errorf("@expand directive not yet supported")

	case "@by":
		if m.currentLayer != nil {
			m.currentLayer.By = strings.Join(parts[1:], " ")
		}

	case "@time":
		if m.currentLayer != nil && len(parts) > 1 {
			t, err := time.Parse(time.RFC3339, parts[1])
			if err != nil {
				return fmt.Errorf("invalid @time: %w", err)
			}
			m.currentLayer.Time = t
		}

	case "@note":
		if m.currentLayer != nil {
			m.currentLayer.Note = strings.Join(parts[1:], " ")
		}

	case "@data":
		if len(parts) > 1 {
			id, err := c4.Parse(parts[1])
			if err != nil {
				return fmt.Errorf("invalid @data C4 ID: %w", err)
			}
			if m.currentLayer != nil {
				m.currentLayer.Data = id
			} else {
				m.Data = id
			}
		}

	case "@end":
		// End of layer - reset current layer
		m.currentLayer = nil
	}

	return nil
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

// directiveError indicates a directive was encountered during parsing
type directiveError struct {
	directive string
}

func (e *directiveError) Error() string {
	return fmt.Sprintf("directive: %s", e.directive)
}
