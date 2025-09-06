# Simplified Scanner Design

## Core Principles
1. **Separate concerns** - Scanning, chunking, and writing are separate phases
2. **Immutable state** - Don't modify entries after creation
3. **Clear ownership** - Each directory owns its content organization
4. **Testable units** - Each function does one thing well

## Key Data Structures

```go
type ScanContext struct {
    rootPath      string        // Original scan path
    currentPath   string        // Current directory being scanned
    currentDepth  int          // Current depth in tree
    isCollapsed   bool         // Are we in a collapsed directory?
    entries       []*Entry     // Accumulated entries
}

type DirectoryPlan struct {
    path          string
    depth         int
    isCollapsed   bool
    files         []os.DirEntry  // Files to process
    regularDirs   []os.DirEntry  // Regular subdirs
    collapsedDirs []string       // Paths of dirs to collapse
}
```

## Processing Pipeline

### Phase 1: Planning
```
planDirectory(path, depth) -> DirectoryPlan
  1. Read directory entries
  2. Separate files and directories
  3. Sort files naturally
  4. Sort directories naturally
  5. Identify which directories should be collapsed
  6. Return organized plan
```

### Phase 2: Execution
```
executeDirectoryPlan(plan, context) -> []*Entry
  1. Add directory entry (if not root)
  2. Process all files -> add entries
  3. Process regular subdirectories -> recurse
  4. Process collapsed directories -> special handling
  5. Return all entries in correct order
```

### Phase 3: Chunking
```
chunkEntries(entries, maxSize) -> []Chunk
  1. Accumulate entries up to limit
  2. When limit reached, finalize chunk
  3. Track continuation state if needed
  4. Handle @base directives properly
```

## Collapsed Directory Handling

```go
func processCollapsedDirectory(path string, originalDepth int) (c4.ID, error) {
    // Save current state
    savedState := scanner.currentState()
    
    // Create isolated context
    scanner.startCollapsedContext(path)
    
    // Scan as if from root (depth 0)
    entries := scanDirectoryRecursive(path, 0)
    
    // Chunk the entries
    chunks := chunkEntries(entries, maxSize)
    
    // Write chunks with @base linking
    lastID := writeCollapsedChunks(chunks)
    
    // Restore state
    scanner.restoreState(savedState)
    
    return lastID, nil
}
```

## Key Invariants to Maintain

1. **Entry Order Invariant**: For any parent directory, files come before subdirectories
2. **Depth Invariant**: Depth only increases by 1, can decrease by any
3. **Root Invariant**: Root directory never appears in output
4. **Collapsed Invariant**: Collapsed dir never appears in its own chunks
5. **Continuation Invariant**: Parent path is preserved in continuation chunks

## Testing Strategy

### Unit Tests
1. `TestFileBeforeDirectory` - Verify ordering at each depth
2. `TestDepthProgression` - Check depth changes are valid
3. `TestRootExclusion` - Ensure root never appears
4. `TestCollapsedDirectory` - Verify isolated chunking
5. `TestContinuation` - Check parent preservation

### Integration Tests
1. Small directory tree (10 files)
2. Directory requiring chunking (1000 files)
3. Nested collapsed directories
4. Mixed scenarios

### Property-Based Tests
- For any valid directory tree, output must satisfy all invariants
- Roundtrip: Parse output back and verify structure matches input