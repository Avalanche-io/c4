# Architectural Catalog

Quick reference for the c4 codebase. Consult before adding types, interfaces,
or packages to avoid duplicating what already exists.

## Package Layout

```
github.com/Avalanche-io/c4
  c4/              Root package: ID, Tree, Identify(), Parse()
  c4m/             c4m format: Entry, Manifest, Encoder, Decoder, patch chains
  scan/            Directory scanner: Generator, ScanMode, guided scans
  store/           Content-addressed storage: Store interface + implementations
  reconcile/       Filesystem reconciliation: Plan, Apply, Distribute
  cmd/c4/          CLI binary (10 commands)
    internal/scan/ Progressive CLI scanner (platform-specific)
```

## Root Package (`c4`)

The core identity primitives. Zero dependencies outside stdlib.

| Type | Description |
|------|-------------|
| `ID [64]byte` | 64-byte SHA-512 digest. Formats as 90-char base58 string with `c4` prefix. |
| `IDs []ID` | Sortable slice. `IDs.Tree()` computes a merkle tree; `IDs.ID()` returns the tree root. |
| `Tree []byte` | Byte-packed merkle tree. Sorted-input guarantee per SMPTE ST 2114. |
| `Digest []byte` | Raw digest slice (DB compat). `.ID()` converts to `ID`. |
| `Identifiable` | Interface: `ID() ID`. |

Key functions:
- `Identify(io.Reader) ID` — hash content to a C4 ID
- `Parse(string) (ID, error)` — decode a 90-char C4 ID string
- `ID.Sum(ID) ID` — pairwise hash (canonical order enforced)
- `ID.Cmp(ID) int` — compare two IDs numerically

## c4m Package

Implements the C4 Manifest Format specification. Depends only on root `c4` and `store`.

### Core types

| Type | Description |
|------|-------------|
| `Manifest` | Collection of entries with optional patch chain base ID and range data. |
| `Entry` | Single filesystem entry: Mode, Timestamp, Size, Name, C4ID, Depth, flow/hard links. |
| `Encoder` | Writes manifests to `io.Writer`. Supports canonical and pretty-print modes. |
| `Decoder` | Reads manifests from `io.Reader`. Auto-sorts to canonical order. Handles block links. |
| `PatchSection` | One section of a patch chain (base or delta). |
| `Validator` | Validates manifest structure, field ranges, and sort order. |

### Operations (c4m/operations.go)

| Type | Description |
|------|-------------|
| `Source` | Interface: `ToManifest() (*Manifest, error)`. |
| `ManifestSource` | Wraps a `*Manifest` as a `Source`. |
| `DiffResult` | Added/removed/modified entries between two manifests. |
| `PatchResult` | Result of applying a patch to a manifest. |
| `Resolver` | Resolves patch chains via content store. |
| `ManifestCache` | Caches resolved manifests by C4 ID. |

### Other c4m types

| Type | Description |
|------|-------------|
| `ManifestBuilder` | Fluent API for constructing manifests programmatically. |
| `DirBuilder` | Helper for building directory subtrees in a manifest. |
| `Sequence` | Represents a media file sequence (`frame.[0001-0100].exr`). |
| `SequenceDetector` | Groups files into sequences by pattern. |
| `SequenceExpander` | Expands sequence entries into individual file entries. |
| `Conflict` | Reports conflicting entries during merge. |

### Key manifest methods

- `AddEntry`, `RemoveEntry`, `SortEntries`
- `ExtractSubtree(path)` — extract a directory and its children
- `EntryPaths(entries)` — reconstruct full paths from depth-based entries
- `Merge(a, b)` — combine two manifests, report conflicts
- `ComputeC4ID(manifest)` — canonical identification
- `Canonicalize(manifest)` — normalize to canonical form before identification

## scan Package

Directory scanner. Depends on `c4` and `c4m`.

| Type | Description |
|------|-------------|
| `Generator` | Configurable scanner: mode, excludes, guide, sequences. |
| `ScanMode` | `ModeStructure` / `ModeMetadata` / `ModeFull` |
| `FileSource` | Wraps a path + generator as a `c4m.Source`. |

Convenience: `scan.Dir(path, ...Option)` for simple scans.

Options: `WithMode`, `WithExclude`, `WithGuide`, `WithSequenceDetection`.

Type aliases re-export `c4m.Entry`, `c4m.Manifest`, `c4m.NewManifest`,
`c4m.NewDecoder`, `c4m.NewEncoder` for backward compatibility.

## store Package

Content-addressed storage. Depends only on root `c4`.

### Interfaces

| Interface | Methods |
|-----------|---------|
| `Source` | `Open(ID) (io.ReadCloser, error)` |
| `Sink` | `Create(ID) (io.WriteCloser, error)` |
| `Store` | `Source` + `Sink` + `Has(ID) bool` + `Put(io.Reader) (ID, error)` + `Remove(ID) error` |

### Implementations

| Type | Description |
|------|-------------|
| `Folder` | Flat directory: one file per ID. |
| `ShardedFolder` | Two-level directory using ID chars 3-4 as shard key. |
| `TreeStore` | Adaptive trie sharding: splits leaf dirs at threshold (default 4096). |
| `S3Store` | S3-compatible object store. SigV4 signing with stdlib only. |
| `MultiStore` | Writes to first, reads from all in order. |
| `RAM` | In-memory store (testing). |
| `Validating` | Wrapper that verifies content hashes on read/write. |
| `Logger` | Wrapper that logs all operations. |
| `DurableWriter` | Atomic write-to-temp-then-rename. |

### Configuration

- `OpenStore()` — opens from `C4_STORE` env var or `~/.c4/config`
- `DefaultStorePath()` — returns `~/.c4/store`

## reconcile Package

Filesystem reconciliation. Depends on `c4`, `c4m`, and `store`.

| Type | Description |
|------|-------------|
| `Reconciler` | Stateful reconciler with content sources and saver. |
| `ContentSource` | Interface: `Has(ID)` + `Open(ID)`. |
| `DirSource` | Wraps a directory + manifest as a `ContentSource`. |
| `Saver` | Interface: `Put(io.Reader) (ID, error)` + `Has(ID) bool`. |
| `Plan` | Ordered operation list with missing-content check. |
| `Operation` | Single filesystem operation (mkdir, create, move, remove, chmod, chtimes). |
| `Result` | Outcome counts and errors from `Apply`. |

Distribution (single-pass multi-target):

| Type | Description |
|------|-------------|
| `Target` | Interface: `Kind() string`. |
| `DirTarget` | Writes files to a directory. |
| `StoreTarget` | Stores content by C4 ID. |
| `DistributeResult` | Per-target outcomes from `Distribute`. |

## cmd/c4 (CLI)

Ten commands dispatched from `main.go`:

| File | Command | Category |
|------|---------|----------|
| `id.go` | `c4 id` | Observer |
| `cat.go` | `c4 cat` | Observer |
| `diff.go` | `c4 diff` | Observer |
| `log.go` | `c4 log` | Observer |
| `explain.go` | `c4 explain` | Observer |
| `paths.go` | `c4 paths` | Observer |
| `intersect.go` | `c4 intersect` | Observer |
| `patch.go` | `c4 patch` | Actor |
| `merge.go` | `c4 merge` | Actor |
| `split.go` | `c4 split` | Actor |

Supporting files: `flags.go` (custom flag parser), `helpers.go` (shared utilities),
`version.go`, `main.go` (dispatch + bare shortcuts).

`cmd/c4/internal/scan/` contains the progressive CLI scanner with platform-specific
implementations (darwin, linux, windows).

## Dependency Graph

```
cmd/c4 --> scan, c4m, store, reconcile, c4
reconcile --> c4m, store, c4
scan --> c4m, c4
c4m --> store, c4
store --> c4
c4 --> (stdlib only)
```

## Design Patterns

- **Content addressing**: all storage and identity flows through `c4.ID`
- **Interface-based storage**: `store.Store` interface with decorator wrappers (Validating, Logger, Multi)
- **Depth-based tree encoding**: c4m entries use `Depth int` + `Name string` (bare filename), not full paths
- **Guided scanning**: reuse IDs from a reference manifest for unchanged files (size + mtime match)
- **Canonical ordering**: entries sorted by natural sort; decoder auto-sorts on read
- **Patch chains**: append-only versioning via bare C4 ID separators in c4m files
- **Single-pass distribution**: `reconcile.Distribute` hashes + copies in one read pass
- **Atomic writes**: `store.DurableWriter` writes to temp file then renames
