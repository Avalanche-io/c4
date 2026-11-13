package c4m

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/store"
)

// The ID list of a folded sequence travels with the c4m that names it,
// inline after the entries when no store holds it (SPECIFICATION.md,
// "Inline Range Data"). Every operation that writes a c4m from c4m inputs
// must carry the list of every range its output contains, and no list for
// a range it does not (design/range-data-through-operations.md). Through
// 2026-11-13 merge, diff, patch, the chain readers and the subtree and
// filter operations dropped them.

// rangeDataFrames is the unfolded tree the range-data fixtures fold: a
// root run of four frames, a run of three inside sub/, and two plain files.
func rangeDataFrames() *Manifest {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	file := func(depth int, name, body string) {
		m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: int64(len(body)), Depth: depth,
			Name: name, C4ID: c4.Identify(strings.NewReader(body))})
	}
	for i := 1; i <= 4; i++ {
		file(0, fmt.Sprintf("plate.%d.exr", 1000+i), fmt.Sprintf("plate%d", i))
	}
	file(0, "readme.txt", "readme")
	m.AddEntry(&Entry{Mode: 0o755 | os.ModeDir, Timestamp: ts, Size: -1, Name: "sub/"})
	for i := 1; i <= 3; i++ {
		file(1, fmt.Sprintf("shot.%03d.exr", i), fmt.Sprintf("shot%d", i))
	}
	file(1, "notes.txt", "notes")
	m.SortEntries()
	Normalize(m)
	return m
}

// rangeDataFolded folds rangeDataFrames (minimum length 3) and re-derives
// the directory aggregates over the folded listing.
func rangeDataFolded() *Manifest {
	m := NewSequenceDetector(3).DetectSequences(rangeDataFrames())
	Normalize(m)
	return m
}

// rangeDataPlain is a description with no ranges: the two plain files.
func rangeDataPlain() *Manifest {
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	m := NewManifest()
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 6, Name: "readme.txt", C4ID: c4.Identify(strings.NewReader("readme"))})
	m.AddEntry(&Entry{Mode: 0o644, Timestamp: ts, Size: 5, Name: "other.txt", C4ID: c4.Identify(strings.NewReader("other"))})
	m.SortEntries()
	return m
}

// withoutRange returns m with the range named name removed, and its list.
func withoutRange(m *Manifest, name string) *Manifest {
	out := m.Copy()
	var kept []*Entry
	for _, e := range out.Entries {
		if e.Name == name && e.IsSequence {
			delete(out.RangeData, e.C4ID)
			continue
		}
		kept = append(kept, e)
	}
	out.Entries = kept
	Normalize(out)
	return out
}

func rangeIDs(entries []*Entry) map[c4.ID]string {
	ids := make(map[c4.ID]string)
	for _, e := range entries {
		if e.IsSequence && !e.C4ID.IsNil() {
			ids[e.C4ID] = e.Name
		}
	}
	return ids
}

// rangeDataVerdict names how out's lists stand against the ranges it
// holds: "keeps" when it carries exactly the list of every range in want,
// otherwise what is missing and what is extra.
func rangeDataVerdict(rd map[c4.ID]string, want map[c4.ID]string, have map[c4.ID]string) string {
	var missing, extra []string
	for id, name := range want {
		list, ok := rd[id]
		switch {
		case !ok:
			missing = append(missing, name)
		case c4.Identify(strings.NewReader(list)) != id:
			missing = append(missing, name+" (list does not hash to the range)")
		}
	}
	for id := range rd {
		if _, ok := want[id]; !ok {
			extra = append(extra, id.String()[:12])
		}
	}
	_ = have
	if len(missing) == 0 && len(extra) == 0 {
		return "keeps"
	}
	return fmt.Sprintf("missing %v extra %v", missing, extra)
}

// wantStateLists: a resolved state carries the list of every range it holds.
func wantStateLists(m *Manifest) map[c4.ID]string { return rangeIDs(m.Entries) }

func encodeRD(t *testing.T, m *Manifest) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := NewEncoder(&b).Encode(m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// patchText writes a patch as `c4 diff` does: the old state's ID, then the
// patch entries and their lists.
func patchText(t *testing.T, pr *PatchResult) []byte {
	t.Helper()
	return append([]byte(pr.OldID.String()+"\n"), encodeRD(t, pr.Patch)...)
}

// TestRangeDataThroughOperations runs every operation that writes a c4m
// from c4m inputs over folded sequences and checks the lists its output
// carries.
func TestRangeDataThroughOperations(t *testing.T) {
	folded := rangeDataFolded()
	if len(folded.RangeData) != 2 {
		t.Fatalf("fixture: folded carries %d lists, want 2", len(folded.RangeData))
	}
	plain := rangeDataPlain()
	noPlate := withoutRange(folded, "plate.[1001-1004].exr")

	type row struct {
		name string
		run  func(t *testing.T) (got map[c4.ID]string, want map[c4.ID]string)
	}
	state := func(m *Manifest) (map[c4.ID]string, map[c4.ID]string) {
		return m.RangeData, wantStateLists(m)
	}
	rows := []row{
		{"Merge(nil, folded, plain)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			m, _, err := Merge(nil, folded, plain)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"Merge(nil, folded, folded)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			m, _, err := Merge(nil, folded, folded)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"Merge(folded, noPlate, folded): range removed", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			m, _, err := Merge(folded, noPlate, folded)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"PatchDiff(plain, folded): ranges added", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			pr := PatchDiff(plain, folded)
			return pr.Patch.RangeData, rangeIDs(folded.Entries)
		}},
		{"PatchDiff(folded, noPlate): range removed", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			pr := PatchDiff(folded, noPlate)
			return pr.Patch.RangeData, map[c4.ID]string{}
		}},
		{"ApplyPatch(plain, diff)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			patch := PatchDiff(plain, folded).Patch
			patch.RangeData = folded.Copy().RangeData
			return state(ApplyPatch(plain, patch))
		}},
		{"ApplyPatch(folded, removal of a range)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			return state(ApplyPatch(folded, PatchDiff(folded, noPlate).Patch))
		}},
		{"ApplyPatchFrom(folded, folded, removal of a range)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			m, err := ApplyPatchFrom(folded, folded, PatchDiff(folded, noPlate).Patch)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"Decode(plain + diff chain)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			chain := append(encodeRD(t, plain), patchText(t, rangeDataDiff(plain, folded))...)
			m, err := Unmarshal(chain)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"ResolvePatchChain(plain + diff chain)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			chain := append(encodeRD(t, plain), patchText(t, rangeDataDiff(plain, folded))...)
			secs, err := DecodePatchChain(bytes.NewReader(chain))
			if err != nil {
				t.Fatal(err)
			}
			return state(ResolvePatchChain(secs, 0))
		}},
		{"ResolvePatchChain(folded, one section)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			secs, err := DecodePatchChain(bytes.NewReader(encodeRD(t, folded)))
			if err != nil {
				t.Fatal(err)
			}
			return state(ResolvePatchChain(secs, 0))
		}},
		{"FoldSection(folded, removal of a range)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			m, err := FoldSection(folded, PatchDiff(folded, noPlate).Patch.Entries, nil)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"ResolveBase(diff on a stored plain base)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			s := store.NewRAM()
			baseID, err := s.Put(bytes.NewReader(encodeRD(t, plain)))
			if err != nil {
				t.Fatal(err)
			}
			pr := rangeDataDiff(plain, folded)
			block := append([]byte(baseID.String()+"\n"), encodeRD(t, pr.Patch)...)
			m, err := Unmarshal(block)
			if err != nil {
				t.Fatal(err)
			}
			r, err := ResolveBase(m, s)
			if err != nil {
				t.Fatal(err)
			}
			return state(r)
		}},
		{"DecodeResolved(range removed on a stored folded base)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			s := store.NewRAM()
			baseID, err := s.Put(bytes.NewReader(encodeRD(t, folded)))
			if err != nil {
				t.Fatal(err)
			}
			block := append([]byte(baseID.String()+"\n"), encodeRD(t, PatchDiff(folded, noPlate).Patch)...)
			r, err := DecodeResolved(bytes.NewReader(block), s)
			if err != nil {
				t.Fatal(err)
			}
			return state(r)
		}},
		{"FilterByPath(\"*.exr\")", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			return state(folded.FilterByPath("*.exr"))
		}},
		{"FilterByPath(\"*.txt\")", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			return state(folded.FilterByPath("*.txt"))
		}},
		{"FilterByPrefix(\"plate\")", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			return state(folded.FilterByPrefix("plate"))
		}},
		{"ExtractSubtree(\"sub/\")", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			m, err := folded.ExtractSubtree("sub/")
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"InjectSubtree(\"sub/\", folded sub)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			sub, err := folded.ExtractSubtree("sub/")
			if err != nil {
				t.Fatal(err)
			}
			sub.RangeData = folded.Copy().RangeData // whatever extract kept, the source's lists
			for id := range sub.RangeData {
				if _, ok := rangeIDs(sub.Entries)[id]; !ok {
					delete(sub.RangeData, id)
				}
			}
			host := withoutRange(noPlate, "shot.[001-003].exr")
			m, err := host.InjectSubtree("sub/", sub)
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"WriteChainBody(a list for an absent range)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			stale := noPlate.Copy()
			stale.RangeData = folded.Copy().RangeData // the plate list too
			var b bytes.Buffer
			if err := WriteChainBody(&b, stale); err != nil {
				t.Fatal(err)
			}
			m, err := Unmarshal(b.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
		{"WriteChainBody(folded)", func(t *testing.T) (map[c4.ID]string, map[c4.ID]string) {
			var b bytes.Buffer
			if err := WriteChainBody(&b, folded); err != nil {
				t.Fatal(err)
			}
			m, err := Unmarshal(b.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			return state(m)
		}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			got, want := r.run(t)
			if v := rangeDataVerdict(got, want, nil); v != "keeps" {
				t.Errorf("%s: %s", r.name, v)
			}
		})
	}
}

// rangeDataDiff is PatchDiff with the lists its added ranges need, as a
// conforming diff writes them, whatever PatchDiff itself carries.
func rangeDataDiff(old, new *Manifest) *PatchResult {
	pr := PatchDiff(old, new)
	pr.Patch.RangeData = make(map[c4.ID]string)
	for id := range rangeIDs(pr.Patch.Entries) {
		if list, ok := new.RangeData[id]; ok {
			pr.Patch.RangeData[id] = list
		}
	}
	return pr
}
