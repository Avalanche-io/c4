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
	// RangeData holds the inline ID lists written in this section, after
	// its entries (SPECIFICATION.md, "Inline Range Data"), keyed by each
	// list's C4 ID. Nil when the section has none.
	RangeData map[c4.ID]string
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

		// Inline ID list lines (range data) don't affect the chain's
		// structure; they belong to the section they sit in, after its
		// entries. Through 2026-11-13 this reader skipped them, so patch,
		// split and log lost every list.
		if isInlineIDList(trimmed) {
			if current.RangeData == nil {
				current.RangeData = make(map[c4.ID]string)
			}
			current.RangeData[c4.Identify(strings.NewReader(trimmed))] = trimmed
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
// ResolvePatchChain panics with an error wrapping ErrInvalidEntry, naming
// the entry, if a section it applies holds an entry whose depth skips past
// its parent directory (a corrupt, truncated, or hand-edited c4m).
func ResolvePatchChain(sections []*PatchSection, stopAt int) *Manifest {
	m, err := resolvePatchChainChecked(sections, stopAt)
	if err != nil {
		panic(err)
	}
	return m
}

// resolvePatchChainChecked is ResolvePatchChain, returning an error
// instead of panicking when a section applied as part of the chain holds
// an entry whose depth skips past its parent directory. The error wraps
// ErrInvalidEntry. On valid input the result is identical to
// ResolvePatchChain.
func resolvePatchChainChecked(sections []*PatchSection, stopAt int) (*Manifest, error) {
	if len(sections) == 0 {
		return NewManifest(), nil
	}

	limit := len(sections)
	if stopAt > 0 && stopAt < limit {
		limit = stopAt
	}

	// First section is the base.
	m := &Manifest{Version: "1.0", Entries: sections[0].Entries}
	if limit == 1 {
		keepRangeData(m, sections[0].RangeData)
		return m, nil
	}

	// Apply subsequent patches.
	for i := 1; i < limit; i++ {
		patch := &Manifest{Version: "1.0", Entries: sections[i].Entries}
		var err error
		m, err = applyPatchChecked(m, patch)
		if err != nil {
			return nil, fmt.Errorf("patch section %d: %w", i+1, err)
		}
	}

	// The lists of the ranges the resolved state holds, from whichever
	// section carried them.
	lists := make([]map[c4.ID]string, 0, limit)
	for _, sec := range sections[:limit] {
		lists = append(lists, sec.RangeData)
	}
	keepRangeData(m, lists...)
	return m, nil
}
