package c4m

import (
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

func streamTestManifest(t *testing.T) *Manifest {
	t.Helper()
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fileID := func(s string) c4.ID { return c4.Identify(strings.NewReader(s)) }

	m := NewManifest()
	m.AddEntry(&Entry{Name: "a.txt", Mode: 0644, Timestamp: ts, Size: 5, C4ID: fileID("alpha"), Depth: 0})
	m.AddEntry(&Entry{Name: "empty/", Mode: 0755 | 0x80000000, Timestamp: ts, Size: 0,
		C4ID: c4.Identify(strings.NewReader("")), Depth: 0})
	m.AddEntry(&Entry{Name: "sub/", Mode: 0755 | 0x80000000, Timestamp: ts, Size: 210, Depth: 0})
	m.AddEntry(&Entry{Name: "b.txt", Mode: 0644, Timestamp: ts, Size: 5, C4ID: fileID("bravo"), Depth: 1})
	m.AddEntry(&Entry{Name: "deep/", Mode: 0755 | 0x80000000, Timestamp: ts, Size: 105, Depth: 1})
	m.AddEntry(&Entry{Name: "c.md", Mode: 0644, Timestamp: ts, Size: 7, C4ID: fileID("charlie"), Depth: 2})

	// Give the directories real listing IDs the way a scan would:
	// bottom-up over their direct children's canonical lines.
	deepID := c4.Identify(strings.NewReader(m.Entries[5].Canonical() + "\n"))
	m.Entries[4].C4ID = deepID
	subID := c4.Identify(strings.NewReader(m.Entries[3].Canonical() + "\n" + m.Entries[4].Canonical() + "\n"))
	m.Entries[2].C4ID = subID
	return m
}

// TestChainStreamRoundTrip pins the streaming shape: the emitted chain
// resolves — through the verifying decoder — to exactly the source
// manifest, and its final line is the source's root ID.
func TestChainStreamRoundTrip(t *testing.T) {
	m := streamTestManifest(t)

	text, err := ChainStream(m)
	if err != nil {
		t.Fatal(err)
	}

	// The final line of the stream is always the root ID.
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	last := lines[len(lines)-1]
	if last != m.ComputeC4ID().String() {
		t.Fatalf("final line %q != root ID %s", last, m.ComputeC4ID())
	}

	// The stream decodes (checkpoint and validator verified) back to
	// the source manifest exactly.
	got, err := Unmarshal([]byte(text))
	if err != nil {
		t.Fatalf("stream must decode cleanly: %v", err)
	}
	if got.ComputeC4ID() != m.ComputeC4ID() {
		t.Fatalf("round-trip root ID mismatch:\n%s", text)
	}
	if got.Canonical() != m.Canonical() {
		t.Fatalf("round-trip canonical text mismatch:\n--- emitted stream\n%s--- resolved\n%s--- source\n%s",
			text, got.Canonical(), m.Canonical())
	}

	// Base directory lines stream with null ID and size; empty
	// directories emit complete.
	if !strings.Contains(text, "- sub/ -") && !strings.Contains(text, " - sub/ -\n") {
		// sub/'s base line must carry null size (-) and null ID (-).
		found := false
		for _, l := range lines {
			f := strings.Fields(l)
			if len(f) >= 2 && f[len(f)-2] == "sub/" && f[len(f)-1] == "-" {
				found = true
			}
		}
		if !found {
			t.Fatalf("sub/'s base line should carry null ID:\n%s", text)
		}
	}
}

// TestChainStreamFlat pins the no-refinement case: a manifest with no
// aggregate-bearing directories streams as base + validator only.
func TestChainStreamFlat(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	m := NewManifest()
	m.AddEntry(&Entry{Name: "only.txt", Mode: 0644, Timestamp: ts, Size: 4,
		C4ID: c4.Identify(strings.NewReader("data")), Depth: 0})

	text, err := ChainStream(m)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("flat stream should be entry + validator, got %d lines:\n%s", len(lines), text)
	}
	if lines[1] != m.ComputeC4ID().String() {
		t.Fatal("flat stream must close with the root ID")
	}
	if _, err := Unmarshal([]byte(text)); err != nil {
		t.Fatalf("flat stream must decode: %v", err)
	}
}
