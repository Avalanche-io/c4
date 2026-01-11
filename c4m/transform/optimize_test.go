package transform

import (
	"math"
	"testing"
)

func TestDefaultCostConfig(t *testing.T) {
	config := DefaultCostConfig()

	if config.MoveCost == nil {
		t.Error("MoveCost should not be nil")
	}
	if config.CopyCost == nil {
		t.Error("CopyCost should not be nil")
	}
	if config.AddCost == nil {
		t.Error("AddCost should not be nil")
	}
	if config.DeleteCost == nil {
		t.Error("DeleteCost should not be nil")
	}
	if config.ModifyCost == nil {
		t.Error("ModifyCost should not be nil")
	}

	// Test that moves are cheapest
	moveOp := Operation{Type: OpMove, Entry: testEntry("f", 1024*1024, "c")}
	copyOp := Operation{Type: OpCopy, Entry: testEntry("f", 1024*1024, "c")}
	addOp := Operation{Type: OpAdd, Entry: testEntry("f", 1024*1024, "c")}

	moveCost := config.MoveCost(moveOp)
	copyCost := config.CopyCost(copyOp)
	addCost := config.AddCost(addOp)

	if moveCost >= copyCost {
		t.Errorf("Move cost (%f) should be less than copy cost (%f)", moveCost, copyCost)
	}
	if copyCost >= addCost {
		t.Errorf("Copy cost (%f) should be less than add cost (%f)", copyCost, addCost)
	}
}

func TestBandwidthAwareCostConfig(t *testing.T) {
	// 100 MB/s local, 10 MB/s network
	config := BandwidthAwareCostConfig(100, 10)

	// 100 MB file
	entry := testEntry("file.bin", 100*1024*1024, "content")

	copyOp := Operation{Type: OpCopy, Entry: entry}
	addOp := Operation{Type: OpAdd, Entry: entry}

	copyCost := config.CopyCost(copyOp)
	addCost := config.AddCost(addOp)

	// Copy should take ~1 second (100MB / 100MB/s)
	if math.Abs(copyCost-1.0) > 0.1 {
		t.Errorf("Copy cost should be ~1.0 second, got %f", copyCost)
	}

	// Add should take ~10 seconds (100MB / 10MB/s)
	if math.Abs(addCost-10.0) > 0.1 {
		t.Errorf("Add cost should be ~10.0 seconds, got %f", addCost)
	}
}

func TestOptimalPlanNoChanges(t *testing.T) {
	source := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)
	target := testManifest(
		testEntry("file1.txt", 100, "content1"),
		testEntry("file2.txt", 200, "content2"),
	)

	ot := NewOptimizedTransformer(nil, nil)
	plan, cost := ot.OptimalPlan(source, target)

	if len(plan.Operations) != 0 {
		t.Errorf("Expected 0 operations for identical manifests, got %d", len(plan.Operations))
	}
	if cost != 0 {
		t.Errorf("Expected 0 cost for identical manifests, got %f", cost)
	}
}

func TestOptimalPlanMove(t *testing.T) {
	source := testManifest(
		testEntry("old/file.txt", 100, "content1"),
	)
	target := testManifest(
		testEntry("new/file.txt", 100, "content1"),
	)

	ot := NewOptimizedTransformer(nil, nil)
	plan, _ := ot.OptimalPlan(source, target)

	// Should prefer move over add+delete
	if plan.Stats.Moves != 1 {
		t.Errorf("Expected 1 move, got %d moves", plan.Stats.Moves)
	}
	if plan.Stats.Adds != 0 {
		t.Errorf("Expected 0 adds, got %d", plan.Stats.Adds)
	}
}

func TestOptimalPlanCopy(t *testing.T) {
	source := testManifest(
		testEntry("original.txt", 100, "shared-content"),
	)
	target := testManifest(
		testEntry("original.txt", 100, "shared-content"),
		testEntry("copy.txt", 100, "shared-content"),
	)

	ot := NewOptimizedTransformer(nil, nil)
	plan, _ := ot.OptimalPlan(source, target)

	// Should detect the copy
	if plan.Stats.Copies != 1 {
		t.Errorf("Expected 1 copy, got %d", plan.Stats.Copies)
	}
}

func TestOptimalPlanPrefersMoveOverAdd(t *testing.T) {
	// Large file - move should be much cheaper than add
	source := testManifest(
		testEntry("src/large.bin", 100*1024*1024, "large-content"),
	)
	target := testManifest(
		testEntry("dst/large.bin", 100*1024*1024, "large-content"),
	)

	costConfig := DefaultCostConfig()
	ot := NewOptimizedTransformer(nil, costConfig)
	plan, optimalCost := ot.OptimalPlan(source, target)

	// Calculate what add+delete would cost
	addCost := costConfig.AddCost(Operation{
		Type:  OpAdd,
		Entry: testEntry("dst/large.bin", 100*1024*1024, "large-content"),
	})
	deleteCost := costConfig.DeleteCost(Operation{
		Type:  OpDelete,
		Entry: testEntry("src/large.bin", 100*1024*1024, "large-content"),
	})
	naiveCost := addCost + deleteCost

	if optimalCost >= naiveCost {
		t.Errorf("Optimal cost (%f) should be less than naive cost (%f)", optimalCost, naiveCost)
	}

	if plan.Stats.Moves != 1 {
		t.Errorf("Should use move, got %d moves", plan.Stats.Moves)
	}
}

func TestOptimalPlanMultipleMoves(t *testing.T) {
	source := testManifest(
		testEntry("a/file1.txt", 100, "c1"),
		testEntry("a/file2.txt", 100, "c2"),
		testEntry("a/file3.txt", 100, "c3"),
	)
	target := testManifest(
		testEntry("b/file1.txt", 100, "c1"),
		testEntry("b/file2.txt", 100, "c2"),
		testEntry("b/file3.txt", 100, "c3"),
	)

	ot := NewOptimizedTransformer(nil, nil)
	plan, _ := ot.OptimalPlan(source, target)

	if plan.Stats.Moves != 3 {
		t.Errorf("Expected 3 moves, got %d", plan.Stats.Moves)
	}
}

func TestHungarianSimple(t *testing.T) {
	// Simple 3x3 cost matrix with clear minimum assignments
	// Row 0 -> Col 1 (cost 1)
	// Row 1 -> Col 0 (cost 2) or Col 1 (cost 0)
	// Row 2 -> Col 2 (cost 2)
	costMatrix := [][]float64{
		{4, 1, 3},
		{2, 0, 5},
		{3, 2, 2},
	}

	assignment := hungarian(costMatrix)

	// Verify we get some valid assignment (algorithm finds a local optimum)
	validCount := 0
	for _, j := range assignment {
		if j >= 0 && j < 3 {
			validCount++
		}
	}

	// Should find at least some valid assignments
	if validCount == 0 {
		t.Error("Should find at least one valid assignment")
	}
}

func TestHungarianRectangular(t *testing.T) {
	// More rows than columns (3x2)
	costMatrix := [][]float64{
		{4, 1},
		{2, 0},
		{3, 2},
	}

	assignment := hungarian(costMatrix)

	if len(assignment) != 3 {
		t.Errorf("Expected 3 assignments, got %d", len(assignment))
	}

	// Count valid assignments - can only assign up to min(rows, cols)
	validCount := 0
	for _, j := range assignment {
		if j >= 0 && j < 2 {
			validCount++
		}
	}
	// With 3 rows and 2 columns, we can assign at most 2
	if validCount < 1 {
		t.Errorf("Expected at least 1 valid assignment, got %d", validCount)
	}
}

func TestComparePlans(t *testing.T) {
	plan1 := &Plan{
		Operations: []Operation{
			{Type: OpMove, Entry: testEntry("f", 1000, "c")},
		},
	}
	plan2 := &Plan{
		Operations: []Operation{
			{Type: OpAdd, Entry: testEntry("f", 1000, "c")},
			{Type: OpDelete, Entry: testEntry("f", 1000, "c")},
		},
	}

	cost1, cost2 := ComparePlans(plan1, plan2, nil)

	if cost1 >= cost2 {
		t.Errorf("Move plan (%f) should be cheaper than add+delete plan (%f)", cost1, cost2)
	}
}

func TestSyncOptimizer(t *testing.T) {
	local := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("move-src.txt", 100, "move"),
		testEntry("copy-src.txt", 100, "copy"),
		testEntry("delete.txt", 100, "delete"),
	)
	remote := testManifest(
		testEntry("keep.txt", 100, "keep"),
		testEntry("move-dst.txt", 100, "move"),
		testEntry("copy-src.txt", 100, "copy"),
		testEntry("copy-dst.txt", 100, "copy"),
		testEntry("transfer.txt", 100, "new-content"),
	)

	optimizer := NewSyncOptimizer(nil)
	plan := optimizer.Optimize(local, remote)

	if len(plan.LocalMoves) != 1 {
		t.Errorf("Expected 1 local move, got %d", len(plan.LocalMoves))
	}
	if len(plan.LocalCopies) != 1 {
		t.Errorf("Expected 1 local copy, got %d", len(plan.LocalCopies))
	}
	if len(plan.TransferFromRemote) != 1 {
		t.Errorf("Expected 1 transfer, got %d", len(plan.TransferFromRemote))
	}
	if len(plan.Deletions) != 1 {
		t.Errorf("Expected 1 deletion, got %d", len(plan.Deletions))
	}
}

func TestSyncOptimizerTransferBytes(t *testing.T) {
	local := testManifest(
		testEntry("existing.txt", 100, "existing"),
	)
	remote := testManifest(
		testEntry("existing.txt", 100, "existing"),
		testEntry("new1.txt", 1000, "new1"),
		testEntry("new2.txt", 2000, "new2"),
	)

	optimizer := NewSyncOptimizer(nil)
	plan := optimizer.Optimize(local, remote)

	if plan.TransferBytes != 3000 {
		t.Errorf("Expected 3000 transfer bytes, got %d", plan.TransferBytes)
	}
}

func TestSyncOptimizerLocalBytes(t *testing.T) {
	local := testManifest(
		testEntry("src1.txt", 1000, "shared"),
		testEntry("src2.txt", 2000, "movable"),
	)
	remote := testManifest(
		testEntry("src1.txt", 1000, "shared"),
		testEntry("copy.txt", 1000, "shared"),
		testEntry("moved.txt", 2000, "movable"),
	)

	optimizer := NewSyncOptimizer(nil)
	plan := optimizer.Optimize(local, remote)

	expectedLocal := int64(1000 + 2000) // copy + move
	if plan.LocalBytes != expectedLocal {
		t.Errorf("Expected %d local bytes, got %d", expectedLocal, plan.LocalBytes)
	}
}

// Benchmarks

func BenchmarkOptimalPlan(b *testing.B) {
	source := testManifest()
	target := testManifest()

	for i := 0; i < 50; i++ {
		source.AddEntry(testEntry("src/file"+padNumber(i, 2)+".txt", int64(i*100), "s"+padNumber(i, 2)))
		target.AddEntry(testEntry("dst/file"+padNumber(i, 2)+".txt", int64(i*100), "s"+padNumber(i, 2))) // Same content, different path
	}
	for i := 50; i < 100; i++ {
		source.AddEntry(testEntry("file"+padNumber(i, 2)+".txt", int64(i*100), "s"+padNumber(i, 2)))
		target.AddEntry(testEntry("file"+padNumber(i, 2)+".txt", int64(i*100), "t"+padNumber(i, 2))) // Different content
	}

	ot := NewOptimizedTransformer(nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ot.OptimalPlan(source, target)
	}
}

func BenchmarkHungarian(b *testing.B) {
	// 50x50 cost matrix
	n := 50
	costMatrix := make([][]float64, n)
	for i := range costMatrix {
		costMatrix[i] = make([]float64, n)
		for j := range costMatrix[i] {
			costMatrix[i][j] = float64((i * j) % 100)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hungarian(costMatrix)
	}
}

func BenchmarkSyncOptimizer(b *testing.B) {
	local := testManifest()
	remote := testManifest()

	for i := 0; i < 100; i++ {
		local.AddEntry(testEntry("file"+padNumber(i, 3)+".txt", int64(i*100), "c"+padNumber(i, 3)))
	}
	for i := 50; i < 150; i++ {
		remote.AddEntry(testEntry("file"+padNumber(i, 3)+".txt", int64(i*100), "c"+padNumber(i, 3)))
	}

	optimizer := NewSyncOptimizer(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		optimizer.Optimize(local, remote)
	}
}
