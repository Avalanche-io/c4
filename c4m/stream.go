package c4m

import (
	"bytes"
	"io"
)

// WriteChainBody renders a manifest as the streaming chain shape
// (design/snapshot-loop/round-3/draft-v9.md §2, as amended by
// design/no-self-id.md and the 2026-11-08 ruling):
//
//  1. the base section — every entry in canonical pre-order, with
//     directory lines carrying null ID and null size (bottom-up
//     aggregates, genuinely unknown at the moment a streaming walk
//     would emit the line); empty directories emit complete, and
//     unreadable directories (null ID in the source) stay null;
//  2. one bare checkpoint line — the accumulated (base) state's ID;
//  3. one refinement patch restating exactly the directory lines
//     whose ID or size changed from their null base form.
//
// The stream ends on the last refined line. Nothing follows it: a
// c4m file never ends with its own ID, so the resolved state's ID is
// never written after the entries (through 1.0.24 a "closing
// validator" line did; that shape is now ErrTrailingID). The output
// is a valid c4m chain that resolves to exactly m. These BYTES are
// the contract at any concurrency; a buffered implementation (this
// one) and a true streaming emitter must produce them identically.
func WriteChainBody(w io.Writer, m *Manifest) error {
	// The base view: directory aggregates nulled. An empty directory
	// (no children in m) keeps its real ID and size — both are known
	// the moment the directory is opened.
	base := NewManifest()
	base.Base = m.Base
	hasChildren := make(map[int]bool, len(m.Entries))
	for i, e := range m.Entries {
		_ = e
		if i+1 < len(m.Entries) && m.Entries[i+1].Depth > m.Entries[i].Depth {
			hasChildren[i] = true
		}
	}
	var restated []*Entry
	for i, e := range m.Entries {
		cp := *e
		if e.IsDir() && hasChildren[i] && !e.C4ID.IsNil() {
			cp.C4ID = [64]byte{}
			cp.Size = -1
			restated = append(restated, m.Entries[i])
		}
		base.AddEntry(&cp)
	}

	for _, e := range base.Entries {
		if _, err := io.WriteString(w, indentOf(e.Depth)+e.Canonical()+"\n"); err != nil {
			return err
		}
	}

	// Nothing to refine (flat tree, empty dirs only, or all-unreadable
	// dirs): the body is the base alone.
	if len(restated) == 0 {
		return nil
	}

	// Checkpoint: the accumulated (base) state.
	if _, err := io.WriteString(w, base.ComputeC4ID().String()+"\n"); err != nil {
		return err
	}

	// Refinement patch: the restated directory lines, pre-order, at
	// their recorded depths (every ancestor of a restated directory is
	// itself restated, so path context is self-contained).
	for _, e := range restated {
		if _, err := io.WriteString(w, indentOf(e.Depth)+e.Canonical()+"\n"); err != nil {
			return err
		}
	}

	return nil
}

// indentOf renders the canonical two-space indentation for a depth.
func indentOf(depth int) string {
	if depth <= 0 {
		return ""
	}
	const spaces = "                                                                "
	need := depth * 2
	out := ""
	for need > len(spaces) {
		out += spaces
		need -= len(spaces)
	}
	return out + spaces[:need]
}

// ChainStream renders WriteChainBody to a string.
func ChainStream(m *Manifest) (string, error) {
	var buf bytes.Buffer
	if err := WriteChainBody(&buf, m); err != nil {
		return "", err
	}
	return buf.String(), nil
}
