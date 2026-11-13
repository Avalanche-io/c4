package c4m

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// A range is expanded only from its ID list, found inline or in the
// expander's source. Without it the range stays one folded entry: no frame
// is given a null C4 ID or the range's own C4 ID, which names the list and
// no frame. Ruled 2026-11-13 by the maintainer: "You can't expand a
// sequence if you don't have a description of it's contents, this is the
// same for any content. However, it is an error to drop the frame IDs in
// any context." Through 2026-11-13 every frame took the range's own ID.
func TestSequenceExpansionNeedsTheIDList(t *testing.T) {
	folded := rangeDataFolded()
	bare := folded.Copy()
	bare.RangeData = nil
	ranges := rangeIDs(folded.Entries)

	// framesOf returns the frames an expansion holds, failing on any that
	// carries a null ID or a range's own ID.
	framesOf := func(what string, m *Manifest) int {
		t.Helper()
		n := 0
		for _, e := range m.Entries {
			if e.IsSequence || !IsSequence(e.Name) && !isFrameName(e.Name) {
				continue
			}
			n++
			if e.C4ID.IsNil() {
				t.Errorf("%s: frame %s has a null C4 ID", what, e.Name)
			}
			if _, ok := ranges[e.C4ID]; ok {
				t.Errorf("%s: frame %s carries its range's own C4 ID", what, e.Name)
			}
		}
		return n
	}
	expand := func(m *Manifest, src store.Source) *Manifest {
		t.Helper()
		ex := NewSequenceExpander(SequenceEmbedded)
		if src != nil {
			ex.SetSource(src)
		}
		out, _, err := ex.ExpandManifest(m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	// Inline lists: both ranges expand, 7 frames, each its own ID, and the
	// output carries the lists of the ranges it keeps.
	out := expand(folded, nil)
	if n := framesOf("lists inline", out); n != 7 {
		t.Errorf("lists inline: %d frames, want 7", n)
	}
	if len(out.RangeData) != 2 {
		t.Errorf("lists inline: the expansion carries %d lists, want 2", len(out.RangeData))
	}

	// No list anywhere: nothing expands; the output is the input's entries.
	out = expand(bare, nil)
	if n := framesOf("no lists", out); n != 0 {
		t.Errorf("no lists: %d frames, want none", n)
	}
	if len(out.Entries) != len(bare.Entries) || out.ComputeC4ID() != bare.ComputeC4ID() {
		t.Errorf("no lists: the expansion is not the folded description unchanged")
	}

	// The lists in a source: found there, verified, carried.
	src := listStore(t, folded.RangeData)
	out = expand(bare, src)
	if n := framesOf("lists in the source", out); n != 7 {
		t.Errorf("lists in the source: %d frames, want 7", n)
	}
	if len(out.RangeData) != 2 {
		t.Errorf("lists in the source: the expansion carries %d lists, want 2", len(out.RangeData))
	}

	// A source whose bytes under a range's ID are not that list: not
	// available, so that range stays folded.
	wrong := bytesSource{}
	for id := range ranges {
		wrong[id] = []byte("not the list")
	}
	if n := framesOf("wrong bytes in the source", expand(bare, wrong)); n != 0 {
		t.Errorf("wrong bytes in the source: %d frames, want none", n)
	}

	// Asking for one range: frames when its list is found, and otherwise
	// the result any unavailable content gives, os.ErrNotExist naming the
	// list's C4 ID.
	for _, e := range bare.Entries {
		if !e.IsSequence {
			continue
		}
		frames, err := NewSequenceExpander(SequenceEmbedded).ExpandRange(bare, e)
		if !errors.Is(err, os.ErrNotExist) || len(frames) != 0 || !bytes.Contains([]byte(err.Error()), []byte(e.C4ID.String())) {
			t.Errorf("ExpandRange %s with no list: %d frames, err %v; want os.ErrNotExist naming %s", e.Name, len(frames), err, e.C4ID)
		}
		frames, err = NewSequenceExpander(SequenceEmbedded).SetSource(src).ExpandRange(bare, e)
		if err != nil || len(frames) == 0 {
			t.Errorf("ExpandRange %s from the source: %d frames, err %v", e.Name, len(frames), err)
		}
	}

	// A range with no C4 ID names no list: it stays folded.
	noID := sequenceWithList("x.[1-3].exr", 3)
	noID.Entries[0].C4ID = c4.ID{}
	noID.RangeData = nil
	if out := expand(noID, nil); len(out.Entries) != 1 {
		t.Errorf("a range with no C4 ID expanded to %d entries, want it alone", len(out.Entries))
	}
}

// isFrameName reports whether name is a numbered frame: the rangeDataFrames
// fixture's frames end in a digit run before .exr.
func isFrameName(name string) bool {
	n := len(name)
	return n > 5 && name[n-4:] == ".exr" && name[n-5] >= '0' && name[n-5] <= '9'
}

// bytesSource serves fixed bytes under each ID, whatever they hash to.
type bytesSource map[c4.ID][]byte

func (s bytesSource) Open(id c4.ID) (io.ReadCloser, error) {
	b, ok := s[id]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// listStore is a store holding the given lists.
func listStore(t *testing.T, lists map[c4.ID]string) *store.RAM {
	t.Helper()
	s := store.NewRAM()
	for id, list := range lists {
		got, err := s.Put(strings.NewReader(list))
		if err != nil || got != id {
			t.Fatalf("storing list %s: %v (got %s)", id, err, got)
		}
	}
	return s
}
