package transform

import (
	"bytes"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// Helper to create a test entry with a C4 ID
func testEntry(name string, size int64, idSuffix string) *c4m.Entry {
	entry := &c4m.Entry{
		Name: name,
		Size: size,
	}
	// Create a deterministic ID from the suffix
	if idSuffix != "" {
		id := c4.Identify(bytes.NewReader([]byte(idSuffix)))
		entry.C4ID = id
	}
	return entry
}

// Helper to create a test manifest
func testManifest(entries ...*c4m.Entry) *c4m.Manifest {
	m := c4m.NewManifest()
	for _, e := range entries {
		m.AddEntry(e)
	}
	return m
}

func TestTransformEmptyToEmpty(t *testing.T) {
	source := testManifest()
	target := testManifest()

	transformer := NewTransformer(nil)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if len(plan.Operations) != 0 {
		t.Errorf("Expected 0 operations, got %d", len(plan.Operations))
	}
}

func TestTransformNoChanges(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)

	transformer := NewTransformer(nil)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if len(plan.Operations) != 0 {
		t.Errorf("Expected 0 operations for identical manifests, got %d", len(plan.Operations))
	}
}

func TestTransformAddition(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)

	transformer := NewTransformer(nil)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if plan.Stats.Adds != 1 {
		t.Errorf("Expected 1 add, got %d", plan.Stats.Adds)
	}
	if plan.Stats.TotalOps != 1 {
		t.Errorf("Expected 1 total op, got %d", plan.Stats.TotalOps)
	}
}

func TestTransformDeletion(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
	)

	transformer := NewTransformer(nil)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if plan.Stats.Deletes != 1 {
		t.Errorf("Expected 1 delete, got %d", plan.Stats.Deletes)
	}
}

func TestTransformModification(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("file1.txt", 150, "content1-modified"),
	)

	transformer := NewTransformer(nil)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if plan.Stats.Modifies != 1 {
		t.Errorf("Expected 1 modify, got %d", plan.Stats.Modifies)
	}
}

func TestTransformMove(t *testing.T) {
	source := testManifest(
		testEntry("old/file.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("new/file.txt", 100, "content1"),
	)

	config := &Config{
		DetectMoves:  true,
		DetectCopies: false,
	}
	transformer := NewTransformer(config)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if plan.Stats.Moves != 1 {
		t.Errorf("Expected 1 move, got %d", plan.Stats.Moves)
	}
	if plan.Stats.BytesToMove != 100 {
		t.Errorf("Expected 100 bytes to move, got %d", plan.Stats.BytesToMove)
	}
}

func TestTransformCopy(t *testing.T) {
	source := testManifest(
		testEntry("original.txt", 100, "shared-content"),
	)
	target := testManifest(
		testEntry("original.txt", 100, "shared-content"),
		testEntry("copy.txt", 100, "shared-content"),
	)

	config := &Config{
		DetectMoves:  true,
		DetectCopies: true,
	}
	transformer := NewTransformer(config)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if plan.Stats.Copies != 1 {
		t.Errorf("Expected 1 copy, got %d", plan.Stats.Copies)
	}
}

func TestTransformMoveDisabled(t *testing.T) {
	source := testManifest(
		testEntry("old/file.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("new/file.txt", 100, "content1"),
	)

	config := &Config{
		DetectMoves:  false,
		DetectCopies: false,
	}
	transformer := NewTransformer(config)
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	// Without move detection, should see add + delete
	if plan.Stats.Moves != 0 {
		t.Errorf("Expected 0 moves with detection disabled, got %d", plan.Stats.Moves)
	}
	if plan.Stats.Adds != 1 {
		t.Errorf("Expected 1 add, got %d", plan.Stats.Adds)
	}
	if plan.Stats.Deletes != 1 {
		t.Errorf("Expected 1 delete, got %d", plan.Stats.Deletes)
	}
}

func TestTransformOperationOrder(t *testing.T) {
	source := testManifest(
		testEntry("delete-me.txt", 100, "delete"),
		testEntry("move-me.txt", 200, "move"),
		testEntry("modify-me.txt", 300, "original"),
	)
	target := testManifest(
		testEntry("moved.txt", 200, "move"),
		testEntry("modify-me.txt", 350, "modified"),
		testEntry("new-file.txt", 400, "new"),
	)

	transformer := NewTransformer(DefaultConfig())
	plan, err := transformer.Transform(source, target)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	// Verify order: deletes, moves, copies, modifies, adds
	lastPriority := -1
	priority := map[OpType]int{
		OpDelete: 1,
		OpMove:   2,
		OpCopy:   3,
		OpModify: 4,
		OpAdd:    5,
	}

	for _, op := range plan.Operations {
		p := priority[op.Type]
		if p < lastPriority {
			t.Errorf("Operations not in order: %s came after higher priority op", op.Type)
		}
		lastPriority = p
	}
}

func TestFindMissing(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file3.txt", 300, "content3"),
	)

	missing := FindMissing(source, target)

	if len(missing.Entries) != 1 {
		t.Errorf("Expected 1 missing entry, got %d", len(missing.Entries))
	}
	if missing.Entries[0].Name != "file3.txt" {
		t.Errorf("Expected file3.txt, got %s", missing.Entries[0].Name)
	}
}

func TestFindExtra(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
	)

	extra := FindExtra(source, target)

	if len(extra.Entries) != 1 {
		t.Errorf("Expected 1 extra entry, got %d", len(extra.Entries))
	}
	if extra.Entries[0].Name != "file2.txt" {
		t.Errorf("Expected file2.txt, got %s", extra.Entries[0].Name)
	}
}

func TestFindCommon(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)
	target := testManifest(
		testEntry("file1-renamed.txt", 100, "content1"),
		testEntry("file3.txt", 300, "content3"),
	)

	common := FindCommon(source, target)

	if len(common.Entries) != 1 {
		t.Errorf("Expected 1 common entry, got %d", len(common.Entries))
	}
}

func TestOpTypeString(t *testing.T) {
	tests := []struct {
		op       OpType
		expected string
	}{
		{OpAdd, "ADD"},
		{OpDelete, "DELETE"},
		{OpModify, "MODIFY"},
		{OpMove, "MOVE"},
		{OpCopy, "COPY"},
		{OpType(99), "UNKNOWN"},
	}

	for _, test := range tests {
		if got := test.op.String(); got != test.expected {
			t.Errorf("OpType(%d).String() = %s, want %s", test.op, got, test.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if !config.DetectMoves {
		t.Error("DetectMoves should be true by default")
	}
	if !config.DetectCopies {
		t.Error("DetectCopies should be true by default")
	}
}

func TestPlanString(t *testing.T) {
	plan := &Plan{
		Operations: []Operation{
			{Type: OpAdd, Target: "new.txt"},
			{Type: OpDelete, Target: "old.txt"},
			{Type: OpMove, Source: "a.txt", Target: "b.txt"},
		},
		Stats: Stats{
			TotalOps: 3,
			Adds:     1,
			Deletes:  1,
			Moves:    1,
		},
	}

	str := plan.String()
	if str == "" {
		t.Error("Plan.String() should not be empty")
	}
	// Verify it contains expected content
	if !contains(str, "ADD: new.txt") {
		t.Error("Plan.String() should contain ADD operation")
	}
	if !contains(str, "DELETE: old.txt") {
		t.Error("Plan.String() should contain DELETE operation")
	}
	if !contains(str, "MOVE: a.txt -> b.txt") {
		t.Error("Plan.String() should contain MOVE operation")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmarks

func BenchmarkTransformSmall(b *testing.B) {
	source := testManifest()
	target := testManifest()

	// Create 100 entries
	for i := 0; i < 100; i++ {
		source.AddEntry(testEntry("file"+string(rune('0'+i%10))+".txt", int64(i*100), "content"+string(rune('0'+i%10))))
	}
	for i := 0; i < 100; i++ {
		target.AddEntry(testEntry("file"+string(rune('0'+i%10))+".txt", int64(i*100), "content"+string(rune('0'+i%10))))
	}

	transformer := NewTransformer(nil)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		transformer.Transform(source, target)
	}
}

func BenchmarkFindMissing(b *testing.B) {
	source := testManifest()
	target := testManifest()

	for i := 0; i < 1000; i++ {
		source.AddEntry(testEntry("source"+string(rune('0'+i%10))+".txt", int64(i), "s"+string(rune('0'+i%10))))
	}
	for i := 0; i < 1000; i++ {
		target.AddEntry(testEntry("target"+string(rune('0'+i%10))+".txt", int64(i), "t"+string(rune('0'+i%10))))
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		FindMissing(source, target)
	}
}
