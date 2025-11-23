# C4M Implementation Notes

## Purpose
This document contains implementation notes, clarifications, and lessons learned while developing the C4M parser and validator. It supplements the formal [C4M Specification](./SPECIFICATION.md) with practical implementation details.

For the formal specification, see [SPECIFICATION.md](./SPECIFICATION.md).
For user documentation, see [README.md](./README.md).

## Overview
These notes clarify edge cases, parser tolerances, and implementation decisions made during development.

## Entry Format

### Required Fields
ALL entries (files, directories, symlinks, special files) MUST have a minimum of 4 fields:

1. **Mode** (10 characters or "-" for null)
2. **Timestamp** (ISO8601 format or "-" for null)
3. **Size** (non-negative integer, "0" minimum, or "-" for null)
4. **Name** (valid filesystem name)
5. **[Optional] C4 ID** (for files with content)

### Critical Rule
**No field may be omitted before the name field.** This prevents ambiguous parsing where a directory named "0 dirname" without a size field would be misparsed.

## Directory Entries

### Size Field Semantics
- **Size represents the sum of all content** within the directory and all subdirectories
- **Size is content only**, not filesystem metadata (blocks, inodes, etc.)
- **Size 0 is canonical** when the directory contains only:
  - Empty files (size 0)
  - Empty subdirectories
  - No content at all

### Why Metadata Is Not Tracked
- Different filesystems (ext4, NTFS, APFS, ZFS) allocate different metadata sizes
- C4 requires representational immutability across all systems
- Same content must produce same C4 ID everywhere

### Directory Name Parsing

#### Canonical Form
```
drwxr-xr-x 2025-09-19T12:00:00Z 1024 my directory name/
                                  ^                    ^
                             size field          name extends to /
```
- Name starts one space after size field
- Name extends until the forward slash `/`
- Spaces in names do NOT require quotes or escaping
- Special characters (newlines, etc.) must be escaped with `\`

#### Non-Canonical But Tolerated (with warning)
- Quoted directory names: `"dirname"/` or `"dirname/"`
- Size value of `-` (null) requiring calculation
- Size value of `0` when actual sum is non-zero (requires recalculation)

#### Invalid Directory Names
- `.` (current directory)
- `..` (parent directory)
- `/` (root)
- Empty string before `/`
- Names containing null bytes
- Names exceeding filesystem limits

## File Entries

### Format
```
-rw-r--r-- 2025-09-19T12:00:00Z 4096 filename.txt c44aMtvPeo...
```
- Size represents actual file content in bytes
- C4 ID is required for non-empty files
- C4 ID may be "-" or omitted for empty files

## Symlink Entries

### Format
```
lrwxrwxrwx 2025-09-19T12:00:00Z 7 link -> target c44aMtvPeo...
```
- Size is the length of the target path in bytes
- Target is preceded by ` -> `
- C4 ID is of the link target path string, not the pointed-to content

## Special Files

### Block/Character Devices, Pipes, Sockets
```
brw-rw---- 2025-09-19T12:00:00Z 0 sda
crw-rw-rw- 2025-09-19T12:00:00Z 0 null
prw-r--r-- 2025-09-19T12:00:00Z 0 mypipe
srwxr-xr-x 2025-09-19T12:00:00Z 0 mysocket.sock
```
- Size is typically 0 (no content)
- No C4 ID (no content to hash)

## Timestamp Handling

### Canonical Format
The canonical format **requires UTC-only timestamps** using a strict subset of RFC3339:
```
2006-01-02T15:04:05Z
```
- MUST end with 'Z' indicating UTC
- No timezone offsets allowed in canonical output
- All timestamps are converted to UTC before formatting

### Parser Flexibility
The parser accepts various ergonomic timestamp formats and converts them to UTC:

1. **Canonical UTC** (preferred):
   ```
   2025-09-19T20:49:47Z
   ```

2. **RFC3339 with timezone offset** (converted to UTC):
   ```
   2025-09-19T20:49:47-05:00  # CDT - becomes 2025-09-20T01:49:47Z
   2025-09-19T20:49:47+09:00  # JST - becomes 2025-09-19T11:49:47Z
   ```

3. **Pretty format with timezone** (converted to UTC):
   ```
   Sep 19 20:49:47 2025 CDT   # becomes 2025-09-20T01:49:47Z
   Jan  2 15:04:05 2006 MST   # note: space padding after month
   ```

4. **Unix date format** (converted to UTC):
   ```
   Mon Jan  2 15:04:05 MST 2006
   ```

### Important Notes
- All non-UTC timestamps are automatically converted to UTC internally
- The pretty format may display local timezone for readability
- The canonical format ALWAYS outputs UTC with 'Z' suffix
- Null timestamp is represented as "-" or Unix epoch (1970-01-01T00:00:00Z)

## Parser Requirements

### Must Fail On
- Missing any of the 4 required fields
- Malformed mode (not 10 chars or "-")
- Unparseable timestamp
- Non-numeric size (except "-")
- Invalid directory names (., .., /)

### Must Warn On (suppressible)
- Non-canonical size values for directories
- Quoted directory names
- Deprecated formats
- Non-UTC timestamps in strict validation mode

### Must Handle Gracefully
- Ergonomic format variations
- Different timestamp formats (converting to UTC)
- Comma-separated numbers in ergonomic form
- Variable whitespace (properly normalized)

## Validation Levels

### Strict Mode
- Enforces canonical format exactly
- Fails on warnings
- Checks sort order
- Verifies size calculations

### Normal Mode
- Accepts non-canonical but valid formats
- Issues warnings
- Allows recalculation of sizes

### Lenient Mode
- Maximum compatibility
- Minimal warnings
- Attempts recovery from errors

## Size Calculation Examples

### Example 1: Mixed Content
```
drwxr-xr-x 2025-09-19T12:00:00Z 150 mydir/
  -rw-r--r-- 2025-09-19T12:00:00Z 100 file1.txt c4...
  -rw-r--r-- 2025-09-19T12:00:00Z 50 file2.txt c4...
  drwxr-xr-x 2025-09-19T12:00:00Z 0 emptydir/
```
Directory size: 100 + 50 + 0 = 150 ✓

### Example 2: Empty Files
```
drwxr-xr-x 2025-09-19T12:00:00Z 0 emptydir/
  -rw-r--r-- 2025-09-19T12:00:00Z 0 empty1.txt c4...
  -rw-r--r-- 2025-09-19T12:00:00Z 0 empty2.txt c4...
```
Directory size: 0 + 0 = 0 ✓ (canonical)

### Example 3: Nested Directories
```
drwxr-xr-x 2025-09-19T12:00:00Z 300 root/
  drwxr-xr-x 2025-09-19T12:00:00Z 200 subdir1/
    -rw-r--r-- 2025-09-19T12:00:00Z 200 data.bin c4...
  -rw-r--r-- 2025-09-19T12:00:00Z 100 readme.txt c4...
```
Directory sizes:
- subdir1: 200
- root: 200 + 100 = 300 ✓

## Common Errors and Solutions

### Error: Missing Size Field
```
Wrong:  drwxr-xr-x 2025-09-19T12:00:00Z dirname/
Right:  drwxr-xr-x 2025-09-19T12:00:00Z 0 dirname/
```

### Error: Ambiguous Parsing
```
Wrong:  drwxr-xr-x 2025-09-19T12:00:00Z 0 dirname/subdir/
                                         ^        ^
                                    Is this size or name?

Right:  drwxr-xr-x 2025-09-19T12:00:00Z 0 "0 dirname"/subdir/
        (If directory is actually named "0 dirname")
```

### Error: Including Metadata in Size
```
Wrong:  drwxr-xr-x 2025-09-19T12:00:00Z 4096 dirname/  # Block size
Right:  drwxr-xr-x 2025-09-19T12:00:00Z 150 dirname/   # Content sum
```

## Implementation Notes

1. Parsers should track line numbers for error reporting
2. Validators should accumulate errors for batch reporting
3. Size calculations should be verified in strict mode
4. Warning suppresssion should be configurable
5. Output formatting should preserve canonical form when possible

## Bundle Extraction

The extract command supports two output formats:

### Pretty Format (Default)
```bash
c4 extract bundle_dir [output.c4m]
```
- Human-readable timestamps with timezone
- Aligned columns for better readability
- Size values may include commas or units

### Canonical Format
```bash
c4 extract --canonical bundle_dir [output.c4m]
```
- Strict UTC timestamps with 'Z' suffix
- Minimal formatting for machine processing
- Exact byte counts without formatting

Both formats preserve the complete manifest structure including @base references for proper reconstruction of unbounded filesystem scans.

## Canonical Form and C4 ID Computation

**CRITICAL REQUIREMENT**: C4 IDs MUST only be computed from manifests in canonical form.

### The Problem

The c4m package currently allows computing C4 IDs from manifests containing null values (Mode=0, Timestamp=Unix(0), Size=-1, C4ID=nil). This creates **non-deterministic identification** where the same filesystem content can produce different C4 IDs depending on how null values are represented.

This violates the fundamental C4 principle: **same content always produces the same C4 ID**.

### Required Changes

A comprehensive specification has been created detailing the required fixes:

**See [CANONICAL_FORM_ENFORCEMENT.md](./CANONICAL_FORM_ENFORCEMENT.md)** for:
- Complete problem statement with concrete examples
- Canonical form requirements (what values are required)
- Ergonomic form support (when nulls are allowed)
- Required API changes (ComputeC4ID returns error, validation methods, canonicalization)
- Implementation plan (4 phases from critical fixes to documentation)
- Migration guide for existing code
- Test requirements
- Complete code examples

### Quick Reference

**Null Value Indicators**:
- Mode: `0` (zero)
- Timestamp: `time.Unix(0, 0).UTC()` (Unix epoch / 1970-01-01)
- Size: `-1` (negative one)
- C4ID: `c4.ID{}` (nil/zero value)

**Text Format**:
- Null mode: `----------` or `-`
- Null timestamp: `-`
- Null size: `-`
- Null C4ID: `-` or omitted

**Validation Levels**:
1. `ValidateStructure()` - Check format, allow nulls (for working manifests)
2. `IsCanonical()` - Check all values explicit (required before C4 ID computation)
3. `IsReadyForSnapshot()` - Comprehensive check for permanent storage

**Workflow**:
```
Working Manifest (may have nulls)
         ↓
   Canonicalize() with MetadataResolver
         ↓
Canonical Manifest (all values explicit)
         ↓
   ComputeC4ID() → deterministic C4 ID
```

**Key Principle**:
- Ergonomic forms with nulls are allowed for **working manifests**
- Canonical form without nulls is required for **C4 ID computation**
- Same content MUST always produce same C4 ID

## Path Resolution Through Manifest Hierarchy

**TODO: Move this functionality from c4d into c4m package**

Path resolution through manifest hierarchies is core c4m functionality that should be shared across all tools (c4d, c4v, c4, etc.).

### Current Implementation (c4d)

c4d currently implements path resolution in `internal/server/resolver.go`:
- `ManifestCache` - caches parsed manifests for performance
- `PathResolver` - traverses manifest hierarchy to resolve paths to C4 IDs
- `ResolveResult` - contains resolved C4 ID, IsDir flag, and manifest (if directory)

### Proposed c4m Package API

```go
package c4m

// Resolver resolves paths through manifest hierarchies
type Resolver struct {
    storage  Storage      // Interface for loading manifests by C4 ID
    cache    *ManifestCache
}

// Storage interface for loading manifests
type Storage interface {
    Get(id c4.ID) (io.ReadCloser, error)
}

// ResolveResult contains the result of path resolution
type ResolveResult struct {
    ID       c4.ID       // C4 ID of the resolved item
    IsDir    bool        // True if this is a directory
    Manifest *Manifest   // If IsDir, the manifest for this directory
}

// NewResolver creates a new path resolver
func NewResolver(storage Storage) *Resolver

// Resolve resolves a path through a manifest hierarchy
func (r *Resolver) Resolve(rootManifestID c4.ID, path string) (*ResolveResult, error)
```

### Use Cases

1. **c4d** - HTTP server path resolution through session views
2. **c4v** - Local workspace path resolution through branch manifests
3. **c4 CLI** - Path queries into manifest hierarchies
4. **c4m tools** - Any tool working with virtual filesystem views

### Design Considerations

- **Manifest Caching** - Essential for performance with deep hierarchies
- **Entry Lookup** - GetEntry() should handle both "dirname" and "dirname/" forms
- **Error Messages** - Should list available entries when path not found (debugging)
- **Path Normalization** - Trim leading/trailing slashes, collapse "//"
- **Root Handling** - Empty path "" resolves to root manifest itself

### Benefits of Moving to c4m

1. **Code Reuse** - All tools benefit from same implementation
2. **Consistency** - Same path resolution behavior everywhere
3. **Testing** - Comprehensive tests in one place
4. **Performance** - Shared optimizations benefit all tools
5. **Simplicity** - Tools don't reimplement core functionality

### Migration Path

1. Move resolver.go from c4d to c4m package
2. Refactor to use storage interface instead of concrete type
3. Add comprehensive tests
4. Update c4d to use c4m.Resolver
5. Use in c4v when implementing workspace operations