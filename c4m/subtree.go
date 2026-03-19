package c4m

import (
	"fmt"
	"strings"
)

// ExtractSubtree returns a new Manifest containing only the children of
// the directory entry at the given tree path. The target entry itself is
// excluded. Child depths are adjusted to be relative (depth 0 for direct
// children).
//
// The path is a tree path within the manifest — e.g., "shared/" for a
// root-level directory, or "project/src/shared/" for a nested one. Each
// component includes the trailing "/" for directories.
//
// Returns an error if the entry is not found or is not a directory.
func (m *Manifest) ExtractSubtree(entryPath string) (*Manifest, error) {
	entry := m.findByTreePath(entryPath)
	if entry == nil {
		return nil, fmt.Errorf("entry not found: %s", entryPath)
	}
	if !entry.IsDir() {
		return nil, fmt.Errorf("entry is not a directory: %s", entryPath)
	}

	var entries []*Entry
	emitSubtree(m, entry, 0, &entries)

	result := NewManifest()
	result.Entries = entries
	return result, nil
}

// InjectSubtree returns a new Manifest with the children of the target
// directory replaced by the entries from sub. The target entry itself is
// preserved (including its flow link metadata). Entries outside the
// target subtree are untouched.
//
// The path parameter uses the same tree-path format as ExtractSubtree.
// Returns an error if the entry is not found or is not a directory.
func (m *Manifest) InjectSubtree(entryPath string, sub *Manifest) (*Manifest, error) {
	entry := m.findByTreePath(entryPath)
	if entry == nil {
		return nil, fmt.Errorf("entry not found: %s", entryPath)
	}
	if !entry.IsDir() {
		return nil, fmt.Errorf("entry is not a directory: %s", entryPath)
	}

	// Find the index range of the target entry and its children in the
	// flat entry list. The target is at position targetIdx; its children
	// are the contiguous block of entries with depth > target.Depth that
	// immediately follow it.
	targetIdx := -1
	for i, e := range m.Entries {
		if e == entry {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		return nil, fmt.Errorf("entry not in manifest: %s", entryPath)
	}

	// Find the end of the children block.
	childrenEnd := targetIdx + 1
	for childrenEnd < len(m.Entries) {
		if m.Entries[childrenEnd].Depth <= entry.Depth {
			break
		}
		childrenEnd++
	}

	// Build the new entry list:
	// [entries before target] + [target entry] + [adjusted sub entries] + [entries after children]
	baseDepth := entry.Depth + 1

	var newEntries []*Entry
	// Copy entries before and including the target.
	for i := 0; i <= targetIdx; i++ {
		cp := *m.Entries[i]
		newEntries = append(newEntries, &cp)
	}

	// Insert sub-manifest entries with adjusted depths.
	for _, se := range sub.Entries {
		cp := *se
		cp.Depth = se.Depth + baseDepth
		newEntries = append(newEntries, &cp)
	}

	// Copy entries after the old children block.
	for i := childrenEnd; i < len(m.Entries); i++ {
		cp := *m.Entries[i]
		newEntries = append(newEntries, &cp)
	}

	result := NewManifest()
	result.Entries = newEntries
	return result, nil
}

// EntryTreePath returns the full tree path of an entry within the
// manifest. For root-level entries this is just the entry's Name. For
// nested entries it is the concatenation of ancestor names from root
// to the entry itself (e.g., "project/src/shared/").
func (m *Manifest) EntryTreePath(e *Entry) string {
	if e == nil {
		return ""
	}
	ancestors := m.Ancestors(e)
	if len(ancestors) == 0 {
		return e.Name
	}

	var b strings.Builder
	// Ancestors are returned from immediate parent to root; reverse.
	for i := len(ancestors) - 1; i >= 0; i-- {
		b.WriteString(ancestors[i].Name)
	}
	b.WriteString(e.Name)
	return b.String()
}

// findByTreePath locates an entry by its full tree path. The path is
// the concatenation of ancestor names — e.g., "shared/" for depth 0,
// "project/src/shared/" for nested entries. Uses the full-path index
// for O(1) lookup.
func (m *Manifest) findByTreePath(treePath string) *Entry {
	if treePath == "" {
		return nil
	}
	return m.GetEntry(treePath)
}

// splitTreePath splits a tree path into its components. Each directory
// component retains its trailing "/". A file component (last, no "/")
// is returned as-is.
//
// Examples:
//
//	"shared/"              → ["shared/"]
//	"project/src/shared/"  → ["project/", "src/", "shared/"]
//	"shared/doc.txt"       → ["shared/", "doc.txt"]
func splitTreePath(p string) []string {
	var parts []string
	for p != "" {
		i := strings.IndexByte(p, '/')
		if i < 0 {
			parts = append(parts, p) // file at the end
			break
		}
		parts = append(parts, p[:i+1]) // include the "/"
		p = p[i+1:]
	}
	return parts
}
