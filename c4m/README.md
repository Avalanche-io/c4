# C4M - C4 Manifest Format

C4M is a human-readable, text-based format for describing filesystem contents with cryptographic content addressing using C4 IDs. It enables efficient tracking, comparison, and verification of directory structures and file contents.

## Documentation Structure

- **README.md** (this file) - User guide and quick start
- **[SPECIFICATION.md](./SPECIFICATION.md)** - Formal C4M format specification v1.0
- **[WORKFLOWS.md](./WORKFLOWS.md)** - Common workflows and usage patterns
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
# Get the C4 ID of a directory
c4 -i myproject/

# Output: c42RgFeXFYL1FjFueMKjPjnwwjyJKnHVasmfEVmrWBekjiCVNjL5xBMtZePcchNPdf8AV8pUwp6L5BTbWfx6J7s7jr
```

### View a directory's c4m listing
```bash
# Full recursive c4m output
c4 myproject/
```

### Verify integrity
```bash
# The C4 ID is computed from the canonical c4m listing
c4 -i myproject/
```

## C4M Format

A C4M manifest is a plain text file with a simple structure:

```
-rw-r--r-- 2024-01-15T10:30:00Z 1234 file.txt c41j3C6Jqga95PL2zmZVBWixAUhoWDNmwamiWiNTDAMRL1UWqe4WdtYjSozRijRSokEsaTnYyxoCBt43u4sfqWG2uB
drwxr-xr-x 2024-01-15T10:30:00Z 4096 docs/ c43pCP9e69EGD253L3pcwcjvzVFHntBMsC7V12jJz83ptDNUoNAa2k3BiafC6UAUgYkyaGk7Z1cbgPvx9zezLagD9M
```

Each line contains:
- Unix permissions
- ISO 8601 timestamp (UTC)
- Size in bytes
- Filename (directories end with `/`)
- C4 ID (content hash)

## Comparing Directories

```bash
# Show what changed between two directories
c4 diff dir1/ dir2/

# Compare a directory against a saved c4m file
c4 diff dir1/ backup.c4m:
```

## Use Cases

### Backup Verification
Create c4m files of important directories and verify backups haven't changed:
```bash
# Create c4m file
c4 ~/Documents > documents.c4m

# Later, verify nothing changed
c4 diff ~/Documents documents.c4m:
```

### Build Reproducibility
Track build outputs to ensure reproducible builds:
```bash
# Generate c4m file of build artifacts
c4 build/ > build-v1.0.0.c4m

# Verify next build produces identical output
c4 diff build/ build-v1.0.0.c4m:
```

### Data Transfer Validation
Ensure data integrity when transferring files:
```bash
# On source machine
c4 data/ > transfer.c4m

# On destination machine (after transfer)
c4 diff data/ transfer.c4m:
```

### Change Detection
Monitor filesystem changes over time:
```bash
# Create baseline
c4 project/ > baseline.c4m

# Check what changed
c4 diff project/ baseline.c4m:

# Update baseline
c4 project/ > baseline.c4m
```

## Advanced Features

### Piping and Composition
c4m output is plain text, native to Unix tools:
```bash
# Filter c4m entries with grep
c4 dir/ | grep "\.jpg"

# Extract all C4 IDs with awk
c4 dir/ | awk '{ print $NF }'

# Files over 1MB
c4 dir/ | awk '$3+0 > 1048576'
```

### Canonical Form
Directories compute their C4 ID from a canonical representation where subdirectories are represented by their computed C4 ID:
```bash
# The C4 ID of a directory is deterministic
c4 mydir/
# Always produces the same ID for the same content
```

## CLI Integration

The `c4` CLI provides built-in filesystem operations using the same c4m format:

```bash
c4 cp ./src/ project.c4m:src/    # Copy into a c4m file
c4 diff dir1/ dir2/              # Compare directories
c4 patch desired.c4m :           # Converge filesystem to target state
```

See the [CLI Reference](../docs/cli-reference.md) for the full command vocabulary.

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

## Package Files

### Core API
- `manifest.go` - Manifest type, sorting, canonicalization, tree index, metadata propagation
- `entry.go` - Entry type, formatting, mode/name/size encoding
- `encoder.go` - Encoder, Marshal, MarshalPretty, Format, Unmarshal
- `decoder.go` - Decoder with character-level parser, timestamp/mode parsing
- `builder.go` - Fluent manifest builder API
- `operations.go` - Diff, Union, Intersect, Subtract, Resolver, ManifestCache
- `validator.go` - Streaming validator with configurable strictness
- `naturalsort.go` - Natural sort (text before numeric, ASCII digits only)
- `sequence.go` - Media file sequence detection and compression

### API Examples

```go
// Decode a manifest
m, err := c4m.NewDecoder(reader).Decode()

// Encode canonical
err = c4m.NewEncoder(writer).Encode(m)

// Encode pretty
err = c4m.NewEncoder(writer).SetPretty(true).Encode(m)

// Build a manifest
m := c4m.NewBuilder().
    AddFile("readme.txt", c4m.WithSize(100), c4m.WithMode(0644)).
    AddDir("src", c4m.WithMode(os.ModeDir|0755)).
        AddFile("main.go", c4m.WithSize(1024)).
    End().
    MustBuild()

// Look up an entry (O(1) indexed)
entry := m.GetEntry("main.go")

// Sort entries for output
m.SortEntries()

// Compute deterministic C4 ID
id := m.ComputeC4ID()
```

## License

Same as the C4 project - see [LICENSE](../LICENSE).