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
func DecodePatchChain(r io.Reader) ([]*PatchSection, error) {
	d := &Decoder{
		reader:      bufio.NewReader(r),
		indentWidth: -1,
	}

	var sections []*PatchSection
	current := &PatchSection{}
	firstLine := true

	for {
		line, err := d.readLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
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
				return nil, fmt.Errorf("line %d: invalid C4 ID: %w", d.lineNum, parseErr)
			}

			if firstLine && len(current.Entries) == 0 {
				// First bare ID: external base reference on first section.
				current.BaseID = id
			} else if len(current.Entries) == 0 && len(sections) > 0 {
				// Consecutive bare IDs with no entries between them (e.g.,
				// closing ID of one diff + opening ID of the next). The
				// second ID supersedes — just update the current base.
				current.BaseID = id
			} else {
				// Flush current section.
				if len(current.Entries) > 0 {
					sections = append(sections, current)
				}
				current = &PatchSection{BaseID: id}
			}
			firstLine = false
			continue
		}

		if strings.HasPrefix(trimmed, "@") {
			return nil, fmt.Errorf("directives not supported (line %d): %s", d.lineNum, line)
		}

		entry, parseErr := d.parseEntryFromLine(line)
		if parseErr != nil {
			return nil, fmt.Errorf("parse error: %v", parseErr)
		}
		if entry != nil {
			current.Entries = append(current.Entries, entry)
		}
		firstLine = false
	}

	// Flush final section — only if it has entries. A trailing bare C4 ID
	// (closing page boundary) is a validator, not a new section.
	if len(current.Entries) > 0 {
		sections = append(sections, current)
	}

	return sections, nil
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
