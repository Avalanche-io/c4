package c4m

import (
	"bytes"
	"errors"
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
//
// The result is a fold, not an expansion. A stored block is a one-level
// record: its subdirectories are single lines carrying their own listing
// IDs. Folding a patch onto it deepens the tree only along the paths the
// patch restates; every other directory stays one collapsed line, correct
// (its ID names its record) but not expanded. A consumer that needs the
// whole tree, to store it, scan against it or materialise it, must inline
// the collapsed records from the store afterwards (cmd/c4's
// expandIfRecord does this and names every record the store lacks).
// Storing the fold as if it were the full tree writes listings whose
// children were never read (a storage-daemon prototype did exactly this, 2026-11-08).
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
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrUnresolvedBase, id, err)
	}
	m, err := resolveBytes(data, src, seen)
	if err != nil && !isUnresolvedOrCycle(err) {
		return nil, fmt.Errorf("%w %s: %v", ErrUnresolvedBase, id, err)
	}
	return m, err
}

// resolveBytes decodes one block and, when it names an external base,
// resolves it section by section against the fetched prior: each section
// is applied in order, and each checkpoint between sections is verified
// against the state accumulated so far (SPECIFICATION.md, "Subsequent Bare
// C4 ID"). The decoder alone cannot do this: with the base unknown it
// defers every checkpoint, and it applies a block's later sections against
// the block's first section only. Removal by exact restatement means
// something only against the right state, so a block is folded here, base
// first, never as one flattened patch.
func resolveBytes(data []byte, src store.Source, seen map[c4.ID]bool) (*Manifest, error) {
	m, err := NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return nil, err
	}
	if m.Base.IsNil() {
		return m, nil
	}
	sections, err := DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return foldSectionsOntoBase(m, sections, src, seen)
}

func isUnresolvedOrCycle(err error) bool {
	return errors.Is(err, ErrUnresolvedBase) || errors.Is(err, ErrBaseCycle) || errors.Is(err, ErrPatchIDMismatch)
}

// foldOntoBase resolves m.Base and folds m on top of it. It is the path for
// a manifest already decoded (ResolveBase); its sections are gone, so its
// internal checkpoints cannot be checked here. Bytes go through
// resolveBytes, which verifies them.
func foldOntoBase(m *Manifest, src store.Source, seen map[c4.ID]bool) (*Manifest, error) {
	return foldSectionsOntoBase(m, []*PatchSection{{BaseID: m.Base, Entries: m.Entries}}, src, seen)
}

// foldSectionsOntoBase fetches m.Base, then applies sections in order,
// verifying every checkpoint (the BaseID of each section after the first)
// against the accumulated state.
func foldSectionsOntoBase(m *Manifest, sections []*PatchSection, src store.Source, seen map[c4.ID]bool) (*Manifest, error) {
	if src == nil {
		return nil, fmt.Errorf("%w %s: no content source to read it from",
			ErrUnresolvedBase, m.Base)
	}
	prior, err := resolveBlock(m.Base, src, seen)
	if err != nil {
		return nil, err
	}

	resolved := prior
	for i, sec := range sections {
		if i > 0 {
			if got := resolved.ComputeC4ID(); got != sec.BaseID {
				return nil, fmt.Errorf("%w: section %d of the block on base %s: accumulated %s, checkpoint %s",
					ErrPatchIDMismatch, i+1, m.Base, got, sec.BaseID)
			}
		}
		resolved = ResolvePatchChain([]*PatchSection{
			{Entries: resolved.Entries},
			{Entries: sec.Entries},
		}, 0)
	}

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
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return resolveBytes(data, src, map[c4.ID]bool{})
}
