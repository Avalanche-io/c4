package c4m

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// Six shapes of a null-ID directory whose children change, built through
// the public API. Through v1.0.21 PatchDiff wrote the unchanged directory
// line before its children and ApplyPatch read that line as removing the
// whole subtree: the applied result was empty, or only the root sibling.
// A directory restated with children under it is deepened, never
// removed. (Reported from a c4m consumer, 2026-11-03.)
func TestPatchShapesOfANullIDDirectory(t *testing.T) {
	file := func(name string, depth int) *Entry {
		return &Entry{Name: name, Depth: depth, Mode: 0o644, Timestamp: NullTimestamp(), Size: 1, C4ID: c4.Identify(strings.NewReader(name))}
	}
	dir := func(name string, depth int) *Entry {
		return &Entry{Name: name, Depth: depth, Mode: fs.ModeDir | 0o755, Timestamp: NullTimestamp(), Size: -1}
	}
	build := func(entries ...*Entry) *Manifest {
		m := NewManifest()
		for _, e := range entries {
			m.AddEntry(e)
		}
		return m
	}
	changed := file("a", 1)
	changed.C4ID = c4.Identify(strings.NewReader("changed"))

	shapes := []struct {
		label    string
		old, new *Manifest
	}{
		{"child replaced", build(dir("d/", 0), file("a", 1), file("same", 1)), build(dir("d/", 0), file("b", 1), file("same", 1))},
		{"child content changed", build(dir("d/", 0), file("a", 1)), build(dir("d/", 0), changed)},
		{"child removed", build(dir("d/", 0), file("a", 1), file("b", 1)), build(dir("d/", 0), file("b", 1))},
		{"child added", build(dir("d/", 0), file("a", 1)), build(dir("d/", 0), file("a", 1), file("b", 1))},
		{"grandchild replaced", build(dir("d/", 0), dir("e/", 1), file("a", 2)), build(dir("d/", 0), dir("e/", 1), file("b", 2))},
		{"with root sibling", build(dir("d/", 0), file("a", 1), file("z", 0)), build(dir("d/", 0), file("b", 1), file("z", 0))},
	}
	for _, s := range shapes {
		pr := PatchDiff(s.old, s.new)
		// in memory, and through the text a reader would hold
		text, err := Marshal(pr.Patch)
		if err != nil {
			t.Fatalf("%s: %v", s.label, err)
		}
		decoded, err := Unmarshal(text)
		if err != nil {
			t.Fatalf("%s: decode patch: %v", s.label, err)
		}
		for form, patch := range map[string]*Manifest{"memory": pr.Patch, "text": decoded} {
			applied := ApplyPatch(s.old, patch)
			if got, want := applied.ComputeC4ID(), s.new.ComputeC4ID(); got != want {
				t.Errorf("%s (%s): applied %s, want %s\npatch:\n%s", s.label, form, got, want, text)
			}
			if EntryPaths(applied.Entries)["d/"] == nil {
				t.Errorf("%s (%s): the directory was removed", s.label, form)
			}
		}
	}
}
