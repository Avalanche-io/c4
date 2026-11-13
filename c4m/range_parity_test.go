package c4m

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// A frame ID list behaves like a subdirectory's listing (ruled 2026-11-13
// by the maintainer: "A frame id list is a part of the c4m format, and is
// the same thing as a subdirectory being referenced by it's ID on a
// parents directory line. Everything that you would expect of the
// creation and retention of that subdirectory c4m file you would also
// expect of the frame id list."). Its format stays the spec's bare
// concatenation of member C4 IDs: "a linked parseable text file that has
// formatting specific to frame ranges, which doesn't need the per file
// metadata". Only the handling is shared: each operation below carries a
// range's list exactly where it carries a subdirectory's entries.

// parityTrees returns two descriptions that differ in one place: dir holds
// f/ with three files listed under it, rng holds the same three files'
// contents as the folded range f.[001-003].exr with its ID list inline.
// With prefix "p/" both are wrapped in a directory p/.
func parityTrees(wrap bool) (dir, rng *Manifest) {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	d := 0
	build := func(folded bool) *Manifest {
		m := NewManifest()
		if wrap {
			m.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Timestamp: ts, Size: -1, Name: "p/"})
			d = 1
		}
		file := func(depth int, name, body string) {
			m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(body)), Depth: depth,
				Name: name, C4ID: c4.Identify(strings.NewReader(body))})
		}
		file(d, "readme.txt", "readme")
		if folded {
			for i := 1; i <= 3; i++ {
				file(d, fmt.Sprintf("f.%03d.exr", i), fmt.Sprintf("frame %d", i))
			}
		} else {
			m.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Timestamp: ts, Size: -1, Depth: d, Name: "f/"})
			for i := 1; i <= 3; i++ {
				file(d+1, fmt.Sprintf("f%d.txt", i), fmt.Sprintf("frame %d", i))
			}
		}
		m.SortEntries()
		if folded {
			m = NewSequenceDetector(3).DetectSequences(m)
		}
		Normalize(m)
		return m
	}
	return build(false), build(true)
}

// parityOther is a description the trees are merged with and diffed from.
func parityOther(wrap bool) *Manifest {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	d := 0
	if wrap {
		m.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Timestamp: ts, Size: -1, Name: "p/"})
		d = 1
	}
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 5, Depth: d, Name: "other.txt", C4ID: c4.Identify(strings.NewReader("other"))})
	Normalize(m)
	return m
}

// keptByID reports whether out still holds what in links by ID at name
// (a path such as "f/" or "f.[001-003].exr"): the line with the same C4 ID,
// and its content, the subdirectory's entries listed under it or the
// range's ID list.
func keptByID(in, out *Manifest, name string) bool {
	inPaths, outPaths := EntryPaths(in.Entries), EntryPaths(out.Entries)
	want := inPaths[name]
	got := outPaths[name]
	if want == nil || got == nil || got.C4ID != want.C4ID {
		return false
	}
	if want.IsDir() {
		for p, e := range inPaths {
			if strings.HasPrefix(p, name) && p != name {
				if o := outPaths[p]; o == nil || o.C4ID != e.C4ID {
					return false
				}
			}
		}
		return true
	}
	return out.RangeData[want.C4ID] == in.RangeData[want.C4ID] && in.RangeData[want.C4ID] != ""
}

// TestRangeListParityWithSubdirectories runs a subdirectory and a folded
// range through each operation side by side; each must keep its
// content-by-ID the same way.
func TestRangeListParityWithSubdirectories(t *testing.T) {
	type op struct {
		name string
		wrap bool
		run  func(t *testing.T, tree, other *Manifest) *Manifest
	}
	encode := func(t *testing.T, m *Manifest) []byte {
		t.Helper()
		var b bytes.Buffer
		if err := NewEncoder(&b).Encode(m); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	chainText := func(t *testing.T, other, tree *Manifest) []byte {
		t.Helper()
		pr := PatchDiff(other, tree)
		return append(append(encode(t, other), []byte(pr.OldID.String()+"\n")...), encode(t, pr.Patch)...)
	}
	ops := []op{
		{"Merge(tree, other)", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			m, _, err := Merge(nil, tree, other)
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		{"Merge(other, tree)", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			m, _, err := Merge(nil, other, tree)
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		{"PatchDiff(other, tree): the patch", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			return PatchDiff(other, tree).Patch
		}},
		{"ApplyPatch(other, PatchDiff(other, tree))", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			return ApplyPatch(other, PatchDiff(other, tree).Patch)
		}},
		{"ApplyPatchFrom(other, other, PatchDiff(other, tree))", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			m, err := ApplyPatchFrom(other, other, PatchDiff(other, tree).Patch)
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		{"Decode(other + diff chain)", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			m, err := Unmarshal(chainText(t, other, tree))
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		{"ResolvePatchChain(other + diff chain)", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			secs, err := DecodePatchChain(bytes.NewReader(chainText(t, other, tree)))
			if err != nil {
				t.Fatal(err)
			}
			return ResolvePatchChain(secs, 0)
		}},
		{"FoldSection(other, diff)", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			// The section's lists join the state's first, as `c4 patch` does.
			pr := PatchDiff(other, tree)
			state := other.Copy()
			state.RangeData = pr.Patch.RangeData
			m, err := FoldSection(state, pr.Patch.Entries, nil)
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		// A filter selects by name; every name under f/ starts with f too,
		// so both trees are selected whole. (A kept directory line whose
		// children are filtered out drops to one-level form, its listing
		// still linked by ID; a range's list has no name of its own, so a
		// kept range line keeps it.)
		{"FilterByPrefix(\"f\")", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			return tree.FilterByPrefix("f")
		}},
		{"ExtractSubtree(\"p/\")", true, func(t *testing.T, tree, other *Manifest) *Manifest {
			m, err := tree.ExtractSubtree("p/")
			if err != nil {
				t.Fatal(err)
			}
			// Extract leaves p/'s contents at the root; put them back
			// under p/ so the same paths compare.
			host := NewManifest()
			host.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Name: "p/", Size: -1})
			out, err := host.InjectSubtree("p/", m)
			if err != nil {
				t.Fatal(err)
			}
			return out
		}},
		{"InjectSubtree(\"p/\", tree's p/)", true, func(t *testing.T, tree, other *Manifest) *Manifest {
			sub, err := tree.ExtractSubtree("p/")
			if err != nil {
				t.Fatal(err)
			}
			m, err := other.InjectSubtree("p/", sub)
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		{"WriteChainBody(tree), decoded", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			var b bytes.Buffer
			if err := WriteChainBody(&b, tree); err != nil {
				t.Fatal(err)
			}
			m, err := Unmarshal(b.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
		{"Encode(tree), decoded", false, func(t *testing.T, tree, other *Manifest) *Manifest {
			m, err := Unmarshal(encode(t, tree))
			if err != nil {
				t.Fatal(err)
			}
			return m
		}},
	}
	var table strings.Builder
	table.WriteString("| Operation | subdirectory f/ | range f.[001-003].exr |\n")
	for _, o := range ops {
		dirTree, rngTree := parityTrees(o.wrap)
		other := parityOther(o.wrap)
		prefix := ""
		if o.wrap {
			prefix = "p/"
		}
		dirKept := keptByID(dirTree, o.run(t, dirTree, other), prefix+"f/")
		rngKept := keptByID(rngTree, o.run(t, rngTree, other), prefix+"f.[001-003].exr")
		word := map[bool]string{true: "kept", false: "lost"}
		fmt.Fprintf(&table, "| %s | %s | %s |\n", o.name, word[dirKept], word[rngKept])
		if !dirKept || !rngKept {
			t.Errorf("%s: subdirectory %s, range %s; both must be kept", o.name, word[dirKept], word[rngKept])
		}
	}
	t.Log("\n" + table.String())
}
