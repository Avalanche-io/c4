# Transform Package (Archived)

## Status: Archived for Future Use

This package was removed from `c4m/transform/` on 2025-01-08 to reduce complexity in the c4m package. The code is preserved here for potential future use in c4d or a dedicated sync package.

## Purpose

Efficient algorithms for transforming one filesystem manifest into another with minimum operations. Key features:

- **Move/Rename Detection**: Uses C4 IDs to detect when files have moved rather than been deleted and re-added
- **Copy Detection**: Identifies when the same content appears in multiple new locations
- **Modification Detection**: Tracks content changes at the same path
- **Tree Edit Distance**: Measures structural similarity between filesystem trees
- **Operation Optimization**: Orders operations for efficiency (deletes first to free space, then moves, then adds)

## Use Cases

1. **Smart Sync/Backup**: Minimize data transfer by detecting moves/copies
2. **Bandwidth Optimization**: For c4d synchronization between nodes
3. **Change Visualization**: Show users what changed between snapshots
4. **Similarity Detection**: Compare filesystem structures

## Files

- `transform.go` - Core implementation (~627 lines)
- `transform_test.go` - Comprehensive tests and benchmarks (~469 lines)

## Revival Instructions

To use this code again:

1. Create a new package (e.g., `c4d/sync/transform` or standalone `c4-transform`)
2. Copy the `.go` files
3. Update import paths
4. Run tests to verify functionality

## Why Archived

The c4m package focused on manifest generation/parsing. Transform operations are more relevant to:
- c4d daemon sync operations
- Dedicated sync tooling
- Higher-level orchestration

Given c4's content-addressable nature, simpler sync approaches may suffice:
1. Compare manifests by C4 ID
2. Fetch missing C4 IDs from peers
3. Let storage layer deduplicate

The transform package's move detection is elegant but may be over-engineering for simple "fetch missing blobs" sync.
