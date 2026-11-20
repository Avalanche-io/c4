package reconcile

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// rangeOf builds, through the c4m package's API, a folded range named
// pattern whose frames hold the given contents, stores every frame in src,
// and returns the range entry and its ID list. A scan never folds a
// stepped range or a single-line discontinuous one, so these come only
// from the API (or another producer); a materialiser must still write them.
func rangeOf(t *testing.T, src *store.RAM, pattern string, depth int, mode os.FileMode, ts time.Time, contents []string) (*c4m.Entry, string) {
	t.Helper()
	var list strings.Builder
	var size int64
	for _, c := range contents {
		id, err := src.Put(strings.NewReader(c))
		if err != nil {
			t.Fatal(err)
		}
		list.WriteString(id.String())
		size += int64(len(c))
	}
	listID := c4.Identify(strings.NewReader(list.String()))
	return &c4m.Entry{
		Name: pattern, Pattern: pattern, IsSequence: true,
		Mode: mode, Timestamp: ts, Size: size + int64(list.Len()),
		C4ID: listID, Depth: depth,
	}, list.String()
}

// Every range form expands to its frames: names from the range expression
// (stepped, discontinuous, padded and unpadded), content by each frame's
// ID from the list, the range line's mode and timestamp on every frame,
// and no size of its own. Plan and Apply then write exactly those files.
func TestExpandRangesWritesEveryRangeForm(t *testing.T) {
	ts := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	src := store.NewRAM()
	target := c4m.NewManifest()
	target.RangeData = map[c4.ID]string{}
	forms := []struct {
		pattern string
		names   []string
	}{
		{"st.[0001-0007:2].exr", []string{"st.0001.exr", "st.0003.exr", "st.0005.exr", "st.0007.exr"}},
		{"dc.[0001-0002,0005-0006].exr", []string{"dc.0001.exr", "dc.0002.exr", "dc.0005.exr", "dc.0006.exr"}},
		{"in.[0002,0004,0009].exr", []string{"in.0002.exr", "in.0004.exr", "in.0009.exr"}},
		{"up.[8-11].exr", []string{"up.8.exr", "up.9.exr", "up.10.exr", "up.11.exr"}},
	}
	want := map[string]string{}
	for _, f := range forms {
		var contents []string
		for _, n := range f.names {
			c := "bytes of " + n + "\n"
			contents = append(contents, c)
			want[n] = c
		}
		e, list := rangeOf(t, src, f.pattern, 0, 0o640, ts, contents)
		target.AddEntry(e)
		target.RangeData[e.C4ID] = list
	}

	out, problems := ExpandRanges(target, nil)
	if len(problems) != 0 {
		t.Fatalf("problems: %v", problems)
	}
	if len(out.Entries) != len(want) {
		t.Fatalf("%d entries, want %d frames", len(out.Entries), len(want))
	}
	for _, e := range out.Entries {
		if e.IsSequence || e.Mode != 0o640 || !e.Timestamp.Equal(ts) || e.Size != -1 {
			t.Errorf("frame %s: sequence %v mode %v time %v size %d; want a plain frame with the range's mode and time and a null size",
				e.Name, e.IsSequence, e.Mode, e.Timestamp, e.Size)
		}
	}

	dest := t.TempDir()
	r := New(WithSource(src))
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("plan incomplete: missing %v, ranges %v", plan.Missing, plan.Ranges)
	}
	if res, err := r.Apply(plan, dest); err != nil || len(res.Errors) > 0 {
		t.Fatalf("apply: %v %v", err, res.Errors)
	}
	for n, c := range want {
		p := filepath.Join(dest, n)
		got, err := ioutil.ReadFile(p)
		if err != nil || string(got) != c {
			t.Errorf("%s: %q, %v; want %q", n, got, err, c)
			continue
		}
		info, _ := os.Stat(p)
		if !info.ModTime().UTC().Equal(ts) {
			t.Errorf("%s: mtime %v, want the range's %v", n, info.ModTime().UTC(), ts)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
			t.Errorf("%s: mode %v, want the range's 0640", n, info.Mode().Perm())
		}
	}
	entries, _ := ioutil.ReadDir(dest)
	if len(entries) != len(want) {
		t.Errorf("dest holds %d entries, want exactly the %d frames", len(entries), len(want))
	}
}

// A list that is not inline is read from the source by the range's C4 ID,
// as a directory's listing is.
func TestExpandRangesReadsTheListFromTheSource(t *testing.T) {
	ts := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	src := store.NewRAM()
	e, list := rangeOf(t, src, "f.[0001-0003].exr", 0, 0o644, ts, []string{"a\n", "b\n", "c\n"})
	if _, err := src.Put(strings.NewReader(list)); err != nil {
		t.Fatal(err)
	}
	target := c4m.NewManifest()
	target.AddEntry(e) // no RangeData: the list is only in src

	out, problems := ExpandRanges(target, src)
	if len(problems) != 0 || len(out.Entries) != 3 {
		t.Fatalf("problems %v, %d entries; want three frames", problems, len(out.Entries))
	}
	// Through the planner, whose sources are its content sources.
	plan, err := New(WithSource(src)).Plan(target, t.TempDir())
	if err != nil || !plan.IsComplete() {
		t.Fatalf("plan: %v, missing %v, ranges %v", err, plan.Missing, plan.Ranges)
	}
}

// No list, no expansion: a range whose list neither the description nor
// any source holds plans nothing, and the missing content named is the
// list. A list under the range's ID that does not name its frames, and a
// range of links, refuse too, each with its own reason.
func TestExpandRangesRefusesWhatItCannotWrite(t *testing.T) {
	ts := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	src := store.NewRAM()
	noList, _ := rangeOf(t, src, "n.[0001-0003].exr", 0, 0o644, ts, []string{"n1", "n2", "n3"})
	short, shortList := rangeOf(t, src, "s.[0001-0004].exr", 0, 0o644, ts, []string{"1", "2", "3"})
	link, linkList := rangeOf(t, src, "l.[0001-0003].exr", 0, os.ModeSymlink|0o777, ts, []string{"x", "y", "z"})
	link.Target = "t.[0001-0003].txt"
	target := c4m.NewManifest()
	target.RangeData = map[c4.ID]string{short.C4ID: shortList, link.C4ID: linkList}
	for _, e := range []*c4m.Entry{link, noList, short} {
		target.AddEntry(e)
	}

	_, problems := ExpandRanges(target, src)
	got := map[string]RangeReason{}
	for _, p := range problems {
		got[p.Path] = p.Reason
	}
	if got["n.[0001-0003].exr"] != RangeNoList || got["s.[0001-0004].exr"] != RangeBadList || got["l.[0001-0003].exr"] != RangeLinks || len(got) != 3 {
		t.Fatalf("problems %v", problems)
	}

	dest := t.TempDir()
	plan, err := New(WithSource(src)).Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.IsComplete() || len(plan.Operations) != 0 {
		t.Fatalf("a plan with unwritable ranges must be incomplete and empty: %d ops", len(plan.Operations))
	}
	if len(plan.Missing) != 1 || plan.Missing[0] != noList.C4ID {
		t.Errorf("missing %v, want only the absent list %s", plan.Missing, noList.C4ID)
	}
	for _, p := range plan.Ranges {
		if p.Reason == RangeNoList && !strings.Contains(p.String(), "the ID list of n.[0001-0003].exr ("+noList.C4ID.String()+")") {
			t.Errorf("message %q", p.String())
		}
	}
}
