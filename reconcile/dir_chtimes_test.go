package reconcile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// A directory whose contents the plan changes gets its recorded mtime set
// back after the change, even when its mtime matched the target at plan
// time: writing a child (temp file plus rename) bumps the directory's
// mtime, so without a chtimes the tree no longer matches the description
// and a re-run plans nothing. (Reported by the fleet lead, 2026-11-03: a
// changed file inside a subdirectory failed restore's verification
// forever.)
func TestPlanAndApplyRestoreDirectoryMtimeAfterChildWrite(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sub/n.txt", "NESTED\n")
	recorded := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	dirTime := time.Date(2026, 11, 4, 0, 42, 6, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "sub", "n.txt"), recorded, recorded)
	// The directory's own mtime already equals the target's: nothing
	// about the directory itself differs at plan time.
	if err := os.Chtimes(filepath.Join(dir, "sub"), dirTime, dirTime); err != nil {
		t.Fatal(err)
	}

	// The target: sub/ at dirTime holding n.txt "nested\n" at recorded.
	srcDir := t.TempDir()
	id := writeFile(t, srcDir, "n.txt", "nested\n")
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "sub/", Depth: 0, Mode: os.ModeDir | 0o755, Timestamp: dirTime, Size: -1})
	target.AddEntry(&c4m.Entry{Name: "n.txt", Depth: 1, Mode: 0o644, Timestamp: recorded, Size: 7, C4ID: id})
	srcManifest := c4m.NewManifest()
	srcManifest.AddEntry(&c4m.Entry{Name: "n.txt", Depth: 0, Mode: 0o644, Timestamp: recorded, Size: 7, C4ID: id})

	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("missing: %v", plan.Missing)
	}
	creates, chtimes := 0, 0
	for _, op := range plan.Operations {
		switch op.Type {
		case OpCreate:
			creates++
		case OpChtimes:
			if filepath.Base(op.Path) == "sub" {
				chtimes++
			}
		}
	}
	if creates != 1 || chtimes != 1 {
		t.Fatalf("planned %d creates and %d chtimes for sub/; want 1 and 1 (the write will bump sub/):\n%v", creates, chtimes, plan.Operations)
	}

	res, err := rec.Apply(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("apply errors: %v", res.Errors)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "sub", "n.txt"))
	if string(got) != "nested\n" {
		t.Fatalf("n.txt holds %q", got)
	}
	info, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Truncate(time.Second).Equal(dirTime) {
		t.Fatalf("sub/ mtime is %s after apply, recorded %s", info.ModTime().UTC(), dirTime)
	}

	// Re-running plans nothing: the tree matches the description.
	again, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Operations) != 0 {
		t.Fatalf("a second plan is not empty: %v", again.Operations)
	}
}

// Removing a file bumps its directory's mtime too, and so does creating
// one in a directory the plan makes: both get the recorded time back.
func TestPlanRestoresDirectoryMtimeAfterRemoveAndMkdir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sub/gone.txt", "x")
	dirTime := time.Date(2026, 11, 4, 0, 42, 6, 0, time.UTC)
	os.Chtimes(filepath.Join(dir, "sub"), dirTime, dirTime)
	newTime := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

	srcDir := t.TempDir()
	id := writeFile(t, srcDir, "f.txt", "fresh")
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "new/", Depth: 0, Mode: os.ModeDir | 0o755, Timestamp: newTime, Size: -1})
	target.AddEntry(&c4m.Entry{Name: "f.txt", Depth: 1, Mode: 0o644, Timestamp: newTime, Size: 5, C4ID: id})
	target.AddEntry(&c4m.Entry{Name: "sub/", Depth: 0, Mode: os.ModeDir | 0o755, Timestamp: dirTime, Size: 0})
	srcManifest := c4m.NewManifest()
	srcManifest.AddEntry(&c4m.Entry{Name: "f.txt", Depth: 0, Mode: 0o644, Timestamp: newTime, Size: 5, C4ID: id})

	rec := New(WithSource(NewDirSource(srcManifest, srcDir)))
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec.Apply(plan, dir); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]time.Time{"sub": dirTime, "new": newTime} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().UTC().Truncate(time.Second).Equal(want) {
			t.Errorf("%s/ mtime is %s after apply, recorded %s", name, info.ModTime().UTC(), want)
		}
	}
	again, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Operations) != 0 {
		t.Fatalf("a second plan is not empty: %v", again.Operations)
	}
}
