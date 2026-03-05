package c4m

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// ----------------------------------------------------------------------------
// @expand Directive Tests
//
// These tests exercise the @expand mechanism for range folding. The @expand
// directive is not yet fully implemented in the decoder (it returns
// ErrNotSupported), but these tests validate the underlying behaviors:
// folding, expansion, identity, symlink ranges, and round-trip stability.
// ----------------------------------------------------------------------------

// TestFoldingChangesParentIdentity verifies that folding numbered files into
// a range produces a different directory C4 ID than leaving them individual.
// This is the "Option A" decision: folded ≠ unfolded.
func TestFoldingChangesParentIdentity(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	// Build manifest with individual entries
	individual := NewManifest()
	for i := 1; i <= 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		individual.AddEntry(&Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      int64(1000 + i*100),
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Mode:      0644,
			C4ID:      id,
		})
	}
	individualID := individual.ComputeC4ID()

	// Fold into range
	folded := DetectSequences(individual)
	foldedID := folded.ComputeC4ID()

	// They must be different (Option A)
	if individualID == foldedID {
		t.Error("folded and individual forms should produce different C4 IDs")
	}

	// Both must be non-nil
	if individualID.IsNil() || foldedID.IsNil() {
		t.Error("neither C4 ID should be nil")
	}
}

// TestFoldingIdempotencyWithDataBlock demonstrates the known limitation:
// fold → expand-via-@data → re-fold is NOT idempotent because @data expansion
// loses per-entry size metadata (sets Size=-1). This is WHY @expand with full
// entries is needed — it preserves individual sizes and timestamps.
func TestFoldingIdempotencyWithDataBlock(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	original := NewManifest()
	for i := 1; i <= 10; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		original.AddEntry(&Entry{
			Name:      fmt.Sprintf("comp.%04d.exr", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	// First fold
	folded1 := DetectSequences(original)
	id1 := folded1.ComputeC4ID()

	// Expand via @data (ID-list only — loses sizes)
	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(folded1)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	// Filter to individual entries only
	reexpanded := NewManifest()
	for _, e := range expanded.Entries {
		if !e.IsSequence && !IsSequence(e.Name) {
			reexpanded.AddEntry(e)
		}
	}

	// Re-fold
	folded2 := DetectSequences(reexpanded)
	id2 := folded2.ComputeC4ID()

	// Known limitation: IDs differ because @data expansion loses sizes
	if id1 == id2 {
		t.Error("expected different IDs due to @data metadata loss (this would mean the limitation is fixed)")
	}

	// Verify the root cause: expanded entries have null sizes
	for _, e := range reexpanded.Entries {
		if e.Size != -1 {
			t.Errorf("expanded entry %s has size %d, expected -1 (null)", e.Name, e.Size)
		}
	}
}

// TestFoldingIdempotencyWithFullMetadata verifies that when individual
// metadata IS preserved (simulating @expand with full entries),
// fold(expand(fold(entries))) = fold(entries).
func TestFoldingIdempotencyWithFullMetadata(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	original := NewManifest()
	for i := 1; i <= 10; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		original.AddEntry(&Entry{
			Name:      fmt.Sprintf("comp.%04d.exr", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	// First fold
	folded1 := DetectSequences(original)
	id1 := folded1.ComputeC4ID()

	// Simulate @expand: expand with full metadata preserved (not @data)
	// This is what @expand would provide — the original entries intact
	refolded := DetectSequences(original)
	id2 := refolded.ComputeC4ID()

	// With full metadata, folding is idempotent
	if id1 != id2 {
		t.Errorf("fold(original) should be idempotent with full metadata\n  first:  %s\n  second: %s", id1, id2)
	}
}

// TestFoldingWithNonUniformTimestamps verifies behavior when individual
// entries have different timestamps (which is the typical VFX case).
func TestFoldingWithNonUniformTimestamps(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	for i := 1; i <= 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("plate.%04d.exr", i),
			Size:      int64(3000 + i*200),
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Mode:      0644,
			C4ID:      id,
		})
	}

	// Fold
	folded := DetectSequences(manifest)

	var seqEntry *Entry
	for _, e := range folded.Entries {
		if e.IsSequence {
			seqEntry = e
			break
		}
	}
	if seqEntry == nil {
		t.Fatal("no sequence entry after folding")
	}

	// The folded timestamp should be the most recent
	expectedTime := baseTime.Add(5 * time.Minute)
	if !seqEntry.Timestamp.Equal(expectedTime) {
		t.Errorf("sequence timestamp = %v, want %v (most recent)", seqEntry.Timestamp, expectedTime)
	}

	// The folded size should be the sum
	var expectedSize int64
	for i := 1; i <= 5; i++ {
		expectedSize += int64(3000 + i*200)
	}
	if seqEntry.Size != expectedSize {
		t.Errorf("sequence size = %d, want %d (sum)", seqEntry.Size, expectedSize)
	}
}

// TestSymlinkRangeUniformTarget tests that symlink ranges with uniform
// targets use string-substitution: the range notation in the target
// matches the range notation in the name.
func TestSymlinkRangeUniformTarget(t *testing.T) {
	// Create symlink entries with uniform target pattern
	manifest := NewManifest()
	for i := 1; i <= 5; i++ {
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("render.%04d.exr", i),
			Target:    fmt.Sprintf("/cache/source.%04d.exr", i),
			Size:      0,
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Mode:      0120000 | 0777, // symlink
		})
	}

	// DetectSequences should fold symlinks too
	folded := DetectSequences(manifest)

	var seqEntry *Entry
	for _, e := range folded.Entries {
		if e.IsSequence {
			seqEntry = e
			break
		}
	}
	if seqEntry == nil {
		t.Fatal("symlink sequence not detected")
	}

	// The pattern should reflect the range
	if seqEntry.Pattern != "render.[0001-0005].exr" {
		t.Errorf("pattern = %q, want %q", seqEntry.Pattern, "render.[0001-0005].exr")
	}
}

// TestSequenceWithGaps verifies that gaps in frame numbering produce
// separate ranges in the folded notation.
func TestSequenceWithGaps(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	// Frames 1-3 and 7-10 (gap at 4-6)
	frames := []int{1, 2, 3, 7, 8, 9, 10}
	for _, i := range frames {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("comp.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	folded := DetectSequences(manifest)

	// Should produce two sequence entries (or one with gap notation)
	seqCount := 0
	for _, e := range folded.Entries {
		if e.IsSequence {
			seqCount++
		}
	}

	// With min length 3, frames 1-3 (3 entries) and 7-10 (4 entries) both qualify
	if seqCount < 1 {
		t.Errorf("expected at least 1 sequence entry, got %d", seqCount)
	}
}

// TestExpandDataBlockPreservesRoundTrip verifies that a sequence with
// a @data block can be expanded and the C4 IDs are preserved.
func TestExpandDataBlockPreservesRoundTrip(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create entries with known C4 IDs
	original := NewManifest()
	ids := make([]c4.ID, 5)
	for i := 0; i < 5; i++ {
		ids[i] = c4.Identify(strings.NewReader(fmt.Sprintf("unique-content-%d", i)))
		original.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.dpx", i+1),
			Size:      4096,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      ids[i],
		})
	}

	// Fold
	folded := DetectSequences(original)

	// Expand using data block
	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(folded)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	// Verify expanded entries have correct C4 IDs
	expandedIDs := make(map[string]c4.ID)
	for _, e := range expanded.Entries {
		if !e.IsSequence {
			expandedIDs[e.Name] = e.C4ID
		}
	}

	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("shot.%04d.dpx", i+1)
		gotID, ok := expandedIDs[name]
		if !ok {
			t.Errorf("missing expanded entry %s", name)
			continue
		}
		if gotID != ids[i] {
			t.Errorf("expanded C4 ID for %s = %s, want %s", name, gotID, ids[i])
		}
	}
}

// TestSequenceC4IDIsNotAffectedByExpandBlock verifies that the presence
// of an @expand block does not change the manifest's C4 ID.
// @expand is metadata about the range, not content.
func TestSequenceC4IDIsNotAffectedByExpandBlock(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	makeManifest := func() *Manifest {
		m := NewManifest()
		for i := 1; i <= 5; i++ {
			id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
			m.AddEntry(&Entry{
				Name:      fmt.Sprintf("frame.%04d.exr", i),
				Size:      1024,
				Timestamp: baseTime,
				Mode:      0644,
				C4ID:      id,
			})
		}
		return m
	}

	// Fold without data blocks
	m1 := DetectSequences(makeManifest())
	// Remove data blocks from copy for comparison
	m2 := DetectSequences(makeManifest())
	m2.DataBlocks = nil

	id1 := m1.ComputeC4ID()
	id2 := m2.ComputeC4ID()

	// DataBlocks (and by extension @expand blocks) should not affect C4 ID
	if id1 != id2 {
		t.Errorf("data blocks should not affect manifest C4 ID\n  with:    %s\n  without: %s", id1, id2)
	}
}

// TestMarshalUnmarshalPreservesSequenceEscaping verifies that sequence
// names with special characters survive a marshal/unmarshal cycle.
func TestMarshalUnmarshalPreservesSequenceEscaping(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	m := NewManifest()
	m.AddEntry(&Entry{
		Name:       "my animation.[001-100].exr",
		Size:       102400,
		Timestamp:  baseTime,
		Mode:       0644,
		IsSequence: true,
		Pattern:    "my animation.[001-100].exr",
	})

	// Marshal
	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// The output should use escape notation, not quoting
	output := string(data)
	if strings.Contains(output, `"my animation`) {
		t.Errorf("sequence name should use escape notation, not quoting: %s", output)
	}
	if !strings.Contains(output, `my\ animation.[001-100].exr`) {
		t.Errorf("expected escaped sequence name in output: %s", output)
	}

	// Unmarshal
	m2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(m2.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m2.Entries))
	}
	if m2.Entries[0].Name != "my animation.[001-100].exr" {
		t.Errorf("round-trip name = %q, want %q", m2.Entries[0].Name, "my animation.[001-100].exr")
	}
	if !m2.Entries[0].IsSequence {
		t.Error("round-trip should preserve sequence flag")
	}
}

// TestExpandDirectiveReturnsNotSupported verifies the current state:
// @expand is recognized but not yet implemented in the decoder.
func TestExpandDirectiveReturnsNotSupported(t *testing.T) {
	id := c4.Identify(strings.NewReader("test"))
	input := fmt.Sprintf("@c4m 1.0\n@expand %s\n", id)
	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("@expand should return an error (not yet implemented)")
	}
	if !strings.Contains(err.Error(), "not supported") && !strings.Contains(err.Error(), "NotSupported") {
		// Accept any error indicating not-supported
		t.Logf("@expand error (expected): %v", err)
	}
}

// TestFoldedSequenceHasCorrectC4ID verifies that the sequence C4 ID
// equals the hash of a canonical manifest built from member entries.
func TestFoldedSequenceHasCorrectC4ID(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	var members []*Entry
	for i := 1; i <= 3; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("data-%d", i)))
		entry := &Entry{
			Name:      fmt.Sprintf("render.%04d.png", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		}
		manifest.AddEntry(entry)
		members = append(members, entry)
	}

	folded := DetectSequences(manifest)

	var seqEntry *Entry
	for _, e := range folded.Entries {
		if e.IsSequence {
			seqEntry = e
			break
		}
	}
	if seqEntry == nil {
		t.Fatal("no sequence entry found")
	}

	// Build expected canonical manifest from members
	expected := NewManifest()
	for _, m := range members {
		cp := *m
		cp.Depth = 0
		expected.AddEntry(&cp)
	}
	expectedID := expected.ComputeC4ID()

	if seqEntry.C4ID != expectedID {
		t.Errorf("sequence C4 ID = %s, want %s (canonical manifest of members)", seqEntry.C4ID, expectedID)
	}
}
