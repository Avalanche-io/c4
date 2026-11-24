package main

import (
	"sort"

	"github.com/Avalanche-io/c4/c4m"
)

// dirSpan is a directory's subtree in a manifest's entries: the directory
// at start, its descendants up to end.
type dirSpan struct{ start, end int }

// directorySpans returns the span of every directory in m that want
// selects, deepest first, so each is derived from children already final.
func directorySpans(m *c4m.Manifest, want func(i int, sp dirSpan) bool) []dirSpan {
	var spans []dirSpan
	for i, e := range m.Entries {
		if !e.IsDir() {
			continue
		}
		end := i + 1
		for end < len(m.Entries) && m.Entries[end].Depth > e.Depth {
			end++
		}
		if sp := (dirSpan{i, end}); want(i, sp) {
			spans = append(spans, sp)
		}
	}
	sort.SliceStable(spans, func(a, b int) bool {
		return m.Entries[spans[a].start].Depth > m.Entries[spans[b].start].Depth
	})
	return spans
}

// rederiveDirectory is this line's one derivation of a directory from its
// listed children, which must already be final, as the scanner derives
// it: its size by PropagateMetadata over its own subtree (the children's
// sizes plus the byte length of its one-level listing; null when a
// child's size is null), its C4 ID from that listing (directoryRecord).
// A stated timestamp is the directory's own mtime and is kept; a null one
// is filled from the newest child's, and stays null when a child's is
// null (SPECIFICATION.md "Directory Sizes", "Directory Timestamps",
// "Computing Directory C4 IDs"). The store pass, the normaliser and
// intersect all call it.
func rederiveDirectory(m *c4m.Manifest, sp dirSpan) {
	dir := m.Entries[sp.start]
	dir.Size = -1
	c4m.PropagateMetadata(m.Entries[sp.start:sp.end])
	_, dir.C4ID = directoryRecord(m, dir)
}
