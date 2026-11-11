package reconcile

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// A folded sequence entry names an ID-list object, not file bytes. Even
// when that object is available, the plan must refuse rather than write
// the ID list under the range pattern's name and remove the real members.
func TestPlanRefusesFoldedSequence(t *testing.T) {
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
	if plan.IsComplete() || len(plan.Missing) != 1 || plan.Missing[0] != listID {
		t.Fatalf("expected the folded entry reported missing, got %v", plan.Missing)
	}
	if len(plan.Operations) != 0 {
		t.Fatalf("expected no operations, got %d", len(plan.Operations))
	}
}
