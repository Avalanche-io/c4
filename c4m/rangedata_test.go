package c4m

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// ----------------------------------------------------------------------------
// Inline Range Data Tests
//
// These tests exercise all aspects of inline range data: encoding, decoding,
// round-trip, patch chain interaction, identity exclusion, ApplyPatch
// preservation, multiple sequences, and edge cases.
// ----------------------------------------------------------------------------

func makeSequenceManifest(t *testing.T, prefix string, count int) (*Manifest, []c4.ID) {
	t.Helper()
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewManifest()
	ids := make([]c4.ID, count)
	for i := 0; i < count; i++ {
		ids[i] = c4.Identify(strings.NewReader(fmt.Sprintf("%s-content-%d", prefix, i)))
		m.AddEntry(&Entry{
			Name:      fmt.Sprintf("%s.%04d.exr", prefix, i+1),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      ids[i],
		})
	}
	return m, ids
}

// TestIsInlineIDList tests the line detection function.
func TestIsInlineIDList(t *testing.T) {
	id1 := c4.Identify(strings.NewReader("a"))
	id2 := c4.Identify(strings.NewReader("b"))
	id3 := c4.Identify(strings.NewReader("c"))

	tests := []struct {
		name string
		line string
		want bool
	}{
		{"empty", "", false},
		{"bare C4 ID (90 chars)", id1.String(), false},
		{"two IDs (180 chars)", id1.String() + id2.String(), true},
		{"three IDs (270 chars)", id1.String() + id2.String() + id3.String(), true},
		{"not multiple of 90", id1.String() + "extra", false},
		{"starts with non-c4", "x4" + strings.Repeat("a", 88) + id1.String(), false},
		{"invalid chunk in middle", id1.String() + strings.Repeat("0", 90), false},
		{"single char", "c", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInlineIDList(tt.line)
			if got != tt.want {
				t.Errorf("isInlineIDList(%d chars) = %v, want %v", len(tt.line), got, tt.want)
			}
		})
	}
}

// TestRangeDataMarshalRoundTrip verifies encode → decode preserves RangeData.
func TestRangeDataMarshalRoundTrip(t *testing.T) {
	original, _ := makeSequenceManifest(t, "frame", 5)
	folded := DetectSequences(original)

	if len(folded.RangeData) != 1 {
		t.Fatalf("expected 1 RangeData, got %d", len(folded.RangeData))
	}

	// Marshal
	data, err := Marshal(folded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Verify output contains inline ID list line
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	idListLines := 0
	for _, line := range lines {
		if isInlineIDList(line) {
			idListLines++
		}
	}
	if idListLines != 1 {
		t.Errorf("expected 1 inline ID list line, got %d", idListLines)
	}

	// Unmarshal
	decoded, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.RangeData) != 1 {
		t.Fatalf("expected 1 RangeData after decode, got %d", len(decoded.RangeData))
	}

	// Verify the RangeData content matches
	for id, content := range folded.RangeData {
		decoded_content, ok := decoded.RangeData[id]
		if !ok {
			t.Errorf("RangeData key %s missing after round-trip", id)
			continue
		}
		if decoded_content != content {
			t.Errorf("RangeData content mismatch for %s", id)
		}
	}
}

// TestRangeDataDoesNotAffectIdentity verifies that adding/removing
// RangeData does not change the manifest's C4 ID.
func TestRangeDataDoesNotAffectIdentity(t *testing.T) {
	original, _ := makeSequenceManifest(t, "shot", 5)
	folded := DetectSequences(original)

	idWith := folded.ComputeC4ID()

	// Strip RangeData
	cp := folded.Copy()
	cp.RangeData = nil
	idWithout := cp.ComputeC4ID()

	if idWith != idWithout {
		t.Errorf("RangeData should not affect C4 ID\n  with:    %s\n  without: %s", idWith, idWithout)
	}
}

// TestRangeDataMultipleSequences verifies that multiple sequences in one
// manifest each get their own RangeData entry.
func TestRangeDataMultipleSequences(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewManifest()

	// First sequence: frame.0001-0005.exr
	for i := 1; i <= 5; i++ {
		m.AddEntry(&Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("frame-%d", i))),
		})
	}

	// Second sequence: comp.0001-0003.png
	for i := 1; i <= 3; i++ {
		m.AddEntry(&Entry{
			Name:      fmt.Sprintf("comp.%04d.png", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("comp-%d", i))),
		})
	}

	folded := DetectSequences(m)

	if len(folded.RangeData) != 2 {
		t.Fatalf("expected 2 RangeData entries, got %d", len(folded.RangeData))
	}

	// Marshal and verify 2 inline ID list lines
	data, err := Marshal(folded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	idListLines := 0
	for _, line := range lines {
		if isInlineIDList(line) {
			idListLines++
		}
	}
	if idListLines != 2 {
		t.Errorf("expected 2 inline ID list lines, got %d", idListLines)
	}

	// Round-trip
	decoded, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded.RangeData) != 2 {
		t.Fatalf("expected 2 RangeData after decode, got %d", len(decoded.RangeData))
	}
}

// TestRangeDataSelfVerifying verifies that the C4 ID of the inline ID list
// content matches the sequence entry's C4 ID.
func TestRangeDataSelfVerifying(t *testing.T) {
	original, _ := makeSequenceManifest(t, "render", 4)
	folded := DetectSequences(original)

	// Find the sequence entry's C4 ID
	var seqID c4.ID
	for _, e := range folded.Entries {
		if e.IsSequence {
			seqID = e.C4ID
			break
		}
	}
	if seqID.IsNil() {
		t.Fatal("no sequence entry found")
	}

	// Verify the RangeData key matches the sequence C4 ID
	content, ok := folded.RangeData[seqID]
	if !ok {
		t.Fatal("RangeData not keyed by sequence C4 ID")
	}

	// Verify content hashes to the same ID
	computedID := c4.Identify(strings.NewReader(content))
	if computedID != seqID {
		t.Errorf("RangeData content hash %s != sequence C4 ID %s", computedID, seqID)
	}
}

// TestRangeDataInPatchChainStream verifies inline ID list lines don't
// break patch chain parsing or corrupt patch boundaries.
func TestRangeDataInPatchChainStream(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build a manifest with a sequence
	m := NewManifest()
	for i := 1; i <= 3; i++ {
		m.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.exr", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("v1-%d", i))),
		})
	}
	folded := DetectSequences(m)
	baseData, err := Marshal(folded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	baseID := folded.ComputeC4ID()

	// Build patch chain: base + range data + boundary + patch entry
	var chain bytes.Buffer
	chain.Write(baseData) // includes inline ID list
	chain.WriteString(baseID.String() + "\n")
	readmeID := c4.Identify(strings.NewReader("readme-content"))
	chain.WriteString(fmt.Sprintf("-rw-r--r-- %s 100 readme.txt %s\n",
		baseTime.Format(TimestampFormat), readmeID))

	// Decode with full verification
	decoded, err := NewDecoder(&chain).Decode()
	if err != nil {
		t.Fatalf("Decode patch chain: %v", err)
	}

	// Should have the sequence entry + readme
	if len(decoded.Entries) != 2 {
		t.Errorf("expected 2 entries after patch, got %d", len(decoded.Entries))
	}

	// RangeData should be recovered from the base section
	if len(decoded.RangeData) != 1 {
		t.Errorf("expected 1 RangeData, got %d", len(decoded.RangeData))
	}
}

// TestRangeDataPreservedInApplyPatch verifies that RangeData survives
// patch application.
func TestRangeDataPreservedInApplyPatch(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Base has a sequence
	base := NewManifest()
	for i := 1; i <= 3; i++ {
		base.AddEntry(&Entry{
			Name:      fmt.Sprintf("frame.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("frame-%d", i))),
		})
	}
	base = DetectSequences(base)

	if len(base.RangeData) != 1 {
		t.Fatalf("expected 1 RangeData in base, got %d", len(base.RangeData))
	}

	// Patch adds a file
	patch := NewManifest()
	patch.AddEntry(&Entry{
		Name:      "readme.txt",
		Size:      100,
		Timestamp: baseTime,
		Mode:      0644,
		C4ID:      c4.Identify(strings.NewReader("readme")),
	})

	result := ApplyPatch(base, patch)

	// RangeData should be preserved
	if len(result.RangeData) != 1 {
		t.Errorf("expected 1 RangeData after patch, got %d", len(result.RangeData))
	}

	// Verify the sequence entry still exists
	hasSeq := false
	for _, e := range result.Entries {
		if e.IsSequence {
			hasSeq = true
			break
		}
	}
	if !hasSeq {
		t.Error("sequence entry lost after patch")
	}
}

// TestRangeDataMergedInApplyPatch verifies that RangeData from both
// base and patch manifests are merged.
func TestRangeDataMergedInApplyPatch(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Base has sequence A
	base := NewManifest()
	for i := 1; i <= 3; i++ {
		base.AddEntry(&Entry{
			Name:      fmt.Sprintf("frameA.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("A-%d", i))),
		})
	}
	base = DetectSequences(base)

	// Patch has sequence B
	patch := NewManifest()
	for i := 1; i <= 4; i++ {
		patch.AddEntry(&Entry{
			Name:      fmt.Sprintf("frameB.%04d.exr", i),
			Size:      2048,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("B-%d", i))),
		})
	}
	patch = DetectSequences(patch)

	result := ApplyPatch(base, patch)

	// Both RangeData entries should be present
	if len(result.RangeData) != 2 {
		t.Errorf("expected 2 RangeData entries after merge, got %d", len(result.RangeData))
	}
}

// TestRangeDataEncoderDeterminism verifies that encoding the same
// RangeData always produces the same output.
func TestRangeDataEncoderDeterminism(t *testing.T) {
	original, _ := makeSequenceManifest(t, "shot", 5)
	folded := DetectSequences(original)

	// Marshal 10 times, all should be identical
	var outputs []string
	for i := 0; i < 10; i++ {
		data, err := Marshal(folded)
		if err != nil {
			t.Fatalf("Marshal %d: %v", i, err)
		}
		outputs = append(outputs, string(data))
	}

	for i := 1; i < len(outputs); i++ {
		if outputs[i] != outputs[0] {
			t.Errorf("Marshal output %d differs from output 0", i)
		}
	}
}

// TestRangeDataCopyDeep verifies that Copy() produces an independent
// RangeData map.
func TestRangeDataCopyDeep(t *testing.T) {
	original, _ := makeSequenceManifest(t, "frame", 3)
	folded := DetectSequences(original)

	cp := folded.Copy()

	// Mutate original's RangeData
	for k := range folded.RangeData {
		folded.RangeData[k] = "MUTATED"
	}

	// Copy should be unaffected
	for _, v := range cp.RangeData {
		if v == "MUTATED" {
			t.Error("Copy() RangeData shares references with original")
		}
	}
}

// TestRangeDataExpandRoundTrip verifies the full cycle:
// individual files → fold → encode → decode → expand → individual IDs recovered
func TestRangeDataExpandRoundTrip(t *testing.T) {
	original, ids := makeSequenceManifest(t, "plate", 5)

	// Fold
	folded := DetectSequences(original)

	// Encode
	data, err := Marshal(folded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Decode
	decoded, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Expand
	expander := NewSequenceExpander(SequenceEmbedded)
	expanded, _, err := expander.ExpandManifest(decoded)
	if err != nil {
		t.Fatalf("ExpandManifest: %v", err)
	}

	// Verify individual IDs
	expandedIDs := make(map[string]c4.ID)
	for _, e := range expanded.Entries {
		if !e.IsSequence {
			expandedIDs[e.Name] = e.C4ID
		}
	}

	for i, id := range ids {
		name := fmt.Sprintf("plate.%04d.exr", i+1)
		got, ok := expandedIDs[name]
		if !ok {
			t.Errorf("missing expanded entry %s", name)
			continue
		}
		if got != id {
			t.Errorf("%s: got %s, want %s", name, got, id)
		}
	}
}

// TestInlineIDListLineLengthDisambiguation verifies that the length-based
// disambiguation between patch boundaries and inline ID lists is correct.
func TestInlineIDListLineLengthDisambiguation(t *testing.T) {
	id := c4.Identify(strings.NewReader("test"))

	// 90 chars = bare C4 ID (patch boundary), NOT an inline list
	if isInlineIDList(id.String()) {
		t.Error("90-char bare C4 ID should NOT be detected as inline ID list")
	}
	if !isBareC4ID(id.String()) {
		t.Error("90-char bare C4 ID should be detected as patch boundary")
	}

	// 180 chars = two concatenated IDs = inline list
	twoIDs := id.String() + c4.Identify(strings.NewReader("test2")).String()
	if !isInlineIDList(twoIDs) {
		t.Error("180-char concatenated IDs should be detected as inline ID list")
	}
	if isBareC4ID(twoIDs) {
		t.Error("180-char line should NOT be detected as patch boundary")
	}
}

// TestRangeDataDecodePatchChainSkipsIDLists verifies that DecodePatchChain
// correctly ignores inline ID list lines.
func TestRangeDataDecodePatchChainSkipsIDLists(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Build inline ID list content
	idList := newIDList()
	for i := 1; i <= 3; i++ {
		idList.Add(c4.Identify(strings.NewReader(fmt.Sprintf("content-%d", i))))
	}
	idListLine := idList.Canonical()

	// Build a c4m with an entry, inline ID list, then more entries
	var buf bytes.Buffer
	seqID := idList.ComputeC4ID()
	fmt.Fprintf(&buf, "-rw-r--r-- %s 3072 frames.[0001-0003].exr %s\n",
		baseTime.Format(TimestampFormat), seqID)
	fmt.Fprintf(&buf, "%s\n", idListLine) // inline ID list

	sections, err := DecodePatchChain(&buf)
	if err != nil {
		t.Fatalf("DecodePatchChain: %v", err)
	}

	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if len(sections[0].Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(sections[0].Entries))
	}
}

// TestEncodePatchWithRangeData verifies that EncodePatch writes
// range data correctly.
func TestEncodePatchWithRangeData(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Create a patch result with RangeData
	oldManifest := NewManifest()
	oldManifest.AddEntry(&Entry{
		Name:      "readme.txt",
		Size:      100,
		Timestamp: baseTime,
		Mode:      0644,
		C4ID:      c4.Identify(strings.NewReader("readme")),
	})
	oldID := oldManifest.ComputeC4ID()

	patchManifest := NewManifest()
	for i := 1; i <= 3; i++ {
		patchManifest.AddEntry(&Entry{
			Name:      fmt.Sprintf("shot.%04d.exr", i),
			Size:      1024,
			Timestamp: baseTime,
			Mode:      0644,
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("shot-%d", i))),
		})
	}
	patchManifest = DetectSequences(patchManifest)

	pr := &PatchResult{
		OldID: oldID,
		Patch: patchManifest,
	}

	var buf bytes.Buffer
	err := NewEncoder(&buf).EncodePatch(pr)
	if err != nil {
		t.Fatalf("EncodePatch: %v", err)
	}

	output := buf.String()

	// Should contain the old ID
	if !strings.Contains(output, oldID.String()) {
		t.Error("output should contain old manifest C4 ID")
	}

	// Should contain an inline ID list line
	foundIDList := false
	for _, line := range strings.Split(output, "\n") {
		if isInlineIDList(line) {
			foundIDList = true
		}
	}
	if !foundIDList {
		t.Error("EncodePatch output should contain inline ID list line")
	}
}
