# Compartmentalized Bundle Chunking Design

## Core Principle

When a subdirectory would cause the current chunk to exceed limits, create an autonomous sub-scan with its own @base chain for that directory. Otherwise, include the subdirectory inline with its full tree structure.

## Design Rules

### Rule 1: Small Directories
Directories with total entries below the chunk threshold are included inline in their parent's manifest:
```c4m
drwxr-xr-x timestamp size small_dir/
  -rw-r--r-- timestamp size file1.txt c4...
  -rw-r--r-- timestamp size file2.txt c4...
```

### Rule 2: Large Directories  
When a directory will exceed chunk limits, it becomes an autonomous sub-scan:

1. **Start sub-scan**: Begin new chunk chain for this directory
2. **Build chain**: Create @base-linked chunks as needed
3. **Complete scan**: Finish with final chunk or snapshot
4. **Reference by ID**: Parent only contains: `drwxr-xr-x timestamp size large_dir/ c4[final_id]`

## Example Structure

Given a 10k file chunk limit:
```
scan_root/
├── huge_dir1/         [11k files - separate chain]
├── huge_dir2/         [11k files - separate chain]  
├── huge_dir3/         [11k files - separate chain]
├── huge_dir4/         [11k files - separate chain]
├── huge_dir5/         [11k files - separate chain]
└── complex_dir/       [5k files total - inline]
    ├── subdir1/
    │   ├── deeply/
    │   │   └── nested/
    │   │       └── files...
    │   └── more_files...
    ├── subdir2/
    │   └── [various files and dirs]
    └── [entire tree structure included]
```

Results in 11 C4M files:
- **5 × 2-chunk chains**: Each huge_dir gets chunk1→chunk2 with @base
- **1 root chunk**: Contains references to 5 huge_dirs + entire complex_dir inline

The root chunk would look like:
```c4m
drwxr-xr-x timestamp totalsize scan_root/
  drwxr-xr-x timestamp size huge_dir1/ c4[final_chunk_id]
  drwxr-xr-x timestamp size huge_dir2/ c4[final_chunk_id]
  drwxr-xr-x timestamp size huge_dir3/ c4[final_chunk_id]
  drwxr-xr-x timestamp size huge_dir4/ c4[final_chunk_id]
  drwxr-xr-x timestamp size huge_dir5/ c4[final_chunk_id]
  drwxr-xr-x timestamp size complex_dir/
    -rw-r--r-- timestamp size file1.txt c4...
    drwxr-xr-x timestamp size subdir1/
      drwxr-xr-x timestamp size deeply/
        drwxr-xr-x timestamp size nested/
          [... all 5k files with full tree structure ...]
```

## Benefits

### 1. Intelligent Chunking
- Large directories become self-contained chains
- Small directories stay inline with full structure
- Natural breakpoints based on size, not depth

### 2. Autonomous Chunks
- Each large directory's chain is complete and independent
- Can be processed/verified separately
- No dependencies on parent context

### 3. Reusability
- Scanning the same large directory produces the same chunk chain
- Enables caching and deduplication
- Useful for common directories (node_modules, .git, etc.)

### 4. Parallel Processing
- Large subdirectories can be scanned concurrently
- Each produces its own chunk chain
- Parent waits for ID, not full content

### 5. User Reasoning
- Easy to understand: "This big directory got its own set of chunks"
- Natural mapping to filesystem structure
- Can explore subdirectory chains independently

## Implementation Strategy

### Phase 1: Detection
When entering a directory, estimate if it will need chunking:
- Quick count of entries
- Check against threshold
- Decide: inline or separate chain

### Phase 2: Sub-scan
If separate chain needed:
```go
func scanLargeDirectory(path string) (c4.ID, error) {
    // Create new chunking context
    subContext := NewChunkContext()
    
    // Scan directory with its own chain
    scanDir(path, subContext)
    
    // Return final ID
    return subContext.FinalID(), nil
}
```

### Phase 3: Parent Reference
Parent manifest simply includes:
```go
entry := Entry{
    Name: "large_dir",
    Mode: dirMode,
    Size: totalSize,
    C4ID: largeDirID, // From sub-scan
}
```

## Threshold Decisions

### When to Create Separate Chain
- Entry count > 50% of chunk limit?
- Estimated size > 50% of size limit?
- Depth-based (always separate for certain paths)?

### Adaptive Thresholds
- Learn from scanning patterns
- Common large directories (node_modules, .git, vendor)
- User-configured rules

## Edge Cases

### 1. Nested Large Directories
- Each gets its own chain
- Parent chain references child chain ID
- Natural recursion

### 2. Extremely Flat Directory
- Single directory with millions of files
- Creates single @base chain
- No subdirectories to compartmentalize

### 3. Mixed Content
- Directory with some files and some large subdirs
- Files go in parent manifest
- Large subdirs get separate chains

## Future Optimizations

### 1. Predictive Chunking
- Recognize patterns (node_modules, .git)
- Start sub-scan immediately
- No need to count first

### 2. Chunk Caching
- Store completed chains by content hash
- Skip rescanning identical large directories
- Massive speedup for common patterns

### 3. Distributed Scanning
- Farm out large subdirectories to workers
- Each produces independent chain
- Merge results by ID reference

## Comparison with Flat Approach

### Flat Approach (Previous)
- Mix entries from different directory levels
- Complex count management
- Hard to reason about chunk contents
- No reusability

### Compartmentalized Approach (New)
- Each chunk is single-directory
- Simple: complete directory = complete chain
- Natural filesystem mapping
- Highly reusable and cacheable

## Conclusion

This compartmentalized approach treats the filesystem as a hierarchy of potentially independent scans, where large directories become self-contained units with their own chunk chains. This maps naturally to how users think about filesystems and enables powerful optimizations.