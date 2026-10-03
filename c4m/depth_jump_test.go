package c4m

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// Malformed structure: an entry whose depth skips past its parent. A
// corrupt, truncated, or hand-edited c4m, or a producer bug, can carry
// one. Applying such a section as a patch indexed past the end of the
// directory stack and panicked the process.

func djFile(name string, depth int) *Entry {
	return &Entry{
		Name:      name,
		Depth:     depth,
		Mode:      0644,
		Size:      int64(len(name)),
		Timestamp: time.Unix(1700000000, 0).UTC(),
		C4ID:      c4.Identify(strings.NewReader(name)),
	}
}

func djDir(name string, depth int) *Entry {
	return &Entry{
		Name:      name,
		Depth:     depth,
		Mode:      os.ModeDir | 0755,
		Size:      -1,
		Timestamp: NullTimestamp(),
	}
}

func djManifest(entries ...*Entry) *Manifest {
	m := NewManifest()
	m.Entries = entries
	return m
}

// assertDepthErr checks the error names the offending entry and its depth.
func assertDepthErr(t *testing.T, err error, name string, depth string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error for an entry whose depth skips past its parent, got nil")
	}
	if !errors.Is(err, ErrInvalidEntry) {
		t.Errorf("error should wrap ErrInvalidEntry: %v", err)
	}
	if !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "depth "+depth) {
		t.Errorf("error should name entry %q and depth %s: %v", name, depth, err)
	}
}

func TestApplyPatchCheckedDepthJumpUnderFile(t *testing.T) {
	base := djManifest(djFile("a.txt", 0))
	// y.txt at depth 1 directly under a depth-0 file.
	patch := djManifest(djFile("z.txt", 0), djFile("y.txt", 1))

	_, err := ApplyPatchChecked(base, patch)
	assertDepthErr(t, err, "y.txt", "1")

	// Malformed base is caught the same way.
	_, err = ApplyPatchChecked(patch, base)
	assertDepthErr(t, err, "y.txt", "1")
}

func TestApplyPatchCheckedDepthJumpUnderDir(t *testing.T) {
	base := djManifest(djFile("a.txt", 0))
	// deep.txt at depth 2 directly under a depth-0 directory.
	patch := djManifest(djDir("d/", 0), djFile("deep.txt", 2))

	_, err := ApplyPatchChecked(base, patch)
	assertDepthErr(t, err, "deep.txt", "2")
}

func TestApplyPatchCheckedNegativeDepth(t *testing.T) {
	base := djManifest(djFile("a.txt", 0))
	patch := djManifest(djFile("neg.txt", -1))

	_, err := ApplyPatchChecked(base, patch)
	assertDepthErr(t, err, "neg.txt", "-1")
}

func TestResolvePatchChainCheckedDepthJump(t *testing.T) {
	sections := []*PatchSection{
		{Entries: []*Entry{djFile("a.txt", 0)}},
		{Entries: []*Entry{djFile("z.txt", 0), djFile("y.txt", 1)}},
	}
	_, err := ResolvePatchChainChecked(sections, 0)
	assertDepthErr(t, err, "y.txt", "1")

	// Stopping before the malformed section resolves cleanly.
	m, err := ResolvePatchChainChecked(sections, 1)
	if err != nil {
		t.Fatalf("stopAt=1 should not touch the malformed section: %v", err)
	}
	if len(m.Entries) != 1 || m.Entries[0].Name != "a.txt" {
		t.Fatalf("unexpected stopAt=1 result: %v", m.Entries)
	}
}

func TestCheckedMatchesUncheckedOnValidInput(t *testing.T) {
	base := djManifest(djFile("a.txt", 0), djDir("d/", 0), djFile("b.txt", 1))
	patch := djManifest(djFile("c.txt", 0), djDir("d/", 0), djFile("e.txt", 1))

	want := ApplyPatch(base, patch)
	got, err := ApplyPatchChecked(base, patch)
	if err != nil {
		t.Fatalf("ApplyPatchChecked on valid input: %v", err)
	}
	if got.Canonical() != want.Canonical() {
		t.Fatalf("checked and unchecked results differ:\n%s\nvs\n%s", got.Canonical(), want.Canonical())
	}

	sections := []*PatchSection{{Entries: base.Entries}, {Entries: patch.Entries}}
	wantChain := ResolvePatchChain(sections, 0)
	gotChain, err := ResolvePatchChainChecked(sections, 0)
	if err != nil {
		t.Fatalf("ResolvePatchChainChecked on valid input: %v", err)
	}
	if gotChain.Canonical() != wantChain.Canonical() {
		t.Fatalf("checked and unchecked chain results differ")
	}
}

// The unchecked forms keep their signatures; on malformed structure they
// panic with the same descriptive error the checked forms return, instead
// of a runtime index error.
func TestApplyPatchUncheckedPanicsWithDescriptiveError(t *testing.T) {
	defer func() {
		r := recover()
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected panic with an error value, got %#v", r)
		}
		assertDepthErr(t, err, "y.txt", "1")
	}()
	ApplyPatch(djManifest(djFile("a.txt", 0)), djManifest(djFile("z.txt", 0), djFile("y.txt", 1)))
}

// The decoder already returns errors; a malformed patch section must
// come back as one rather than a panic.
func TestDecodePatchSectionDepthJump(t *testing.T) {
	base := djManifest(djFile("a.txt", 0), djDir("d/", 0), djFile("b.txt", 1))
	baseText := base.Canonical()
	// Entry lines written with two-space indentation (b.txt fixes the width).
	var sb strings.Builder
	for _, e := range base.Entries {
		sb.WriteString(e.Format(2, false))
		sb.WriteString("\n")
	}
	sb.WriteString(c4.Identify(strings.NewReader(baseText)).String())
	sb.WriteString("\n")
	sb.WriteString(djFile("z.txt", 0).Format(2, false) + "\n")
	sb.WriteString(djFile("y.txt", 1).Format(2, false) + "\n")
	chain := sb.String()

	_, err := Unmarshal([]byte(chain))
	assertDepthErr(t, err, "y.txt", "1")

	// Sections decode fine (structure is checked when applied) ...
	sections, err := DecodePatchChain(strings.NewReader(chain))
	if err != nil {
		t.Fatalf("DecodePatchChain: %v", err)
	}
	// ... and resolving them reports the malformed entry.
	_, err = ResolvePatchChainChecked(sections, 0)
	assertDepthErr(t, err, "y.txt", "1")

	// A malformed base followed by a patch is reported too.
	bad := djFile("z.txt", 0).Format(2, false) + "\n" +
		djFile("y.txt", 1).Format(2, false) + "\n" +
		c4.Identify(strings.NewReader("x")).String() + "\n" +
		djFile("q.txt", 0).Format(2, false) + "\n"
	_, err = Unmarshal([]byte(bad))
	assertDepthErr(t, err, "y.txt", "1")
}

// A flat (single-section) c4m with an orphaned entry is tolerated by the
// decoder, as before: no patch is applied, so nothing changes there.
func TestDecodeFlatDepthJumpUnchanged(t *testing.T) {
	flat := djFile("z.txt", 0).Format(2, false) + "\n" +
		djFile("y.txt", 1).Format(2, false) + "\n"
	m, err := Unmarshal([]byte(flat))
	if err != nil {
		t.Fatalf("flat c4m decode behavior changed: %v", err)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m.Entries))
	}
}
