package transform

import (
	"sync/atomic"
	"testing"
)

func TestStreamingTransformEmpty(t *testing.T) {
	source := testManifest()
	target := testManifest()

	st := NewStreamingTransformer(nil, 4)
	plan := st.TransformStreaming(source, target)

	count := 0
	for range plan.Operations() {
		count++
	}

	if err := plan.Wait(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 operations, got %d", count)
	}
}

func TestStreamingTransformAdditions(t *testing.T) {
	source := testManifest()
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)

	st := NewStreamingTransformer(nil, 4)
	plan := st.TransformStreaming(source, target)

	var ops []Operation
	for op := range plan.Operations() {
		ops = append(ops, op)
	}

	if err := plan.Wait(); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	stats := plan.Stats()
	if stats.Adds != 2 {
		t.Errorf("Expected 2 adds, got %d", stats.Adds)
	}
}

func TestStreamingTransformMove(t *testing.T) {
	source := testManifest(
		testEntry("old/file.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("new/file.txt", 100, "content1"),
	)

	config := &Config{DetectMoves: true, DetectCopies: false}
	st := NewStreamingTransformer(config, 4)
	plan := st.TransformStreaming(source, target)

	var ops []Operation
	for op := range plan.Operations() {
		ops = append(ops, op)
	}

	plan.Wait()
	stats := plan.Stats()

	if stats.Moves != 1 {
		t.Errorf("Expected 1 move, got %d", stats.Moves)
	}
}

func TestStreamingTransformMixed(t *testing.T) {
	source := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("delete.txt", 100, "delete"),
		testEntry("modify.txt", 100, "original"),
		testEntry("move-old.txt", 100, "moveme"),
	)
	target := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("modify.txt", 150, "modified"),
		testEntry("move-new.txt", 100, "moveme"),
		testEntry("add.txt", 100, "new"),
	)

	st := NewStreamingTransformer(DefaultConfig(), 4)
	plan := st.TransformStreaming(source, target)

	var ops []Operation
	for op := range plan.Operations() {
		ops = append(ops, op)
	}

	plan.Wait()
	stats := plan.Stats()

	if stats.Moves != 1 {
		t.Errorf("Expected 1 move, got %d", stats.Moves)
	}
	if stats.Modifies != 1 {
		t.Errorf("Expected 1 modify, got %d", stats.Modifies)
	}
	if stats.Adds != 1 {
		t.Errorf("Expected 1 add, got %d", stats.Adds)
	}
	if stats.Deletes != 1 {
		t.Errorf("Expected 1 delete, got %d", stats.Deletes)
	}
}

func TestParallelExecutor(t *testing.T) {
	plan := &Plan{
		Operations: []Operation{
			{Type: OpAdd, Target: "file1.txt"},
			{Type: OpAdd, Target: "file2.txt"},
			{Type: OpAdd, Target: "file3.txt"},
		},
	}

	var count int32
	handler := func(op Operation) error {
		atomic.AddInt32(&count, 1)
		return nil
	}

	exec := NewParallelExecutor(4, handler)
	if err := exec.Execute(plan); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected 3 operations executed, got %d", count)
	}
}

func TestParallelExecutorOrdering(t *testing.T) {
	plan := &Plan{
		Operations: []Operation{
			{Type: OpAdd, Target: "add.txt"},
			{Type: OpDelete, Target: "del.txt"},
			{Type: OpMove, Source: "a.txt", Target: "b.txt"},
		},
	}

	var order []OpType
	handler := func(op Operation) error {
		order = append(order, op.Type)
		return nil
	}

	exec := NewParallelExecutor(1, handler) // Single thread to preserve order
	exec.Execute(plan)

	// Should be: Delete, Move, Add
	expected := []OpType{OpDelete, OpMove, OpAdd}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d ops, got %d", len(expected), len(order))
	}
	for i, op := range order {
		if op != expected[i] {
			t.Errorf("Position %d: expected %v, got %v", i, expected[i], op)
		}
	}
}

func TestExecuteStreaming(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)

	st := NewStreamingTransformer(nil, 4)
	plan := st.TransformStreaming(source, target)

	var count int32
	handler := func(op Operation) error {
		atomic.AddInt32(&count, 1)
		return nil
	}

	exec := NewParallelExecutor(4, handler)
	if err := exec.ExecuteStreaming(plan); err != nil {
		t.Fatalf("ExecuteStreaming failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 operation, got %d", count)
	}
}

func TestDiffIterator(t *testing.T) {
	source := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("remove.txt", 100, "remove"),
		testEntry("modify.txt", 100, "original"),
	)
	target := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("modify.txt", 150, "modified"),
		testEntry("add.txt", 100, "add"),
	)

	iter := NewDiffIterator(source, target)
	diffs := iter.All()

	added := 0
	removed := 0
	modified := 0

	for _, d := range diffs {
		switch d.Type {
		case DiffAdded:
			added++
		case DiffRemoved:
			removed++
		case DiffModified:
			modified++
		}
	}

	if added != 1 {
		t.Errorf("Expected 1 added, got %d", added)
	}
	if removed != 1 {
		t.Errorf("Expected 1 removed, got %d", removed)
	}
	if modified != 1 {
		t.Errorf("Expected 1 modified, got %d", modified)
	}
}

func TestDiffIteratorOnlyChanges(t *testing.T) {
	source := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("remove.txt", 100, "remove"),
	)
	target := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("add.txt", 100, "add"),
	)

	iter := NewDiffIterator(source, target)
	changes := iter.OnlyChanges()

	if len(changes) != 2 {
		t.Errorf("Expected 2 changes, got %d", len(changes))
	}
}

func TestDiffTypeString(t *testing.T) {
	tests := []struct {
		dt       DiffType
		expected string
	}{
		{DiffAdded, "ADDED"},
		{DiffRemoved, "REMOVED"},
		{DiffModified, "MODIFIED"},
		{DiffUnchanged, "UNCHANGED"},
		{DiffType(99), "UNKNOWN"},
	}

	for _, test := range tests {
		if got := test.dt.String(); got != test.expected {
			t.Errorf("DiffType(%d).String() = %s, want %s", test.dt, got, test.expected)
		}
	}
}

// Benchmarks

func BenchmarkStreamingTransform(b *testing.B) {
	source := testManifest()
	target := testManifest()

	for i := 0; i < 100; i++ {
		source.AddEntry(testEntry("src"+string(rune('0'+i%10))+".txt", int64(i), "s"+string(rune('0'+i%10))))
		target.AddEntry(testEntry("tgt"+string(rune('0'+i%10))+".txt", int64(i), "t"+string(rune('0'+i%10))))
	}

	st := NewStreamingTransformer(nil, 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		plan := st.TransformStreaming(source, target)
		for range plan.Operations() {
		}
		plan.Wait()
	}
}

func BenchmarkParallelExecutor(b *testing.B) {
	plan := &Plan{
		Operations: make([]Operation, 100),
	}
	for i := range plan.Operations {
		plan.Operations[i] = Operation{Type: OpAdd, Target: "file.txt"}
	}

	handler := func(op Operation) error {
		return nil
	}

	exec := NewParallelExecutor(8, handler)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		exec.Execute(plan)
	}
}

func BenchmarkDiffIterator(b *testing.B) {
	source := testManifest()
	target := testManifest()

	for i := 0; i < 1000; i++ {
		source.AddEntry(testEntry("file"+string(rune('0'+i%10))+".txt", int64(i), "c"+string(rune('0'+i%10))))
		target.AddEntry(testEntry("file"+string(rune('0'+i%10))+".txt", int64(i), "d"+string(rune('0'+i%10))))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iter := NewDiffIterator(source, target)
		iter.All()
	}
}
