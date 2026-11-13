package c4m

import (
	"bytes"
	"fmt"
	"io"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// FoldSection applies one patch section to state, the one fold every
// chain resolver uses: each collapsed directory of state that the section
// restates with children has its listing fetched from src, verified and
// inlined first (Deepen), then the section is applied. A listing
// src cannot supply, or a nil src when one is needed, is ErrUnresolvedBase
// naming it; nothing is guessed. state is not modified.
func FoldSection(state *Manifest, entries []*Entry, src store.Source) (*Manifest, error) {
	deep, err := Deepen(state, entries, src)
	if err != nil {
		return nil, err
	}
	// state's lists travel with the fold: the result keeps those of the
	// ranges it still holds. A caller folding a section that carries lists
	// of its own adds them to state.RangeData first (lists are keyed by
	// their own C4 ID, so the union is safe).
	return ResolvePatchChain([]*PatchSection{{Entries: deep.Entries, RangeData: state.RangeData}, {Entries: entries}}, 0), nil
}

// Deepen returns state with every directory that entries (a patch
// section) restate with children under it made known, so the section can
// be applied or compared against it. state is not modified. A fetched base is a
// one-level record: a subdirectory is one line carrying its listing's ID,
// with no children inline. Applying a patch that deepens such a line
// against it would treat the directory as empty, so a restated child that
// removes a file would read as an addition and the fold would describe a
// tree its own directory IDs contradict (found 2026-11-12 by an
// independent review of shape-B output chains). Each such directory's
// listing is fetched from src, verified against its ID and inlined, one
// level at a time from the root down, before the patch applies. A listing
// src cannot supply is an unresolved base: the fold refuses rather than
// guess. A directory whose ID is null has no listing to fetch; it stays as
// the base states it (partial knowledge), as before.
func Deepen(state *Manifest, sec []*Entry, src store.Source) (*Manifest, error) {
	for _, path := range restatedDirs(sec) {
		idx := indexOfPath(state.Entries, path)
		if idx < 0 {
			continue // new in this section: nothing in the base to deepen
		}
		dir := state.Entries[idx]
		if !dir.IsDir() || dir.C4ID.IsNil() || dir.C4ID == emptyListingID {
			continue
		}
		if idx+1 < len(state.Entries) && state.Entries[idx+1].Depth > dir.Depth {
			continue // children already inline
		}
		children, err := fetchListing(dir.C4ID, path, src)
		if err != nil {
			return nil, err
		}
		inlined := make([]*Entry, 0, len(state.Entries)+len(children))
		inlined = append(inlined, state.Entries[:idx+1]...)
		for _, c := range children {
			cc := *c
			cc.Depth += dir.Depth + 1
			inlined = append(inlined, &cc)
		}
		inlined = append(inlined, state.Entries[idx+1:]...)
		next := *state
		next.Entries = inlined
		state = &next
	}
	return state, nil
}

// fetchListing reads one directory listing by ID and verifies the bytes
// hash to it. A listing names no base of its own.
func fetchListing(id c4.ID, path string, src store.Source) ([]*Entry, error) {
	if src == nil {
		return nil, fmt.Errorf("%w: listing %s of %s, which a patch deepens: no content source to read it from", ErrUnresolvedBase, id, path)
	}
	rc, err := src.Open(id)
	if err != nil {
		return nil, fmt.Errorf("%w: listing %s of %s, which a patch deepens: %v", ErrUnresolvedBase, id, path, err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("%w: listing %s of %s, which a patch deepens: %v", ErrUnresolvedBase, id, path, err)
	}
	if got := c4.Identify(bytes.NewReader(data)); got != id {
		return nil, fmt.Errorf("%w: listing of %s: source returned bytes with ID %s for %s", ErrPatchIDMismatch, path, got, id)
	}
	m, err := NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, fmt.Errorf("%w: listing %s of %s does not parse: %v", ErrUnresolvedBase, id, path, err)
	}
	if !m.Base.IsNil() {
		return nil, fmt.Errorf("%w: listing %s of %s names a base; a directory listing never does", ErrUnresolvedBase, id, path)
	}
	return m.Entries, nil
}

// restatedDirs lists, parents first, the path of every directory in a patch
// section that has children under it.
func restatedDirs(entries []*Entry) []string {
	var out []string
	var stack []string
	for i, e := range entries {
		stack = append(stack[:clampDepth(e.Depth, len(stack))], e.Name)
		if !e.IsDir() || i+1 >= len(entries) || entries[i+1].Depth <= e.Depth {
			continue
		}
		out = append(out, joinPath(stack))
	}
	return out
}

// indexOfPath finds the entry at path ("a/b/" for a nested directory) in a
// flat, depth-indented entry list, or -1.
func indexOfPath(entries []*Entry, path string) int {
	var stack []string
	for i, e := range entries {
		stack = append(stack[:clampDepth(e.Depth, len(stack))], e.Name)
		if joinPath(stack) == path {
			return i
		}
	}
	return -1
}

func joinPath(stack []string) string {
	var b bytes.Buffer
	for _, s := range stack {
		b.WriteString(s)
	}
	return b.String()
}

// clampDepth bounds a depth to the current path stack (Go 1.16 has no min).
func clampDepth(depth, n int) int {
	if depth < n {
		return depth
	}
	return n
}
