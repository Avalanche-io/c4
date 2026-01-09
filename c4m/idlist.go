package c4m

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Avalanche-io/c4"
)

// IDList represents an ordered list of C4 IDs, typically used for range/sequence files.
// The canonical form is one C4 ID per line with trailing newlines.
type IDList struct {
	IDs []c4.ID
}

// c4IDPattern matches a valid C4 ID (c4 followed by 88 base58 characters = 90 total)
var c4IDPattern = regexp.MustCompile(`^c4[1-9A-HJ-NP-Za-km-z]{88}$`)

// NewIDList creates a new empty ID list
func NewIDList() *IDList {
	return &IDList{
		IDs: make([]c4.ID, 0),
	}
}

// Add appends a C4 ID to the list
func (l *IDList) Add(id c4.ID) {
	l.IDs = append(l.IDs, id)
}

// Count returns the number of IDs in the list
func (l *IDList) Count() int {
	return len(l.IDs)
}

// Get returns the ID at the given index, or nil ID if out of bounds
func (l *IDList) Get(index int) c4.ID {
	if index < 0 || index >= len(l.IDs) {
		return c4.ID{}
	}
	return l.IDs[index]
}

// Canonical returns the canonical form of the ID list as a string.
// One C4 ID per line, trailing newline on each line.
func (l *IDList) Canonical() string {
	var buf strings.Builder
	for _, id := range l.IDs {
		buf.WriteString(id.String())
		buf.WriteByte('\n')
	}
	return buf.String()
}

// Bytes returns the canonical form as bytes
func (l *IDList) Bytes() []byte {
	return []byte(l.Canonical())
}

// ComputeC4ID computes the C4 ID of the canonical form of this ID list
func (l *IDList) ComputeC4ID() c4.ID {
	return c4.Identify(strings.NewReader(l.Canonical()))
}

// ParseIDList parses an ID list from a reader.
// It is tolerant of whitespace variations but validates that each line is a valid C4 ID.
func ParseIDList(r io.Reader) (*IDList, error) {
	list := NewIDList()
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

// ParseIDListFromString parses an ID list from a string
func ParseIDListFromString(s string) (*IDList, error) {
	return ParseIDList(strings.NewReader(s))
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

// DataBlock represents an embedded @data block in a manifest
type DataBlock struct {
	ID      c4.ID  // The C4 ID this block provides content for
	Content []byte // The raw content (decoded if it was base64)
	IsIDList bool  // True if content is a plain ID list, false if base64 encoded
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
		idList, err := ParseIDListFromString(content)
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

// GetIDList returns the content as an IDList if it is one, otherwise error
func (db *DataBlock) GetIDList() (*IDList, error) {
	if !db.IsIDList {
		return nil, fmt.Errorf("data block is not an ID list")
	}
	return ParseIDList(bytes.NewReader(db.Content))
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

// CreateDataBlockFromIDList creates a DataBlock from an IDList
func CreateDataBlockFromIDList(idList *IDList) *DataBlock {
	content := idList.Bytes()
	return &DataBlock{
		ID:       c4.Identify(bytes.NewReader(content)),
		Content:  content,
		IsIDList: true,
	}
}
