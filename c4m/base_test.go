package c4m

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// blockSource is a content source holding raw c4m block text keyed by the
// C4 ID of those bytes — the shape a store presents to a chain resolver.
type blockSource struct {
	blocks map[c4.ID]string
	opens  int
}

func newBlockSource() *blockSource {
	return &blockSource{blocks: make(map[c4.ID]string)}
}

// put stores block text and returns its content ID.
func (s *blockSource) put(text string) c4.ID {
	id := c4.Identify(strings.NewReader(text))
	s.blocks[id] = text
	return id
}

// putRaw stores block text under an ID that is NOT its content ID, for
// building a cycle that no honest producer could create.
func (s *blockSource) putRaw(id c4.ID, text string) {
	s.blocks[id] = text
}

func (s *blockSource) Open(id c4.ID) (io.ReadCloser, error) {
	s.opens++
	text, ok := s.blocks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(strings.NewReader(text)), nil
}

func entryLine(name, content string) string {
	id := c4.Identify(strings.NewReader(content))
	return "-rw-r--r-- 2026-01-01T00:00:00Z " +
		itoa(len(content)) + " " + name + " " + id.String() + "\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// A block naming a prior block resolves to both blocks' content — the
// accreting-chain case the snapshot loop depends on (A1, A4).
func TestResolveBaseBackfillsPriorBlock(t *testing.T) {
	src := newBlockSource()

	block1 := entryLine("a.txt", "aaa")
	id1 := src.put(block1)

	block2 := id1.String() + "\n" + entryLine("b.txt", "bbb")

	m, err := DecodeResolved(strings.NewReader(block2), src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	names := entryNames(m)
	if !names["a.txt"] {
		t.Errorf("a.txt from the base is missing; got %v", names)
	}
	if !names["b.txt"] {
		t.Errorf("b.txt from this block is missing; got %v", names)
	}
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(m.Entries), names)
	}
}

// Bases follow transitively: the newest block's ID is enough to reconstruct
// every prior (A4).
func TestResolveBaseFollowsChainTransitively(t *testing.T) {
	src := newBlockSource()

	id1 := src.put(entryLine("a.txt", "aaa"))
	id2 := src.put(id1.String() + "\n" + entryLine("b.txt", "bbb"))
	block3 := id2.String() + "\n" + entryLine("c.txt", "ccc")

	m, err := DecodeResolved(strings.NewReader(block3), src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	names := entryNames(m)
	for _, want := range []string{"a.txt", "b.txt", "c.txt"} {
		if !names[want] {
			t.Errorf("%s missing after transitive backfill; got %v", want, names)
		}
	}
	if len(m.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(m.Entries), names)
	}
}

// A later block refining an earlier one wins — this is what makes the chain
// a progressive refinement rather than an append-only log.
func TestResolveBaseLaterBlockRefinesEarlier(t *testing.T) {
	src := newBlockSource()

	// Block 1 knows the name only: null mode, timestamp, size and ID.
	id1 := src.put("---------- - - a.txt -\n")
	// Block 2 fills in everything it learned.
	block2 := id1.String() + "\n" + entryLine("a.txt", "aaa")

	m, err := DecodeResolved(strings.NewReader(block2), src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
	e := m.Entries[0]
	if e.C4ID.IsNil() {
		t.Error("refinement did not carry the resolved C4 ID forward")
	}
	if e.Size != 3 {
		t.Errorf("expected refined size 3, got %d", e.Size)
	}
}

// The whole point of the fix: a base that cannot be read is an error, never
// a manifest missing its base presented as complete (A3, A5).
func TestResolveBaseMissingFromSourceIsAnError(t *testing.T) {
	src := newBlockSource()
	absent := c4.Identify(strings.NewReader("never stored"))
	block := absent.String() + "\n" + entryLine("b.txt", "bbb")

	m, err := DecodeResolved(strings.NewReader(block), src)
	if err == nil {
		t.Fatalf("expected an error, got manifest with %d entries", len(m.Entries))
	}
	if !errors.Is(err, ErrUnresolvedBase) {
		t.Errorf("expected ErrUnresolvedBase, got %v", err)
	}
	if !strings.Contains(err.Error(), absent.String()) {
		t.Errorf("error must name the missing ID %s, got %v", absent, err)
	}
}

// With no source at all, the same rule holds: fail, naming the ID (A3).
func TestResolveBaseWithoutSourceIsAnError(t *testing.T) {
	id := c4.Identify(strings.NewReader("some base"))
	block := id.String() + "\n" + entryLine("b.txt", "bbb")

	_, err := DecodeResolved(strings.NewReader(block), nil)
	if err == nil {
		t.Fatal("expected an error resolving a base with no source")
	}
	if !errors.Is(err, ErrUnresolvedBase) {
		t.Errorf("expected ErrUnresolvedBase, got %v", err)
	}
	if !strings.Contains(err.Error(), id.String()) {
		t.Errorf("error must name the unresolved ID %s, got %v", id, err)
	}
}

// A description with no base never consults the source and is returned
// unchanged (A2 — resolution is opt-in, so `id` can stay store-free).
func TestResolveBaseWithoutBaseNeverReadsSource(t *testing.T) {
	src := newBlockSource()
	m, err := DecodeResolved(strings.NewReader(entryLine("a.txt", "aaa")), src)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src.opens != 0 {
		t.Errorf("source was read %d times for a baseless description", src.opens)
	}
	if len(m.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.Entries))
	}
}

// A cycle terminates with an error instead of recursing without bound (A4).
func TestResolveBaseCycleIsDetected(t *testing.T) {
	src := newBlockSource()

	idA := c4.Identify(strings.NewReader("block-a"))
	idB := c4.Identify(strings.NewReader("block-b"))
	// A names B, B names A.
	src.putRaw(idA, idB.String()+"\n"+entryLine("a.txt", "aaa"))
	src.putRaw(idB, idA.String()+"\n"+entryLine("b.txt", "bbb"))

	block := idA.String() + "\n" + entryLine("c.txt", "ccc")

	_, err := DecodeResolved(strings.NewReader(block), src)
	if err == nil {
		t.Fatal("expected a cycle error")
	}
	if !errors.Is(err, ErrBaseCycle) {
		t.Errorf("expected ErrBaseCycle, got %v", err)
	}
}

// A block whose base is itself unreadable content fails loudly rather than
// yielding a partial result.
func TestResolveBaseUndecodableBaseIsAnError(t *testing.T) {
	src := newBlockSource()
	id := src.put("@c4 journal 1\n2026-01-01T00:00:00Z " +
		c4.Identify(strings.NewReader("x")).String() + "\n")
	block := id.String() + "\n" + entryLine("b.txt", "bbb")

	_, err := DecodeResolved(strings.NewReader(block), src)
	if err == nil {
		t.Fatal("expected an error decoding a non-c4m base")
	}
	if !errors.Is(err, ErrUnresolvedBase) {
		t.Errorf("expected ErrUnresolvedBase, got %v", err)
	}
}
