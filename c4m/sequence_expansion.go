package c4m

import (
	"fmt"
	"strings"
)

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
			// Parse the sequence
			seq, err := ParseSequence(entry.Name)
			if err != nil {
				// Not a valid sequence, keep as-is
				expanded.AddEntry(entry)
				continue
			}

			// Add the sequence notation entry to the main manifest
			expanded.AddEntry(entry)

			// Create expanded entries
			expandedFiles := seq.Expand()
			for _, filename := range expandedFiles {
				// Create a new entry for each expanded file
				expandedEntry := &Entry{
					Name:      filename,
					Size:      entry.Size,       // Inherit size from sequence entry
					Timestamp: entry.Timestamp,  // Inherit timestamp
					Mode:      entry.Mode,       // Inherit mode
					C4ID:      entry.C4ID,       // Note: In real use, each file would have unique ID
					Depth:     entry.Depth,
				}

				if se.mode == SequenceEmbedded {
					// Add to the same manifest with a layer marker
					// Note: Layer directives are handled separately, just add the expanded entry
					expanded.AddEntry(expandedEntry)
				} else {
					// Add to separate expansion manifest
					expansions.AddEntry(expandedEntry)
				}
			}
		} else {
			// Regular entry, copy as-is
			expanded.AddEntry(entry)
		}
	}

	if se.mode == SequenceEmbedded {
		return expanded, nil, nil
	} else {
		return expanded, expansions, nil
	}
}

// ProcessManifestWithSequences reads a manifest and expands sequences according to mode
func ProcessManifestWithSequences(manifest *Manifest, mode SequenceExpansionMode) (*Manifest, error) {
	expander := NewSequenceExpander(mode)
	result, expansions, err := expander.ExpandManifest(manifest)
	if err != nil {
		return nil, err
	}

	if mode == SequenceEmbedded {
		return result, nil
	}

	// For standalone mode, merge the expansions back if needed
	// This could be written to a separate file instead
	if expansions != nil && len(expansions.Entries) > 0 {
		// Add a comment about the expansion being in a separate file
		// The actual writing of the separate file would be handled by the caller
	}

	return result, nil
}

// sanitizeLayerName creates a valid layer name from a sequence pattern
func sanitizeLayerName(pattern string) string {
	// Remove special characters and spaces
	name := strings.ReplaceAll(pattern, "[", "_")
	name = strings.ReplaceAll(name, "]", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")

	// Remove consecutive underscores
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}

	// Trim underscores from ends
	name = strings.Trim(name, "_")

	return name
}

// ExpandSequenceEntry expands a single sequence entry into individual file entries
func ExpandSequenceEntry(entry *Entry) ([]*Entry, error) {
	seq, err := ParseSequence(entry.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sequence: %w", err)
	}

	var entries []*Entry
	expandedFiles := seq.Expand()

	for _, filename := range expandedFiles {
		// Create a new entry for each expanded file
		expandedEntry := &Entry{
			Name:      filename,
			Size:      entry.Size,      // Inherit properties from sequence entry
			Timestamp: entry.Timestamp,
			Mode:      entry.Mode,
			C4ID:      entry.C4ID, // Note: In real use, each would have unique ID
			Depth:     entry.Depth,
		}
		entries = append(entries, expandedEntry)
	}

	return entries, nil
}