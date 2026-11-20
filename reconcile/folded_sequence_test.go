package reconcile

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// A folded sequence entry names an ID-list object, not file bytes. The
// plan writes its frames from that list; it never writes the list under
// the range pattern's name, and never removes the real members (through
// 2026-11-20 it refused instead; before 7afd4d8 it did both).
func TestPlanWritesAFoldedSequencesFramesNeverItsList(t *testing.T) {
	dstDir := t.TempDir()
	var members []c4.ID
	for i := 1; i <= 3; i++ {
		members = append(members, writeFile(t, dstDir, fmt.Sprintf("frame.000%d.exr", i), fmt.Sprintf("f%d", i)))
	}
	list := c4m.IDListBytes(members)
	s := store.NewRAM()
	listID, err := s.Put(bytes.NewReader(list))
	if err != nil {
		t.Fatal(err)
	}

	target := buildManifest(t, []testEntry{
		{name: "frame.[0001-0003].exr", content: string(list), id: listID, mode: 0644},
	})
	target.Entries[0].IsSequence = true

	plan, err := New(WithSource(s)).Plan(target, dstDir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.IsComplete() {
		t.Fatalf("the list is in the store: the plan should be complete, missing %v, ranges %v", plan.Missing, plan.Ranges)
	}
	for _, op := range plan.Operations {
		if op.Type == OpRemove || op.Type == OpRmdir {
			t.Errorf("the members are the frames: nothing to remove, got %v %s", op.Type, op.Path)
		}
		if bytes.Contains([]byte(op.Path), []byte("[0001-0003]")) {
			t.Errorf("an operation on the range pattern's name: %v %s", op.Type, op.Path)
		}
	}
}
