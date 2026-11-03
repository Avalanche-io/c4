package reconcile

import (
	"os"
	"path/filepath"
	"testing"
)

// One source, two targets: the folder holds a plate once, the target lists
// the same content in two shot folders. The removed file moves to the
// first target and the second is a create copied from the move's
// destination. Through c75eb94 the planner made both targets moves from
// the one source, the second rename failed with ENOENT, and the folder was
// left half switched (found by a demo scout, 2026-11-03).
func TestPlanAndApplyOneSourceTwoTargets(t *testing.T) {
	dir := t.TempDir()
	id := writeFile(t, dir, "plates/plate.exr", "plate bytes")

	target := buildManifest(t, []testEntry{
		{name: "sh010/", isDir: true},
		{name: "sh010/plate.exr", content: "plate bytes", id: id, mode: 0644},
		{name: "sh020/", isDir: true},
		{name: "sh020/plate.exr", content: "plate bytes", id: id, mode: 0644},
	})

	rec := New()
	plan, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("content reported missing: %v", plan.Missing)
	}
	moves, creates, removes := 0, 0, 0
	for _, op := range plan.Operations {
		switch op.Type {
		case OpMove:
			moves++
		case OpCreate:
			creates++
			if op.SrcPath == "" {
				t.Errorf("the second target's create names no local source")
			}
		case OpRemove:
			removes++
		}
	}
	if moves != 1 || creates != 1 || removes != 0 {
		t.Fatalf("planned %d moves, %d creates, %d removes; want 1, 1, 0:\n%v", moves, creates, removes, plan.Operations)
	}

	res, err := rec.Apply(plan, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("apply errors: %v", res.Errors)
	}
	for _, p := range []string{"sh010/plate.exr", "sh020/plate.exr"} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if string(got) != "plate bytes" {
			t.Errorf("%s holds %q", p, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "plates", "plate.exr")); !os.IsNotExist(err) {
		t.Errorf("the source still exists after its move: %v", err)
	}

	// Re-planning the finished folder is a no-op.
	again, err := rec.Plan(target, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range again.Operations {
		if op.Type == OpMove || op.Type == OpCreate || op.Type == OpRemove {
			t.Errorf("second plan still has %v", op)
		}
	}
}
