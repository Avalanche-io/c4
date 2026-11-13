package c4m

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/Avalanche-io/c4"
)

// PatchSection represents one section of a patch chain: either the base
// manifest or a subsequent patch delta. BaseID is the C4 ID that precedes
// this section (empty for the first/base section).
type PatchSection struct {
	BaseID  c4.ID    // C4 ID line preceding this section (nil for first)
	Entries []*Entry // Entries in this section
}

// DecodePatchChain reads a c4m file and returns each section separately
// without resolving patches. The first section is the base manifest;
// subsequent sections are patch deltas separated by bare C4 ID lines.
// A bare C4 ID at end of input is ErrTrailingID.
func DecodePatchChain(r io.Reader) ([]*PatchSection, error) {
	sections, _, err := decodePatchChain(r, false)
	return sections, err
}

// DecodePatchChainMigrating is DecodePatchChain for objects written
// before 2026-11-08 that cannot be rewritten: changesets and chains that
// are immutable and named by ID (a signed record, a stored object). A
// bare C4 ID at end of input is taken off and returned as trailing
// instead of refused; it is the ID of the state the sections resolve to,
// and the caller MUST verify it against the resolved state, as the
// resolving Decoder would (this reader does not resolve). trailing is
// nil when the stream ends on entries. Nothing may write the trailing ID
// back out: the migrated form ends on its last section.
func DecodePatchChainMigrating(r io.Reader) (sections []*PatchSection, trailing c4.ID, err error) {
	return decodePatchChain(r, true)
}

func decodePatchChain(r io.Reader, allowTrailing bool) ([]*PatchSection, c4.ID, error) {
	d := &Decoder{
		reader:      bufio.NewReader(r),
		indentWidth: -1,
	}

	var sections []*PatchSection
	current := &PatchSection{}
	firstLine := true
	lastWasID := false // the previous non-blank line was a bare C4 ID
	idLine := 0        // the line that bare ID is on

	for {
		line, err := d.readLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, c4.ID{}, err
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip inline ID list lines (range data). These don't affect
		// patch chain structure — they're supplementary content.
		if isInlineIDList(trimmed) {
			continue
		}

		if isBareC4ID(trimmed) {
			id, parseErr := c4.Parse(trimmed)
			if parseErr != nil {
				return nil, c4.ID{}, fmt.Errorf("line %d: invalid C4 ID: %w", d.lineNum, parseErr)
			}

			if firstLine && len(current.Entries) == 0 {
				// First bare ID: external base reference on first section.
				current.BaseID = id
			} else if len(current.Entries) == 0 && len(sections) > 0 {
				// Consecutive bare IDs with no entries between them are
				// checkpoint repetition: both name the same accumulated
				// state, so they must be the same ID. This reader cannot
				// verify either against the state (it does not resolve),
				// but it can refuse a pair that disagrees instead of
				// silently keeping the second (found by the storage project, 2026-11-09).
				if current.BaseID != id {
					return nil, c4.ID{}, fmt.Errorf("%w (line %d): consecutive checkpoints disagree: %s then %s",
						ErrPatchIDMismatch, d.lineNum, current.BaseID, id)
				}
				lastWasID = true
				idLine = d.lineNum
			} else {
				// Flush current section. A first section that carries
				// the external base is kept even with no entries: a
				// checkpoint directly after the first-line base names
				// the base's resolved state, and dropping the section
				// would lose the base, so patch and log would fetch the
				// checkpoint's ID as if it were the base (found
				// 2026-11-13 porting the chain rules to libc4).
				if len(current.Entries) > 0 || (len(sections) == 0 && !current.BaseID.IsNil()) {
					sections = append(sections, current)
				}
				current = &PatchSection{BaseID: id}
				lastWasID = true
				idLine = d.lineNum
			}
			firstLine = false
			continue
		}
		lastWasID = false

		if strings.HasPrefix(trimmed, "@") {
			return nil, c4.ID{}, fmt.Errorf("directives not supported (line %d): %s", d.lineNum, line)
		}

		entry, parseErr := d.parseEntryFromLine(line)
		if parseErr != nil {
			return nil, c4.ID{}, fmt.Errorf("parse error: %v", parseErr)
		}
		if entry != nil {
			current.Entries = append(current.Entries, entry)
		}
		firstLine = false
	}

	// A bare C4 ID at end of input is a format error (2026-11-08): a c4m
	// file never ends with its own ID. Through 1.0.26 this reader dropped
	// it silently as a "closing validator"; now every shape-level verb
	// (patch, log, split, explain) refuses the file and names the fix,
	// the same as the resolving Decoder. The normaliser migrates it.
	var trailing c4.ID
	if lastWasID {
		if !allowTrailing {
			return nil, c4.ID{}, fmt.Errorf("%w (line %d): delete that line, or rewrite the file with `c4 id FILE.c4m > fixed.c4m`",
				ErrTrailingID, idLine)
		}
		// Migrating: the trailing ID names the state the sections resolve
		// to. It is handed back for the caller to verify, never kept as a
		// section.
		trailing = current.BaseID
		current = &PatchSection{}
	}

	// Flush the final section.
	if len(current.Entries) > 0 {
		sections = append(sections, current)
	}

	return sections, trailing, nil
}

// ResolvePatchChain resolves a series of patch sections into a final manifest.
// If stopAt > 0, resolution stops after that many sections (1-based).
//
// The base tree is built once and every section is applied into it, then the
// result is flattened and sorted once. Folding through ApplyPatch per section
// instead would rebuild a node and a map for every entry in the whole base on
// each fold — making resolution O(K*N) in sections times entries, so that a
// one-entry patch costs what a full restatement costs. That shape is what made
// a long chain unaffordable at scale; see design/chain-resolution.md.
//
// The intermediate flatten/sort ApplyPatch performs between sections is not
// observable: applyPatchTree and flattenPatchTree never read Entry.Depth, and
// Entry.IsDir depends only on Mode and Name, so the accumulated tree carries
// the same (path -> entry) mapping either way.
//
// ResolvePatchChain panics with an error wrapping ErrInvalidEntry, naming
// the entry, if a section holds an entry whose depth skips past its parent
// directory.
func ResolvePatchChain(sections []*PatchSection, stopAt int) *Manifest {
	if len(sections) == 0 {
		return NewManifest()
	}

	limit := len(sections)
	if stopAt > 0 && stopAt < limit {
		limit = stopAt
	}

	// First section is the base. With nothing to apply, it is the answer
	// as-is — matching the historical contract that a single-section chain
	// resolves to its own entries, unsorted and uncopied.
	base := &Manifest{Version: "1.0", Entries: sections[0].Entries}
	if limit == 1 {
		return base
	}

	tree := buildPatchTree(base)
	for i := 1; i < limit; i++ {
		patch := &Manifest{Version: "1.0", Entries: sections[i].Entries}
		applyPatchTree(tree, buildPatchTree(patch))
	}

	var entries []*Entry
	flattenPatchTree(tree, 0, &entries)

	result := NewManifest()
	result.Entries = entries
	result.SortEntries()
	return result
}
