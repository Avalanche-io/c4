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

// folderTimes says whether rederiveDirectory may fill a null folder time.
type folderTimes bool

const (
	// fillNullTimes is the store pass's released behaviour (06f5f8f):
	// PropagateMetadata fills a null folder time from the newest child.
	// The maintainer, 2026-11-24: "the mod time should NEVER be based on
	// contents, that has bad sideffects." Changing the store pass is his
	// to order; until then it keeps this.
	fillNullTimes folderTimes = true
	// keepNullTimes leaves every null folder time null: no time is
	// derived from contents.
	keepNullTimes folderTimes = false
)

// rederiveDirectory is this line's one derivation of a directory's size
// and C4 ID from its listed children, which must already be final, as the
// scanner derives them (SPECIFICATION.md "Directory Sizes", "Computing
// Directory C4 IDs"): its C4 ID is that of its one-level listing
// (directoryRecord), and its size the sum of its children's sizes plus
// the listing's byte length, null when a child's size is null. A stated
// timestamp is the directory's own mtime and is kept. PropagateMetadata
// runs over the subtree, as it always has, for the fills it makes; with
// keepNullTimes every folder in the span whose time was null is null again
// before the listing is read, so neither the ID nor the size is computed
// over a time filled from contents. The store pass, the normaliser and
// intersect all call it.
func rederiveDirectory(m *c4m.Manifest, sp dirSpan, times folderTimes) {
	span := m.Entries[sp.start:sp.end]
	var null []*c4m.Entry
	if times == keepNullTimes {
		for _, e := range span {
			if e.IsDir() && e.TimestampIsNull() {
				null = append(null, e)
			}
		}
	}
	dir := span[0]
	dir.Size = -1
	c4m.PropagateMetadata(span)
	for _, e := range null {
		e.Timestamp = c4m.NullTimestamp()
	}
	listing, id := directoryRecord(m, dir)
	dir.C4ID = id
	dir.Size = listedSize(m.Children(dir), listing)
}

// listedSize is a directory's size from its direct children and its
// one-level listing: the children's sizes (a subdirectory's already counts
// its own listings) plus the listing's bytes; null when a child's is null.
func listedSize(children []*c4m.Entry, listing string) int64 {
	total := int64(len(listing))
	for _, c := range children {
		if c.Size < 0 {
			return -1
		}
		total += c.Size
	}
	return total
}
