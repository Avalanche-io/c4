# C4M - C4 Manifest Format

C4M is a human-readable, text-based format for describing filesystem contents with cryptographic content addressing using C4 IDs. It enables efficient tracking, comparison, and verification of directory structures and file contents.

## Documentation Structure

- **README.md** (this file) - User guide and quick start
- **[SPECIFICATION.md](./SPECIFICATION.md)** - Formal C4M format specification v1.0
- **[IMPLEMENTATION_NOTES.md](./IMPLEMENTATION_NOTES.md)** - Implementation clarifications and edge cases

## What is C4M?

C4M (C4 Manifest) treats filesystems as documents, providing:
- **Content-addressable storage** - Every file and directory has a unique C4 ID based on its content
- **Human-readable format** - Plain text UTF-8 format that's easy to read and process
- **Efficient comparisons** - Boolean operations to diff, merge, and compare directory structures
- **Cryptographic verification** - Verify file integrity using C4 IDs (SHA-512 based)

## Quick Start

### Generate a C4 ID for a directory
```bash
# Get the C4 ID of a directory (computed from its C4M manifest)
c4 myproject/

# Output: c42RgFeXFYL1FjFueMKjPjnwwjyJKnHVasmfEVmrWBekjiCVNjL5xBMtZePcchNPdf8AV8pUwp6L5BTbWfx6J7s7jr
```

### View a directory's manifest
```bash
# Show one-level manifest
c4 -m myproject/

# Show recursive manifest
c4 -mr myproject/
```

### Verify manifest integrity
```bash
# These should produce the same C4 ID
c4 myproject/
c4 -m myproject/ | c4
```

## C4M Format

A C4M manifest is a plain text file with a simple structure:

```
@c4m 1.0
-rw-r--r-- 2024-01-15T10:30:00Z 1234 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB
drwxr-xr-x 2024-01-15T10:30:00Z 4096 docs/ c43pCP9e69EGD253L3pcwcjvzVFHntBMsC7V12jJz83ptDNUoNAa2k3BiafC6UAUgYkyaGk7Z1cbgPvx9zezLagD9M
```

Each line contains:
- Unix permissions
- ISO 8601 timestamp (UTC)
- Size in bytes
- Filename (directories end with `/`)
- C4 ID (content hash)

## Boolean Operations

C4M supports set operations for powerful manifest manipulation:

### Compare directories (diff)
```bash
# Show files that differ between two directories
c4 diff dir1/ dir2/

# Compare a directory against a saved manifest
c4 diff dir1/ backup.c4m
```

### Combine manifests (union)
```bash
# Merge manifests from multiple sources
c4 union source1/ source2/ > combined.c4m
```

### Find common files (intersect)
```bash
# Show files present in both directories
c4 intersect project-v1/ project-v2/
```

### Exclude files (subtract)
```bash
# Show files in dir1 that aren't in dir2
c4 subtract dir1/ dir2/
```

## Use Cases

### Backup Verification
Create manifests of important directories and verify backups haven't changed:
```bash
# Create manifest
c4 -m ~/Documents > documents.c4m

# Later, verify nothing changed
c4 diff ~/Documents documents.c4m
```

### Build Reproducibility
Track build outputs to ensure reproducible builds:
```bash
# Generate manifest of build artifacts
c4 -m build/ > build-v1.0.0.c4m

# Verify next build produces identical output
c4 diff build/ build-v1.0.0.c4m
```

### Data Transfer Validation
Ensure data integrity when transferring files:
```bash
# On source machine
c4 -m data/ > transfer.c4m

# On destination machine (after transfer)
c4 diff data/ transfer.c4m
```

### Change Detection
Monitor filesystem changes over time:
```bash
# Create baseline
c4 -m project/ > baseline.c4m

# Check what changed
c4 diff project/ baseline.c4m

# Update baseline
c4 -m project/ > baseline.c4m
```

## Advanced Features

### Piping and Composition
C4M auto-detects format when piped:
```bash
# Filter and process manifests
c4 -m dir/ | grep "\.jpg" | c4
```

### Recursive Manifests
View complete directory structure:
```bash
c4 -mr project/
```
Output shows nested structure with proper indentation.

### Canonical Form
Directories compute their C4 ID from a canonical representation where subdirectories are represented by their computed C4 ID:
```bash
# The C4 ID of a directory is deterministic
c4 mydir/
# Always produces the same ID for the same content
```

## Integration Ideas

We're exploring filesystem tool integration for seamless C4 workflows:
- `c4-cp` - Copy with automatic C4 verification
- `c4-mv` - Move with manifest updates
- `c4-rm` - Remove with manifest tracking
- `c4-sync` - Sync directories using C4M manifests

These would maintain standard Unix tool interfaces while adding C4 integrity tracking.

## Technical Details

- **C4 IDs**: SMPTE ST 2114:2017 compliant identifiers using SHA-512
- **Format**: UTF-8 text, Unix line endings
- **Permissions**: Standard Unix file mode representation
- **Timestamps**: ISO 8601 format in UTC
- **Natural Sort**: Intelligent ordering (file2.txt before file10.txt)

## Specification

For complete format details, see [SPECIFICATION.md](SPECIFICATION.md).

## Contributing

C4M is part of the C4 reference implementation. We welcome contributions for:
- Additional language implementations
- Tool integrations
- Workflow examples
- Documentation improvements

## Package Contents

### Complexity Analysis

The c4m package contains approximately 7,500 lines of Go code (excluding tests). The areas of highest complexity are:

#### High Complexity Components (>500 lines)

1. **`progressive_scanner.go` (700 lines, 18 functions)** - The most complex component, implementing concurrent three-phase scanning with:
   - Parallel directory traversal using worker pools
   - Three-stage pipeline: structure discovery → metadata collection → C4 ID computation
   - Signal handling for progress reporting (Ctrl+T on macOS)
   - Complex synchronization with channels and wait groups
   - Platform-specific optimizations

2. **`generator.go` (543 lines, 19 functions)** - Traditional recursive manifest generator with:
   - Configurable C4 ID computation strategies
   - Media sequence detection and compression
   - Symlink handling with cycle detection
   - Multiple output formats (canonical, ergonomic, recursive)

3. **`parser.go` (540 lines, 12 functions)** - Comprehensive C4M format parser handling:
   - Multiple timestamp formats and timezone parsing
   - Escape sequence processing in quoted strings
   - Null value representation (-)
   - All @-directives (version, base, data, layer)
   - Error recovery and line tracking

4. **`manifest.go` (500 lines)** - Core data structure with:
   - Multiple serialization formats
   - Entry sorting algorithms
   - Canonical form generation for C4 ID computation
   - Layer management for incremental updates

5. **`bundle.go` (485 lines, 16 functions)** - Bundle storage system managing:
   - Atomic file operations with temporary files
   - Scan session lifecycle
   - Chunk management with @base linking
   - Progress tracking and resumption

6. **`bundle_scanner_v2.go` (486 lines, 11 functions)** - Production scanner with:
   - Three-phase architecture (count → scan → chunk)
   - Collapsed directory handling (>70K entries)
   - Depth tracking and natural sorting
   - Independent scan contexts for large directories

#### Medium Complexity Components (200-500 lines)

- **`progressive_cli.go` (417 lines)** - CLI orchestration with progress display
- **`operations.go` (234 lines)** - Set operations on manifests
- **`entry.go` (237 lines)** - Entry formatting and serialization
- **`sequence.go` (204 lines)** - Media sequence detection algorithms

#### Lower Complexity Utilities (<200 lines)

- **`naturalsort.go` (154 lines)** - Natural sorting algorithm
- **`manifest_sort.go` (115 lines)** - Hierarchical sibling sorting
- **`timing.go` (45 lines)** - Simple timing helpers
- **Platform-specific files** - Signal handling and system call optimizations

### Core Components

- **`manifest.go`** *(500 lines, high complexity)* - Core manifest data structure and operations. Implements the C4M format with version handling, entry management, and canonical form generation for C4 ID computation. Contains multiple serialization strategies and sorting algorithms.

- **`entry.go`** *(237 lines, medium complexity)* - Individual filesystem entry representation. Handles file/directory metadata including permissions, timestamps, sizes, names, symlink targets, and C4 IDs. Includes formatting logic for different output modes.

- **`parser.go`** *(540 lines, high complexity)* - Robust C4M format parser. Handles all @-directives, multiple timestamp formats, quoted filenames, escape sequences, and null values. One of the most complex components due to format flexibility requirements.

### Bundle System

- **`bundle.go`** *(485 lines, high complexity)* - Content-addressed storage container for large filesystem scans. Manages chunked manifests, scan sessions, progress tracking, and atomic file operations. Complex due to concurrent access patterns and atomic operations.

- **`bundle_scanner_v2.go`** *(486 lines, high complexity)* - Current production scanner with three-phase architecture (count, scan, chunk). Handles collapsed directories as separate scan contexts. Critical component with intricate depth tracking and chunking logic.


### CLI Components


- **`bundle_cli_simple.go`** - Simplified CLI wrapper using ScannerV2 for bundle creation.


- **`progressive_cli.go`** *(417 lines, medium complexity)* - CLI for progressive scanning with real-time output and signal handling. Manages output coordination and user interaction.

### Scanning Infrastructure

- **`progressive_scanner.go`** *(700 lines, highest complexity)* - Multi-stage concurrent scanner with three phases: structure discovery, metadata collection, and C4 ID computation. Most complex component due to concurrent worker pools, channel orchestration, and signal handling.

- **`scanner_darwin.go`** *(low complexity)* - macOS-specific optimizations using Darwin system calls.

- **`scanner_linux.go`** *(low complexity)* - Linux-specific optimizations for efficient filesystem traversal.

- **`scanner_generic.go`** *(low complexity)* - Fallback implementation for other platforms.

- **`generator.go`** *(543 lines, high complexity)* - Traditional filesystem-to-manifest generator with configurable options for C4 ID computation, symlink following, and sequence detection. Complex due to recursive traversal and multiple configuration options.

### Streaming Components


### Utilities

- **`naturalsort.go`** *(154 lines, low complexity)* - Natural sorting implementation for mixed alphanumeric filenames (file2 before file10).

- **`sequence.go`** *(204 lines, medium complexity)* - Media file sequence detection and compression. Handles patterns like `frame[0001-0100].png`. Contains pattern matching algorithms.

- **`operations.go`** *(234 lines, medium complexity)* - Manifest comparison and set operations: diff, union, intersect, subtract. Implements efficient set algorithms.

- **`manifest_sort.go`** *(115 lines, low complexity)* - Sorting utilities for manifest entries maintaining C4M format rules.

- **`timing.go`** *(45 lines, minimal complexity)* - Performance timing utilities for debugging and optimization.

### Platform Support

- **`signal_darwin.go`** - macOS signal handling (SIGINFO/Ctrl+T for progress).

- **`signal_other.go`** - Signal handling for non-Darwin platforms.

### Documentation

- **`doc.go`** - Package documentation and API overview.

### Test Files

The package includes comprehensive test coverage:
- **Test files** (>3,800 lines total):
  - `generator_test.go` (947 lines) - Comprehensive generator testing
  - `parser_test.go` (632 lines) - Parser edge cases and error handling
  - `operations_test.go` (597 lines) - Set operation testing
  - `manifest_test.go` (574 lines) - Core manifest functionality
  - Additional specialized tests for sorting, sequences, and null values

## License

Same as the C4 project - see [LICENSE](../LICENSE).