package c4m

import (
	"sort"
	"strings"
)

// SortSiblingsHierarchically sorts manifest entries to maintain proper C4M format:
// - Preserves hierarchical depth-first traversal  
// - Files before directories at same level
// - Natural sort for names within siblings
func (m *Manifest) SortSiblingsHierarchically() {
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
		
		// Sort the children (files before dirs, then natural sort)
		sort.Slice(children, func(i, j int) bool {
			a, b := children[i].entry, children[j].entry
			
			// Files before directories
			aIsDir := a.Mode.IsDir() || strings.HasSuffix(a.Name, "/")
			bIsDir := b.Mode.IsDir() || strings.HasSuffix(b.Name, "/")
			
			if aIsDir != bIsDir {
				return !aIsDir // files first
			}
			
			// Natural sort for names
			return NaturalLess(a.Name, b.Name)
		})
		
		// Process sorted children
		for _, c := range children {
			used[c.index] = true
			result = append(result, c.entry)
			
			// If it's a directory, recursively process its children
			if c.entry.Mode.IsDir() || strings.HasSuffix(c.entry.Name, "/") {
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

// buildSortKey constructs a sortable key for an entry using null-byte separators
func buildSortKey(entry *Entry, allEntries []*Entry, entryIndex int) string {
	// Build path components from this entry up to root
	components := []string{}
	currentIndex := entryIndex
	currentDepth := entry.Depth

	// Add the entry's own name
	name := strings.TrimSuffix(entry.Name, "/")
	
	// For natural sorting, pad numbers in the name
	name = padNumbers(name)
	
	// Prepend 'D' for directories, 'F' for files to ensure files come first
	if entry.Mode.IsDir() || strings.HasSuffix(entry.Name, "/") {
		name = "D" + name
	} else {
		name = "F" + name
	}
	components = append([]string{name}, components...)

	// Walk backwards to find parent directories
	for currentDepth > 0 {
		// Find the parent directory (at depth - 1)
		for i := currentIndex - 1; i >= 0; i-- {
			e := allEntries[i]
			if e.Depth == currentDepth-1 && (e.Mode.IsDir() || strings.HasSuffix(e.Name, "/")) {
				parentName := strings.TrimSuffix(e.Name, "/")
				parentName = padNumbers(parentName)
				parentName = "D" + parentName // Parents are always directories
				components = append([]string{parentName}, components...)
				currentIndex = i
				currentDepth--
				break
			}
		}
	}

	// Join with null bytes
	return strings.Join(components, "\x00")
}

// padNumbers pads numeric sequences in a string to enable natural sorting
func padNumbers(s string) string {
	var result strings.Builder
	var numBuffer strings.Builder
	inNumber := false

	for _, r := range s {
		if r >= '0' && r <= '9' {
			if !inNumber {
				inNumber = true
			}
			numBuffer.WriteRune(r)
		} else {
			if inNumber {
				// Pad the number to 10 digits
				padded := strings.Repeat("0", 10-numBuffer.Len()) + numBuffer.String()
				result.WriteString(padded)
				numBuffer.Reset()
				inNumber = false
			}
			result.WriteRune(r)
		}
	}

	// Handle trailing number
	if inNumber {
		padded := strings.Repeat("0", 10-numBuffer.Len()) + numBuffer.String()
		result.WriteString(padded)
	}

	return result.String()
}