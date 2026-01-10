# Layered C4M Workflow Example

This document demonstrates building manifests incrementally across sessions using `@base` references and `@layer` changesets.

---

## Step 1: Create Initial Manifest (2 files)

**Go Code:**
```go
package main

import (
    "fmt"
    "time"
    "github.com/Avalanche-io/c4/c4m"
)

func main() {
    ts := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

    m := c4m.NewBuilder().
        AddFile("readme.txt", c4m.WithSize(100), c4m.WithMode(0644), c4m.WithTimestamp(ts)).
        AddFile("config.json", c4m.WithSize(250), c4m.WithMode(0644), c4m.WithTimestamp(ts)).
        MustBuild()

    data, _ := c4m.Marshal(m)
    fmt.Print(string(data))

    // Save the manifest's C4 ID for next session
    fmt.Printf("\n# Manifest ID: %s\n", m.ComputeC4ID())
}
```

**Resulting `step1.c4m`:**
```
@c4m 1.0
-rw-r--r-- 2024-01-01T12:00:00Z 100 readme.txt
-rw-r--r-- 2024-01-01T12:00:00Z 250 config.json
```

---

## Step 2: Add a New File (extends Step 1)

**Go Code:**
```go
package main

import (
    "fmt"
    "time"
    "github.com/Avalanche-io/c4"
    "github.com/Avalanche-io/c4/c4m"
)

func main() {
    // Reference the previous manifest by its C4 ID
    step1ID, _ := c4.Parse("c4...") // ID from step 1

    ts := time.Date(2024, 2, 15, 9, 30, 0, 0, time.UTC)

    m := c4m.NewBuilder().
        WithBaseID(step1ID).
        AddFile("main.go", c4m.WithSize(1500), c4m.WithMode(0644), c4m.WithTimestamp(ts)).
        MustBuild()

    data, _ := c4m.Marshal(m)
    fmt.Print(string(data))
}
```

**Resulting `step2.c4m`:**
```
@c4m 1.0
@base c4xY7abc123...  # ID of step1.c4m

-rw-r--r-- 2024-02-15T09:30:00Z 1500 main.go
```

**Effective contents (after resolving @base):**
```
readme.txt     (from step1)
config.json    (from step1)
main.go        (added in step2)
```

---

## Step 3: Remove a File and Move Another to Subfolder

The builder API handles layer creation automatically. Use `Remove()` to mark paths for removal (validated against base if provided) and layer metadata methods for attribution.

**Go Code:**
```go
package main

import (
    "fmt"
    "time"
    "github.com/Avalanche-io/c4/c4m"
)

func main() {
    // Load previous manifest (for validation)
    step2, _ := c4m.Unmarshal(step2Data)

    ts := time.Date(2024, 3, 20, 14, 0, 0, 0, time.UTC)

    m, err := c4m.NewBuilder().
        WithBase(step2).                      // Enables path validation
        By("admin").Note("Reorganize project structure").At(ts).
        Remove("config.json").                // Validated against step2
        Remove("readme.txt").                 // Will be moved to src/
        AddDir("src").
            AddFile("readme.txt", c4m.WithSize(100), c4m.WithMode(0644), c4m.WithTimestamp(ts)).
        End().
        Build()

    if err != nil {
        fmt.Println("Validation warnings:", err)
    }

    data, _ := c4m.Marshal(m)
    fmt.Print(string(data))
}
```

**Resulting `step3.c4m`:**
```
@c4m 1.0
@base c4zQ9def456...  # ID of step2.c4m

@remove by:admin at:2024-03-20T14:00:00Z note:"Reorganize project structure"
config.json
readme.txt

drwxr-xr-x 2024-03-20T14:00:00Z - src/
  -rw-r--r-- 2024-03-20T14:00:00Z 100 readme.txt
```

**Effective contents (after resolving):**
```
main.go           (from step2, unchanged)
src/
  readme.txt      (moved from root)
```

---

## Step 4: Rename File and Add Deeply Nested File

When you only have the manifest ID (not the full manifest), use `WithBaseID()`. Validation is skipped but a warning is emitted.

**Go Code:**
```go
package main

import (
    "fmt"
    "time"
    "github.com/Avalanche-io/c4"
    "github.com/Avalanche-io/c4/c4m"
)

func main() {
    step3ID, _ := c4.Parse("c4...") // ID from step 3

    ts := time.Date(2024, 4, 10, 16, 45, 0, 0, time.UTC)

    builder := c4m.NewBuilder().
        WithBaseID(step3ID).                  // ID only - no validation
        By("developer").Note("Rename main.go and add nested structure").At(ts).
        Remove("main.go").                    // Cannot validate without manifest
        AddFile("app.go", c4m.WithSize(1500), c4m.WithMode(0644), c4m.WithTimestamp(ts)).
        AddDir("dir1").
            AddDir("dir2").
                AddDir("dir3").
                    AddFile("step4_file.txt", c4m.WithSize(50), c4m.WithMode(0644), c4m.WithTimestamp(ts)).
                EndDir().
            EndDir().
        End()

    // Check for warnings (unvalidatable base)
    if warnings := builder.Warnings(); len(warnings) > 0 {
        fmt.Println("Warnings:", warnings)
    }

    m := builder.MustBuild()
    data, _ := c4m.Marshal(m)
    fmt.Print(string(data))
}
```

**Resulting `step4.c4m`:**
```
@c4m 1.0
@base c4wR8ghi789...  # ID of step3.c4m

@remove by:developer at:2024-04-10T16:45:00Z note:"Rename main.go and add nested structure"
main.go

-rw-r--r-- 2024-04-10T16:45:00Z 1500 app.go
drwxr-xr-x 2024-04-10T16:45:00Z - dir1/
  drwxr-xr-x 2024-04-10T16:45:00Z - dir2/
    drwxr-xr-x 2024-04-10T16:45:00Z - dir3/
      -rw-r--r-- 2024-04-10T16:45:00Z 50 step4_file.txt
```

**Effective contents (after resolving all layers):**
```
app.go                        (renamed from main.go)
src/
  readme.txt                  (from step3)
dir1/
  dir2/
    dir3/
      step4_file.txt          (added in step4)
```

---

## Step 5: Application-Specific Views with Additional Layers

Additional `@layer` blocks (add layers) beyond the first are **not materialized by default**. They exist as virtual overlays that specific applications can choose to interpret. This enables application-specific views without affecting the canonical filesystem state.

**Example: Sort Order Hints**

Imagine an application that displays files in a custom order (like a playlist or chapter sequence). It could look for a specially-noted layer containing alternate names with numbered prefixes. These alternates can use the exact same paths as the originals - there's no conflict because additional layers aren't materialized.

**Go Code:**
```go
package main

import (
    "fmt"
    "time"
    "github.com/Avalanche-io/c4"
    "github.com/Avalanche-io/c4/c4m"
)

func main() {
    step4ID, _ := c4.Parse("c4...") // ID from step 4

    ts := time.Date(2024, 5, 5, 10, 0, 0, 0, time.UTC)

    // Get the C4 IDs of existing content (from step4)
    appGoID, _ := c4.Parse("c4AppGoContentID...")
    readmeID, _ := c4.Parse("c4ReadmeContentID...")

    m := c4m.NewManifest()
    m.Base, _ = c4.Parse(step4ID.String())

    // Add a layer with sort-order hints (application-specific)
    m.Layers = append(m.Layers, &c4m.Layer{
        Type: c4m.LayerTypeAdd,
        By:   "curator",
        Time: ts,
        Note: "sort-order:playlist",  // App-specific convention in note
    })

    // Alternate names at the SAME paths - no conflict since not materialized
    m.Builder().
        AddFile("01_zephyr.txt", c4m.WithC4ID(readmeID), c4m.WithSize(100)).
        AddFile("02_alphabet.txt", c4m.WithC4ID(appGoID), c4m.WithSize(1500))

    data, _ := c4m.Marshal(m)
    fmt.Print(string(data))
}
```

**Resulting `step5.c4m`:**
```
@c4m 1.0
@base c4Step4ID...  # ID of step4.c4m

@layer by:curator at:2024-05-05T10:00:00Z note:"sort-order:playlist"
-rw-r--r-- 2024-05-05T10:00:00Z 100 01_zephyr.txt c4ReadmeContentID...
-rw-r--r-- 2024-05-05T10:00:00Z 1500 02_alphabet.txt c4AppGoContentID...
```

**How it works:**
- This is just a regular `@layer` (add layer) - no special syntax
- Additional add layers are NOT materialized by default
- Apps unaware of this convention see only the base manifest's files
- A playlist app could scan layers for `note:"sort-order:*"` and use those names
- The numbered alternates reference the same content IDs as existing files
- No path conflicts because these entries exist only in the virtual layer

---

## Step 6: Sidecar Metadata with @data Block

Use a `@data` block to embed metadata directly in the manifest. This is useful for sidecar files (like `.meta.json`) that describe other content. The embedded data has its own C4 ID computed from the content.

**Go Code:**
```go
package main

import (
    "fmt"
    "time"
    "github.com/Avalanche-io/c4"
    "github.com/Avalanche-io/c4/c4m"
)

func main() {
    step5ID, _ := c4.Parse("c4...") // ID from step 5

    ts := time.Date(2024, 6, 1, 8, 0, 0, 0, time.UTC)

    // The sidecar metadata we want to embed
    metadataJSON := []byte(`{
  "title": "Project Documentation",
  "author": "Development Team",
  "version": "2.0",
  "files": {
    "app.go": {"language": "go", "lines": 150},
    "readme.txt": {"format": "markdown", "words": 500}
  }
}`)

    // Compute the C4 ID of the metadata
    metadataID := c4.Identify(bytes.NewReader(metadataJSON))

    m := c4m.NewManifest()
    m.Base, _ = c4.Parse(step5ID.String())

    // Add the sidecar layer
    m.Layers = append(m.Layers, &c4m.Layer{
        Type: c4m.LayerTypeAdd,
        By:   "metadata-system",
        Time: ts,
        Note: "Add project metadata sidecar",
    })

    // Add the hidden sidecar file pointing to the embedded data
    m.Builder().
        AddFile(".project.meta.json",
            c4m.WithC4ID(metadataID),
            c4m.WithSize(int64(len(metadataJSON))),
            c4m.WithMode(0644),
            c4m.WithTimestamp(ts))

    // Embed the actual data in the manifest
    m.AddDataBlock(&c4m.DataBlock{
        ID:      metadataID,
        Content: metadataJSON,
    })

    data, _ := c4m.Marshal(m)
    fmt.Print(string(data))
}
```

**Resulting `step6.c4m`:**
```
@c4m 1.0
@base c4Step5ID...  # ID of step5.c4m

@layer by:metadata-system at:2024-06-01T08:00:00Z note:"Add project metadata sidecar"
-rw-r--r-- 2024-06-01T08:00:00Z 156 .project.meta.json c4MetadataID...

@data c4MetadataID...
ewogICJ0aXRsZSI6ICJQcm9qZWN0IERvY3VtZW50YXRpb24iLAogICJhdXRob3IiOiAi
RGV2ZWxvcG1lbnQgVGVhbSIsCiAgInZlcnNpb24iOiAiMi4wIiwKICAiZmlsZXMiOiB7
CiAgICAiYXBwLmdvIjogeyJsYW5ndWFnZSI6ICJnbyIsICJsaW5lcyI6IDE1MH0sCiAg
ICAicmVhZG1lLnR4dCI6IHsiZm9ybWF0IjogIm1hcmtkb3duIiwgIndvcmRzIjogNTAw
fQogIH0KfQ==
```

**How it works:**
- The `.project.meta.json` entry is a hidden sidecar file (dot-prefix)
- Its C4 ID points to metadata about the project files
- The `@data` block embeds the JSON content directly (base64 encoded)
- No external storage needed - the manifest is self-contained
- Applications can retrieve the data with `manifest.GetDataBlock(metadataID)`

**Reading the embedded data:**
```go
// Later, when processing the manifest:
block := m.GetDataBlock(metadataID)
if block != nil {
    var meta map[string]interface{}
    json.Unmarshal(block.Content, &meta)
    fmt.Printf("Project: %s by %s\n", meta["title"], meta["author"])
}
```

---

## Step 7: Merge All Layers into a Flat Manifest

To produce a standalone manifest with no `@base` reference, resolve the entire layer chain and materialize the result. This "flattens" all the deltas into a single canonical manifest.

**What gets materialized:**
- All entries from the `@base` chain (recursively resolved)
- All `@remove` layers applied (entries deleted)
- The **first** add layer's entries (the default/canonical view)
- Embedded `@data` blocks (if the content is still referenced)

**What does NOT get materialized:**
- Additional add layers (app-specific views like sort-order hints)
- The `@base` reference itself (we're flattening it away)
- Layer metadata (by/note/time) - though you could preserve this as a comment

**Go Code:**
```go
package main

import (
    "fmt"
    "os"
    "github.com/Avalanche-io/c4/c4m"
    "github.com/Avalanche-io/c4/store"
)

func main() {
    // Load step6.c4m from disk
    f, _ := os.Open("step6.c4m")
    defer f.Close()
    step6, _ := c4m.NewDecoder(f).Decode()

    // Option 1: Use a c4 store (parses manifests on demand)
    folder := store.NewFolder("./manifests")
    merged, err := step6.Merge(c4m.FromStore(folder))

    // Option 2: Use cached store (reuses parsed manifests)
    // cache := c4m.NewManifestCache(folder)
    // merged, err := step6.Merge(cache)

    // Option 3: Use pre-loaded manifests (no I/O during merge)
    // preloaded := c4m.MapGetter{
    //     step1.ComputeC4ID(): step1,
    //     step2.ComputeC4ID(): step2,
    //     // ...
    // }
    // merged, err := step6.Merge(preloaded)

    if err != nil {
        fmt.Fprintf(os.Stderr, "merge failed: %v\n", err)
        os.Exit(1)
    }

    // Output the flat manifest
    c4m.NewEncoder(os.Stdout).Encode(merged)
}
```

**Resulting `merged.c4m`:**
```
@c4m 1.0
-rw-r--r-- 2024-04-10T16:45:00Z 1500 app.go c4AppGoContentID...
-rw-r--r-- 2024-06-01T08:00:00Z 156 .project.meta.json c4MetadataID...
drwxr-xr-x 2024-03-20T14:00:00Z - dir1/
  drwxr-xr-x 2024-03-20T14:00:00Z - dir2/
    drwxr-xr-x 2024-03-20T14:00:00Z - dir3/
      -rw-r--r-- 2024-04-10T16:45:00Z 50 step4_file.txt c4Step4FileID...
drwxr-xr-x 2024-03-20T14:00:00Z - src/
  -rw-r--r-- 2024-03-20T14:00:00Z 100 readme.txt c4ReadmeContentID...

@data c4MetadataID...
ewogICJ0aXRsZSI6ICJQcm9qZWN0IERvY3VtZW50YXRpb24iLAogICJhdXRob3IiOiAi
...
```

**What happened:**
- `readme.txt` and `config.json` from step1 → removed in step3
- `main.go` from step2 → renamed to `app.go` in step4
- `src/readme.txt` added in step3 (the moved file)
- `dir1/dir2/dir3/step4_file.txt` added in step4
- `.project.meta.json` added in step6 with embedded `@data`
- Step5's sort-order layer was NOT materialized (app-specific view)
- No `@base` reference - this is a complete standalone manifest

**Use cases for merging:**
- Archival: Create a self-contained snapshot
- Distribution: Share without requiring the base chain
- Optimization: Reduce resolution overhead for frequently-accessed manifests
- Verification: Compare materialized state against expected contents

---

## Summary: Chain of Manifests

```
step1.c4m (standalone)
    ↓
step2.c4m (@base → step1)
    ↓
step3.c4m (@base → step2, @remove + additions)
    ↓
step4.c4m (@base → step3, @remove + additions)
    ↓
step5.c4m (@base → step4, app-specific layer)
    ↓
step6.c4m (@base → step5, @layer + @data block)
    ↓
merged.c4m (flattened, no @base)
```

Each manifest only stores its **delta** from the previous state. The full filesystem state is reconstructed by resolving the `@base` chain and applying layers in order. Merging collapses this chain into a standalone manifest.

### Key Patterns

| Operation | How to Express |
|-----------|---------------|
| Add files | `AddFile()`, `AddDir()` in builder |
| Remove files | `Remove()` - auto-creates `@remove` layer |
| Remove directory | `RemoveDir()` - removes dir + contents |
| Move files | `Remove()` old path + `AddFile()` new path |
| Rename files | `Remove()` old name + `AddFile()` new name (same content ID) |
| Extend previous | `WithBase(manifest)` or `WithBaseID(id)` |
| Attribution | `By(author)`, `Note(text)`, `At(timestamp)` |
| App-specific views | Additional `@layer` blocks (not materialized) for custom app behavior |
| Embed metadata | `AddDataBlock()` with `@data` directive for self-contained manifests |
| Sidecar files | Hidden files (`.name`) with C4 ID pointing to embedded `@data` |
| Merge/flatten | `manifest.Merge(getter)` resolves `@base` chain, applies removes |
| From c4 store | `FromStore(store.Source)` wraps store for on-demand parsing |
| Cached parsing | `NewManifestCache(storage)` caches parsed manifests |
| Pre-loaded | `MapGetter{id: manifest, ...}` for already-parsed manifests |

### Validation Behavior

| Scenario | Result |
|----------|--------|
| `WithBase(manifest)` + `Remove("exists")` | OK, no error |
| `WithBase(manifest)` + `Remove("notexists")` | Error returned, but removal still added |
| `WithBaseID(id)` + `Remove("anything")` | Warning: cannot validate |
| No base + `Remove("anything")` | Error: no base set |

Build always returns the manifest - errors are informational. Use `MustBuild()` in tests to panic on errors.
