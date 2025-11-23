package c4m

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// Entry represents a single file or directory entry in a C4M manifest
type Entry struct {
	Mode      os.FileMode // Unix-style permissions
	Timestamp time.Time   // UTC timestamp
	Size      int64       // File size in bytes
	Name      string      // File/directory name
	Target    string      // Symlink target (if applicable)
	C4ID      c4.ID       // Content identifier
	Depth     int         // Indentation level
	
	// For sequences
	IsSequence bool
	Pattern    string // Original sequence pattern
}

// IsDir returns true if the entry represents a directory
func (e *Entry) IsDir() bool {
	return e.Mode.IsDir()
}

// IsSymlink returns true if the entry represents a symbolic link
func (e *Entry) IsSymlink() bool {
	return e.Mode&os.ModeSymlink != 0
}

// BaseName returns the base name without path
func (e *Entry) BaseName() string {
	return path.Base(e.Name)
}

// String returns the formatted string representation of the entry
func (e *Entry) String() string {
	return e.Format(0, false)
}

// Format returns the formatted string representation with options
func (e *Entry) Format(indentWidth int, displayFormat bool) string {
	// Build indentation
	indent := strings.Repeat(" ", e.Depth*indentWidth)
	
	// Format mode (handle null value)
	var modeStr string
	if e.Mode == 0 && !e.IsDir() && !e.IsSymlink() {
		modeStr = "----------"  // Null mode
	} else {
		modeStr = formatMode(e.Mode)
	}
	
	// Format timestamp (handle null value)
	var timeStr string
	if e.Timestamp.Unix() == 0 {
		timeStr = "-"  // Null timestamp
	} else {
		// Canonical format MUST be UTC only
		timeStr = e.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
	}
	
	// Format size (handle null value)
	var sizeStr string
	if e.Size < 0 {
		sizeStr = "-"  // Null size
	} else {
		sizeStr = formatSize(e.Size, displayFormat)
	}
	
	// Format name (with quotes if needed)
	nameStr := formatName(e.Name)
	
	// Build the line
	parts := []string{indent + modeStr, timeStr, sizeStr, nameStr}
	
	// Add symlink target if present
	if e.Target != "" {
		parts = append(parts, "->", e.Target)
	}
	
	// Add C4 ID if present
	if !e.C4ID.IsNil() {
		parts = append(parts, e.C4ID.String())
	}
	
	return strings.Join(parts, " ")
}

// Canonical returns the canonical form for C4 ID computation
func (e *Entry) Canonical() string {
	// No indentation in canonical form
	modeStr := formatMode(e.Mode)
	// Canonical format MUST be UTC only
	timeStr := e.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
	sizeStr := fmt.Sprintf("%d", e.Size) // No formatting in canonical
	nameStr := formatName(e.Name)
	
	parts := []string{modeStr, timeStr, sizeStr, nameStr}
	
	if e.Target != "" {
		parts = append(parts, "->", e.Target)
	}
	
	if !e.C4ID.IsNil() {
		parts = append(parts, e.C4ID.String())
	}
	
	return strings.Join(parts, " ")
}

// formatMode converts os.FileMode to Unix-style permission string
func formatMode(mode os.FileMode) string {
	const str = "dalTLDpSugct?"
	var buf [10]byte
	
	// File type
	switch mode & os.ModeType {
	case 0: // regular file
		buf[0] = '-'
	case os.ModeDir:
		buf[0] = 'd'
	case os.ModeSymlink:
		buf[0] = 'l'
	case os.ModeNamedPipe:
		buf[0] = 'p'
	case os.ModeSocket:
		buf[0] = 's'
	case os.ModeDevice:
		buf[0] = 'b'
	case os.ModeCharDevice:
		buf[0] = 'c'
	default:
		buf[0] = '?'
	}
	
	// Permissions
	rwx := "rwxrwxrwx"
	for i := 0; i < 9; i++ {
		if mode&(1<<uint(9-1-i)) != 0 {
			buf[i+1] = rwx[i]
		} else {
			buf[i+1] = '-'
		}
	}
	
	// Special bits
	if mode&os.ModeSetuid != 0 {
		if buf[3] == 'x' {
			buf[3] = 's'
		} else {
			buf[3] = 'S'
		}
	}
	if mode&os.ModeSetgid != 0 {
		if buf[6] == 'x' {
			buf[6] = 's'
		} else {
			buf[6] = 'S'
		}
	}
	if mode&os.ModeSticky != 0 {
		if buf[9] == 'x' {
			buf[9] = 't'
		} else {
			buf[9] = 'T'
		}
	}
	
	return string(buf[:])
}

// formatSize formats the size field
func formatSize(size int64, displayFormat bool) string {
	if !displayFormat {
		return fmt.Sprintf("%d", size)
	}
	
	// Add thousands separators for display
	s := fmt.Sprintf("%d", size)
	if len(s) <= 3 {
		return s
	}
	
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// formatName adds quotes if the name contains special characters
func formatName(name string) string {
	// For directories (ending with /), never use quotes
	// The trailing slash makes the boundary unambiguous
	if strings.HasSuffix(name, "/") {
		// Still escape backslashes and newlines for safety
		escaped := strings.ReplaceAll(name, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, "\n", `\n`)
		return escaped
	}

	// For files, check if quoting is needed
	needsQuotes := false
	for _, c := range name {
		if c == ' ' || c == '"' || c == '\\' || c == '\n' {
			needsQuotes = true
			break
		}
	}

	// Check for leading/trailing whitespace
	if name != strings.TrimSpace(name) {
		needsQuotes = true
	}

	if !needsQuotes {
		return name
	}

	// Escape special characters and quote
	escaped := strings.ReplaceAll(name, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)

	return fmt.Sprintf(`"%s"`, escaped)
}

// IsDevice returns true if the entry represents a device
func (e *Entry) IsDevice() bool {
	return e.Mode&os.ModeDevice != 0 || e.Mode&os.ModeCharDevice != 0
}

// IsPipe returns true if the entry represents a named pipe
func (e *Entry) IsPipe() bool {
	return e.Mode&os.ModeNamedPipe != 0
}

// IsSocket returns true if the entry represents a socket
func (e *Entry) IsSocket() bool {
	return e.Mode&os.ModeSocket != 0
}

// HasNullValues returns true if entry has any null metadata
func (e *Entry) HasNullValues() bool {
	// Mode can be 0 for certain file types, so check type
	hasNullMode := e.Mode == 0 && !e.IsDir() && !e.IsSymlink() && !e.IsDevice() && !e.IsPipe() && !e.IsSocket()
	hasNullTimestamp := e.Timestamp.Unix() == 0
	hasNullSize := e.Size < 0
	// C4ID being nil is OK for empty files or directories without computed IDs yet

	return hasNullMode || hasNullTimestamp || hasNullSize
}

// GetNullFields returns list of fields that have null values
func (e *Entry) GetNullFields() []string {
	var nullFields []string

	if e.Mode == 0 && !e.IsDir() && !e.IsSymlink() {
		nullFields = append(nullFields, "Mode")
	}
	if e.Timestamp.Unix() == 0 {
		nullFields = append(nullFields, "Timestamp")
	}
	if e.Size < 0 {
		nullFields = append(nullFields, "Size")
	}

	return nullFields
}

// IsFullySpecified returns true if all required metadata is explicit
func (e *Entry) IsFullySpecified() bool {
	return !e.HasNullValues()
}