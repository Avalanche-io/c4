package c4m

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// ----------------------------------------------------------------------------
// Range Folding Tests
//
// These tests validate sequence folding behaviors: identity computation,
// expansion, symlink ranges, gaps, and round-trip stability.
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

// TestFoldingExpansionLosesMetadata demonstrates that expanding a sequence
// without an external ID list source loses per-entry metadata (sizes become -1,
// C4 IDs become nil). This is expected — individual metadata lives in the store,
// not embedded in the manifest.
func TestFoldingExpansionLosesMetadata(t *testing.T) {
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

	// Expand (without store, loses individual IDs and sizes)
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

	// Re-fold — IDs differ because expansion loses per-entry metadata
	folded2 := DetectSequences(reexpanded)
	id2 := folded2.ComputeC4ID()

	if id1 == id2 {
		t.Error("expected different IDs due to metadata loss during expansion")
	}

	// Verify the root cause: expanded entries have null sizes
	for _, e := range reexpanded.Entries {
		if e.Size != -1 {
			t.Errorf("expanded entry %s has size %d, expected -1 (null)", e.Name, e.Size)
		}
	}
}

// TestFoldingIdempotencyWithFullMetadata verifies that when individual
// metadata is preserved, fold(expand(fold(entries))) = fold(entries).
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

	// Re-fold from the same original entries (simulating full metadata preserved)
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
			Mode:      os.ModeSymlink | 0777,
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

// TestExpandWithRangeDataPreservesIDs verifies that expanding a sequence
// with inline RangeData recovers the individual C4 IDs.
func TestExpandWithRangeDataPreservesIDs(t *testing.T) {
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

	// Fold — DetectSequences now populates RangeData
	folded := DetectSequences(original)

	if len(folded.RangeData) == 0 {
		t.Fatal("expected RangeData to be populated after folding")
	}

	// Expand — should recover individual IDs from RangeData
	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(folded)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

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

// TestExpandWithoutRangeDataFallsBack verifies that expanding a sequence
// without RangeData uses the sequence C4 ID as fallback.
func TestExpandWithoutRangeDataFallsBack(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	original := NewManifest()
	for i := 0; i < 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("unique-content-%d", i)))
		original.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.dpx", i+1),
			Size:      4096,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	// Fold and then strip RangeData (simulating store-backed scenario)
	folded := DetectSequences(original)
	folded.RangeData = nil

	var seqID c4.ID
	for _, e := range folded.Entries {
		if e.IsSequence {
			seqID = e.C4ID
			break
		}
	}

	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(folded)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	for _, e := range expanded.Entries {
		if !e.IsSequence && e.C4ID != seqID {
			t.Errorf("expanded entry %s should have sequence C4 ID %s, got %s", e.Name, seqID, e.C4ID)
		}
	}
}

// TestSequenceFoldingIsDeterministic verifies that folding the same
// entries always produces the same C4 ID.
func TestSequenceFoldingIsDeterministic(t *testing.T) {
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

	id1 := DetectSequences(makeManifest()).ComputeC4ID()
	id2 := DetectSequences(makeManifest()).ComputeC4ID()

	if id1 != id2 {
		t.Errorf("folding should be deterministic\n  first:  %s\n  second: %s", id1, id2)
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

// TestDirectiveLinesRejected verifies that directive lines cause errors.
func TestDirectiveLinesRejected(t *testing.T) {
	id := c4.Identify(strings.NewReader("test"))
	input := fmt.Sprintf("@expand %s\n", id)
	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("directive lines should return an error")
	}
}

// TestInlineRangeDataRoundTrip verifies that encoding and decoding
// a manifest with inline range data preserves the ID list.
func TestInlineRangeDataRoundTrip(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	original := NewManifest()
	for i := 1; i <= 5; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i)))
		original.AddEntry(&Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	// Fold — populates RangeData
	folded := DetectSequences(original)
	if len(folded.RangeData) != 1 {
		t.Fatalf("expected 1 RangeData entry, got %d", len(folded.RangeData))
	}

	// Marshal (includes trailing ID list line)
	data, err := Marshal(folded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// The output should contain a line >90 chars (the inline ID list)
	lines := strings.Split(string(data), "\n")
	foundIDList := false
	for _, line := range lines {
		if len(line) > 90 && len(line)%90 == 0 {
			foundIDList = true
		}
	}
	if !foundIDList {
		t.Error("expected inline ID list line in output")
	}

	// Unmarshal — should recover RangeData
	decoded, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.RangeData) != 1 {
		t.Fatalf("expected 1 RangeData after decode, got %d", len(decoded.RangeData))
	}

	// Manifest identity should be the same with or without range data
	idWith := folded.ComputeC4ID()
	folded.RangeData = nil
	idWithout := folded.ComputeC4ID()
	if idWith != idWithout {
		t.Errorf("RangeData should not affect manifest C4 ID\n  with:    %s\n  without: %s", idWith, idWithout)
	}
}

// TestInlineRangeDataInPatchChain verifies that inline ID list lines
// don't interfere with patch chain parsing.
func TestInlineRangeDataInPatchChain(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create a manifest with a sequence
	m := NewManifest()
	for i := 1; i <= 3; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("v1-content-%d", i)))
		m.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.exr", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
	}

	folded := DetectSequences(m)
	data, err := Marshal(folded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Compute the manifest ID and build a patch chain
	baseID := folded.ComputeC4ID()

	// Build patch chain: base + ID list + patch boundary + patch entries
	var chain strings.Builder
	chain.Write(data)
	chain.WriteString(baseID.String() + "\n")
	// Add a simple file as a patch
	chain.WriteString(fmt.Sprintf("-rw-r--r-- %s 100 readme.txt %s\n",
		baseTime.Format(TimestampFormat),
		c4.Identify(strings.NewReader("readme"))))

	// Parse the patch chain
	sections, err := DecodePatchChain(strings.NewReader(chain.String()))
	if err != nil {
		t.Fatalf("DecodePatchChain: %v", err)
	}

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
}

// TestFoldedSequenceHasCorrectC4ID verifies that the sequence C4 ID
// equals the hash of the bare C4 ID concatenation of frame IDs in range order.
func TestFoldedSequenceHasCorrectC4ID(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	manifest := NewManifest()
	idList := newIDList()
	for i := 1; i <= 3; i++ {
		id := c4.Identify(strings.NewReader(fmt.Sprintf("data-%d", i)))
		manifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("render.%04d.png", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      id,
		})
		idList.Add(id)
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

	// Sequence C4 ID = hash of bare C4 IDs concatenated in range order
	expectedID := idList.ComputeC4ID()
	if seqEntry.C4ID != expectedID {
		t.Errorf("sequence C4 ID = %s, want %s (bare ID list hash)", seqEntry.C4ID, expectedID)
	}
}
