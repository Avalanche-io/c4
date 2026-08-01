package c4m

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// TestDeepenVectorConformance runs the shared cross-implementation fixture
// for the removal-rule narrowing. Both files restate the same directory line
// byte-identically; they differ only in whether a child follows it, and must
// therefore resolve to different manifests.
func TestDeepenVectorConformance(t *testing.T) {
	dir := "testdata/deepen-vector/"

	cases := []struct {
		file   string
		idFile string
		want   []string
	}{
		{"deepen.c4m", "deepen-root-id.txt", []string{"a.txt", "docs/", "readme.md", "guide.md"}},
		{"remove.c4m", "remove-root-id.txt", []string{"a.txt"}},
	}

	ids := make(map[string]string)
	for _, tc := range cases {
		data, err := os.ReadFile(dir + tc.file)
		if err != nil {
			t.Fatal(err)
		}
		m, err := Unmarshal(data)
		if err != nil {
			t.Fatalf("%s must be accepted: %v", tc.file, err)
		}

		// Conformance is on the resolved entry set, not only on the error:
		// a non-verifying chain reader carrying the old rule returns the
		// wrong manifest without ever reporting a mismatch.
		got := entryNames(m)
		if len(got) != len(tc.want) {
			t.Errorf("%s resolved to %d entries, want %d: %v",
				tc.file, len(got), len(tc.want), got)
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %s missing from resolved manifest: %v", tc.file, w, got)
			}
		}

		wantID, err := os.ReadFile(dir + tc.idFile)
		if err != nil {
			t.Fatal(err)
		}
		gotID := m.ComputeC4ID().String()
		if gotID != strings.TrimSpace(string(wantID)) {
			t.Errorf("%s resolved to %s, want %s", tc.file, gotID, strings.TrimSpace(string(wantID)))
		}
		ids[tc.file] = gotID
	}

	// The contract in one line: the same restated directory means opposite
	// things depending on whether children follow it.
	if ids["deepen.c4m"] == ids["remove.c4m"] {
		t.Errorf("deepen and remove resolved identically (%s) — the narrowing is not in effect",
			ids["deepen.c4m"])
	}
}

// resolveByRepeatedApplyPatch is the pre-2026-07-31 fold: one ApplyPatch per
// section, each rebuilding the whole base tree and re-sorting the result.
// Kept as the differential oracle for ResolvePatchChain — the fix must not
// change what a chain resolves to, only what it costs.
func resolveByRepeatedApplyPatch(sections []*PatchSection, stopAt int) *Manifest {
	if len(sections) == 0 {
		return NewManifest()
	}
	limit := len(sections)
	if stopAt > 0 && stopAt < limit {
		limit = stopAt
	}
	m := &Manifest{Version: "1.0", Entries: sections[0].Entries}
	for i := 1; i < limit; i++ {
		patch := &Manifest{Version: "1.0", Entries: sections[i].Entries}
		m = ApplyPatch(m, patch)
	}
	return m
}

// TestResolvePatchChainMatchesRepeatedApplyPatch is the C2 guard: for every
// chain shape below, the single-tree resolver and the repeated-ApplyPatch
// fold must produce byte-identical canonical text, at every stopAt.
func TestResolvePatchChainMatchesRepeatedApplyPatch(t *testing.T) {
	for _, tc := range foldEquivalenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			for stopAt := 0; stopAt <= len(tc.sections); stopAt++ {
				got := ResolvePatchChain(tc.sections, stopAt)
				want := resolveByRepeatedApplyPatch(tc.sections, stopAt)

				gotText := canonicalOf(got)
				wantText := canonicalOf(want)
				if gotText != wantText {
					t.Errorf("stopAt=%d canonical text differs\n--- got ---\n%s\n--- want ---\n%s",
						stopAt, gotText, wantText)
				}
				if got.ComputeC4ID() != want.ComputeC4ID() {
					t.Errorf("stopAt=%d resolved ID differs: %s vs %s",
						stopAt, got.ComputeC4ID(), want.ComputeC4ID())
				}
			}
		})
	}
}

// canonicalOf renders a manifest's canonical text without mutating it.
func canonicalOf(m *Manifest) string {
	c := m.Copy()
	c.Canonicalize()
	c.SortEntries()
	return c.Canonical()
}

type foldCase struct {
	name     string
	sections []*PatchSection
}

func foldEquivalenceCases() []foldCase {
	dirID := c4.Identify(strings.NewReader("dir"))
	otherID := c4.Identify(strings.NewReader("other"))

	file := func(name, content string, depth int) *Entry {
		return &Entry{
			Name:      name,
			Mode:      0644,
			Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
			Size:      int64(len(content)),
			C4ID:      c4.Identify(strings.NewReader(content)),
			Depth:     depth,
		}
	}
	dir := func(name string, id c4.ID, depth int) *Entry {
		return &Entry{
			Name:      name,
			Mode:      os.ModeDir | 0755,
			Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
			Size:      -1,
			C4ID:      id,
			Depth:     depth,
		}
	}

	base := []*Entry{
		file("a.txt", "aaa", 0),
		dir("docs/", dirID, 0),
		file("readme.md", "readme", 1),
	}

	return []foldCase{
		{
			name:     "base_only",
			sections: []*PatchSection{{Entries: base}},
		},
		{
			name: "clobber_leaf",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{file("a.txt", "changed", 0)}},
			},
		},
		{
			name: "add_leaf",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{file("b.txt", "bbb", 0)}},
			},
		},
		{
			name: "remove_leaf",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{file("a.txt", "aaa", 0)}},
			},
		},
		{
			name: "deepen_identical_dir",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{
					dir("docs/", dirID, 0),
					file("guide.md", "guide", 1),
				}},
			},
		},
		{
			name: "graft_new_subtree",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{
					dir("src/", otherID, 0),
					file("main.go", "package main", 1),
					dir("internal/", dirID, 1),
					file("util.go", "package internal", 2),
				}},
			},
		},
		{
			name:     "many_small_sections",
			sections: manySmallSections(base, 25),
		},
		{
			name: "full_restatement_layer",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{
					file("a.txt", "aaa-v2", 0),
					dir("docs/", otherID, 0),
					file("readme.md", "readme-v2", 1),
				}},
			},
		},
		{
			name: "interleaved_add_clobber_remove",
			sections: []*PatchSection{
				{Entries: base},
				{Entries: []*Entry{file("b.txt", "bbb", 0)}},
				{Entries: []*Entry{file("a.txt", "changed", 0)}},
				{Entries: []*Entry{file("b.txt", "bbb", 0)}}, // identical -> removal
				{Entries: []*Entry{
					dir("docs/", dirID, 0),
					file("guide.md", "guide", 1),
				}},
			},
		},
	}
}

func manySmallSections(base []*Entry, k int) []*PatchSection {
	sections := []*PatchSection{{Entries: base}}
	for i := 0; i < k; i++ {
		sections = append(sections, &PatchSection{Entries: []*Entry{{
			Name:      fmt.Sprintf("gen%03d.dat", i),
			Mode:      0644,
			Timestamp: time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
			Size:      int64(i + 1),
			C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("gen-%d", i))),
			Depth:     0,
		}}})
	}
	return sections
}

// BenchmarkResolvePatchChain_SectionScaling is the C5 guard. Resolving a chain
// must cost the base tree once plus the sections, not the base once per
// section. Doubling the section count over a fixed base must not double the
// time; if it does, the O(K*N) fold has regressed.
func BenchmarkResolvePatchChain_SectionScaling(b *testing.B) {
	for _, entries := range []int{10000, 40000} {
		base := syntheticWalkOrdered(entries)
		for _, k := range []int{2, 32, 128} {
			sections := manySmallSections(base.Entries, k)
			b.Run(fmt.Sprintf("entries=%d/sections=%d", len(base.Entries), k), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					ResolvePatchChain(sections, 0)
				}
			})
		}
	}
}

// BenchmarkResolvePatchChain_VersusRepeatedApplyPatch measures the delta the
// single-tree fold buys, so the improvement stays a number rather than a
// claim. Run both and compare ns/op at the same entry and section counts.
func BenchmarkResolvePatchChain_VersusRepeatedApplyPatch(b *testing.B) {
	base := syntheticWalkOrdered(40000)
	sections := manySmallSections(base.Entries, 128)

	b.Run("single_tree", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ResolvePatchChain(sections, 0)
		}
	})
	b.Run("repeated_applypatch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resolveByRepeatedApplyPatch(sections, 0)
		}
	})
}

// syntheticWalkOrdered builds a walk-ordered manifest of about n entries with
// a realistic directory ratio — direct children contiguous after their parent,
// which is what buildPatchTree requires.
func syntheticWalkOrdered(n int) *Manifest {
	m := NewManifest()
	ts := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	count := 0

	var fill func(depth int)
	fill = func(depth int) {
		if count >= n || depth > 6 {
			return
		}
		for i := 0; i < 8 && count < n; i++ {
			m.AddEntry(&Entry{
				Depth:     depth,
				Name:      fmt.Sprintf("file%04d.dat", i),
				Mode:      0644,
				Timestamp: ts,
				Size:      int64(1024 * (i + 1)),
				C4ID:      c4.Identify(strings.NewReader(fmt.Sprintf("c-%d", count))),
			})
			count++
		}
		for d := 0; d < 4 && count < n; d++ {
			m.AddEntry(&Entry{
				Depth:     depth,
				Name:      fmt.Sprintf("dir%03d/", d),
				Mode:      os.ModeDir | 0755,
				Timestamp: ts,
				Size:      -1,
			})
			count++
			fill(depth + 1)
		}
	}
	fill(0)
	m.SortEntries()
	return m
}
