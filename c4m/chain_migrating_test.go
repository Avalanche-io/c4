package c4m

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// Consumers holding immutable objects written before 2026-11-08 (a
// changeset in a signed record) cannot rewrite them. DecodePatchChainMigrating
// takes the trailing ID off and hands it back for the caller to verify;
// the strict reader still refuses the same bytes.
func TestDecodePatchChainMigratingReturnsTrailingID(t *testing.T) {
	data, err := os.ReadFile("testdata/legacy/diff-chain-1.0.24.c4m")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodePatchChain(bytes.NewReader(data)); !errors.Is(err, ErrTrailingID) {
		t.Fatalf("strict reader must refuse the 1.0.24 chain, got %v", err)
	}
	sections, trailing, err := DecodePatchChainMigrating(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if trailing.IsNil() {
		t.Fatal("the trailing ID must be returned")
	}
	if len(sections) != 2 {
		t.Fatalf("expected base + one patch, got %d sections", len(sections))
	}
	// The trailing ID is the state the sections resolve to: the caller's
	// check, done here to pin that the returned ID is the right one.
	if got := ResolvePatchChain(sections, 0).ComputeC4ID(); got != trailing {
		t.Fatalf("sections resolve to %s, trailing ID says %s", got, trailing)
	}

	// A stream that ends on entries has no trailing ID either way.
	clean := []byte(strings.TrimSuffix(string(data), trailing.String()+"\n"))
	sections2, none, err := DecodePatchChainMigrating(bytes.NewReader(clean))
	if err != nil || !none.IsNil() || len(sections2) != 2 {
		t.Fatalf("clean chain: %d sections, trailing %v, err %v", len(sections2), none, err)
	}
}

// Two consecutive bare IDs are checkpoint repetition and must agree; the
// shape-level reader used to keep the second and drop the first without
// comparing them (found by c4d, 2026-11-09).
func TestDecodePatchChainConsecutiveCheckpointsMustAgree(t *testing.T) {
	base := NewManifest()
	base.AddEntry(&Entry{Name: "a.txt", Size: 5, C4ID: c4.Identify(strings.NewReader("hello"))})
	var buf bytes.Buffer
	NewEncoder(&buf).Encode(base)
	id := base.ComputeC4ID()
	other := c4.Identify(strings.NewReader("not the state"))
	patch := "-rw-r--r-- 2026-03-06T12:00:00Z 5 b.txt " + c4.Identify(strings.NewReader("world")).String() + "\n"

	good := buf.String() + id.String() + "\n" + id.String() + "\n" + patch
	if sections, err := DecodePatchChain(strings.NewReader(good)); err != nil || len(sections) != 2 {
		t.Fatalf("equal consecutive checkpoints must be accepted: %d sections, %v", len(sections), err)
	}
	bad := buf.String() + id.String() + "\n" + other.String() + "\n" + patch
	if _, err := DecodePatchChain(strings.NewReader(bad)); !errors.Is(err, ErrPatchIDMismatch) {
		t.Fatalf("disagreeing consecutive checkpoints must be ErrPatchIDMismatch, got %v", err)
	}
}
