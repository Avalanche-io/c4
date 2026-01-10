# Bundle Chunking Implementation Notes

## Entry Count Management

When implementing chunking for C4M bundles, there are two distinct cases for managing the entry count:

### Case 1: Flat Directory Continuation (@base chain)

When we hit the entry limit while processing entries at the same directory level:

1. Write out all accumulated entries to a chunk
2. Next chunk starts with `@base` pointing to previous chunk
3. **Reset entry count to 0** - we've output everything accumulated
4. Continue processing remaining entries in the directory

Example:
```
Chunk 1: entries 1-100,000 of large_dir/
Chunk 2: @base chunk1, entries 100,001-200,000 of large_dir/
Count after flush: 0 (complete reset)
```

### Case 2: Subdirectory Compaction

When we complete scanning a large subdirectory and decide to compact it:

1. Complete scanning of subdirectory and all its children
2. Write subdirectory as separate chunk or reference
3. **Subtract exact subdirectory entry count** from current count
4. Continue with parent directory entries

Example:
```
Processing parent_dir/:
  - file1.txt (count: 1)
  - file2.txt (count: 2)
  - huge_subdir/ (50,000 entries)
  - file3.txt 

After compacting huge_subdir:
  - Output huge_subdir as separate chunk
  - Subtract 50,000 from count (not reset to 0)
  - Count is now 2 (for file1.txt and file2.txt)
  - Continue with file3.txt (count: 3)
```

## Key Difference

- **Flat continuation**: Reset to 0 because we're outputting accumulated entries
- **Subdirectory compaction**: Subtract specific count because parent may have other entries

## Implementation Considerations

1. Track entry counts at each directory level
2. Maintain stack of directory contexts during recursion
3. Calculate exact subtree sizes for proper subtraction
4. Decide compaction threshold (e.g., compact subdirs > 10,000 entries)

## Future Optimizations

1. **Parallel scanning**: Multiple workers for different subdirectories
2. **Smart compaction**: Compact based on size AND depth
3. **Incremental output**: Stream chunks as they're ready
4. **Memory management**: Limit in-memory accumulation