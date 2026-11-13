package c4m

import (
	"bytes"
	"testing"
	"time"
)

// DetectSequences returns a new manifest that shares nothing with its
// input: normalising or editing the fold leaves the caller's manifest, and
// its encoded bytes, exactly as they were. Through 2026-11-13 the entries it
// did not fold were the input's own, so normalising a fold rewrote the
// input's directory sizes and C4 IDs (a directory holding a range took the
// folded listing's size and ID).
func TestDetectSequencesDoesNotShareItsInput(t *testing.T) {
	// sub/ holding three frames, normalised, beside a lone numbered file
	// that groups but does not fold.
	frames := sequenceSizeFrames()
	lone := *frames.Entries[1]
	lone.Name, lone.Depth = "lone.007.exr", 0
	frames.Entries = append([]*Entry{&lone}, frames.Entries...)
	encode := func(m *Manifest) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	before := encode(frames)
	ptrs := make(map[*Entry]bool, len(frames.Entries))
	for _, e := range frames.Entries {
		ptrs[e] = true
	}

	folded := NewSequenceDetector(3).DetectSequences(frames)
	for _, e := range folded.Entries {
		if ptrs[e] {
			t.Errorf("the fold holds the input's own entry %s", e.Name)
		}
	}
	Normalize(folded)
	if after := encode(frames); !bytes.Equal(before, after) {
		t.Errorf("normalising the fold changed the input:\n--- before\n%s--- after\n%s", before, after)
	}
	for _, e := range folded.Entries {
		e.Timestamp = e.Timestamp.Add(time.Hour) // any edit to the fold
	}
	if after := encode(frames); !bytes.Equal(before, after) {
		t.Errorf("editing the fold changed the input:\n--- before\n%s--- after\n%s", before, after)
	}
}
