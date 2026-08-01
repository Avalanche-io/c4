package c4m

import (
	"fmt"
	"io"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// ResolveBase resolves a description's external base reference — the
// first-line bare C4 ID that names the block this one refines — by fetching
// it from src and folding this description on top of it. Bases are followed
// transitively: a fetched base may name its own base, so an accreting chain
// of blocks reconstructs by backfilling all priors from the newest block's
// ID alone.
//
// A manifest with no base is returned unchanged, and src is never consulted.
// A manifest WITH a base and no way to read it is an error: partial knowledge
// is first-class within a description (null fields), but "I could not read
// the base" is ignorance ABOUT the description, and must never be presented
// as a complete manifest. Decode deliberately leaves the reference unresolved
// and defers checkpoint verification here rather than guessing.
func ResolveBase(m *Manifest, src store.Source) (*Manifest, error) {
	if m == nil {
		return NewManifest(), nil
	}
	if m.Base.IsNil() {
		return m, nil
	}
	return foldOntoBase(m, src, map[c4.ID]bool{})
}

// ResolveBaseID resolves one block by ID, following its own base chain — the
// entry point for a consumer holding only the newest block's ID, and for
// resolving a chain whose base reference sits outside the sections it parsed.
func ResolveBaseID(id c4.ID, src store.Source) (*Manifest, error) {
	if src == nil {
		return nil, fmt.Errorf("%w %s: no content source to read it from",
			ErrUnresolvedBase, id)
	}
	return resolveBlock(id, src, map[c4.ID]bool{})
}

// resolveBlock fetches one block and resolves it against its own base.
func resolveBlock(id c4.ID, src store.Source, seen map[c4.ID]bool) (*Manifest, error) {
	if seen[id] {
		return nil, fmt.Errorf("%w: %s revisited", ErrBaseCycle, id)
	}
	seen[id] = true

	rc, err := src.Open(id)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrUnresolvedBase, id, err)
	}
	defer rc.Close()

	m, err := NewDecoder(rc).Decode()
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrUnresolvedBase, id, err)
	}
	if m.Base.IsNil() {
		return m, nil
	}
	return foldOntoBase(m, src, seen)
}

// foldOntoBase resolves m.Base and folds m on top of it.
func foldOntoBase(m *Manifest, src store.Source, seen map[c4.ID]bool) (*Manifest, error) {
	if src == nil {
		return nil, fmt.Errorf("%w %s: no content source to read it from",
			ErrUnresolvedBase, m.Base)
	}
	prior, err := resolveBlock(m.Base, src, seen)
	if err != nil {
		return nil, err
	}

	resolved := ResolvePatchChain([]*PatchSection{
		{Entries: prior.Entries},
		{Entries: m.Entries},
	}, 0)

	// Carry range data forward; the newer block wins on collision.
	for _, layer := range []*Manifest{prior, m} {
		for k, v := range layer.RangeData {
			if resolved.RangeData == nil {
				resolved.RangeData = make(map[c4.ID]string)
			}
			resolved.RangeData[k] = v
		}
	}
	return resolved, nil
}

// DecodeResolved reads a description from r and resolves any external base
// reference against src in one step — the form consumers that already read
// the store should use. With a nil src it is Decode plus a loud failure on
// an unresolved base, never a silently truncated manifest.
func DecodeResolved(r io.Reader, src store.Source) (*Manifest, error) {
	m, err := NewDecoder(r).Decode()
	if err != nil {
		return nil, err
	}
	return ResolveBase(m, src)
}
