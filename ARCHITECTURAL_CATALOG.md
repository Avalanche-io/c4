# Architectural Catalog

Quick reference for the c4 codebase. Consult before adding types, interfaces,
or packages to avoid duplicating what already exists.

## Package Layout

```
github.com/Avalanche-io/c4
  c4/              Root package: ID, Tree, Identify(), Parse()
  c4m/             c4m format: Entry, Manifest, Encoder, Decoder, patch chains, Journal
  scan/            Directory scanner: Generator, ScanMode, reuse guides
  store/           Content-addressed storage: Store interface + implementations
  reconcile/       Filesystem reconciliation: Plan, Apply, Distribute
  cmd/c4/          CLI binary (12 verbs)
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
| `DiffResult` | Added/removed/modified entries between two manifests. `Diff` matches entries by full path (never bare name), so same-named files in different directories cannot collide. |
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

### Journal (c4m/journal.go)

The store journal `<store>/log.c4m` — an ordinary c4m patch chain in
which the store records every claim (one section per claim: entry line
+ bare-ID boundary). The print-barrier leg between store durability and
stdout.

| Type | Description |
|------|-------------|
| `Journal` | `OpenJournal(storeRoot)`; `Append(Claim)` (durable before return: OS lock that dies with its holder, torn-tail truncation, fsync); `Claims()` (drops a torn tail, warns once). |
| `Claim` | ScanStart (the walk's start instant, NOT append time — the re-scan racy rule measures against it), Size, Name, Origin, ID. `EntryLine()` renders the recorded text (what `c4 log` reprints). |

Platform locks: `journal_lock_unix.go` (flock, EINTR loop) /
`journal_lock_windows.go` (LockFileEx).

### Key manifest methods

- `AddEntry`, `RemoveEntry`, `SortEntries`
- `ExtractSubtree(path)` — extract a directory and its children
- `EntryPaths(entries)` — reconstruct full paths from depth-based entries
- `Merge(a, b)` — combine two manifests, report conflicts
- `ComputeC4ID(manifest)` — canonical identification (streams canonical
  bytes through `io.Pipe` into `c4.Identify`, no full-string allocation)
- `Canonicalize(manifest)` — normalize to canonical form before identification
- `Canonical() string` — returns the canonical text (uses `WriteCanonical` internally)
- `WriteCanonical(io.Writer) error` — stream canonical bytes, one entry per
  line, for hashing large manifests without materializing the full string

### Metadata propagation

`c4m.PropagateMetadata([]*Entry)` is the **One True Implementation** for
resolving null directory `Size` and `Timestamp` from descendants. Producers
of `*c4m.Manifest` — including `scan` and `cmd/c4/internal/scan` — call it
directly rather than maintaining parallel copies (the duplicates were
removed in v1.0.13 along with the underlying O(D × N) algorithm).

Properties:

- Single-pass depth-stack accumulator: O(N) work, O(max-depth) memory.
- Nil-infectious per `c4m/SPECIFICATION.md`: any null descendant Size /
  Timestamp poisons the parent recursively to root.
- Empty directories resolve to Size = 0, null Timestamp.
- Whole-manifest early-out: returns immediately if no directory has any
  null values (fast path for repeated `Canonicalize`).
- **Precondition**: entries must be in filesystem-walk order. `SortEntries`
  produces this; `scan.GenerateFromPath` emits it directly.

## scan Package

Directory scanner. Depends on `c4` and `c4m`.

| Type | Description |
|------|-------------|
| `Generator` | Configurable scanner: mode, excludes, guides, sequences. |
| `ScanMode` | `ModeStructure` / `ModeMetadata` / `ModeFull` / `ModeContent` (the content projection: mode+timestamp null at every level; machine/clock/umask-independent IDs) |
| `FileSource` | Wraps a path + generator as a `c4m.Source`. |

Convenience: `scan.Dir(path, ...Option)` for simple scans.

Options:

| Option | Purpose |
|---|---|
| `WithMode` | structure / metadata / full |
| `WithSymlinks` | follow symlinks |
| `WithHidden` | include dotfiles |
| `WithSequenceDetection` | collapse `file.[0001-0100].exr` patterns |
| `WithExclude`, `WithExcludeFile` | glob exclusions |
| `WithGuide` | restrict the scan to paths present in a reference manifest (a root-relative path FILTER — it does not reuse the guide's IDs; no CLI flag drives it since v8 repurposed `-c`) |
| `WithReuseGuide(m, scanStart)` | metadata-trusted re-scan (the CLI's `-c`): a file reuses the guide's ID iff path+size+mtime match at second precision AND mtime is strictly older than scanStart (the racy rule — git's racy-index shape, no constants). `ReuseStats()` returns (reused, rehashed). |
| `WithProgress(cb)` | periodic `ScanStats` callbacks; zero overhead when unset |
| `WithMaxConcurrency(n)` | cap worker pool (0 = auto, 1 = sequential, n > 1 = explicit) |
| `WithContext(ctx)` | cancellation observed at directory + entry boundaries |
| `WithEntryStream(cb)` | per-entry callback; non-nil error halts scan with partial manifest |

Parallel walk: subdirectories run on a bounded worker pool (default
`min(GOMAXPROCS, 16)`) via a shared semaphore inherited by all `clone()`d
sub-scans, so the cap is global. Each parent stitches its children back in
source order before the final `SortEntries` pass — **output is
byte-identical regardless of concurrency level**. `WithMaxConcurrency(1)`
forces purely sequential.

Directory identity (ModeFull): computed **bottom-up** during the walk —
each directory's C4 ID is the hash of the canonical one-level listing of
its already-scanned direct children (spec: files before dirs, natural
sort, one canonical line each; empty dir hashes the empty string), and its
null Size/Timestamp are resolved via `c4m.PropagateMetadata` over
`[self, direct children]`. Directories are therefore never re-scanned
(O(N) total, previously O(2^depth)) and guided scans stay root-anchored
(guide paths are only matched against the single main walk). The private
`Generator.dirIdentity` function field is the seam for alternate
directory canonicalizations (e.g. a future content mode); the default is
`canonicalDirID`. `clone()` is used only for symlink-target sub-scans and
deliberately does not copy the guide (guide paths are root-relative).
Directory entries stream post-order via `WithEntryStream` — fully
resolved at emit time.

Streaming + cancellation: when `WithContext` or `WithEntryStream` is set,
`Dir` / `GenerateFromPath` return the *partial* manifest alongside any
error (cancellation or callback failure) rather than `nil`. Existing
callers that pass neither option keep the historical nil-on-error
contract. The entry-stream callback is serialized via an internal mutex so
it stays single-threaded from the callee's perspective even under parallel
walk; order is discovery order (non-deterministic) unless paired with
`WithMaxConcurrency(1)`.

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
| `Syncer` | Optional: `Sync() error` — batch durability barrier: makes every object written so far durable. Implemented by `TreeStore` and `MultiStore` (forwards). |

### Implementations

| Type | Description |
|------|-------------|
| `Folder` | Flat directory: one file per ID. |
| `ShardedFolder` | Two-level directory using ID chars 3-4 as shard key. |
| `TreeStore` | Adaptive trie sharding: splits leaf dirs at threshold (default 4096; O(1) amortized split accounting via cached per-leaf counts). `SetSyncMode` selects the write-durability policy (`SyncEach` default / `SyncBatch` / `SyncNone`); `Sync` is the batch barrier. `Put` is safe for concurrent use. `Walk` enumerates every stored object (verification/tests; the seam a future journal-rooted collector needs). |
| `S3Store` | S3-compatible object store. SigV4 signing with stdlib only. |
| `MultiStore` | Writes to first, reads from all in order. |
| `RAM` | In-memory store (testing). |
| `Validating` | Wrapper that verifies content hashes on read/write. |
| `Logger` | Wrapper that logs all operations. |
| `DurableWriter` | Atomic write-to-temp-then-rename; Close flushes per its `SyncMode`. `NewDurableWriter` flushes to stable storage on Close (`SyncEach`); `NewAtomicWriter` skips the flush (`SyncNone` — atomic but not crash-durable, for scratch writes re-materializable from a store); `TreeStore.Create` hands out writers following the store's mode. `ReadFrom` delegates to the temp file so io.Copy gets OS copy acceleration (copy_file_range on Linux). |

Local-path access: `Folder`, `ShardedFolder`, `TreeStore`, and
`MultiStore` implement `ContentPath(id) (string, bool)`, returning the
local filesystem path for existing content. This satisfies
`reconcile.LocalSource`, enabling file-to-file copy fast paths.

### Configuration

- `OpenStore()` — opens from `C4_STORE` env var or `~/.c4/config`
- `DefaultStorePath()` — returns `~/.c4/store`

## reconcile Package

Filesystem reconciliation. Depends on `c4`, `c4m`, and `store`.

| Type | Description |
|------|-------------|
| `Reconciler` | Stateful reconciler with content sources and saver. |
| `ContentSource` | Interface: `Has(ID)` + `Open(ID)`. Must tolerate concurrent `Open` when Apply concurrency > 1. |
| `LocalSource` | Optional interface: `ContentPath(ID) (string, bool)`. Apply prefers local paths for file-to-file copies (CoW clone hook point — see design/reconcile-performance.md). |
| `DirSource` | Wraps a directory + manifest as a `ContentSource` (also a `LocalSource`). |
| `Saver` | Interface: `Put(io.Reader) (ID, error)` + `Has(ID) bool`. |
| `Plan` | Ordered operation list with missing-content check. |
| `Operation` | Single filesystem operation (mkdir, create, move, remove, chmod, chtimes). |
| `Result` | Outcome counts and errors from `Apply`. |

Options:

| Option | Purpose |
|---|---|
| `WithSource` | add a content source |
| `WithDryRun` | plan-only Apply |
| `WithStoreRemovals` | store content before removal |
| `WithSyncMode` | created-file durability, mirroring `store.SyncMode`: `SyncEach` default (durable per file); `SyncBatch` = atomic writes + one device barrier at end of Apply (the CLI's mode); `SyncNone` scratch only |
| `WithMaxConcurrency` | Apply create workers: 0 = auto (min(GOMAXPROCS, 16)), 1 = sequential |
| `WithTrustedMetadata` | default false; true lets Plan reuse target IDs on size+mtime match (guided-scan contract) instead of hashing |

Apply runs consecutive create operations on a bounded worker pool,
merging counters and errors in operation order (deterministic output).
Creates prefer a `LocalSource` path — file-to-file copy with OS
acceleration, falling back to streaming `Open` on any failure.

Distribution (single-pass multi-target):

| Type | Description |
|------|-------------|
| `Target` | Interface: `Kind() string`. |
| `DirTarget` | Writes files to a directory. |
| `StoreTarget` | Stores content by C4 ID. |
| `DistributeResult` | Per-target outcomes from `Distribute`. |

## cmd/c4 (CLI)

Twelve verbs dispatched from `main.go` (surface of record:
`design/snapshot-loop/round-1/surface-v7.md` + round-2 amendments; the
reference pages live in `help.go` and print via each verb's `--help`):

| File | Command | Category |
|------|---------|----------|
| `id.go` | `c4 id` | Observer; writer only with `-s` |
| `restore.go` | `c4 restore` | THE tree writer (dry-run default; `--force`) |
| `cat.go` | `c4 cat` | Observer (verified reads; ID/path descent) |
| `diff.go` | `c4 diff` | Observer |
| `patch.go` | `c4 patch` | Observer (text algebra; never touches directories) |
| `log.go` | `c4 log` | Observer (journal + chain sections) |
| `explain.go` | `c4 explain` | Observer |
| `paths.go` | `c4 paths` | Observer |
| `intersect.go` | `c4 intersect` | Observer |
| `merge.go` | `c4 merge` | Observer (text out) |
| `split.go` | `c4 split` | Writes its two named output files |
| `version.go` | `c4 version` | Observer |

The print barrier (`design/snapshot-loop`): no ID reaches stdout before
its content is durable, its journal entry appended and fsynced
(`journalClaim` in `helpers.go` — fatal on append failure), and the
store barrier passed. Crash trial: `design/snapshot-loop/kill9-crucible.sh`.

Safety defaults:

- **Self-capturing snapshots** — `c4 id -s` stores every file's bytes,
  every directory's one-level record deepest-first, and the root record
  (`storeManifestSelf`, ID = `ComputeC4ID` = THE snapshot ID), journals
  the claim, then prints the ID (implies `-q`). `stored:` on stderr is
  narration.
- **Restore pre-image** — `restore --force` snapshots the destination
  (complete default scan) durably and journals it BEFORE the first
  destructive operation; stdout line 1 is the pre-image ID (the undo
  handle), line 2 the recomputed as-built ID. Targets resolve via
  `resolveRestoreTarget` (store address with optional `storeDescend`
  path descent, or .c4m file); one-level records expand through stored
  directory records (`expandIfRecord` in `cat.go`) and incomplete
  expansions are refused (`validateRevertTarget` in `restore.go`).
- **Verified reads** — `readVerified` / `verifiedManifestFromStore`
  (`cat.go`): every store read rehashes or nothing is written.
- **Re-scan trust** — `id -c` wires `scan.WithReuseGuide`; the guide's
  scan start resolves from the journal claim matching the guide's root
  ID (`resolveGuideScanStart`), falling back to the guide file's mtime.
  `--verify` forces full re-hash and reports obfuscated changes
  (`reportObfuscated`). Partial scans declare on stderr and exit 2
  (`reportPartial`).

Supporting files: `flags.go` (custom flag parser + `--help` pages),
`helpers.go` (shared utilities incl. `journalClaim`, `claimName`,
`claimOrigin`), `help.go` (the reference pages), `main.go` (dispatch +
bare read-only shortcuts; `-s` is the only write opt-in).

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
- **Batch durability barrier**: CLI ingest lands objects with cheap per-object fsyncs and issues one `F_FULLFSYNC` (`store.Syncer.Sync`) at completion — durable-at-completion instead of durable-per-object (`design/store-ingest-performance.md`)
- **Print barrier**: content durable → store barrier → journal append fsynced → only then the ID prints ("printed ⇒ durable ⇒ recoverable"); the journal is an ordinary c4m patch chain inside the store
- **Identity unification**: THE snapshot ID = the root directory's one-level canonical listing ID (`ComputeC4ID`); no full-text manifest object is stored — the tree recovers through per-directory records
- **Safety defaults**: snapshots are self-capturing and journaled; restore is dry-run by default and `--force` journals the pre-image (the undo handle) before destroying anything; every store read is rehash-verified
