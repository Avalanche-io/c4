package c4m

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Encoder writes manifests to an output stream.
type Encoder struct {
	w           io.Writer
	pretty      bool
	indentWidth int
}

// NewEncoder creates a new Encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{
		w:           w,
		pretty:      false,
		indentWidth: 2,
	}
}

// SetPretty enables or disables pretty-printing (human-readable format).
// When enabled, output includes aligned columns, formatted sizes with commas,
// and timestamps in local time. When disabled (default), output is in canonical form.
func (e *Encoder) SetPretty(pretty bool) *Encoder {
	e.pretty = pretty
	return e
}

// SetIndent sets the indentation width for nested entries.
// Default is 2 spaces.
func (e *Encoder) SetIndent(width int) *Encoder {
	e.indentWidth = width
	return e
}

// Encode writes the manifest to the encoder's output stream.
func (e *Encoder) Encode(m *Manifest) error {
	// Ensure entries are properly sorted before output
	m.SortEntries()

	// Calculate formatting parameters if pretty-printing
	var maxSize int64
	var c4IDColumn int
	if e.pretty {
		for _, entry := range m.Entries {
			if entry.Size > maxSize {
				maxSize = entry.Size
			}
		}
		c4IDColumn = e.calculateC4IDColumn(m)
	}

	// Write header
	if _, err := fmt.Fprintf(e.w, "@c4m %s\n", m.Version); err != nil {
		return err
	}

	// Write @intent if set
	if m.Intent {
		if _, err := fmt.Fprint(e.w, "@intent\n"); err != nil {
			return err
		}
	}

	// Write metadata if present
	if !m.Data.IsNil() {
		if _, err := fmt.Fprintf(e.w, "@data %s\n", m.Data); err != nil {
			return err
		}
	}

	// Write base if present
	if !m.Base.IsNil() {
		if _, err := fmt.Fprintf(e.w, "@base %s\n", m.Base); err != nil {
			return err
		}
	}

	// Write non-remove entries
	for _, entry := range m.Entries {
		if entry.removeLayer {
			continue
		}
		var line string
		if e.pretty {
			line = e.formatEntryPretty(entry, maxSize, c4IDColumn)
		} else {
			line = entry.Format(e.indentWidth, false)
		}
		if _, err := fmt.Fprintf(e.w, "%s\n", line); err != nil {
			return err
		}
	}

	// Write layers
	for _, layer := range m.Layers {
		if err := e.writeLayer(layer); err != nil {
			return err
		}
		// Write remove entries within @remove sections
		if layer.Type == LayerTypeRemove {
			for _, entry := range m.Entries {
				if !entry.removeLayer {
					continue
				}
				line := entry.Format(e.indentWidth, false)
				if _, err := fmt.Fprintf(e.w, "%s\n", line); err != nil {
					return err
				}
			}
		}
	}

	// Write embedded data blocks
	for _, block := range m.DataBlocks {
		formatted := FormatDataBlock(block)
		if _, err := fmt.Fprint(e.w, formatted); err != nil {
			return err
		}
	}

	return nil
}

// writeLayer writes a layer section
func (e *Encoder) writeLayer(layer *Layer) error {
	// Write layer type
	var layerType string
	switch layer.Type {
	case LayerTypeAdd:
		layerType = "@layer"
	case LayerTypeRemove:
		layerType = "@remove"
	}

	if _, err := fmt.Fprintf(e.w, "%s\n", layerType); err != nil {
		return err
	}

	// Write metadata
	if layer.By != "" {
		if _, err := fmt.Fprintf(e.w, "@by %s\n", layer.By); err != nil {
			return err
		}
	}

	if !layer.Time.IsZero() {
		if _, err := fmt.Fprintf(e.w, "@time %s\n", layer.Time.Format(TimestampFormat)); err != nil {
			return err
		}
	}

	if layer.Note != "" {
		if _, err := fmt.Fprintf(e.w, "@note %s\n", layer.Note); err != nil {
			return err
		}
	}

	if !layer.Data.IsNil() {
		if _, err := fmt.Fprintf(e.w, "@data %s\n", layer.Data); err != nil {
			return err
		}
	}

	return nil
}

// calculateC4IDColumn determines the appropriate column for C4 ID alignment
func (e *Encoder) calculateC4IDColumn(m *Manifest) int {
	// First find the maximum size to determine padding width
	maxSize := int64(0)
	for _, entry := range m.Entries {
		if entry.Size > maxSize {
			maxSize = entry.Size
		}
	}
	maxSizeWidth := len(formatSizeWithCommas(maxSize))

	maxLen := 0
	for _, entry := range m.Entries {
		// Calculate line length without C4 ID
		indent := strings.Repeat(" ", entry.Depth*e.indentWidth)
		modeStr := formatMode(entry.Mode)
		timeStr := formatTimestampPretty(entry.Timestamp)
		nameStr := formatName(entry.Name, entry.IsSequence)

		lineLen := len(indent) + len(modeStr) + 1 + len(timeStr) + 1 + maxSizeWidth + 1 + len(nameStr)
		if entry.Target != "" {
			lineLen += 4 + len(entry.Target) // " -> " + target
		}

		if lineLen > maxLen {
			maxLen = lineLen
		}
	}

	// Start at column 80, shift by 10 if needed
	// Use minimum 10 spaces between content and C4 ID
	minSpacing := 10
	column := 80
	for maxLen+minSpacing > column {
		column += 10
	}
	return column
}

// formatEntryPretty formats an entry with ergonomic pretty-printing
func (e *Encoder) formatEntryPretty(entry *Entry, maxSize int64, c4IDColumn int) string {
	// Build indentation
	indent := strings.Repeat(" ", entry.Depth*e.indentWidth)

	// Format mode (handle null value)
	var modeStr string
	if entry.Mode == 0 && !entry.IsDir() && !entry.IsSymlink() {
		modeStr = "----------" // Null mode
	} else {
		modeStr = formatMode(entry.Mode)
	}

	// Format timestamp (handle null value)
	var timeStr string
	if entry.Timestamp.Equal(NullTimestamp()) {
		timeStr = "-                        " // Null timestamp (padded to match typical timestamp width)
	} else {
		timeStr = formatTimestampPretty(entry.Timestamp)
	}

	// Format size with padding and commas (handle null value)
	var sizeStr string
	if entry.Size < 0 {
		// Calculate padding for null size
		maxSizeStr := formatSizeWithCommas(maxSize)
		padding := len(maxSizeStr) - 1
		sizeStr = strings.Repeat(" ", padding) + "-"
	} else {
		sizeStr = formatSizePretty(entry.Size, maxSize)
	}

	// Format name (with quotes if needed)
	nameStr := formatName(entry.Name, entry.IsSequence)

	// Build base line
	parts := []string{indent + modeStr, timeStr, sizeStr, nameStr}

	// Add symlink target if present
	if entry.Target != "" {
		parts = append(parts, "->", formatTarget(entry.Target))
	}

	baseLine := strings.Join(parts, " ")

	// Add C4 ID with column alignment if present
	if !entry.C4ID.IsNil() {
		padding := c4IDColumn - len(baseLine)
		if padding < 10 {
			padding = 10 // Minimum 10 spaces for readability
		}
		return baseLine + strings.Repeat(" ", padding) + entry.C4ID.String()
	}

	return baseLine
}

// ----------------------------------------------------------------------------
// Convenience Functions
// ----------------------------------------------------------------------------

// Marshal returns the canonical C4M encoding of m.
func Marshal(m *Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalPretty returns the pretty-printed C4M encoding of m.
// Pretty format includes aligned columns, formatted sizes with commas,
// and timestamps in local time.
func MarshalPretty(m *Manifest) ([]byte, error) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf).SetPretty(true)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal parses the C4M-encoded data and returns a Manifest.
func Unmarshal(data []byte) (*Manifest, error) {
	dec := NewDecoder(bytes.NewReader(data))
	return dec.Decode()
}

// Format parses and re-formats src in canonical C4M style.
// It returns the formatted result or an error if src is not valid C4M.
func Format(src []byte) ([]byte, error) {
	m, err := Unmarshal(src)
	if err != nil {
		return nil, err
	}
	return Marshal(m)
}

// FormatPretty parses and re-formats src in pretty-printed C4M style.
// It returns the formatted result or an error if src is not valid C4M.
func FormatPretty(src []byte) ([]byte, error) {
	m, err := Unmarshal(src)
	if err != nil {
		return nil, err
	}
	return MarshalPretty(m)
}
