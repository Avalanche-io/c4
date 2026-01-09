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

// Parser handles parsing of C4M manifest files
type Parser struct {
	reader      *bufio.Reader
	lineNum     int
	version     string
	strict      bool
	indentWidth int // detected indent width
}

// NewParser creates a new C4M parser
func NewParser(r io.Reader) *Parser {
	return &Parser{
		reader:      bufio.NewReader(r),
		strict:      false,
		indentWidth: -1, // will be detected
	}
}

// NewStrictParser creates a parser that enforces strict validation
func NewStrictParser(r io.Reader) *Parser {
	p := NewParser(r)
	p.strict = true
	return p
}

// Version returns the parsed manifest version
func (p *Parser) Version() string {
	return p.version
}

// Parse reads and validates the version header
func (p *Parser) ParseHeader() error {
	line, err := p.readLine()
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}
	
	if !strings.HasPrefix(line, "@c4m ") {
		return fmt.Errorf("invalid header: expected '@c4m X.Y', got %q", line)
	}
	
	p.version = strings.TrimPrefix(line, "@c4m ")
	if p.version == "" {
		return fmt.Errorf("missing version number")
	}
	
	// Currently only support version 1.0
	if !strings.HasPrefix(p.version, "1.") {
		return fmt.Errorf("unsupported version: %s", p.version)
	}
	
	return nil
}

// ParseEntry parses a single manifest entry
func (p *Parser) ParseEntry() (*Entry, error) {
	line, err := p.readLine()
	if err != nil {
		return nil, err
	}
	
	// Handle directives
	if strings.HasPrefix(line, "@") {
		return nil, &DirectiveError{Directive: line}
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
	if p.indentWidth == -1 && indent > 0 {
		p.indentWidth = indent
	}
	
	depth := 0
	if p.indentWidth > 0 {
		depth = indent / p.indentWidth
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
		return nil, fmt.Errorf("line %d: line too short", p.lineNum)
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
			return nil, fmt.Errorf("line %d: invalid mode %q: %w", p.lineNum, modeStr, err)
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
			return nil, fmt.Errorf("line %d: cannot parse timestamp from %q", p.lineNum, line)
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
			return nil, fmt.Errorf("line %d: invalid timestamp %q: %w", p.lineNum, timestampStr, err)
		}
	}
	entry.Timestamp = timestamp
	
	// Parse remaining fields (size, name, optional target and C4 ID)
	fields := strings.Fields(remainingLine)
	if len(fields) < 2 {
		return nil, fmt.Errorf("line %d: insufficient fields after timestamp", p.lineNum)
	}
	
	// Parse size (handle null value and strip commas for ergonomic forms)
	var size int64
	if fields[0] == "-" {
		// Null size - use -1 to indicate unspecified
		size = -1
	} else {
		sizeStr := strings.ReplaceAll(fields[0], ",", "")
		var err error
		size, err = strconv.ParseInt(sizeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("line %d: invalid size %q: %w", p.lineNum, fields[0], err)
		}
	}
	entry.Size = size
	
	// Parse name - check if it's quoted
	nameIdx := 1
	if len(fields[nameIdx]) > 0 && fields[nameIdx][0] == '"' {
		// Handle quoted name
		quotedName := fields[nameIdx]
		for j := nameIdx + 1; j < len(fields) && !strings.HasSuffix(quotedName, "\""); j++ {
			quotedName = quotedName + " " + fields[j]
			nameIdx = j
		}
		// Remove quotes
		entry.Name = strings.Trim(quotedName, "\"")
	} else {
		entry.Name = fields[nameIdx]
	}
	
	// Parse optional fields
	i := nameIdx + 1
	
	// Check for symlink target
	if i < len(fields) && fields[i] == "->" && i+1 < len(fields) {
		// Handle quoted target
		if len(fields[i+1]) > 0 && fields[i+1][0] == '"' {
			quotedTarget := fields[i+1]
			targetEndIdx := i+1
			for j := i+2; j < len(fields) && !strings.HasSuffix(quotedTarget, "\""); j++ {
				quotedTarget = quotedTarget + " " + fields[j]
				targetEndIdx = j
			}
			entry.Target = strings.Trim(quotedTarget, "\"")
			i = targetEndIdx + 1
		} else {
			entry.Target = fields[i+1]
			i += 2
		}
	}
	
	// Check for C4 ID (handle null value)
	if i < len(fields) {
		if fields[i] == "-" {
			// Null C4 ID - leave as zero value
			entry.C4ID = c4.ID{}
		} else if strings.HasPrefix(fields[i], "c4") {
			id, err := c4.Parse(fields[i])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid C4 ID %q: %w", p.lineNum, fields[i], err)
			}
			entry.C4ID = id
		}
		i++
	}
	
	// Check for sequence notation
	if strings.Contains(entry.Name, "[") && strings.Contains(entry.Name, "]") {
		entry.IsSequence = true
		entry.Pattern = entry.Name
	}
	
	return entry, nil
}

// ParseAll parses the entire manifest
func (p *Parser) ParseAll() (*Manifest, error) {
	if err := p.ParseHeader(); err != nil {
		return nil, err
	}
	
	m := &Manifest{
		Version: p.version,
		Entries: make([]*Entry, 0),
	}
	
	for {
		entry, err := p.ParseEntry()
		if err != nil {
			if err == io.EOF {
				break
			}
			if _, ok := err.(*DirectiveError); ok {
				// Handle directive
				if err := p.handleDirective(m, err.(*DirectiveError).Directive); err != nil {
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

// readLine reads a line from the input
func (p *Parser) readLine() (string, error) {
	line, err := p.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	
	p.lineNum++
	
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
// but the parser accepts various ergonomic formats and converts them to UTC.
func parseTimestamp(s string) (time.Time, error) {
	// Try canonical format first (2006-01-02T15:04:05Z) - strict UTC subset of RFC3339
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
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

// parseFields splits a line into fields, respecting quotes
func (p *Parser) parseFields(line string) []string {
	var fields []string
	var current strings.Builder
	inQuotes := false
	escape := false
	
	for i, c := range line {
		if escape {
			switch c {
			case 'n':
				current.WriteRune('\n')
			case 't':
				current.WriteRune('\t')
			case '\\', '"':
				current.WriteRune(c)
			default:
				current.WriteRune('\\')
				current.WriteRune(c)
			}
			escape = false
			continue
		}
		
		if c == '\\' {
			escape = true
			continue
		}
		
		if c == '"' {
			if inQuotes {
				// End of quoted field
				fields = append(fields, current.String())
				current.Reset()
				inQuotes = false
				// Skip any spaces after closing quote
				for i+1 < len(line) && line[i+1] == ' ' {
					i++
				}
			} else {
				// Start of quoted field
				inQuotes = true
			}
			continue
		}
		
		if !inQuotes && c == ' ' {
			if current.Len() > 0 {
				fields = append(fields, current.String())
				current.Reset()
			}
			continue
		}
		
		current.WriteRune(c)
	}
	
	// Add final field
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	
	return fields
}

// handleDirective processes @ directives
func (p *Parser) handleDirective(m *Manifest, directive string) error {
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
		m.CurrentLayer = &Layer{Type: LayerTypeAdd}
		
	case "@remove":
		m.CurrentLayer = &Layer{Type: LayerTypeRemove}
		
	case "@expand":
		if len(parts) < 2 {
			return fmt.Errorf("@expand requires C4 ID")
		}
		// Store expansion reference
		
	case "@by":
		if m.CurrentLayer != nil {
			m.CurrentLayer.By = strings.Join(parts[1:], " ")
		}
		
	case "@time":
		if m.CurrentLayer != nil && len(parts) > 1 {
			t, err := time.Parse(time.RFC3339, parts[1])
			if err != nil {
				return fmt.Errorf("invalid @time: %w", err)
			}
			m.CurrentLayer.Time = t
		}
		
	case "@note":
		if m.CurrentLayer != nil {
			m.CurrentLayer.Note = strings.Join(parts[1:], " ")
		}
		
	case "@data":
		if len(parts) > 1 {
			id, err := c4.Parse(parts[1])
			if err != nil {
				return fmt.Errorf("invalid @data C4 ID: %w", err)
			}
			if m.CurrentLayer != nil {
				m.CurrentLayer.Data = id
			} else {
				m.Data = id
			}
		}
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
	if permChars[0] == 'r' { perms |= 0400 }
	if permChars[1] == 'w' { perms |= 0200 }
	if permChars[2] == 'x' || permChars[2] == 's' { perms |= 0100 }
	
	// Group permissions
	if permChars[3] == 'r' { perms |= 0040 }
	if permChars[4] == 'w' { perms |= 0020 }
	if permChars[5] == 'x' || permChars[5] == 's' { perms |= 0010 }
	
	// Other permissions
	if permChars[6] == 'r' { perms |= 0004 }
	if permChars[7] == 'w' { perms |= 0002 }
	if permChars[8] == 'x' || permChars[8] == 't' { perms |= 0001 }
	
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

// DirectiveError indicates a directive was encountered
type DirectiveError struct {
	Directive string
}

func (e *DirectiveError) Error() string {
	return fmt.Sprintf("directive: %s", e.Directive)
}

// GenerateFromReader parses a C4M manifest from a reader
// This is a convenience function that creates a parser and parses the entire manifest
func GenerateFromReader(r io.Reader) (*Manifest, error) {
	parser := NewParser(r)
	return parser.ParseAll()
}