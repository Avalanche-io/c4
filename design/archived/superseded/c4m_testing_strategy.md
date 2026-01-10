# C4M Testing Strategy

## Core Insight: C4M as Changesets

Each C4M file represents a changeset that transforms the previous state into a new state. This mental model fundamentally changes how we should test the system.

## Testing Requirements

### 1. Synthetic Filesystem Generator

We need a deterministic way to generate large filesystem structures without actual disk I/O:

```go
type SyntheticFS struct {
    seed      int64
    maxDepth  int
    maxWidth  int
    fileCount int
}

// Generate entries progressively, simulating discovery order
func (s *SyntheticFS) GenerateEntries(count int, startFrom int) []*Entry
```

### 2. Changeset Simulation

Test the progressive nature of scanning:

```go
type ChangesetSimulator struct {
    baseState    *Manifest
    discoveries  chan *Entry  // Simulates finding new files
    updates      chan *Update // Simulates C4 ID calculations completing
}

// Simulates realistic scan progression:
// - Files appear in discovery order (not sorted)
// - Directories may appear before their contents
// - C4 IDs may be calculated asynchronously
// - Must maintain sort invariants within each chunk
```

### 3. Bundle Chain Validation

Test that @base chains work correctly:

```go
type BundleChainTest struct {
    chunks []string // Raw C4M content for each chunk
}

func (b *BundleChainTest) ValidateChain() error {
    // Verify each chunk:
    // 1. Is valid C4M
    // 2. Has correct @base reference
    // 3. Maintains sort order within chunk
    // 4. Doesn't duplicate entries from previous chunks
    // 5. Correctly handles path context repetition
}

func (b *BundleChainTest) Materialize() (*Manifest, error) {
    // Follow @base chain to build complete manifest
    // Verify final result is correctly sorted
}
```

### 4. Progressive Sort Testing

Test that sorting works correctly for incremental additions:

```go
func TestProgressiveSort(t *testing.T) {
    scenarios := []struct {
        name     string
        existing []*Entry     // Already sorted entries
        new      []*Entry     // New discoveries (unsorted)
        expected []*Entry     // Expected final order
    }{
        {
            name: "add_files_to_existing_dir",
            // Existing: dir1/, dir1/a.txt, dir2/
            // New: dir1/b.txt, dir1/c.txt
            // Expected: dir1/, dir1/a.txt, dir1/b.txt, dir1/c.txt, dir2/
        },
        {
            name: "add_nested_directory",
            // Tests path repetition requirement
        },
    }
}
```

### 5. Memory-Constrained Testing

Simulate memory pressure:

```go
type MemoryConstrainedScanner struct {
    maxEntriesInMemory int
    flushThreshold     int
}

// Should trigger chunk writes when thresholds are hit
// Verify chunks are valid and chain correctly
```

### 6. Update/Modification Testing

Test that entries can be updated in later chunks:

```go
func TestEntryUpdates(t *testing.T) {
    // Chunk 1: file.txt without C4 ID
    // Chunk 2: @layer update adding C4 ID to file.txt
    // Verify materialized view has the C4 ID
}
```

## Test Data Patterns

### Pattern 1: Wide Directory
- 1M files in single directory
- Tests memory chunking
- Tests sort performance

### Pattern 2: Deep Hierarchy
- 1000 levels deep, few files per level
- Tests path repetition
- Tests depth handling

### Pattern 3: Mixed Evolution
- Start with structure A
- Progressively discover structure B nested within A
- Add files to A while exploring B
- Tests interleaved discovery

### Pattern 4: Large Files Discovery
- Discover files first without sizes/C4 IDs
- Update with sizes as "stat" completes
- Update with C4 IDs as hashing completes
- Tests @layer updates

## Implementation Plan

1. **Phase 1: Synthetic FS Generator**
   - Create deterministic filesystem generator
   - Generate entries with controllable patterns
   - Include timing simulation for progressive discovery

2. **Phase 2: Changeset Framework**
   - Build framework for expressing filesystem changes
   - Implement changeset-to-C4M converter
   - Add validation for changeset semantics

3. **Phase 3: Bundle Chain Testing**
   - Create test harness for multi-chunk bundles
   - Implement chain materialization
   - Add chain validation tests

4. **Phase 4: Performance Testing**
   - Memory usage monitoring
   - Sort performance with large datasets
   - Bundle size optimization

5. **Phase 5: Integration Tests**
   - Full scan simulation with interruption/resume
   - Multi-version bundle creation
   - Concurrent scan testing

## Success Metrics

1. Can generate and validate 10M+ entry filesystems without real disk I/O
2. Bundle chains correctly materialize regardless of chunk boundaries
3. Sort invariants maintained within and across chunks
4. Memory usage stays within configured limits
5. Resume correctly continues from interruption point
6. Updates/modifications correctly overlay previous entries

## Example Test Case

```go
func TestRealisticScan(t *testing.T) {
    fs := NewSyntheticFS(seed: 42, maxDepth: 10, maxWidth: 1000)
    scanner := NewProgressiveScanner(maxEntriesPerChunk: 100)

    // Simulate progressive discovery
    for i := 0; i < 1000; i++ {
        entries := fs.DiscoverNext(10) // Find 10 more entries
        scanner.Add(entries)

        if scanner.ShouldFlush() {
            chunk := scanner.CreateChunk()
            validateChunk(t, chunk)
        }
    }

    // Validate complete chain
    bundle := scanner.GetBundle()
    manifest := bundle.Materialize()

    assert.Equal(t, 10000, len(manifest.Entries))
    assert.True(t, manifest.IsSorted())
}
```

## Key Invariants to Test

1. **Sort Invariant**: Files before directories at same level
2. **No Duplication**: Entries appear exactly once across all chunks
3. **Path Context**: Nested additions include parent path
4. **Chain Integrity**: Each chunk correctly references previous
5. **Materialization**: Complete chain produces valid sorted manifest
6. **Update Semantics**: Later updates override earlier entries
7. **Memory Bounds**: Scanner respects memory limits

This testing strategy treats C4M files as versioned changesets rather than static snapshots, which aligns with the actual use case of progressive filesystem scanning.