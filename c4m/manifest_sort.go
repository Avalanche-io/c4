package c4m

import "sort"

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