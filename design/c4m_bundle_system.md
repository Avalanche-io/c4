# C4M Bundle System Requirements

## Overview

The C4M Bundle System enables C4 to handle unbounded filesystems by chunking output into multiple C4M files stored in a structured bundle format. This system allows for incremental output, resumable scans, and versioning of filesystem snapshots.

## Motivation

Current C4M generation requires holding entire directory structures in memory before output, which fails for:
- Filesystems with millions of files
- Directories with hundreds of thousands of entries
- Deep directory hierarchies
- Long-running scans that may be interrupted

## Bundle Structure

```
name.c4m_bundle/
├── header.c4      # Single C4 ID pointing to header manifest
└── c4/            # Content-addressed storage directory
    ├── c41abc...  # Header manifest file
    ├── c42def...  # Path text files
    ├── c43ghi...  # Progress chunk files
    └── c44jkl...  # Snapshot files
```

### Header Manifest Format

The header.c4 file contains a single C4 ID pointing to the actual header manifest in the c4/ directory.

The header manifest uses C4M format to describe the bundle structure:

```c4m
@c4m 1.0
d--------- - - scans/
  d--------- 2025-09-01T14:30:22Z - 1/
    ---------- 2025-09-01T14:30:22Z 24 path.txt c42def...
    ---------- 2025-09-01T14:45:18Z - snapshot.c4m c43ghi...
    d--------- 2025-09-01T14:30:25Z - progress/
      ---------- 2025-09-01T14:30:25Z - 1.c4m c44jkl...
      ---------- 2025-09-01T14:31:12Z - 2.c4m c45mno...
```

### Metadata Encoding

- **Scan number**: Directory name under scans/ (1, 2, 3, ...)
- **Scan start time**: Modification time of scan directory
- **Scan completion**: Presence and modification time of snapshot.c4m
- **Scan path**: Content of path.txt file
- **Progress order**: Sequential naming of progress chunks (1.c4m, 2.c4m, ...)

## Chunking Strategy

### Dimension 1: Directory Width

When a directory contains too many entries, use @base notation:

```c4m
# Chunk 1: First 100k entries
@c4m 1.0
-rw-r--r-- 2025-01-01T00:00:00Z 1024 file1.txt c41...
# ... up to configured limit

# Chunk 2: Next 100k entries  
@c4m 1.0
@base c4[chunk1_id]
-rw-r--r-- 2025-01-01T00:00:00Z 2048 file100001.txt c42...
```

### Dimension 2: Tree Depth

When a subdirectory is complete and large, replace with its C4 ID:

```c4m
@c4m 1.0
drwxr-xr-x 2025-01-01T00:00:00Z - projects/
  drwxr-xr-x 2025-01-01T00:00:00Z - vendor/ c4[vendor_manifest_id]
  -rw-r--r-- 2025-01-01T00:00:00Z 1024 readme.txt c45...
```

### Chunk Triggers

Create a new chunk when:
1. **Entry limit**: Chunk reaches configured entry count (e.g., 100k)
2. **Size limit**: Chunk buffer exceeds memory threshold (e.g., 100MB)
3. **Time limit**: Elapsed time since last chunk (e.g., 30 seconds)
4. **Subdirectory completion**: Large subdirectory can be compacted
5. **User interrupt**: Graceful shutdown saves current progress

## Configuration

### Limits

```go
type BundleConfig struct {
    MaxEntriesPerChunk  int    // Default: 100,000
    MaxBytesPerChunk    int64  // Default: 100MB
    MaxChunkInterval    time.Duration // Default: 30s
    CompactThreshold    int    // Entries before compacting subdir
    BundleDir          string  // Output directory for bundle
}
```

### Development Mode

```go
const (
    DEV_MODE = true
    
    // Development limits for easier testing
    DEV_ENTRIES_PER_CHUNK = 10
    DEV_COMPACT_THRESHOLD = 50
    
    // Production limits
    PROD_ENTRIES_PER_CHUNK = 100000
    PROD_COMPACT_THRESHOLD = 50000
)
```

## Operations

### Create New Bundle

```bash
c4 --bundle /path/to/scan
```

Creates:
- New bundle directory named after path + timestamp
- Initial scan directory (scan #1)
- path.txt with scan path
- Begins writing progress chunks

### Resume Interrupted Scan

```bash
c4 --bundle --resume path.c4m_bundle/
```

Behavior:
- Finds highest numbered incomplete scan (missing snapshot.c4m)
- Reads last progress chunk to determine resume point
- Continues scanning from last known position
- Appends new progress chunks

### Start New Scan

```bash
c4 --bundle path.c4m_bundle/
```

If bundle exists:
- Creates new scan directory (next number)
- Compares path.txt to determine if same source
- Begins fresh scan with new progress chunks

### Complete Scan

When scan finishes:
1. Write final progress chunk
2. Generate snapshot.c4m pointing to last chunk via @base
3. Update header manifest with snapshot entry
4. Recompute header.c4 ID

## Progress Chunk Format

Each progress chunk is a valid C4M file:

```c4m
@c4m 1.0
@base c4[previous_chunk_id]  # Omitted for first chunk
# Entries for this chunk
-rw-r--r-- 2025-01-01T00:00:00Z 1024 file1.txt c41...
drwxr-xr-x 2025-01-01T00:00:00Z - subdir/
  -rw-r--r-- 2025-01-01T00:00:00Z 2048 file2.txt c42...
```

## Snapshot Generation

The snapshot.c4m provides the complete view:

```c4m
@c4m 1.0
@base c4[last_progress_chunk_id]
# Complete view achieved by following @base chain
# May include additional compaction or reorganization
```

## Bundle Validation

A bundle is valid if:
1. header.c4 contains a valid C4 ID
2. Referenced C4 ID exists in c4/ directory
3. Header manifest is valid C4M format
4. All referenced C4 IDs in header exist in c4/
5. Progress chunks form valid @base chain
6. Snapshot (if present) correctly references final chunk

## Error Handling

### Corrupted Bundle
- Detect via C4 ID mismatch
- Offer to rebuild header from c4/ contents
- Identify last valid progress chunk

### Missing Files
- Report missing C4 IDs
- Allow partial recovery
- Skip to next valid chunk

### Concurrent Access
- Use file locking on header.c4
- Atomic writes for new chunks
- Safe for reading while writing

## Future Extensions

### Phase 1 (Current)
- Basic bundle creation
- Progress chunking
- Simple resume capability

### Phase 2
- Subdirectory compaction
- Memory-based chunking triggers
- Optimized @base chain loading

### Phase 3
- SQLite index for searching
- Compression of chunks
- Network bundle transfer
- Differential scans

### Phase 4
- Merge multiple bundles
- Bundle garbage collection
- Distributed scanning coordination

## Testing Strategy

### Unit Tests
- Chunk size limits
- @base chain construction
- Header manifest generation
- C4 ID computation

### Integration Tests
- Small filesystem with DEV_MODE limits
- Resume after interrupt
- Multiple scans of same path
- Corrupt bundle recovery

### Performance Tests
- Million file directory
- Deep hierarchy (1000+ levels)
- Large file handling
- Memory usage under limits

## Success Criteria

1. Successfully scan filesystem with 10M+ files
2. Resume interrupted scan without re-processing
3. Output chunks continuously during scan
4. Maintain memory usage under 1GB regardless of filesystem size
5. Complete scan produces valid C4M via @base chain
6. Bundle format is self-describing and tool-independent