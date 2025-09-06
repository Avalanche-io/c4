package c4m

import (
	"sort"
	"strings"
)

// HierarchicalEntry represents an entry with its full path for sorting
type HierarchicalEntry struct {
	Entry    *Entry
	FullPath string // Full path for hierarchical sorting
	Parent   string // Parent directory path
}

// ReorganizeHierarchically takes a flat list of entries and reorganizes them
// into proper hierarchical depth-first order as required by C4M format
func ReorganizeHierarchically(entries []*Entry) []*Entry {
	if len(entries) == 0 {
		return entries
	}

	// Group entries by depth
	depthGroups := make(map[int][]*Entry)
	maxDepth := 0
	for _, entry := range entries {
		depthGroups[entry.Depth] = append(depthGroups[entry.Depth], entry)
		if entry.Depth > maxDepth {
			maxDepth = entry.Depth
		}
	}
	
	// Sort each depth group (files before directories, then natural sort)
	for depth := 0; depth <= maxDepth; depth++ {
		group := depthGroups[depth]
		sort.Slice(group, func(i, j int) bool {
			a, b := group[i], group[j]
			
			// Files before directories
			aIsDir := a.Mode.IsDir() || strings.HasSuffix(a.Name, "/")
			bIsDir := b.Mode.IsDir() || strings.HasSuffix(b.Name, "/")
			
			if aIsDir != bIsDir {
				return !aIsDir // files first
			}
			
			// Natural sort for names
			return NaturalLess(a.Name, b.Name)
		})
		depthGroups[depth] = group
	}
	
	// Build the result using depth-first traversal
	result := []*Entry{}
	visited := make(map[*Entry]bool)
	
	var addEntry func(entry *Entry)
	addEntry = func(entry *Entry) {
		if visited[entry] {
			return
		}
		visited[entry] = true
		result = append(result, entry)
		
		// If this is a directory, find and add all its immediate children
		if entry.Mode.IsDir() || strings.HasSuffix(entry.Name, "/") {
			childDepth := entry.Depth + 1
			if children, ok := depthGroups[childDepth]; ok {
				// Find children that belong to this directory
				// We need to match based on position in the original list
				// since we don't have explicit parent-child relationships
				
				// Collect this directory's children
				var dirChildren []*Entry
				for _, child := range children {
					// A simple heuristic: if we haven't visited this child yet
					// and it appears after the parent in the original list,
					// it might be a child of this directory
					if !visited[child] {
						// Check if this child could belong to this directory
						// by seeing if it comes after the directory entry
						if isLikelyChild(entry, child, entries) {
							dirChildren = append(dirChildren, child)
						}
					}
				}
				
				// Sort children (files first, then natural sort)
				sort.Slice(dirChildren, func(i, j int) bool {
					a, b := dirChildren[i], dirChildren[j]
					
					// Files before directories
					aIsDir := a.Mode.IsDir() || strings.HasSuffix(a.Name, "/")
					bIsDir := b.Mode.IsDir() || strings.HasSuffix(b.Name, "/")
					
					if aIsDir != bIsDir {
						return !aIsDir // files first
					}
					
					// Natural sort
					return NaturalLess(a.Name, b.Name)
				})
				
				// Recursively add children
				for _, child := range dirChildren {
					addEntry(child)
				}
			}
		}
	}
	
	// Start with depth 0 entries
	if rootEntries, ok := depthGroups[0]; ok {
		for _, entry := range rootEntries {
			addEntry(entry)
		}
	}
	
	return result
}

// isLikelyChild determines if child is likely a child of parent based on position
func isLikelyChild(parent, child *Entry, allEntries []*Entry) bool {
	parentIdx := -1
	childIdx := -1
	
	for i, e := range allEntries {
		if e == parent {
			parentIdx = i
		}
		if e == child {
			childIdx = i
		}
	}
	
	// Child should come after parent
	if childIdx <= parentIdx {
		return false
	}
	
	// Check if there's a closer parent directory at the same depth as parent
	// between parent and child
	for i := parentIdx + 1; i < childIdx; i++ {
		e := allEntries[i]
		if e.Depth == parent.Depth && (e.Mode.IsDir() || strings.HasSuffix(e.Name, "/")) {
			// There's another directory at parent's depth between parent and child
			// So child likely belongs to that directory instead
			return false
		}
	}
	
	return true
}

// buildFullPath reconstructs the full path of an entry based on its depth and name
func buildFullPath(target *Entry, allEntries []*Entry) string {
	if target.Depth == 0 {
		return strings.TrimSuffix(target.Name, "/")
	}
	
	// Build path components from this entry up to root
	components := []string{strings.TrimSuffix(target.Name, "/")}
	currentDepth := target.Depth
	
	// Find parent entries by walking backwards and matching depths
	for i := indexOf(allEntries, target) - 1; i >= 0 && currentDepth > 0; i-- {
		entry := allEntries[i]
		if entry.Depth == currentDepth-1 && (entry.Mode.IsDir() || strings.HasSuffix(entry.Name, "/")) {
			components = append([]string{strings.TrimSuffix(entry.Name, "/")}, components...)
			currentDepth--
			if currentDepth == 0 {
				break
			}
		}
	}
	
	return strings.Join(components, "/")
}

// indexOf finds the index of an entry in the slice
func indexOf(entries []*Entry, target *Entry) int {
	for i, e := range entries {
		if e == target {
			return i
		}
	}
	return -1
}

// OrganizeManifest reorganizes a manifest's entries into proper hierarchical order
func (m *Manifest) OrganizeHierarchically() {
	m.Entries = ReorganizeHierarchically(m.Entries)
}