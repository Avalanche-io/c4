package reconcile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// On a platform that cannot hold a name, the plan refuses that entry,
// says which and why, and plans the rest. A refused directory is reported
// once with the count of entries under it; a link whose target the
// platform cannot hold is refused (Windows only). Nothing is planned for
// a refused entry, and no refused name is joined into a path.
func TestPlanRefusesNamesThePlatformCannotHold(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	file := func(name string, depth int) *c4m.Entry {
		return &c4m.Entry{Name: name, Depth: depth, Mode: 0o644, Timestamp: ts, Size: 1,
			C4ID: c4.Identify(strings.NewReader(name))}
	}
	target := c4m.NewManifest()
	target.AddEntry(file("ok.txt", 0))
	target.AddEntry(file(`a\b.txt`, 0))
	target.AddEntry(&c4m.Entry{Name: "link", Mode: os.ModeSymlink | 0o777, Timestamp: ts, Size: 3, Target: "x:y"})
	target.AddEntry(&c4m.Entry{Name: "con/", Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1})
	target.AddEntry(file("inside.txt", 1))
	target.AddEntry(&c4m.Entry{Name: "fine/", Mode: os.ModeDir | 0o755, Timestamp: ts, Size: -1})
	target.AddEntry(file("kept.txt", 1))
	target.SortEntries()

	src := store.NewRAM()
	for _, n := range []string{"ok.txt", `a\b.txt`, "inside.txt", "kept.txt"} {
		if _, err := src.Put(strings.NewReader(n)); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	plan, err := New(withPlatform("windows"), WithSource(src)).Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range plan.Refused {
		got = append(got, r.Path+" ×"+string(rune('0'+r.Entries)))
	}
	want := []string{`a\b.txt ×1`, "con/ ×2", "link ×1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("refused %v, want %v", got, want)
	}
	// Only the holdable entries are planned, and no operation's path holds
	// a refused name.
	planned := map[string]bool{}
	for _, op := range plan.Operations {
		rel, _ := filepath.Rel(dir, op.Path)
		planned[filepath.ToSlash(rel)] = true
	}
	for _, p := range []string{"ok.txt", "fine", "fine/kept.txt"} {
		if !planned[p] {
			t.Errorf("%s not planned; planned %v", p, planned)
		}
	}
	for p := range planned {
		if strings.Contains(p, `\`) || strings.HasPrefix(p, "con") || p == "link" {
			t.Errorf("planned an operation for refused %q", p)
		}
	}

	// On linux nothing in this target is refused.
	plan, err = New(withPlatform("linux"), WithSource(src)).Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Refused) != 0 {
		t.Errorf("linux refused %v, want none", plan.Refused)
	}
}
