# C4M Workflows

This document describes common workflows for working with C4M manifests. For format details, see [SPECIFICATION.md](./SPECIFICATION.md).

## Table of Contents

1. [Extending a Base Manifest](#extending-a-base-manifest)
2. [Creating Nested Inline Manifests](#creating-nested-inline-manifests)
3. [Extracting from Nested Manifests](#extracting-from-nested-manifests)
4. [Working with File Ranges](#working-with-file-ranges)
5. [Layered Changesets](#layered-changesets)

---

## Extending a Base Manifest

The `@base` directive allows you to build upon an existing manifest without duplicating its content. This is useful for:
- Adding new files to an existing directory structure
- Creating incremental updates
- Maintaining a history of changes

### Basic Pattern

```
@c4m 1.0
@base c4ABC123...  # Reference to base manifest

# New entries are added to the base
-rw-r--r-- 2025-01-01T12:00:00Z 2048 new_file.txt c4DEF456...
drwxr-xr-x 2025-01-01T12:00:00Z 4096 new_dir/
  -rw-r--r-- 2025-01-01T12:00:00Z 1024 nested.txt c4GHI789...
```

### Programmatic Example

```go
package main

import (
    "os"

    "github.com/Avalanche-io/c4"
    "github.com/Avalanche-io/c4/c4m"
)

func extendManifest(baseID c4.ID) error {
    // Create new manifest referencing the base
    manifest := c4m.NewManifest()
    manifest.Base = baseID

    // Add new entries
    manifest.AddEntry(&c4m.Entry{
        Name:      "new_file.txt",
        Mode:      0644,
        Size:      2048,
        Timestamp: time.Now().UTC(),
        C4ID:      c4.Identify(strings.NewReader("new content")),
    })

    // Encode to output
    return c4m.NewEncoder(os.Stdout).Encode(manifest)
}
```

### Resolution Behavior

When processing a manifest with `@base`:

1. Load the base manifest by its C4 ID
2. Apply entries from the current manifest on top
3. Entries with matching paths replace base entries
4. New entries are added alongside base entries

### Use Cases

**Incremental Backup:**
```
# Day 1: Full backup
c4 /data > backup-day1.c4m
BASE_ID=$(c4 backup-day1.c4m)

# Day 2: Incremental (only changed files)
@c4m 1.0
@base c4<BASE_ID>
-rw-r--r-- 2025-01-02T10:00:00Z 512 changed_file.txt c4NEW...
```

**Package Releases:**
```
# v1.0.0 base
@c4m 1.0
-rw-r--r-- ... 10240 app.exe c4APP100...
-rw-r--r-- ...  2048 config.json c4CFG100...

# v1.0.1 patch (only patched files)
@c4m 1.0
@base c4<v1.0.0 manifest ID>
-rw-r--r-- ... 10240 app.exe c4APP101...  # Updated binary
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
@c4m 1.0
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
package main

import (
    "os"
    "time"

    "github.com/Avalanche-io/c4/c4m"
)

func buildNestedManifest() error {
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

    return c4m.NewEncoder(os.Stdout).Encode(manifest)
}
```

### Using the Builder API

```go
m := c4m.NewBuilder().
    AddDir("project", c4m.WithMode(os.ModeDir|0755)).
        AddFile("README.md", c4m.WithSize(1024), c4m.WithMode(0644)).
        AddDir("src", c4m.WithMode(os.ModeDir|0755)).
            AddFile("main.go", c4m.WithSize(2048), c4m.WithMode(0644)).
        End().
    End().
    MustBuild()
```

---

## Extracting from Nested Manifests

Extract a subdirectory from a nested manifest to create a standalone manifest.

### Using FilterByPrefix

```go
// Original manifest contains:
// project/
//   src/
//     main.go
//     util.go
//   docs/
//     guide.md

// Extract just the src/ directory
srcManifest := manifest.FilterByPrefix("src/")

// Result contains:
//   main.go
//   util.go
```

### Manual Extraction with Depth Adjustment

```go
func extractDirectory(manifest *c4m.Manifest, dirPath string) *c4m.Manifest {
    result := c4m.NewManifest()

    // Find the target directory entry
    var baseDepth int
    inTarget := false

    for _, entry := range manifest.Entries {
        // Check if this is the target directory
        if entry.Name == dirPath+"/" || entry.Name == dirPath {
            inTarget = true
            baseDepth = entry.Depth
            continue
        }

        // Check if we're inside the target directory
        if inTarget {
            // Check if we've exited the target directory
            if entry.Depth <= baseDepth {
                break
            }

            // Adjust depth relative to target directory
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
// Load full manifest
manifest, _ := c4m.Unmarshal(fullManifestData)

// Extract src/ directory
srcManifest := extractDirectory(manifest, "src")

// Export as standalone manifest
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
// Create a sequence entry for 100 frames
entry := &c4m.Entry{
    Name:       "render.[0001-0100].exr",
    Mode:       0644,
    Size:       1024000,  // Total size of all frames
    Timestamp:  time.Now().UTC(),
    IsSequence: true,
    Pattern:    "render.[0001-0100].exr",
    C4ID:       sequenceID,  // C4 ID of the ID list
}
```

### Manifest with Sequences

```
@c4m 1.0
-rw-r--r-- 2025-01-01T12:00:00Z 102400000 frames.[0001-0100].exr c4SEQID...

@data c4SEQID...
c4FRAME001...
c4FRAME002...
c4FRAME003...
...
c4FRAME100...
```

### Expanding Sequences

```go
// Parse and expand a sequence
seq, _ := c4m.ParseSequence("render.[0001-0100].exr")
files := seq.Expand()
// files = ["render.0001.exr", "render.0002.exr", ..., "render.0100.exr"]

// Expand with manifest (uses embedded @data block)
expander := c4m.NewSequenceExpander(c4m.SequenceEmbedded)
expandedManifest, _, _ := expander.ExpandManifest(manifest)
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

### Inline Expansion with @expand

For self-contained manifests, use `@expand` to include the full expansion:

```
@c4m 1.0
-rw-r--r-- 2025-01-01T12:00:00Z 30720 frames.[01-03].exr c4SEQID...

@expand c4SEQID
-rw-r--r-- 2025-01-01T12:00:00Z 10240 frames.01.exr c4F001...
-rw-r--r-- 2025-01-01T12:00:00Z 10240 frames.02.exr c4F002...
-rw-r--r-- 2025-01-01T12:00:00Z 10240 frames.03.exr c4F003...
```

---

## Layered Changesets

Layers provide a way to represent modifications as changesets, useful for versioning and collaborative workflows.

### Layer Directives

| Directive | Purpose |
|-----------|---------|
| `@layer` | Start an addition/modification layer |
| `@remove` | Start a removal layer |
| `@by` | Attribution (who made the change) |
| `@time` | When the change was made |
| `@note` | Human-readable description |
| `@end` | End current layer (optional) |

### Adding Files with a Layer

```
@c4m 1.0
@base c4ORIGINAL...

@layer
@by "Alice Developer"
@time 2025-01-15T10:00:00Z
@note "Add new feature module"
drwxr-xr-x 2025-01-15T10:00:00Z 4096 feature/
  -rw-r--r-- 2025-01-15T10:00:00Z 2048 handler.go c4HANDLER...
  -rw-r--r-- 2025-01-15T10:00:00Z 1024 types.go c4TYPES...
```

### Removing Files with @remove

```
@c4m 1.0
@base c4PREVIOUS...

@remove
@by "Bob Maintainer"
@time 2025-01-16T14:00:00Z
@note "Remove deprecated files"
drwxr-xr-x 2025-01-01T00:00:00Z 0 old_module/
-rw-r--r-- 2025-01-01T00:00:00Z 0 legacy.txt
```

### Multiple Layers

```
@c4m 1.0
@base c4BASE...

@layer
@by "Dev Team"
@note "Feature additions"
-rw-r--r-- 2025-01-15T10:00:00Z 2048 new_feature.go c4NEW...

@remove
@by "Dev Team"
@note "Cleanup deprecated code"
-rw-r--r-- - - deprecated.go

@layer
@by "QA Team"
@note "Add test files"
-rw-r--r-- 2025-01-16T11:00:00Z 4096 feature_test.go c4TEST...
```

### Programmatic Layer Construction

```go
manifest := c4m.NewManifest()
manifest.Base = baseManifestID

// Create addition layer
addLayer := &c4m.Layer{
    Type: c4m.LayerTypeAdd,
    By:   "Developer",
    Time: time.Now().UTC(),
    Note: "Add new files",
}
manifest.Layers = append(manifest.Layers, addLayer)

// Add entries to the layer
manifest.AddEntry(&c4m.Entry{
    Name:      "new_file.go",
    Mode:      0644,
    Size:      2048,
    Timestamp: time.Now().UTC(),
    C4ID:      newFileID,
})

// Create removal layer
removeLayer := &c4m.Layer{
    Type: c4m.LayerTypeRemove,
    By:   "Developer",
    Time: time.Now().UTC(),
    Note: "Remove old files",
}
manifest.Layers = append(manifest.Layers, removeLayer)
```

---

## Complete Example: Versioned Package Updates

This example demonstrates combining multiple workflows:

```
# Base package (v1.0.0)
@c4m 1.0
drwxr-xr-x 2025-01-01T12:00:00Z 16384 mypackage/
  -rw-r--r-- 2025-01-01T12:00:00Z 1024 README.md c4README...
  drwxr-xr-x 2025-01-01T12:00:00Z 8192 lib/
    -rw-r--r-- 2025-01-01T12:00:00Z 4096 core.so c4CORE100...
    -rw-r--r-- 2025-01-01T12:00:00Z 2048 utils.so c4UTIL100...
  drwxr-xr-x 2025-01-01T12:00:00Z 4096 assets/
    -rw-r--r-- 2025-01-01T12:00:00Z 102400 images.[001-100].png c4IMGSEQ...

@data c4IMGSEQ...
c4IMG001...
c4IMG002...
... (100 image IDs)
c4IMG100...
```

```
# Patch release (v1.0.1) - extends base
@c4m 1.0
@base c4<v1.0.0 manifest ID>

@layer
@by "Security Team"
@time 2025-01-10T09:00:00Z
@note "Security patch for core library"
drwxr-xr-x 2025-01-10T09:00:00Z 8192 lib/
  -rw-r--r-- 2025-01-10T09:00:00Z 4096 core.so c4CORE101...

@remove
@by "Security Team"
@note "Remove vulnerable utility"
drwxr-xr-x - - lib/
  -rw-r--r-- - - utils.so
```

```
# Feature release (v1.1.0) - extends patch
@c4m 1.0
@base c4<v1.0.1 manifest ID>

@layer
@by "Feature Team"
@time 2025-01-15T14:00:00Z
@note "Add new rendering module"
drwxr-xr-x 2025-01-15T14:00:00Z 4096 lib/
  -rw-r--r-- 2025-01-15T14:00:00Z 3072 render.so c4RENDER...
drwxr-xr-x 2025-01-15T14:00:00Z 51200 assets/
  -rw-r--r-- 2025-01-15T14:00:00Z 51200 textures.[001-050].png c4TEXSEQ...

@data c4TEXSEQ...
c4TEX001...
c4TEX002...
... (50 texture IDs)
c4TEX050...
```

---

## See Also

- [README.md](./README.md) - User guide and quick start
- [SPECIFICATION.md](./SPECIFICATION.md) - Formal format specification
- [IMPLEMENTATION_NOTES.md](./IMPLEMENTATION_NOTES.md) - Implementation details
