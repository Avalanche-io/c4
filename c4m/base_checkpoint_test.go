package c4m

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// A block that names an external base is resolved section by section: the
// prior is fetched, each section applies in order, and every checkpoint
// between sections is verified against the state accumulated so far
// (SPECIFICATION.md, "Subsequent Bare C4 ID"). Before, the decoder deferred
// every checkpoint of such a block and nothing verified them, and the
// block's later sections were applied against its first section alone.

func descriptionID(t *testing.T, text string) c4.ID {
	t.Helper()
	m, err := Unmarshal([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return m.ComputeC4ID()
}

func resolvedNames(t *testing.T, m *Manifest) string {
	t.Helper()
	var names []string
	for _, e := range m.Entries {
		names = append(names, e.Name)
	}
	return strings.Join(names, ",")
}

func TestResolveBaseVerifiesABlocksCheckpoints(t *testing.T) {
	src := newBlockSource()
	base := src.put(entryLine("a.txt", "a"))
	good := descriptionID(t, entryLine("a.txt", "a")+entryLine("x.txt", "x"))

	block := base.String() + "\n" + entryLine("x.txt", "x") + good.String() + "\n" + entryLine("y.txt", "y")
	m, err := DecodeResolved(strings.NewReader(block), src)
	if err != nil {
		t.Fatalf("a block whose checkpoint names the accumulated state must resolve: %v", err)
	}
	if got := resolvedNames(t, m); got != "a.txt,x.txt,y.txt" {
		t.Errorf("resolved to %s, want a.txt,x.txt,y.txt", got)
	}

	// The same through a stored block, reached from the newest ID alone.
	if m, err := ResolveBaseID(src.put(block), src); err != nil || resolvedNames(t, m) != "a.txt,x.txt,y.txt" {
		t.Errorf("ResolveBaseID: %v, %v", err, m)
	}

	wrong := c4.Identify(strings.NewReader("not the accumulated state"))
	bad := base.String() + "\n" + entryLine("x.txt", "x") + wrong.String() + "\n" + entryLine("y.txt", "y")
	if _, err := DecodeResolved(strings.NewReader(bad), src); !errors.Is(err, ErrPatchIDMismatch) {
		t.Errorf("a block with a wrong checkpoint must be refused with ErrPatchIDMismatch, got %v", err)
	}
	if _, err := ResolveBaseID(src.put(bad), src); !errors.Is(err, ErrPatchIDMismatch) {
		t.Errorf("a stored block with a wrong checkpoint must be refused with ErrPatchIDMismatch, got %v", err)
	}
}

func TestResolveBaseAppliesABlocksSectionsAgainstTheBase(t *testing.T) {
	// The base holds a.txt. Section 1 modifies it to a2; section 2 restates
	// a2 exactly, which removes it. Applied in order against the base, the
	// result is empty. Applying section 2 against section 1 alone (a2 is
	// there, so it is removed) and then folding the empty remainder onto
	// the base left the base's a.txt in place.
	src := newBlockSource()
	base := src.put(entryLine("a.txt", "a"))
	a2 := entryLine("a.txt", "a2")
	afterOne := descriptionID(t, a2)
	block := base.String() + "\n" + a2 + afterOne.String() + "\n" + a2

	var buf bytes.Buffer
	buf.WriteString(block)
	m, err := DecodeResolved(&buf, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := resolvedNames(t, m); got != "" {
		t.Errorf("resolved to %q, want nothing: section 2 removes the a.txt section 1 wrote", got)
	}
}
