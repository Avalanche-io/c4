package c4m

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// depthJumpChain is a library-written chain whose patch holds an entry at
// depth 1 under a file: the damage a truncated or hand-edited c4m carries.
// The base is ordinary; the patch's y.txt has no parent directory.
func depthJumpChain(t *testing.T) []byte {
	t.Helper()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	file := func(name string, depth int) *Entry {
		return &Entry{Name: name, Depth: depth, Mode: 0o644, Timestamp: ts, Size: 1,
			C4ID: c4.Identify(strings.NewReader(name))}
	}
	base := NewManifest()
	base.AddEntry(file("a.txt", 0))
	patch := NewManifest()
	patch.AddEntry(file("z.txt", 0))
	patch.AddEntry(file("y.txt", 1))

	var b bytes.Buffer
	if err := NewEncoder(&b).Encode(base); err != nil {
		t.Fatal(err)
	}
	if err := NewEncoder(&b).EncodePatch(&PatchResult{OldID: base.ComputeC4ID(), Patch: patch}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// An entry whose depth skips past its parent directory is a named error
// wrapping ErrInvalidEntry, never a bare index-out-of-range panic (release
// 1.0.x: 59ce7cc, 6cfba2c). Decoder.Decode returns it; ApplyPatch and
// ResolvePatchChain panic with it, so a caller can tell it from a bug.
func TestDepthJumpIsANamedError(t *testing.T) {
	data := depthJumpChain(t)

	_, err := NewDecoder(bytes.NewReader(data)).Decode()
	if !errors.Is(err, ErrInvalidEntry) || !strings.Contains(err.Error(), `"y.txt" at depth 1 has no parent directory`) {
		t.Errorf("Decode: got %v, want an ErrInvalidEntry naming y.txt", err)
	}

	sections, err := DecodePatchChain(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DecodePatchChain: %v", err)
	}
	mustPanicInvalid := func(name string, fn func()) {
		t.Helper()
		defer func() {
			r := recover()
			e, ok := r.(error)
			if !ok || !errors.Is(e, ErrInvalidEntry) {
				t.Errorf("%s: panic value %v (%T), want an error wrapping ErrInvalidEntry", name, r, r)
			}
		}()
		fn()
	}
	mustPanicInvalid("ResolvePatchChain", func() { ResolvePatchChain(sections, 0) })
	mustPanicInvalid("ApplyPatch", func() {
		ApplyPatch(&Manifest{Version: "1.0", Entries: sections[0].Entries},
			&Manifest{Version: "1.0", Entries: sections[1].Entries})
	})
}
