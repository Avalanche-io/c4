package c4m

import (
	"bytes"
	"sort"
	"strings"

	"github.com/Avalanche-io/c4"
)

// ListingRecord returns a directory's canonical one-level record from its
// direct children, and the record's C4 ID, which is the directory's own
// ID: files first, then directories, each group in natural order, one
// canonical line per child. Children must be final (sizes, timestamps
// and IDs resolved). No children gives an empty record and the
// empty-content ID. This is the one derivation of a directory's identity:
// the scanner, the store writer and the normaliser all call it.
func ListingRecord(children []*Entry) (string, c4.ID) {
	if len(children) == 0 {
		return "", c4.Identify(bytes.NewReader(nil))
	}
	sorted := make([]*Entry, len(children))
	copy(sorted, children)
	sort.Slice(sorted, func(i, j int) bool {
		iDir := strings.HasSuffix(sorted[i].Name, "/")
		jDir := strings.HasSuffix(sorted[j].Name, "/")
		if iDir != jDir {
			return !iDir // files first
		}
		return NaturalLess(sorted[i].Name, sorted[j].Name)
	})
	var buf strings.Builder
	for _, e := range sorted {
		buf.WriteString(e.Canonical())
		buf.WriteByte('\n')
	}
	text := buf.String()
	return text, c4.Identify(strings.NewReader(text))
}

// DirectoryRecord is ListingRecord over dir's listed children in m.
func DirectoryRecord(m *Manifest, dir *Entry) (string, c4.ID) {
	return ListingRecord(m.Children(dir))
}

// A Change is a stated, non-null directory value the normaliser replaced
// with the value its listed children determine. Old and New are in
// canonical text form ("-" for null).
type Change struct {
	Path  string
	Field string // "size", "timestamp" or "id"
	Old   string
	New   string
}

// Normalize re-derives the size and C4 ID of every directory whose
// children are listed, by the same rules the scanner applies
// (PropagateMetadata for size, ListingRecord for the ID), filling nulls
// and replacing stale values alike: the listed children determine those
// two. A directory's timestamp is its own modification time as observed
// on disk (SPECIFICATION.md "Directory Timestamps"; Joshua ruled reading
// A of design/directory-timestamp.md on 2026-11-03): the children do not
// determine it, so a stated timestamp is kept and only a null one is
// filled from the newest descendant, exactly as PropagateMetadata has
// always done. A directory listed without children keeps its stated
// values. It returns one Change per stated non-null value that the
// derivation replaced, in entry order; filled nulls are not changes.
// Projection only ever removes knowledge, and a value the listed children
// cannot support (a null child) derives to null: dropped, and reported.
//
// For writers that update a tree in place: Normalize re-derives every
// directory with listed children, not only the ones a patch touched, so
// a stored tree whose folder listings were produced under an earlier
// derivation rule (before 2026-03-26 a folder's size excluded its
// subdirectories' records) will have those listings re-derived to new
// IDs. A writer that stores only the listings above the files it edited
// then holds a head naming listings that are not in the store; store
// every listing the patch restates, or every listing the derivation
// changes (the returned Changes name them). Found in the storage
// project's commit prototype, 2026-11-09.
func Normalize(m *Manifest) []Change {
	type stated struct {
		size int64
		id   c4.ID
	}
	before := make(map[*Entry]stated)
	for _, e := range m.Entries {
		if !e.IsDir() || len(m.Children(e)) == 0 {
			continue
		}
		before[e] = stated{e.Size, e.C4ID}
		e.Size = -1
		e.C4ID = c4.ID{}
	}
	if len(before) == 0 {
		return nil
	}
	propagateMetadata(m.Entries, true)

	paths := make(map[*Entry]string, len(m.Entries))
	for p, e := range EntryPaths(m.Entries) {
		paths[e] = p
	}
	var changes []Change
	for _, e := range m.Entries {
		old, ok := before[e]
		if !ok {
			continue
		}
		path := paths[e]
		if old.size >= 0 && old.size != e.Size {
			changes = append(changes, Change{path, "size", sizeText(old.size), sizeText(e.Size)})
		}
		if !old.id.IsNil() && old.id != e.C4ID {
			changes = append(changes, Change{path, "id", idText(old.id), idText(e.C4ID)})
		}
	}
	return changes
}

func sizeText(n int64) string {
	if n < 0 {
		return "-"
	}
	return formatInt(n)
}

func idText(id c4.ID) string {
	if id.IsNil() {
		return "-"
	}
	return id.String()
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
