package c4m

import "fmt"

// SequenceExpansionMode defines how sequences should be expanded
type SequenceExpansionMode int

const (
	// SequenceEmbedded expands sequences as layers within the same manifest
	SequenceEmbedded SequenceExpansionMode = iota
	// SequenceStandalone expands sequences into a separate manifest file
	SequenceStandalone
)

// SequenceExpander handles expansion of sequence notation into individual entries
type SequenceExpander struct {
	mode SequenceExpansionMode
}

// NewSequenceExpander creates a new sequence expander with the given mode
func NewSequenceExpander(mode SequenceExpansionMode) *SequenceExpander {
	return &SequenceExpander{mode: mode}
}

// ExpandManifest processes a manifest and expands all sequence entries
func (se *SequenceExpander) ExpandManifest(manifest *Manifest) (*Manifest, *Manifest, error) {
	expanded := NewManifest()
	expansions := NewManifest()

	for _, entry := range manifest.Entries {
		// Check if entry name contains sequence notation
		if sequencePattern.MatchString(entry.Name) {
			// Add the sequence notation entry to the main manifest
			expanded.AddEntry(entry)

			// Expand using ID list if available
			expandedEntries, err := ExpandSequenceEntryWithManifest(entry, manifest)
			if err != nil {
				// Not a valid sequence, keep as-is without expansion
				continue
			}

			for _, expandedEntry := range expandedEntries {
				if se.mode == SequenceEmbedded {
					expanded.AddEntry(expandedEntry)
				} else {
					expansions.AddEntry(expandedEntry)
				}
			}
		} else {
			// Regular entry, copy as-is
			expanded.AddEntry(entry)
		}
	}

	// Copy data blocks to expanded manifest (they may be needed for further operations)
	for _, block := range manifest.DataBlocks {
		expanded.AddDataBlock(block)
	}

	if se.mode == SequenceEmbedded {
		return expanded, nil, nil
	} else {
		return expanded, expansions, nil
	}
}

// ExpandSequenceEntry expands a single sequence entry into individual file entries.
// The idList parameter provides individual C4 IDs for each file in order.
// If idList is nil, the entry's C4ID is used for all expanded files (legacy behavior).
func ExpandSequenceEntry(entry *Entry, idList *IDList) ([]*Entry, error) {
	seq, err := ParseSequence(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sequence: %w", err)
	}

	var entries []*Entry
	expandedFiles := seq.Expand()

	// Validate ID list count if provided
	if idList != nil && idList.Count() != len(expandedFiles) {
		return nil, fmt.Errorf("ID list count (%d) doesn't match expanded file count (%d)",
			idList.Count(), len(expandedFiles))
	}

	for i, filename := range expandedFiles {
		// Get individual C4 ID from ID list, or fall back to entry's C4ID
		var fileID = entry.C4ID
		if idList != nil {
			fileID = idList.Get(i)
		}

		// Create a new entry for each expanded file
		// Note: Size is unknown for individual files (use -1 for null)
		expandedEntry := &Entry{
			Name:      filename,
			Size:      -1,              // Individual size unknown from range
			Timestamp: entry.Timestamp, // Shared timestamp
			Mode:      entry.Mode,      // Shared mode
			C4ID:      fileID,
			Depth:     entry.Depth,
		}
		entries = append(entries, expandedEntry)
	}

	return entries, nil
}

// ExpandSequenceEntryWithManifest expands a sequence entry using embedded DataBlocks
func ExpandSequenceEntryWithManifest(entry *Entry, manifest *Manifest) ([]*Entry, error) {
	// Try to get the ID list from embedded data blocks
	var idList *IDList
	if !entry.C4ID.IsNil() {
		list, err := manifest.GetIDList(entry.C4ID)
		if err == nil {
			idList = list
		}
		// If not found in manifest, idList remains nil (legacy behavior)
	}

	return ExpandSequenceEntry(entry, idList)
}