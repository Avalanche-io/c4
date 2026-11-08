package c4m

import (
	"bytes"
	"errors"
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

func TestDecodeVerifiedCheckpoint(t *testing.T) {
	// Grammar erratum (draft-v9 §4): a checkpoint names the accumulated
	// manifest state and a resolving decoder MUST verify it. A wrong
	// checkpoint is rejected; the correct one is accepted.
	wrong := "-rw-r--r-- 2026-03-06T12:00:00Z 100 a.txt\n" +
		c4.Identify(strings.NewReader("wrong")).String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt\n"
	if _, err := Unmarshal([]byte(wrong)); err == nil {
		t.Fatal("wrong checkpoint must be rejected (ErrPatchIDMismatch)")
	}

	baseOnly, err := Unmarshal([]byte("-rw-r--r-- 2026-03-06T12:00:00Z 100 a.txt\n"))
	if err != nil {
		t.Fatal(err)
	}
	input := "-rw-r--r-- 2026-03-06T12:00:00Z 100 a.txt\n" +
		baseOnly.ComputeC4ID().String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt\n"

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("verified checkpoint should be accepted: %v", err)
	}
	// The patch (b.txt) should be applied to the base (a.txt).
	// Since b.txt is new, the result should have both entries.
	if len(m.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m.Entries))
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

func TestDecodeTrailingID(t *testing.T) {
	// A bare C4 ID at EOF is a format error (2026-11-08): a c4m file
	// never ends with its own ID. The 2026-07-13 erratum had made it
	// the legal "closing validator"; that is withdrawn.
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

	// Stream: base entries, then the state's own ID: refused, and the
	// error says what to do.
	input := baseText + baseID.String() + "\n"
	_, err := Unmarshal([]byte(input))
	if !errors.Is(err, ErrTrailingID) {
		t.Fatalf("trailing ID must be ErrTrailingID, got %v", err)
	}
	if !strings.Contains(err.Error(), "c4 id FILE.c4m") {
		t.Fatalf("error must say how to fix the file, got %v", err)
	}

	// A WRONG trailing ID is still a checkpoint mismatch first.
	bad := baseText + c4.Identify(strings.NewReader("not the state")).String() + "\n"
	if _, err := Unmarshal([]byte(bad)); !errors.Is(err, ErrPatchIDMismatch) {
		t.Fatalf("wrong trailing ID must be ErrPatchIDMismatch, got %v", err)
	}

	// The migration path: a lenient decoder verifies the trailing ID,
	// drops it, and reports that it did.
	d := NewDecoder(strings.NewReader(input))
	d.AllowTrailingID()
	m, err := d.Decode()
	if err != nil {
		t.Fatalf("AllowTrailingID must accept a correct trailing ID: %v", err)
	}
	if !d.DroppedTrailingID() {
		t.Fatal("DroppedTrailingID must report the dropped line")
	}
	if m.ComputeC4ID() != baseID {
		t.Fatal("migrated chain must resolve to the dropped ID's state")
	}
	d = NewDecoder(strings.NewReader(bad))
	d.AllowTrailingID()
	if _, err := d.Decode(); !errors.Is(err, ErrPatchIDMismatch) {
		t.Fatalf("AllowTrailingID must still verify the ID it drops, got %v", err)
	}

	// A chain that ends on its entries is the valid form, and a bare ID
	// before a patch is still a boundary.
	patch := &Manifest{Version: "1.0", Entries: []*Entry{
		{Name: "b.txt", Mode: 0644, Timestamp: ts, Size: 3,
			C4ID: c4.Identify(strings.NewReader("bbb")), Depth: 0}}}
	var patchBuf bytes.Buffer
	NewEncoder(&patchBuf).Encode(patch)
	d = NewDecoder(strings.NewReader(baseText + baseID.String() + "\n" + patchBuf.String()))
	m, err = d.Decode()
	if err != nil {
		t.Fatalf("boundary before a patch must be accepted: %v", err)
	}
	if d.DroppedTrailingID() || len(m.Entries) != 2 {
		t.Fatalf("chain must resolve to both entries with nothing dropped, got %d", len(m.Entries))
	}
}

func TestDecodeConsecutiveCheckpoints(t *testing.T) {
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

	// Consecutive checkpoints both name the same accumulated state:
	// close-then-supersede — legal when equal, rejected when not. A
	// patch must follow them: a stream never ends with a bare ID.
	patch := &Manifest{Version: "1.0", Entries: []*Entry{
		{Name: "b.txt", Mode: 0644, Timestamp: ts, Size: 3,
			C4ID: c4.Identify(strings.NewReader("bbb")), Depth: 0}}}
	var patchBuf bytes.Buffer
	NewEncoder(&patchBuf).Encode(patch)
	input := baseText +
		baseID.String() + "\n" +
		baseID.String() + "\n" +
		patchBuf.String()

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("consecutive equal checkpoints must be accepted: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("chain must resolve to the checkpointed state plus the patch, got %d entries", len(m.Entries))
	}
	if _, err := Unmarshal([]byte(baseText + baseID.String() + "\n" + baseID.String() + "\n")); !errors.Is(err, ErrTrailingID) {
		t.Fatalf("consecutive checkpoints at EOF end the file with a bare ID: want ErrTrailingID, got %v", err)
	}

	bad := baseText +
		baseID.String() + "\n" +
		c4.Identify(strings.NewReader("other")).String() + "\n"
	if _, err := Unmarshal([]byte(bad)); err == nil {
		t.Fatal("consecutive differing checkpoint must be rejected here (no fetch to re-anchor)")
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

// TestDecodeExternalBaseDefersAllCheckpoints is the regression test
// for the deferral bug found by the libc4 conformance port: after a
// first-line external base reference, EVERY checkpoint's verification
// defers to the fetching resolver — including checkpoints after the
// first patch application — and the base reference survives decoding.
func TestDecodeExternalBaseDefersAllCheckpoints(t *testing.T) {
	base := c4.Identify(strings.NewReader("the external base"))
	// Two patch sections with boundary IDs that could never match a
	// base-less fold of the patch entries (they are the appended-diff
	// shape: resolved-state IDs of a state we cannot reconstruct here).
	// The stream ends on its last patch's entries; even with an
	// external base a trailing bare ID is a format error.
	boundary1 := c4.Identify(strings.NewReader("resolved state one"))
	boundary2 := c4.Identify(strings.NewReader("resolved state two"))
	input := base.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 100 a.txt " + c4.Identify(strings.NewReader("aaa")).String() + "\n" +
		boundary1.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 200 b.txt " + c4.Identify(strings.NewReader("bbb")).String() + "\n" +
		boundary2.String() + "\n" +
		"-rw-r--r-- 2026-03-06T12:00:00Z 300 c.txt " + c4.Identify(strings.NewReader("ccc")).String() + "\n"
	if _, err := Unmarshal([]byte(strings.TrimSuffix(input, "-rw-r--r-- 2026-03-06T12:00:00Z 300 c.txt "+c4.Identify(strings.NewReader("ccc")).String()+"\n"))); !errors.Is(err, ErrTrailingID) {
		t.Fatalf("external-base chain ending with a bare ID must be ErrTrailingID, got %v", err)
	}

	m, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("external-base chain must decode with verification deferred: %v", err)
	}
	if m.Base != base {
		t.Fatal("the external base reference must survive patch application")
	}
	if len(m.Entries) != 3 {
		t.Fatalf("expected 3 folded entries, got %d", len(m.Entries))
	}
}
