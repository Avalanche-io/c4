package c4m

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func TestDecodeNoPatch(t *testing.T) {
	// A normal c4m with no bare C4 ID lines decodes as before.
	input := "-rw-r--r-- 2026-03-06T12:00:00Z 100 a.txt\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(m.Entries))
	}
	if !m.Base.IsNil() {
		t.Error("Base should be nil for non-patch c4m")
	}
}

func TestDecodeFirstLineBareID(t *testing.T) {
	// A bare C4 ID on the first line sets Base (external reference).
	baseID := c4.Identify(strings.NewReader("base content"))

	input := baseID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 100 new.txt\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Base != baseID {
		t.Errorf("Base = %s, want %s", m.Base, baseID)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(m.Entries))
	}
	if m.Entries[0].Name != "new.txt" {
		t.Errorf("entry name = %q, want %q", m.Entries[0].Name, "new.txt")
	}
}

func TestDecodeFirstLineBareIDWithLeadingBlankLines(t *testing.T) {
	// Blank lines before a bare C4 ID should not prevent it being
	// recognized as an external base reference (first non-blank line).
	baseID := c4.Identify(strings.NewReader("base content"))

	input := "\n\n" + baseID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 100 new.txt\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if m.Base != baseID {
		t.Errorf("Base = %s, want %s", m.Base, baseID)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(m.Entries))
	}
}

func TestDecodeInlinePatchAdd(t *testing.T) {
	// Build a base manifest, compute its ID, then write a stream with
	// the base entries, a bare C4 ID checkpoint, and a patch that adds a file.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{{
			Name:      "a.txt",
			Mode:      0644,
			Timestamp: ts,
			Size:      100,
			C4ID:      c4.Identify(strings.NewReader("aaa")),
			Depth:     0,
		}},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// Stream: base entries, checkpoint, then patch adds b.txt.
	input := baseText +
		baseID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt " + c4.Identify(strings.NewReader("bbb")).String() + "\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(m.Entries))
	}

	names := map[string]bool{}
	for _, e := range m.Entries {
		names[e.Name] = true
	}
	if !names["a.txt"] || !names["b.txt"] {
		t.Errorf("expected a.txt and b.txt, got entries: %v", m.Entries)
	}
}

func TestDecodeInlinePatchRemove(t *testing.T) {
	// Patch removes a file by repeating it identically.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	aID := c4.Identify(strings.NewReader("aaa"))
	bID := c4.Identify(strings.NewReader("bbb"))

	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100, C4ID: aID, Depth: 0},
			{Name: "b.txt", Mode: 0644, Timestamp: ts, Size: 200, C4ID: bID, Depth: 0},
		},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// Patch: repeat b.txt identically → removal.
	input := baseText +
		baseID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt " + bID.String() + "\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(m.Entries))
	}
	if m.Entries[0].Name != "a.txt" {
		t.Errorf("remaining entry = %q, want a.txt", m.Entries[0].Name)
	}
}

func TestDecodeInlinePatchModify(t *testing.T) {
	// Patch modifies a file by repeating it with different metadata.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	aID := c4.Identify(strings.NewReader("aaa"))

	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100, C4ID: aID, Depth: 0},
		},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// Patch: same name, new C4 ID → modification.
	newID := c4.Identify(strings.NewReader("aaa-v2"))
	input := baseText +
		baseID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 a.txt " + newID.String() + "\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(m.Entries))
	}
	if m.Entries[0].C4ID != newID {
		t.Errorf("C4ID = %s, want %s", m.Entries[0].C4ID, newID)
	}
	if m.Entries[0].Size != 200 {
		t.Errorf("Size = %d, want 200", m.Entries[0].Size)
	}
}

func TestDecodeInlinePatchIDMismatch(t *testing.T) {
	// A bare C4 ID that doesn't match the accumulated content must fail.
	input := "-rw-r--r-- 2026-03-06T12:00:00Z 100 a.txt\n" +
		c4.Identify(strings.NewReader("wrong")).String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt\n"

	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("expected ErrPatchIDMismatch, got nil")
	}
	if !strings.Contains(err.Error(), "patch ID does not match") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeMultiplePatches(t *testing.T) {
	// Stream with two successive patches.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	aID := c4.Identify(strings.NewReader("aaa"))

	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100, C4ID: aID, Depth: 0},
		},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// First patch: add b.txt.
	bID := c4.Identify(strings.NewReader("bbb"))
	patch1Entry := &Entry{Name: "b.txt", Mode: 0644, Timestamp: ts, Size: 200, C4ID: bID, Depth: 0}
	state1 := ApplyPatch(base, &Manifest{Version: "1.0", Entries: []*Entry{patch1Entry}})
	state1ID := state1.ComputeC4ID()

	// Second patch: add c.txt.
	cID := c4.Identify(strings.NewReader("ccc"))

	input := baseText +
		baseID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt " + bID.String() + "\n" +
		state1ID.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 300 c.txt " + cID.String() + "\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(m.Entries))
	}

	names := map[string]bool{}
	for _, e := range m.Entries {
		names[e.Name] = true
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if !names[name] {
			t.Errorf("missing entry %q", name)
		}
	}
}

func TestEncodePatchRoundTrip(t *testing.T) {
	// PatchDiff → EncodePatch → Decode should produce the correct result.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	aID := c4.Identify(strings.NewReader("aaa"))
	bID := c4.Identify(strings.NewReader("bbb"))

	old := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100, C4ID: aID, Depth: 0},
		},
	}
	new := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100, C4ID: aID, Depth: 0},
			{Name: "b.txt", Mode: 0644, Timestamp: ts, Size: 200, C4ID: bID, Depth: 0},
		},
	}

	pr := PatchDiff(old, new)

	// Encode: old manifest + checkpoint + patch.
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(old)
	NewEncoder(&buf).EncodePatch(pr)

	// Decode the combined stream.
	m, err := Unmarshal(buf.Bytes())
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(m.Entries))
	}
}

func TestDecodeEmptyPatchAtEOF(t *testing.T) {
	// A bare C4 ID followed by nothing (empty patch) must be rejected.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100,
				C4ID: c4.Identify(strings.NewReader("aaa")), Depth: 0},
		},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// Stream: base entries, then checkpoint with nothing after.
	input := baseText + baseID.String() + "\n"

	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("expected ErrEmptyPatch, got nil")
	}
	if !strings.Contains(err.Error(), "empty patch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeEmptyPatchBetweenIDs(t *testing.T) {
	// Two consecutive bare C4 IDs (empty patch section between them).
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100,
				C4ID: c4.Identify(strings.NewReader("aaa")), Depth: 0},
		},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// Stream: base entries, checkpoint, empty, checkpoint again.
	input := baseText +
		baseID.String() + "\n" +
		baseID.String() + "\n"

	_, err := Unmarshal([]byte(input))
	if err == nil {
		t.Fatal("expected ErrEmptyPatch, got nil")
	}
	if !strings.Contains(err.Error(), "empty patch") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecodeMetadataOnlyPatch(t *testing.T) {
	// Patch changes only metadata (chmod), not content.
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	aID := c4.Identify(strings.NewReader("aaa"))

	base := &Manifest{
		Version: "1.0",
		Entries: []*Entry{
			{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 100, C4ID: aID, Depth: 0},
		},
	}

	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseText := baseBuf.String()
	baseID := base.ComputeC4ID()

	// Patch: same C4 ID but mode changed to 0755.
	input := baseText +
		baseID.String() + "\n" +
		"-rwxr-xr-x 2026-03-06T12:00:00Z 100 a.txt " + aID.String() + "\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(m.Entries))
	}
	if m.Entries[0].Mode != 0755 {
		t.Errorf("Mode = %o, want 755", m.Entries[0].Mode)
	}
	if m.Entries[0].C4ID != aID {
		t.Error("C4 ID should be preserved for metadata-only change")
	}
}
