# Transform Package - Revived

## Overview

The transform package provides algorithms for computing minimum-operation transformation plans between filesystem manifests. It leverages C4 IDs for intelligent move/copy detection, streaming for large manifests, and graph optimization for minimum-cost planning.

**Location**: `c4m/transform/`
**Size**: ~2,500 lines implementation, ~1,200 lines tests
**Status**: ✅ Revived 2025-01-11, all 4 phases complete

## Feature Summary

| Feature | Status | Quality |
|---------|--------|---------|
| Move/rename detection | ✅ Complete | Optimized with direct C4 ID comparison |
| Modification detection | ✅ Complete | With size fallback |
| Add/delete detection | ✅ Complete | Efficient index-based |
| Copy detection | ✅ Complete | True OpCopy emission |
| Tree edit distance | ✅ Complete | Memoized for performance |
| Operation ordering | ✅ Complete | Priority-based optimization |
| Streaming processing | ✅ Complete | Channel-based for large manifests |
| Parallel execution | ✅ Complete | Configurable parallelism |
| Directory operations | ✅ Complete | mkdir, rmdir, dir moves |
| Attribute detection | ✅ Complete | chmod, touch operations |
| Sequence intelligence | ✅ Complete | Frame range detection, gap analysis |
| Graph optimization | ✅ Complete | Hungarian algorithm, cost functions |

## Package Structure

```
c4m/transform/
├── transform.go       # Core transformation (466 lines)
├── transform_test.go  # Core tests (290 lines)
├── tree.go            # Tree operations, edit distance (357 lines)
├── tree_test.go       # Tree tests (250 lines)
├── streaming.go       # Streaming & parallel execution (453 lines)
├── streaming_test.go  # Streaming tests (200 lines)
├── enhanced.go        # Dir ops, attributes, sequences (534 lines)
├── enhanced_test.go   # Enhanced tests (320 lines)
├── optimize.go        # Graph optimization (708 lines)
└── optimize_test.go   # Optimization tests (310 lines)
```

## Phase 1: Core Revival ✅

Fixed all issues from the original archived package:

### 1. Direct C4 ID Comparison
```go
// Before (slow, allocates)
if sourceEntry.C4ID.String() != targetEntry.C4ID.String() {

// After (fast, zero allocation)
if sourceEntry.C4ID != targetEntry.C4ID {
```

### 2. Consolidated Index Building
```go
type Transformer struct {
    config       *Config
    sourceByID   map[c4.ID][]*c4m.Entry  // Built once
    sourceByPath map[string]*c4m.Entry   // Reused across phases
    targetByID   map[c4.ID][]*c4m.Entry
    targetByPath map[string]*c4m.Entry
}

func (t *Transformer) buildIndices(source, target *c4m.Manifest) {
    // Single pass index building
}
```

### 3. True Copy Detection
```go
func (t *Transformer) detectCopies(...) []Operation {
    // Actually emits OpCopy when content exists at source
    // and appears at new location while source remains
    ops = append(ops, Operation{
        Type:   OpCopy,
        Source: copySource.Name,
        Target: targetEntry.Name,
        Entry:  targetEntry,
    })
}
```

### 4. Memoized Tree Edit Distance
```go
type TreeEditCalculator struct {
    cache map[string]int
}

func (c *TreeEditCalculator) Compute(source, target *TreeNode) int {
    key := c.cacheKey(source, target)
    if cached, ok := c.cache[key]; ok {
        return cached  // O(1) cache hit
    }
    result := c.compute(source, target)
    c.cache[key] = result
    return result
}
```

## Phase 2: Streaming & Parallelism ✅

### Streaming Transformer
```go
// Channel-based operation delivery for large manifests
func (t *StreamingTransformer) TransformStreaming(source, target *c4m.Manifest) *StreamingPlan {
    plan := &StreamingPlan{
        ops:  make(chan Operation, 100),  // Buffered channel
        done: make(chan struct{}),
    }
    go func() {
        // Stream operations as they're detected
        for op := range detected {
            plan.ops <- op
        }
    }()
    return plan
}
```

### Parallel Executor
```go
// Execute operations with configurable parallelism
type ParallelExecutor struct {
    parallelism int
    handler     OperationHandler
}

func (e *ParallelExecutor) Execute(plan *Plan) error {
    // Operations of same type run in parallel
    // Different types run sequentially (delete before add)
    for _, opType := range []OpType{OpDelete, OpMove, OpCopy, OpModify, OpAdd} {
        if err := e.executeParallel(groups[opType]); err != nil {
            return err
        }
    }
}
```

### Diff Iterator
```go
// Memory-efficient manifest comparison
type DiffIterator struct {
    source, target *c4m.Manifest
}

func (d *DiffIterator) OnlyChanges() []DiffEntry {
    // Returns only added, removed, modified entries
}
```

## Phase 3: Enhanced Features ✅

### Extended Operation Types
```go
const (
    OpAdd    OpType = iota
    OpDelete
    OpModify
    OpMove
    OpCopy
    OpMkdir  = iota + 10  // Create directory
    OpRmdir               // Remove empty directory
    OpChmod               // Permission change
    OpTouch               // Timestamp update
    OpSeqAdd              // Add frames to sequence
    OpSeqDel              // Remove frames from sequence
)
```

### Directory Operations
```go
func (t *EnhancedTransformer) enhanceWithDirOps(plan *Plan, ...) {
    // Detect directory moves (all children move together)
    dirMoves := t.detectDirMoves(source, target)

    // Filter individual file moves covered by dir move
    plan.Operations = filterChildOps(plan.Operations, move.Source, move.Target)

    // Add mkdir/rmdir for directory changes
}
```

### Attribute Detection
```go
func (t *EnhancedTransformer) enhanceWithAttributes(plan *Plan, ...) {
    // Same content, different mode -> OpChmod
    if sourceEntry.Mode != targetEntry.Mode {
        plan.Operations = append(plan.Operations, Operation{Type: OpChmod, ...})
    }

    // Same content, different timestamp -> OpTouch
    if !sourceEntry.Timestamp.Equal(targetEntry.Timestamp) {
        plan.Operations = append(plan.Operations, Operation{Type: OpTouch, ...})
    }
}
```

### Sequence Intelligence
```go
type Sequence struct {
    Pattern   string           // Printf-style: "render.%04d.exr"
    Frames    []int            // [1, 2, 3, 5, 6]
    FrameToID map[int]c4.ID    // Per-frame content identity
}

func (s *Sequence) MissingFrames() []int {
    // Detects gaps: [4] for frames [1,2,3,5,6]
}

func CompareSequences(source, target *c4m.Manifest) []SequenceDiff {
    // Returns added, removed, modified frames per sequence
}
```

## Phase 4: Graph Optimization ✅

### Cost-Based Planning
```go
type CostConfig struct {
    MoveCost   CostFunc  // Cheap (local rename)
    CopyCost   CostFunc  // Medium (local I/O)
    AddCost    CostFunc  // Expensive (network transfer)
    DeleteCost CostFunc  // Cheap
    ModifyCost CostFunc  // Expensive (network transfer)
}

// Bandwidth-aware costs
func BandwidthAwareCostConfig(localMBps, networkMBps float64) *CostConfig {
    return &CostConfig{
        CopyCost: func(op Operation) float64 {
            return float64(op.Entry.Size) / 1024 / 1024 / localMBps
        },
        AddCost: func(op Operation) float64 {
            return float64(op.Entry.Size) / 1024 / 1024 / networkMBps
        },
    }
}
```

### Hungarian Algorithm
```go
// Optimal assignment for move matching
func hungarian(costMatrix [][]float64) []int {
    // 1. Subtract row/column minimums
    // 2. Find augmenting paths
    // 3. Return optimal assignment
}

func (t *OptimizedTransformer) OptimalPlan(source, target *c4m.Manifest) (*Plan, float64) {
    // Model as bipartite graph
    candidates := t.findMoveCandidates(source, target, ...)

    // Find minimum-cost assignment
    assignment, cost := t.hungarianMatch(candidates)

    return plan, totalCost
}
```

### Sync Optimizer
```go
type SyncPlan struct {
    TransferFromRemote []*c4m.Entry  // Need network transfer
    LocalCopies        []Operation   // Can copy locally
    LocalMoves         []Operation   // Can move locally
    Deletions          []Operation   // To be deleted
    TransferBytes      int64         // Network transfer size
    LocalBytes         int64         // Local operation size
}

func (s *SyncOptimizer) Optimize(local, remote *c4m.Manifest) *SyncPlan {
    // Prioritizes: exact match > move > copy > transfer
}
```

## Benchmark Results

```
BenchmarkTransformSmall-16        10000     10834 ns/op    (~11µs)
BenchmarkStreamingTransform-16     9493     13722 ns/op    (~14µs)
BenchmarkTreeEditDistance-16       2775     44499 ns/op    (~44µs)
BenchmarkTreeSimilarity-16         6276     20952 ns/op    (~21µs)
BenchmarkEnhancedTransform-16       514    231129 ns/op   (~231µs)
BenchmarkSequenceDetection-16      1657     68229 ns/op    (~68µs)
BenchmarkOptimalPlan-16            1185     98530 ns/op    (~99µs)
BenchmarkHungarian-16              7105     17077 ns/op    (~17µs)
BenchmarkSyncOptimizer-16          3820     28049 ns/op    (~28µs)
BenchmarkParallelExecutor-16       2706     47868 ns/op    (~48µs)
BenchmarkFindMissing-16              55   2127035 ns/op    (~2.1ms)
```

## Test Coverage

- **70 tests** covering all functionality
- All tests passing
- Benchmarks for performance validation

## Integration Points

### C4ChangeSetTrait (c4d-openassetio)
```go
// Compute change set between versions
transformer := NewTransformer(nil)
plan, _ := transformer.Transform(baseManifest, targetManifest)

changeset := &ChangeSet{
    AddedCount:    plan.Stats.Adds,
    ModifiedCount: plan.Stats.Modifies,
    RemovedCount:  plan.Stats.Deletes,
    DeltaSize:     plan.Stats.BytesToAdd,
}
```

### Smart Sync
```go
// Optimize sync between peers
optimizer := NewSyncOptimizer(BandwidthAwareCostConfig(100, 10))
syncPlan := optimizer.Optimize(localManifest, remoteManifest)

fmt.Printf("Transfer: %d bytes over network\n", syncPlan.TransferBytes)
fmt.Printf("Local ops: %d bytes (moves + copies)\n", syncPlan.LocalBytes)
```

### Sequence Validation
```go
// Detect missing frames in render output
sequences := DetectSequences(manifest)
for _, seq := range sequences {
    if missing := seq.MissingFrames(); len(missing) > 0 {
        fmt.Printf("%s: missing frames %v\n", seq.Pattern, missing)
    }
}
```

## API Reference

### Core Types
- `Transformer` - Basic transformation
- `StreamingTransformer` - Large manifest support
- `EnhancedTransformer` - Full feature set
- `OptimizedTransformer` - Cost optimization

### Key Functions
- `Transform()` - Generate transformation plan
- `TransformStreaming()` - Stream operations
- `TransformEnhanced()` - With dir/attr/seq detection
- `OptimalPlan()` - Minimum-cost plan
- `SyncOptimizer.Optimize()` - Sync planning

### Utility Functions
- `FindMissing()` - Entries in target not in source
- `FindExtra()` - Entries in source not in target
- `FindCommon()` - Entries in both
- `DetectSequences()` - Find frame sequences
- `CompareSequences()` - Diff sequences
- `ComparePlans()` - Compare plan costs

## Conclusion

The transform package is now fully revived with all planned features:

| Phase | Status | Key Deliverables |
|-------|--------|------------------|
| 1. Core Revival | ✅ | Direct C4 ID comparison, consolidated indexing, true copy detection |
| 2. Performance | ✅ | Streaming, parallel execution, memoized tree edit distance |
| 3. Enhanced | ✅ | Directory ops, attribute detection, sequence intelligence |
| 4. Optimization | ✅ | Hungarian algorithm, cost functions, sync optimizer |

The package is ready for integration with c4d-openassetio for:
- C4ChangeSetTrait implementation
- Smart sync between c4d peers
- Transfer optimization
- Sequence validation
