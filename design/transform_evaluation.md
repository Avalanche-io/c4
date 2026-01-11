# Transform Package Evaluation

## Overview

The archived transform package provides algorithms for computing minimum-operation transformation plans between filesystem manifests. It leverages C4 IDs for intelligent move/copy detection.

**Location**: `design/archived/transform/`
**Size**: ~630 lines implementation, ~470 lines tests
**Status**: Archived 2025-01-08, preserved for c4d integration

## Feature Summary

| Feature | Status | Quality |
|---------|--------|---------|
| Move/rename detection | ✅ Implemented | Good |
| Modification detection | ✅ Implemented | Good |
| Add/delete detection | ✅ Implemented | Good |
| Copy detection | ⚠️ Partial | Needs work |
| Tree edit distance | ✅ Implemented | Needs optimization |
| Operation ordering | ✅ Implemented | Good |
| Large manifest handling | ⚠️ Partial | Needs streaming |

## Code Quality Assessment

### Strengths

**1. Clean Architecture**
```go
// Well-defined operation types
type Operation struct {
    Type   OpType
    Source string
    Target string
    Entry  *c4m.Entry
}

// Configurable transformer
type Transformer struct {
    config    *Config
    idIndex   map[c4.ID][]*c4m.Entry
    pathIndex map[string]*c4m.Entry
}
```

**2. Phase-Based Processing**
Clear separation into detection phases:
1. Move detection (C4 ID matching)
2. Modification detection (same path, different ID)
3. Add/delete detection (remainder)
4. Operation optimization (reordering)

**3. Configurable Behavior**
```go
type Config struct {
    SimilarityThreshold float64  // For fuzzy matching
    DetectMoves         bool
    DetectCopies        bool
    MaxExactTreeSize    int
    UseHeuristics       bool
}
```

**4. Good Test Coverage**
- Unit tests for all major operations
- Benchmarks for large manifests (10,000 entries)
- Edge cases (empty manifests, no changes)

### Weaknesses

**1. Inefficient C4 ID Comparison**
```go
// Current: String comparison (allocates, slow)
if sourceEntry.C4ID.String() != targetEntry.C4ID.String() {

// Better: Direct comparison
if sourceEntry.C4ID != targetEntry.C4ID {
```

**2. Redundant Index Building**
Multiple methods rebuild similar indices:
```go
// In detectMoves()
targetPaths := make(map[string]*c4m.Entry)
sourcePaths := make(map[string]*c4m.Entry)

// In detectModifications()
targetMap := make(map[string]*c4m.Entry)

// In detectAddsDeletes()
sourceMap := make(map[string]*c4m.Entry)
targetMap := make(map[string]*c4m.Entry)
```
Should build once in `buildIndices()` and reuse.

**3. No True Copy Detection**
The code has `DetectCopies` config but doesn't actually emit `OpCopy`:
```go
if hasExactMatch && t.config.DetectCopies {
    // Skip move detection - let this be handled as an addition (copy)
    continue  // Just falls through to Add, not Copy
}
```

**4. Recursive Tree Edit Distance Without Memoization**
```go
func ComputeTreeEditDistance(source, target *TreeNode) int {
    // Recursive without caching - O(n^2) to O(n^3) depending on structure
    matchCost := ComputeTreeEditDistance(sourceChildren[i-1], targetChildren[j-1])
}
```
For large trees, this can be very slow.

**5. No Streaming/Incremental Processing**
All operations load full manifests into memory. For manifests with millions of entries, this could be problematic.

**6. Old c4m API**
Uses older API patterns that may not match current c4m implementation:
```go
manifest.AddEntry(entry)      // May need update
manifest.Entries              // Direct field access
manifest.Sort()               // May not exist
```

## Algorithm Analysis

### Move Detection
**Algorithm**: Build C4 ID → entries index, match IDs across manifests
**Complexity**: O(n + m) where n = source entries, m = target entries
**Quality**: Good, handles multiple files with same content correctly

### Modification Detection
**Algorithm**: Path comparison with C4 ID check
**Complexity**: O(n) with hash map lookup
**Quality**: Good, with fallback to size/timestamp when C4 ID unavailable

### Tree Edit Distance
**Algorithm**: Simplified APTED with DP for child matching
**Complexity**: O(n² × m²) worst case for unbalanced trees
**Quality**: Functional but could be optimized with Zhang-Shasha or APTED proper

### Operation Optimization
**Algorithm**: Priority-based sorting
**Priority Order**:
1. Deletes (free space first)
2. Moves (cheap, same filesystem)
3. Copies
4. Modifies
5. Adds (require new data)

**Quality**: Good heuristic for typical sync operations

## Missing Features for c4d Integration

### 1. Directory-Level Operations
Currently operates on files only. Should detect:
- Directory renames (all children move together)
- Directory copies
- Empty directory creation/deletion

### 2. Symlink/Hardlink Handling
No support for:
- Symlink creation/deletion/modification
- Hardlink detection (same C4 ID, same inode)

### 3. Attribute Operations
Doesn't track:
- Permission changes (same content, different mode)
- Timestamp updates
- Extended attributes

### 4. Sequence Awareness
Could leverage C4M sequence support:
- Detect frame range changes
- Optimize sequence operations

### 5. Graph-Based Optimization
For complex transforms, minimum-cost graph algorithms could help:
- Hungarian algorithm for optimal move matching
- Min-cost max-flow for multi-copy scenarios

### 6. Streaming Processing
For large manifests:
- Sorted merge for diff
- Bloom filters for existence checks
- Chunked processing

## Recommendations for Revival

### Phase 1: API Alignment

1. Update to current c4m API
2. Fix C4 ID comparison (use direct comparison)
3. Consolidate index building
4. Add proper error handling

```go
// Updated for current c4m
func (t *Transformer) Transform(source, target *c4m.Manifest) (*Plan, error) {
    // Use iterator pattern for large manifests
    sourceIter := source.Entries()
    targetIter := target.Entries()
    // ...
}
```

### Phase 2: Performance

1. Implement memoization for tree edit distance
2. Add streaming mode for large manifests
3. Parallel processing for independent operations

```go
// Memoized tree edit distance
type tedCache struct {
    mu    sync.RWMutex
    cache map[tedKey]int
}

func (c *tedCache) ComputeTreeEditDistance(s, t *TreeNode) int {
    key := tedKey{s.hash(), t.hash()}
    if v, ok := c.get(key); ok {
        return v
    }
    // ... compute and cache
}
```

### Phase 3: Enhanced Features

1. True copy detection with `OpCopy` emission
2. Directory-level operations
3. Attribute change detection
4. Sequence-aware operations

```go
// Enhanced operation types
const (
    OpAdd OpType = iota
    OpDelete
    OpModify
    OpMove
    OpCopy
    OpMkdir      // Create directory
    OpRmdir      // Remove empty directory
    OpChmod      // Permission change
    OpTouch      // Timestamp update
    OpSeqExtend  // Extend frame range
    OpSeqTrim    // Trim frame range
)
```

### Phase 4: Graph Optimization

1. Model as bipartite matching problem
2. Use Hungarian algorithm for optimal assignment
3. Consider bandwidth costs in optimization

```go
// Cost-based optimization
type TransformCost struct {
    MoveCost   func(src, dst string) float64  // e.g., same filesystem = cheap
    CopyCost   func(entry *Entry) float64     // e.g., proportional to size
    DeleteCost func(entry *Entry) float64
    AddCost    func(entry *Entry) float64     // Network transfer cost
}

func (t *Transformer) OptimalPlan(source, target *c4m.Manifest, costs TransformCost) (*Plan, error) {
    // Build cost matrix
    // Apply Hungarian algorithm
    // Return minimum-cost plan
}
```

## Integration with c4d-openassetio

### Use Cases

1. **Change Set Computation**
   - Compute C4ChangeSetTrait data
   - Efficient version comparison
   - Delta transfer planning

2. **Smart Sync**
   - Minimize network transfer
   - Detect local renames (no re-download)
   - Prioritize operations

3. **Conflict Detection**
   - Identify concurrent modifications
   - Three-way merge support
   - Conflict resolution strategies

4. **Storage Optimization**
   - Identify duplicate content
   - Plan deduplication
   - Estimate storage savings

### Proposed Package Structure

```
c4d/
├── sync/
│   ├── transform/           # Core transform algorithms
│   │   ├── transform.go
│   │   ├── operations.go
│   │   ├── tree.go
│   │   └── optimize.go
│   ├── diff/                # Fast diff algorithms
│   │   ├── manifest.go      # Manifest comparison
│   │   ├── content.go       # Content-level diff
│   │   └── streaming.go     # Large manifest support
│   ├── plan/                # Execution planning
│   │   ├── plan.go
│   │   ├── cost.go
│   │   └── schedule.go
│   └── execute/             # Plan execution
│       ├── executor.go
│       ├── rollback.go
│       └── progress.go
```

## Benchmark Results (from archived tests)

```
BenchmarkTransformLargeManifest-8    100    15234521 ns/op
BenchmarkFindMissing-8               500     2876543 ns/op
```

- Transform of 10,000 entries: ~15ms
- FindMissing on 5,000 vs 5,000: ~3ms

These are reasonable but could be improved with optimization.

## Conclusion

The transform package provides a solid foundation for manifest diff and transformation. Key improvements needed:

1. **Must Fix**: C4 ID comparison, API alignment
2. **Should Fix**: Redundant indexing, copy detection
3. **Nice to Have**: Graph optimization, streaming, parallelization

**Recommendation**: Revive as `c4d/sync/transform` with Phase 1 fixes immediately, Phase 2-4 as the sync system matures.

The package is particularly valuable for:
- C4ChangeSetTrait implementation
- Smart sync between c4d peers
- OpenAssetIO version comparison features
- Transfer optimization
