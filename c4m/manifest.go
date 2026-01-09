package c4m

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// Manifest represents a complete C4M manifest
type Manifest struct {
	Version      string
	Entries      []*Entry
	Base         c4.ID // For layered manifests
	Layers       []*Layer
	CurrentLayer *Layer // Current layer being parsed
	Data         c4.ID  // Application-specific metadata
	DataBlocks   []*DataBlock // Embedded @data blocks (for self-contained manifests)
}

// Layer represents a changeset layer
type Layer struct {
	Type LayerType
	By   string
	Time time.Time
	Note string
	Data c4.ID
}

// LayerType represents the type of layer
type LayerType int

const (
	LayerTypeAdd LayerType = iota
	LayerTypeRemove
)

// NewManifest creates a new empty manifest
func NewManifest() *Manifest {
	return &Manifest{
		Version: "1.0",
		Entries: make([]*Entry, 0),
	}
}

// AddEntry adds an entry to the manifest
func (m *Manifest) AddEntry(e *Entry) {
	m.Entries = append(m.Entries, e)
}

// Sort sorts entries using natural sort algorithm
func (m *Manifest) Sort() {
	sort.Slice(m.Entries, func(i, j int) bool {
		return NaturalLess(m.Entries[i].Name, m.Entries[j].Name)
	})
}

// WriteTo writes the manifest to a writer in canonical form
func (m *Manifest) WriteTo(w io.Writer) (int64, error) {
	return m.writeWithOptions(w, false, 2)
}

// WritePretty writes the manifest in ergonomic form with pretty-printing
func (m *Manifest) WritePretty(w io.Writer) (int64, error) {
	return m.writeWithOptions(w, true, 2)
}

// SortEntries sorts all entries in the manifest to ensure correct C4M ordering:
// files before directories at the same depth level, maintaining parent-child hierarchy.
// This is an alias for SortSiblingsHierarchically.
func (m *Manifest) SortEntries() {
	m.SortSiblingsHierarchically()
}


// writeWithOptions writes the manifest with formatting options
func (m *Manifest) writeWithOptions(w io.Writer, prettyPrint bool, indentWidth int) (int64, error) {
	// Ensure entries are properly sorted before output
	m.SortEntries()
	
	var written int64
	
	// Calculate formatting parameters if pretty-printing
	var maxSize int64
	var c4IDColumn int
	if prettyPrint {
		// Find max size and longest line for formatting
		for _, entry := range m.Entries {
			if entry.Size > maxSize {
				maxSize = entry.Size
			}
		}
		
		// Calculate C4 ID column position
		c4IDColumn = m.calculateC4IDColumn(indentWidth)
	}
	
	// Write header
	n, err := fmt.Fprintf(w, "@c4m %s\n", m.Version)
	written += int64(n)
	if err != nil {
		return written, err
	}
	
	// Write metadata if present
	if !m.Data.IsNil() {
		n, err = fmt.Fprintf(w, "@data %s\n", m.Data)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	// Write base if present
	if !m.Base.IsNil() {
		n, err = fmt.Fprintf(w, "@base %s\n", m.Base)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	// Write entries
	for _, entry := range m.Entries {
		var line string
		if prettyPrint {
			line = m.formatEntryPretty(entry, indentWidth, maxSize, c4IDColumn)
		} else {
			line = entry.Format(indentWidth, false)
		}
		n, err = fmt.Fprintf(w, "%s\n", line)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	// Write layers
	for _, layer := range m.Layers {
		n2, err := m.writeLayer(w, layer)
		written += n2
		if err != nil {
			return written, err
		}
	}

	// Write embedded data blocks
	for _, block := range m.DataBlocks {
		formatted := FormatDataBlock(block)
		n, err = fmt.Fprint(w, formatted)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}

	return written, nil
}

// writeLayer writes a layer section
func (m *Manifest) writeLayer(w io.Writer, layer *Layer) (int64, error) {
	var written int64
	
	// Write layer type
	var layerType string
	switch layer.Type {
	case LayerTypeAdd:
		layerType = "@layer"
	case LayerTypeRemove:
		layerType = "@remove"
	}
	
	n, err := fmt.Fprintf(w, "%s\n", layerType)
	written += int64(n)
	if err != nil {
		return written, err
	}
	
	// Write metadata
	if layer.By != "" {
		n, err = fmt.Fprintf(w, "@by %s\n", layer.By)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	if !layer.Time.IsZero() {
		n, err = fmt.Fprintf(w, "@time %s\n", layer.Time.Format(time.RFC3339))
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	if layer.Note != "" {
		n, err = fmt.Fprintf(w, "@note %s\n", layer.Note)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	if !layer.Data.IsNil() {
		n, err = fmt.Fprintf(w, "@data %s\n", layer.Data)
		written += int64(n)
		if err != nil {
			return written, err
		}
	}
	
	return written, nil
}

// AllEntriesString returns a string with all entries formatted hierarchically
func (m *Manifest) AllEntriesString() string {
	var buf bytes.Buffer
	
	// Write all entries with proper indentation
	for _, entry := range m.Entries {
		indent := strings.Repeat("  ", entry.Depth)
		buf.WriteString(indent)
		buf.WriteString(entry.Canonical())
		buf.WriteString("\n")
	}
	
	return buf.String()
}

// Canonical returns the canonical form for C4 ID computation
func (m *Manifest) Canonical() string {
	var buf bytes.Buffer
	
	// For canonical form, we need to find the minimum depth
	// and only include entries at that depth level
	minDepth := -1
	for _, entry := range m.Entries {
		// Skip directory entries themselves - we only want their contents at top level
		if strings.HasSuffix(entry.Name, "/") && entry.Depth > 0 {
			continue
		}
		if minDepth == -1 || entry.Depth < minDepth {
			minDepth = entry.Depth
		}
	}
	
	// If no entries, return empty
	if minDepth == -1 {
		return ""
	}
	
	// Collect entries at the minimum depth (top level of this manifest)
	topLevel := make([]*Entry, 0)
	for _, entry := range m.Entries {
		// For the top level, include files and directory entries (with trailing /)
		if entry.Depth == minDepth {
			// For directories at this level, they should have their C4 ID
			topLevel = append(topLevel, entry)
		}
	}
	
	// Sort entries
	sort.Slice(topLevel, func(i, j int) bool {
		// Files before directories
		iIsDir := strings.HasSuffix(topLevel[i].Name, "/")
		jIsDir := strings.HasSuffix(topLevel[j].Name, "/")
		if iIsDir != jIsDir {
			return !iIsDir // files first
		}
		return NaturalLess(topLevel[i].Name, topLevel[j].Name)
	})
	
	// Write canonical form
	for _, entry := range topLevel {
		buf.WriteString(entry.Canonical())
		buf.WriteByte('\n')
	}
	
	return buf.String()
}

// ComputeC4ID computes the C4 ID for the manifest
// IMPORTANT: This automatically canonicalizes the manifest before computing the ID
// This ensures deterministic IDs even if the manifest was created with null values
func (m *Manifest) ComputeC4ID() c4.ID {
	// Make a copy to avoid modifying the original
	canonical := m.Copy()

	// Ensure manifest is in canonical form
	canonical.Canonicalize()

	// Compute ID from canonical form
	canonicalText := canonical.Canonical()
	return c4.Identify(strings.NewReader(canonicalText))
}

// IsCanonical checks if manifest is in canonical form (no null values)
// Returns nil if canonical, or an error describing what's missing
func (m *Manifest) IsCanonical() error {
	var issues []string

	for i, entry := range m.Entries {
		nullFields := entry.GetNullFields()
		if len(nullFields) > 0 {
			issues = append(issues,
				fmt.Sprintf("Entry %d (%s) has null values: %s",
					i, entry.Name, strings.Join(nullFields, ", ")))
		}
	}

	if len(issues) > 0 {
		return fmt.Errorf("manifest not in canonical form:\n  %s",
			strings.Join(issues, "\n  "))
	}

	return nil
}

// Canonicalize resolves all null values in the manifest to explicit values
// This makes the manifest ready for C4 ID computation
func (m *Manifest) Canonicalize() {
	// First propagate metadata from children to parents
	PropagateMetadata(m.Entries)

	// Then apply defaults for any remaining null values
	for _, entry := range m.Entries {
		// Mode defaults
		if entry.Mode == 0 {
			if entry.IsDir() {
				entry.Mode = 0755 | os.ModeDir
			} else {
				entry.Mode = 0644
			}
		}

		// Timestamp defaults to current time if still null
		if entry.Timestamp.Unix() == 0 {
			entry.Timestamp = time.Now().UTC()
		}

		// Size defaults
		if entry.Size < 0 {
			entry.Size = 0 // Empty/unknown size
		}
	}
}

// Copy creates a deep copy of the manifest
func (m *Manifest) Copy() *Manifest {
	copy := &Manifest{
		Version: m.Version,
		Base:    m.Base,
		Entries: make([]*Entry, len(m.Entries)),
	}

	for i, e := range m.Entries {
		entryCopy := *e
		copy.Entries[i] = &entryCopy
	}

	return copy
}

// HasNullValues checks if any entries have null values
func (m *Manifest) HasNullValues() bool {
	for _, entry := range m.Entries {
		if entry.HasNullValues() {
			return true
		}
	}
	return false
}

// GetEntry finds an entry by path
func (m *Manifest) GetEntry(path string) *Entry {
	for _, e := range m.Entries {
		if e.Name == path {
			return e
		}
	}
	return nil
}

// GetEntriesAtDepth returns all entries at a specific depth
func (m *Manifest) GetEntriesAtDepth(depth int) []*Entry {
	var entries []*Entry
	for _, e := range m.Entries {
		if e.Depth == depth {
			entries = append(entries, e)
		}
	}
	return entries
}

// calculateC4IDColumn determines the appropriate column for C4 ID alignment
func (m *Manifest) calculateC4IDColumn(indentWidth int) int {
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
		indent := strings.Repeat(" ", entry.Depth*indentWidth)
		modeStr := formatMode(entry.Mode)
		// Use pretty timestamp format for length calculation
		timeStr := formatTimestampPretty(entry.Timestamp)
		
		// Use the padded size width (all sizes align to the same width)
		// This ensures proper calculation for the actual formatted output
		nameStr := formatName(entry.Name)
		
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
func (m *Manifest) formatEntryPretty(entry *Entry, indentWidth int, maxSize int64, c4IDColumn int) string {
	// Build indentation
	indent := strings.Repeat(" ", entry.Depth*indentWidth)
	
	// Format mode (handle null value)
	var modeStr string
	if entry.Mode == 0 && !entry.IsDir() && !entry.IsSymlink() {
		modeStr = "----------"  // Null mode
	} else {
		modeStr = formatMode(entry.Mode)
	}
	
	// Format timestamp (handle null value)
	var timeStr string
	if entry.Timestamp.Unix() == 0 {
		timeStr = "-                        "  // Null timestamp (padded to match typical timestamp width)
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
	nameStr := formatName(entry.Name)
	
	// Build base line
	parts := []string{indent + modeStr, timeStr, sizeStr, nameStr}
	
	// Add symlink target if present
	if entry.Target != "" {
		parts = append(parts, "->", entry.Target)
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

// formatSizePretty formats size with padding and thousand separators
func formatSizePretty(size, maxSize int64) string {
	// Format with commas
	sizeWithCommas := formatSizeWithCommas(size)
	
	// Calculate padding based on max size with commas
	maxSizeStr := formatSizeWithCommas(maxSize)
	padding := len(maxSizeStr) - len(sizeWithCommas)
	
	return strings.Repeat(" ", padding) + sizeWithCommas
}

// formatTimestampPretty formats timestamp in human-readable format with timezone
// Format: "Jan  2 15:04:05 2006 MST" (similar to ls -lT)
func formatTimestampPretty(t time.Time) string {
	// Convert to local time
	local := t.Local()
	// Use a format similar to ls -lT with full precision
	// Fixed width: 3 chars month + 3 chars day + 9 chars time + 5 chars year + 4 chars tz = 24 chars
	return local.Format("Jan _2 15:04:05 2006 MST")
}

// formatSizeWithCommas adds thousand separators to a number
func formatSizeWithCommas(size int64) string {
	if size == 0 {
		return "0"
	}
	
	sign := ""
	if size < 0 {
		sign = "-"
		size = -size
	}
	
	s := fmt.Sprintf("%d", size)
	
	// Add commas every 3 digits from the right
	var result []byte
	for i := len(s) - 1; i >= 0; i-- {
		if len(result) > 0 && (len(s)-i-1)%3 == 0 {
			result = append([]byte{','}, result...)
		}
		result = append([]byte{s[i]}, result...)
	}
	
	return sign + string(result)
}

// Validate performs validation on the manifest
func (m *Manifest) Validate() error {
	// Check version
	if m.Version == "" {
		return fmt.Errorf("missing version")
	}
	
	// Check for duplicate paths
	seen := make(map[string]bool)
	for _, e := range m.Entries {
		if seen[e.Name] {
			return fmt.Errorf("duplicate path: %s", e.Name)
		}
		seen[e.Name] = true
		
		// Validate entry
		if e.Name == "" {
			return fmt.Errorf("empty name in entry")
		}
		
		if e.Timestamp.IsZero() {
			return fmt.Errorf("zero timestamp for %s", e.Name)
		}
		
		if e.Size < 0 {
			return fmt.Errorf("negative size for %s", e.Name)
		}
		
		// Check for path traversal
		if strings.Contains(e.Name, "../") || strings.Contains(e.Name, "./") {
			return fmt.Errorf("path traversal in %s", e.Name)
		}
	}

	return nil
}

// AddDataBlock adds an embedded data block to the manifest
func (m *Manifest) AddDataBlock(block *DataBlock) {
	m.DataBlocks = append(m.DataBlocks, block)
}

// GetDataBlock retrieves an embedded data block by its C4 ID
func (m *Manifest) GetDataBlock(id c4.ID) *DataBlock {
	for _, block := range m.DataBlocks {
		if block.ID == id {
			return block
		}
	}
	return nil
}

// HasDataBlock checks if a data block with the given ID is embedded
func (m *Manifest) HasDataBlock(id c4.ID) bool {
	return m.GetDataBlock(id) != nil
}

// GetIDList retrieves an embedded ID list by its C4 ID
// Returns nil if not found or if the block is not an ID list
func (m *Manifest) GetIDList(id c4.ID) (*IDList, error) {
	block := m.GetDataBlock(id)
	if block == nil {
		return nil, fmt.Errorf("data block not found: %s", id)
	}
	return block.GetIDList()
}