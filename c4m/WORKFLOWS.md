# C4M Workflows

This document describes common workflows for working with C4M manifests. For format details, see [SPECIFICATION.md](./SPECIFICATION.md).

## Table of Contents

1. [Patch Chains](#patch-chains)
2. [Creating Nested Inline Manifests](#creating-nested-inline-manifests)
3. [Extracting from Nested Manifests](#extracting-from-nested-manifests)
4. [Working with File Ranges](#working-with-file-ranges)

---

## Patch Chains

A c4m file can contain a base manifest followed by one or more patches,
separated by bare C4 ID lines. Each bare C4 ID validates the state of
everything above it, then subsequent entries are applied as a patch.

### Basic Pattern

```
-rw-r--r-- 2025-01-01T12:00:00Z 2048 file1.txt c4ABC123...
-rw-r--r-- 2025-01-01T12:00:00Z 1024 file2.txt c4DEF456...
c4<C4 ID of the manifest above>
-rw-r--r-- 2025-01-02T10:00:00Z 512 file2.txt c4NEW789...
```

The bare C4 ID line after the base entries validates the manifest state,
then `file2.txt` is patched with its new content.

### Programmatic Example

```go
// Create a patch diff between two manifests
oldManifest, _ := c4m.Unmarshal(oldData)
newManifest, _ := c4m.Unmarshal(newData)
patch := c4m.PatchDiff(oldManifest, newManifest)

// Encode the patch (writes old C4 ID + changed entries)
encoder := c4m.NewEncoder(os.Stdout)
encoder.EncodePatch(patch)
```

### Use Cases

**Incremental Updates:**
```bash
# Day 1: Full scan
c4 id ./project/ > project.c4m

# Day 2: Append a patch for changes
c4 diff project.c4m <(c4 id ./project/) >> project.c4m

# View the change history
c4 log project.c4m
```

**Splitting History:**
```bash
# Split at patch 3 to create a branch point
c4 split project.c4m 3 before.c4m after.c4m
```

---

## Creating Nested Inline Manifests

Nested manifests use indentation to represent directory hierarchies. Each level of indentation indicates nesting within the parent directory.

### Indentation Rules

- Use consistent spacing (2 or 4 spaces recommended)
- Directories end with `/`
- Children are indented under their parent directory
- Depth is calculated from indentation width

### Basic Nested Structure

```
drwxr-xr-x 2025-01-01T12:00:00Z 8192 project/
  -rw-r--r-- 2025-01-01T12:00:00Z 1024 README.md c4README...
  drwxr-xr-x 2025-01-01T12:00:00Z 4096 src/
    -rw-r--r-- 2025-01-01T12:00:00Z 2048 main.go c4MAIN...
    -rw-r--r-- 2025-01-01T12:00:00Z 1536 util.go c4UTIL...
  drwxr-xr-x 2025-01-01T12:00:00Z 2048 docs/
    -rw-r--r-- 2025-01-01T12:00:00Z 512 guide.md c4GUIDE...
```

### Programmatic Construction

```go
manifest := c4m.NewManifest()
now := time.Now().UTC()

// Root directory (depth 0)
manifest.AddEntry(&c4m.Entry{
    Name:      "project/",
    Mode:      0755 | os.ModeDir,
    Size:      8192,
    Timestamp: now,
    Depth:     0,
})

// File in root (depth 1)
manifest.AddEntry(&c4m.Entry{
    Name:      "README.md",
    Mode:      0644,
    Size:      1024,
    Timestamp: now,
    Depth:     1,
    C4ID:      readmeID,
})

// Subdirectory (depth 1)
manifest.AddEntry(&c4m.Entry{
    Name:      "src/",
    Mode:      0755 | os.ModeDir,
    Size:      4096,
    Timestamp: now,
    Depth:     1,
})

// File in subdirectory (depth 2)
manifest.AddEntry(&c4m.Entry{
    Name:      "main.go",
    Mode:      0644,
    Size:      2048,
    Timestamp: now,
    Depth:     2,
    C4ID:      mainID,
})

c4m.NewEncoder(os.Stdout).Encode(manifest)
```

---

## Extracting from Nested Manifests

Extract a subdirectory from a nested manifest to create a standalone manifest.

### Manual Extraction with Depth Adjustment

```go
func extractDirectory(manifest *c4m.Manifest, dirPath string) *c4m.Manifest {
    result := c4m.NewManifest()

    var baseDepth int
    inTarget := false

    for _, entry := range manifest.Entries {
        if entry.Name == dirPath+"/" || entry.Name == dirPath {
            inTarget = true
            baseDepth = entry.Depth
            continue
        }

        if inTarget {
            if entry.Depth <= baseDepth {
                break
            }

            adjusted := *entry
            adjusted.Depth = entry.Depth - baseDepth - 1
            result.AddEntry(&adjusted)
        }
    }

    return result
}
```

### Example: Extract and Export

```go
manifest, _ := c4m.Unmarshal(fullManifestData)
srcManifest := extractDirectory(manifest, "src")
data, _ := c4m.Marshal(srcManifest)
os.WriteFile("src.c4m", data, 0644)
```

---

## Working with File Ranges

File ranges (sequences) provide compact representation for numbered files, common in media workflows.

### Sequence Notation

```
# Basic range: frame.0001.exr through frame.0100.exr
frame.[0001-0100].exr

# Stepped range: every other frame
frame.[0001-0100:2].exr

# Discontinuous range: gaps in sequence
frame.[0001-0050,0075-0100].exr

# Individual frames
frame.[0001,0005,0010].exr
```

### Creating Sequence Entries

```go
entry := &c4m.Entry{
    Name:       "render.[0001-0100].exr",
    Mode:       0644,
    Size:       1024000,  // Total size of all frames
    Timestamp:  time.Now().UTC(),
    IsSequence: true,
    Pattern:    "render.[0001-0100].exr",
    C4ID:       sequenceID,  // C4 ID of the ID list file
}
```

### Manifest with Sequences

```
-rw-r--r-- 2025-01-01T12:00:00Z 102400000 frames.[0001-0100].exr c4SEQID...
```

The sequence entry's C4 ID is the hash of the ID list file — a plain file
containing the bare-concatenated C4 IDs of each frame (90 chars per ID, no
separators). This file lives in the content store and can be retrieved with
`c4 cat c4SEQID...`.

### Expanding Sequences

```go
// Parse and expand a sequence pattern
seq, _ := c4m.ParseSequence("render.[0001-0100].exr")
files := seq.Expand()
// files = ["render.0001.exr", "render.0002.exr", ..., "render.0100.exr"]
```

### Creating Sequences from Files

```go
// Detect sequences from a list of files
entries := []*c4m.Entry{
    {Name: "frame.0001.exr", C4ID: id1},
    {Name: "frame.0002.exr", C4ID: id2},
    {Name: "frame.0003.exr", C4ID: id3},
}

manifest := &c4m.Manifest{Entries: entries}
sequences := c4m.DetectSequences(manifest)
// sequences contains collapsed sequence entries
```

---

## See Also

- [README.md](./README.md) - User guide and quick start
- [SPECIFICATION.md](./SPECIFICATION.md) - Formal format specification
- [C4M-STANDARD.md](./C4M-STANDARD.md) - Formal standard with ABNF grammar
