package reconcile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// WithKnownState: the plan reuses the caller's own scan for a file that
// scan provably hashed (same size, same mtime, mtime strictly older than
// the scan start), and hashes everything else. The reuse is proven by
// giving the known state an ID the bytes do not have: if the plan
// believed it, no create is planned; if it rehashed, one is.
func TestPlanReusesKnownStateOnlyForFilesTheScanHashed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello world\n")
	old := time.Now().Add(-10 * time.Second).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(dir, "a.txt"), old, old); err != nil {
		t.Fatal(err)
	}
	// The target's bytes, held elsewhere so a create can be planned (a
	// plan with missing content lists no operations).
	srcDir := t.TempDir()
	wrongID := writeFile(t, srcDir, "a.txt", "xxxxxxxxxxxx")
	target := buildManifest(t, []testEntry{{name: "a.txt", content: "xxxxxxxxxxxx", id: wrongID, mode: 0644}})
	source := WithSource(NewDirSource(target, srcDir))

	known := func(ts time.Time) *c4m.Manifest {
		m := c4m.NewManifest()
		m.AddEntry(&c4m.Entry{Name: "a.txt", Mode: 0o644, Timestamp: ts, Size: 12, C4ID: wrongID})
		return m
	}

	// Provably hashed by the scan: the plan trusts the scan's ID and
	// finds the target already in place.
	rec := New(source, WithKnownState(known(old), time.Now()))
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := countOps(plan, OpCreate); n != 0 {
		t.Fatalf("the plan rehashed a file the known scan had hashed: %d creates", n)
	}

	// The scan started before the file's mtime second: racy, so it is
	// rehashed, and the real bytes do not match the target.
	rec = New(source, WithKnownState(known(old), old.Add(-5*time.Second)))
	plan, err = rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := countOps(plan, OpCreate); n != 1 {
		t.Fatalf("a file not strictly older than the scan start was not rehashed: %d creates", n)
	}

	// A different mtime than the scan recorded: rehashed.
	rec = New(source, WithKnownState(known(old.Add(-time.Second)), time.Now()))
	plan, err = rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := countOps(plan, OpCreate); n != 1 {
		t.Fatalf("a file whose mtime differs from the scan's was not rehashed: %d creates", n)
	}

	// Without a known state, the real bytes are hashed.
	rec = New(source)
	plan, err = rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if n := countOps(plan, OpCreate); n != 1 {
		t.Fatalf("without known state the file was not hashed: %d creates", n)
	}
}

func countOps(p *Plan, op Op) int {
	n := 0
	for _, o := range p.Operations {
		if o.Type == op {
			n++
		}
	}
	return n
}
