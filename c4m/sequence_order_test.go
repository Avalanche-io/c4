package c4m

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// Folding must keep the tree in order: every directory before its
// children, siblings in their original order, the folded range in the
// place of its first frame. Through 1.0.19 the detector appended folded
// ranges (in map order) and then the directories, so a nested sequence
// preceded its parent and anything walking Entries with a directory
// stack (c4 -S storing) resolved its frames against the wrong directory.
func TestDetectSequencesKeepsTreeOrder(t *testing.T) {
	ts := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	file := func(name string, depth int, content string) *Entry {
		return &Entry{Name: name, Mode: 0644, Timestamp: ts, Size: int64(len(content)), C4ID: c4.Identify(strings.NewReader(content)), Depth: depth}
	}
	dir := func(name string, depth int) *Entry {
		return &Entry{Name: name, Mode: os.ModeDir | 0755, Timestamp: ts, Depth: depth}
	}

	m := NewManifest()
	m.AddEntry(file("shot.nk", 0, "nk"))
	m.AddEntry(dir("plates/", 0))
	for i := 1001; i <= 1004; i++ {
		m.AddEntry(file(fmt.Sprintf("plate.%d.exr", i), 1, fmt.Sprintf("p%d", i)))
	}
	m.AddEntry(dir("deep/", 0))
	m.AddEntry(dir("a/", 1))
	for i := 1; i <= 3; i++ {
		m.AddEntry(file(fmt.Sprintf("g.%03d.png", i), 2, fmt.Sprintf("g%d", i)))
	}
	m.AddEntry(file("notes.txt", 2, "n"))

	out := DetectSequences(m)

	var got []string
	for _, e := range out.Entries {
		got = append(got, fmt.Sprintf("%d:%s", e.Depth, e.Name))
	}
	want := []string{
		"0:shot.nk",
		"0:plates/",
		"1:plate.[1001-1004].exr",
		"0:deep/",
		"1:a/",
		"2:g.[001-003].png",
		"2:notes.txt",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("folded order\n got %v\nwant %v", got, want)
	}

	// Determinism: the same input folds to the same order every time.
	for i := 0; i < 20; i++ {
		again := DetectSequences(m)
		var g []string
		for _, e := range again.Entries {
			g = append(g, fmt.Sprintf("%d:%s", e.Depth, e.Name))
		}
		if strings.Join(g, " ") != strings.Join(want, " ") {
			t.Fatalf("run %d folded differently: %v", i, g)
		}
	}
}

// A range too short to fold, and a frame with a nil ID, keep their
// entries where they were.
func TestDetectSequencesLeavesUnfoldableInPlace(t *testing.T) {
	ts := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	m.AddEntry(&Entry{Name: "a.1.exr", Mode: 0644, Timestamp: ts, C4ID: c4.Identify(strings.NewReader("1")), Depth: 0})
	m.AddEntry(&Entry{Name: "a.2.exr", Mode: 0644, Timestamp: ts, C4ID: c4.Identify(strings.NewReader("2")), Depth: 0})
	m.AddEntry(&Entry{Name: "b.1.exr", Mode: 0644, Timestamp: ts, Depth: 0}) // nil ID
	m.AddEntry(&Entry{Name: "b.2.exr", Mode: 0644, Timestamp: ts, C4ID: c4.Identify(strings.NewReader("b2")), Depth: 0})
	m.AddEntry(&Entry{Name: "b.3.exr", Mode: 0644, Timestamp: ts, C4ID: c4.Identify(strings.NewReader("b3")), Depth: 0})
	m.AddEntry(&Entry{Name: "z/", Mode: os.ModeDir | 0755, Timestamp: ts, Depth: 0})

	out := DetectSequences(m)
	var got []string
	for _, e := range out.Entries {
		got = append(got, e.Name)
	}
	want := "a.1.exr a.2.exr b.1.exr b.2.exr b.3.exr z/"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %v, want %s", got, want)
	}
}
