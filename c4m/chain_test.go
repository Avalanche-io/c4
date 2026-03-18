package c4m

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestDecodePatchChainBase(t *testing.T) {
	// Simple manifest — no patches. Use a real C4 ID.
	id := c4.Identify(strings.NewReader("hello"))
	input := "-rw-r--r-- 2026-01-01T00:00:00Z 5 hello.txt " + id.String() + "\n"
	sections, err := DecodePatchChain(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(sections) != 1 {
		t.Fatalf("expected 1 section, got %d", len(sections))
	}
	if len(sections[0].Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(sections[0].Entries))
	}
	if sections[0].Entries[0].Name != "hello.txt" {
		t.Fatalf("expected hello.txt, got %s", sections[0].Entries[0].Name)
	}
}

func TestDecodePatchChainWithPatch(t *testing.T) {
	// Create a base manifest, compute its ID, then append a patch.
	base := NewManifest()
	base.AddEntry(&Entry{
		Name: "a.txt",
		Size: 5,
		C4ID: c4.Identify(strings.NewReader("hello")),
	})

	// Encode base to get canonical form.
	var baseBuf bytes.Buffer
	NewEncoder(&baseBuf).Encode(base)
	baseID := base.ComputeC4ID()

	// Create a patch section.
	patch := NewManifest()
	patch.AddEntry(&Entry{
		Name: "b.txt",
		Size: 5,
		C4ID: c4.Identify(strings.NewReader("world")),
	})

	// Build the chained file: base entries, base ID, patch entries.
	var chainBuf bytes.Buffer
	chainBuf.Write(baseBuf.Bytes())
	chainBuf.WriteString(baseID.String() + "\n")
	NewEncoder(&chainBuf).Encode(patch)

	sections, err := DecodePatchChain(bytes.NewReader(chainBuf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}

	if len(sections[0].Entries) != 1 {
		t.Fatalf("base section: expected 1 entry, got %d", len(sections[0].Entries))
	}
	if sections[0].Entries[0].Name != "a.txt" {
		t.Fatalf("base entry: expected a.txt, got %s", sections[0].Entries[0].Name)
	}

	if len(sections[1].Entries) != 1 {
		t.Fatalf("patch section: expected 1 entry, got %d", len(sections[1].Entries))
	}
	if sections[1].Entries[0].Name != "b.txt" {
		t.Fatalf("patch entry: expected b.txt, got %s", sections[1].Entries[0].Name)
	}
	if sections[1].BaseID != baseID {
		t.Fatalf("patch base ID mismatch: got %s, want %s", sections[1].BaseID, baseID)
	}
}

func TestDecodePatchChainTrailingID(t *testing.T) {
	// A trailing bare ID (closing boundary) should not create an empty section.
	base := NewManifest()
	base.AddEntry(&Entry{
		Name: "a.txt",
		Size: 5,
		C4ID: c4.Identify(strings.NewReader("hello")),
	})

	var buf bytes.Buffer
	NewEncoder(&buf).Encode(base)
	baseID := base.ComputeC4ID()
	buf.WriteString(baseID.String() + "\n")

	sections, err := DecodePatchChain(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	if len(sections) != 1 {
		t.Fatalf("expected 1 section (trailing ID should not create empty section), got %d", len(sections))
	}
}

func TestResolvePatchChain(t *testing.T) {
	aID := c4.Identify(strings.NewReader("hello"))
	bID := c4.Identify(strings.NewReader("world"))

	sections := []*PatchSection{
		{Entries: []*Entry{{Name: "a.txt", Size: 5, C4ID: aID}}},
		{Entries: []*Entry{{Name: "b.txt", Size: 5, C4ID: bID}}},
	}

	// Full resolution.
	m := ResolvePatchChain(sections, 0)
	if len(m.Entries) != 2 {
		t.Fatalf("expected 2 entries after resolution, got %d", len(m.Entries))
	}

	// Partial resolution (stop at 1).
	m1 := ResolvePatchChain(sections, 1)
	if len(m1.Entries) != 1 {
		t.Fatalf("expected 1 entry at patch 1, got %d", len(m1.Entries))
	}
	if m1.Entries[0].Name != "a.txt" {
		t.Fatalf("expected a.txt at patch 1, got %s", m1.Entries[0].Name)
	}
}

func TestResolvePatchChainEmpty(t *testing.T) {
	m := ResolvePatchChain(nil, 0)
	if len(m.Entries) != 0 {
		t.Fatalf("expected 0 entries for empty chain, got %d", len(m.Entries))
	}
}
