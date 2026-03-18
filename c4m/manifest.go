package c4m

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Avalanche-io/c4"
)

// TimestampFormat is the canonical C4M timestamp format (RFC3339 UTC).
const TimestampFormat = "2006-01-02T15:04:05Z"

// nullTimestamp is the internal sentinel for null/unspecified timestamps.
var nullTimestamp = time.Unix(0, 0).UTC()

// NullTimestamp returns the sentinel value for null/unspecified timestamps (Unix epoch).
func NullTimestamp() time.Time { return nullTimestamp }

// Manifest represents a complete C4M manifest
type Manifest struct {
	Version   string
	Base      c4.ID            // External base manifest (from first-line bare C4 ID)
	Entries   []*Entry
	RangeData map[c4.ID]string // Inline ID lists keyed by sequence C4 ID (bare concatenation)
	index     *treeIndex       // Lazily-built tree index for O(1) navigation
}

// NewManifest creates a new empty manifest
func NewManifest() *Manifest {
	return &Manifest{
		Version: "1.0",
		Entries: make([]*Entry, 0),
	}
}

// AddEntry adds an entry to the manifest.
func (m *Manifest) AddEntry(e *Entry) {
	m.Entries = append(m.Entries, e)
	m.invalidateIndex()
}

// isPathName returns true if name contains any path semantics.
// A valid c4m entry name is a bare filename optionally followed by a
// trailing "/" to mark directories. The base name (without trailing slash)
// must not be empty, must not be "." or "..", and must not contain "/" or
// "\" or null bytes. This is fully decidable because c4m unifies all
// paths to Unix-style separators.
func isPathName(name string) bool {
	if name == "" {
		return true
	}
	// Strip trailing "/" (directory marker) to get the base name.
	base := strings.TrimSuffix(name, "/")
	if base == "" {
		return true // name was just "/"
	}
	if base == "." || base == ".." {
		return true
	}
	if strings.ContainsAny(base, "/\\\x00") {
		return true
	}
	return false
}

// RemoveEntry removes an entry from the manifest by pointer identity.
func (m *Manifest) RemoveEntry(e *Entry) {
	for i, existing := range m.Entries {
		if existing == e {
			m.Entries = append(m.Entries[:i], m.Entries[i+1:]...)
			break
		}
	}
	m.invalidateIndex()
}

// InvalidateIndex forces the tree index to be rebuilt on next access.
func (m *Manifest) InvalidateIndex() {
	m.invalidateIndex()
}

// SortEntries sorts all entries in the manifest to ensure correct C4M ordering:
// files before directories at the same depth level, maintaining parent-child hierarchy.
func (m *Manifest) SortEntries() {
	m.sortSiblingsHierarchically()
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

// Canonicalize resolves all null values in the manifest to explicit values,
// modifying the receiver in place. This makes the manifest ready for C4 ID
// computation. Use Copy() first if you need to preserve the original.
func (m *Manifest) Canonicalize() {
	// Propagate metadata from children to parents (e.g., directory sizes, timestamps)
	propagateMetadata(m.Entries)

	// Null values stay null — they render as "-" in canonical form.
	// No default substitution: unknown mode/size/timestamp remain as-is.
}

// Copy creates a deep copy of the manifest
func (m *Manifest) Copy() *Manifest {
	cp := &Manifest{
		Version: m.Version,
		Base:    m.Base,
		Entries: make([]*Entry, len(m.Entries)),
	}

	for i, e := range m.Entries {
		entryCopy := *e
		cp.Entries[i] = &entryCopy
	}

	if len(m.RangeData) > 0 {
		cp.RangeData = make(map[c4.ID]string, len(m.RangeData))
		for k, v := range m.RangeData {
			cp.RangeData[k] = v
		}
	}

	return cp
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

// GetEntry returns an entry by its path (O(1) after index build).
func (m *Manifest) GetEntry(path string) *Entry {
	idx := m.ensureIndex()
	return idx.byPath[path]
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
	
	// Check for duplicate paths by computing full paths from the tree.
	seen := make(map[string]bool)
	var dirStack []string
	for _, e := range m.Entries {
		// Validate entry — name must be a bare filename, never a path
		if e.Name == "" {
			return fmt.Errorf("%w: empty name", ErrInvalidEntry)
		}
		if isPathName(e.Name) {
			return fmt.Errorf("%w: %s", ErrPathTraversal, e.Name)
		}

		// Build full path from parent context
		if e.Depth < len(dirStack) {
			dirStack = dirStack[:e.Depth]
		}
		var fullPath string
		if len(dirStack) > 0 {
			fullPath = strings.Join(dirStack, "") + e.Name
		} else {
			fullPath = e.Name
		}
		if seen[fullPath] {
			return fmt.Errorf("%w: %s", ErrDuplicatePath, fullPath)
		}
		seen[fullPath] = true

		if e.IsDir() {
			for len(dirStack) <= e.Depth {
				dirStack = append(dirStack, "")
			}
			dirStack[e.Depth] = e.Name
		}
	}

	return nil
}

// ----------------------------------------------------------------------------
// Sorting
// ----------------------------------------------------------------------------

// sortSiblingsHierarchically sorts manifest entries to maintain proper C4M format:
// - Preserves hierarchical depth-first traversal
// - Files before directories at same level
// - Natural sort for names within siblings
func (m *Manifest) sortSiblingsHierarchically() {
	if len(m.Entries) == 0 {
		return
	}

	// We'll build a new sorted list while preserving hierarchy
	result := make([]*Entry, 0, len(m.Entries))
	used := make([]bool, len(m.Entries))

	// Process entries depth-first, sorting siblings at each level
	var processLevel func(parentIdx int, parentDepth int)
	processLevel = func(parentIdx int, parentDepth int) {
		// Find all children at the next depth level
		childDepth := parentDepth + 1
		startIdx := parentIdx + 1

		// Special case for root level
		if parentIdx == -1 {
			startIdx = 0
			childDepth = 0
		}

		// Collect all immediate children
		type child struct {
			entry *Entry
			index int
		}
		children := []child{}

		for i := startIdx; i < len(m.Entries); i++ {
			if used[i] {
				continue
			}

			entry := m.Entries[i]

			// Stop when we've gone back up the hierarchy
			if entry.Depth < childDepth {
				break
			}

			// Skip deeper descendants - they'll be processed recursively
			if entry.Depth > childDepth {
				continue
			}

			// This is an immediate child
			children = append(children, child{entry, i})
		}

		// Deduplicate siblings by name, keeping the last occurrence
		// (most recently added entry wins). Mark replaced entries as
		// used so they don't reappear as orphans.
		{
			seen := make(map[string]int) // name -> index in children
			deduped := children[:0]
			for _, c := range children {
				if idx, ok := seen[c.entry.Name]; ok {
					used[deduped[idx].index] = true // mark replaced entry
					deduped[idx] = c
				} else {
					seen[c.entry.Name] = len(deduped)
					deduped = append(deduped, c)
				}
			}
			children = deduped
		}

		// Sort the children (files before dirs, then natural sort)
		sort.Slice(children, func(i, j int) bool {
			a, b := children[i].entry, children[j].entry

			// Files before directories
			if a.IsDir() != b.IsDir() {
				return !a.IsDir() // files first
			}

			// Natural sort for names
			return NaturalLess(a.Name, b.Name)
		})

		// Process sorted children
		for _, c := range children {
			used[c.index] = true
			result = append(result, c.entry)

			// If it's a directory, recursively process its children
			if c.entry.IsDir() {
				processLevel(c.index, c.entry.Depth)
			}
		}
	}

	// Start from root level
	processLevel(-1, -1)

	// If any entries weren't processed (orphaned), add them at the end
	// This can happen with incomplete chunks
	for i, entry := range m.Entries {
		if !used[i] {
			// Silently handle orphaned entries - this is expected in continuation chunks
			result = append(result, entry)
		}
	}

	m.Entries = result
}

// ----------------------------------------------------------------------------
// Metadata Propagation
// ----------------------------------------------------------------------------

// propagateMetadata resolves null values in entries by propagating from children.
// This is used for directory entries to compute size and timestamp from contents.
// Iterates in reverse so child directories are resolved before their parents.
func propagateMetadata(entries []*Entry) {
	// Process deepest directories first (reverse order)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]

		if entry.IsDir() && entry.HasNullValues() {
			// Get children of this directory
			children := getDirectoryChildren(entries, entry)

			// Propagate size if null
			if entry.Size < 0 {
				entry.Size = calculateDirectorySize(children)
			}

			// Propagate timestamp if null
			if entry.Timestamp.Equal(NullTimestamp()) {
				entry.Timestamp = getMostRecentModtime(children)
			}
		}
	}
}

// getDirectoryChildren returns all entries that are direct children of a directory
func getDirectoryChildren(entries []*Entry, dir *Entry) []*Entry {
	var children []*Entry
	dirDepth := dir.Depth

	// Find entries at depth+1 that appear after this directory
	collecting := false
	for _, e := range entries {
		if e == dir {
			collecting = true
			continue
		}
		if collecting {
			if e.Depth == dirDepth+1 {
				children = append(children, e)
			} else if e.Depth <= dirDepth {
				// Reached next sibling or parent, stop
				break
			}
		}
	}

	return children
}

// calculateDirectorySize computes the total size of direct children.
// Nil-infectious: if any child has null size (-1), the result is null (-1).
func calculateDirectorySize(entries []*Entry) int64 {
	var total int64
	for _, e := range entries {
		if e.Size < 0 {
			return -1 // unknown propagates upward
		}
		total += e.Size
	}
	return total
}

// getMostRecentModtime finds the most recent modification time among entries.
// Nil-infectious: if any child has a null timestamp, the result is null.
func getMostRecentModtime(entries []*Entry) time.Time {
	var mostRecent time.Time
	null := NullTimestamp()

	for _, e := range entries {
		if e.Timestamp.Equal(null) {
			return null // unknown propagates upward
		}
		if e.Timestamp.After(mostRecent) {
			mostRecent = e.Timestamp
		}
	}

	if mostRecent.IsZero() {
		return null
	}

	return mostRecent
}

// ----------------------------------------------------------------------------
// Tree Index and Navigation
// ----------------------------------------------------------------------------

// treeIndex provides O(1) navigation through manifest hierarchy
type treeIndex struct {
	byPath   map[string]*Entry   // path -> entry
	children map[*Entry][]*Entry // parent -> direct children
	parent   map[*Entry]*Entry   // child -> parent
	root     []*Entry            // depth-0 entries
}

// invalidateIndex marks the tree index as stale
func (m *Manifest) invalidateIndex() {
	m.index = nil
}

// ensureIndex builds the tree index if needed
func (m *Manifest) ensureIndex() *treeIndex {
	if m.index != nil {
		return m.index
	}

	idx := &treeIndex{
		byPath:   make(map[string]*Entry),
		children: make(map[*Entry][]*Entry),
		parent:   make(map[*Entry]*Entry),
		root:     make([]*Entry, 0),
	}

	// Build path lookup and collect root entries
	for _, e := range m.Entries {
		idx.byPath[e.Name] = e
		if e.Depth == 0 {
			idx.root = append(idx.root, e)
		}
	}

	// Build parent-child relationships
	// For each entry, find its parent based on depth and position
	for i, e := range m.Entries {
		if e.Depth == 0 {
			continue // Root entries have no parent
		}

		// Search backwards for parent (first directory at depth-1)
		for j := i - 1; j >= 0; j-- {
			candidate := m.Entries[j]
			if candidate.Depth == e.Depth-1 && candidate.IsDir() {
				idx.parent[e] = candidate
				idx.children[candidate] = append(idx.children[candidate], e)
				break
			}
			// Stop if we've gone past possible parents
			if candidate.Depth < e.Depth-1 {
				break
			}
		}
	}

	m.index = idx
	return idx
}

// Children returns the direct children of an entry
func (m *Manifest) Children(e *Entry) []*Entry {
	if e == nil || !e.IsDir() {
		return nil
	}
	idx := m.ensureIndex()
	return idx.children[e]
}

// Parent returns the parent directory of an entry
func (m *Manifest) Parent(e *Entry) *Entry {
	if e == nil || e.Depth == 0 {
		return nil
	}
	idx := m.ensureIndex()
	return idx.parent[e]
}

// Siblings returns entries at the same depth with the same parent
func (m *Manifest) Siblings(e *Entry) []*Entry {
	if e == nil {
		return nil
	}

	idx := m.ensureIndex()
	parent := idx.parent[e]

	var siblings []*Entry
	if parent == nil {
		// Root level - siblings are other root entries
		for _, r := range idx.root {
			if r != e {
				siblings = append(siblings, r)
			}
		}
	} else {
		// Non-root - siblings are other children of same parent
		for _, c := range idx.children[parent] {
			if c != e {
				siblings = append(siblings, c)
			}
		}
	}

	return siblings
}

// Ancestors returns all parent entries from immediate parent to root
func (m *Manifest) Ancestors(e *Entry) []*Entry {
	if e == nil || e.Depth == 0 {
		return nil
	}

	idx := m.ensureIndex()
	var ancestors []*Entry

	current := idx.parent[e]
	for current != nil {
		ancestors = append(ancestors, current)
		current = idx.parent[current]
	}

	return ancestors
}

// Descendants returns all entries nested under this entry
func (m *Manifest) Descendants(e *Entry) []*Entry {
	if e == nil || !e.IsDir() {
		return nil
	}

	idx := m.ensureIndex()
	var descendants []*Entry

	var collect func(*Entry)
	collect = func(parent *Entry) {
		for _, child := range idx.children[parent] {
			descendants = append(descendants, child)
			if child.IsDir() {
				collect(child)
			}
		}
	}

	collect(e)
	return descendants
}

// Root returns all depth-0 entries
func (m *Manifest) Root() []*Entry {
	idx := m.ensureIndex()
	return idx.root
}