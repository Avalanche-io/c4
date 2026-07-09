# Changelog

## Unreleased

### Safety defaults: self-capturing snapshots and patch pre-state

Two gaps between "the store is the safety net" and what the CLI put in
it are closed. See `design/safety-defaults.md`.

**Behavior change — `c4 patch` reconcile forms now store the prior
state by default.** `c4 patch <file.c4m> <dir>`, `c4 patch <dir>
<dir>`, and `c4 patch -r` capture the destination's pre-state before
applying: content that would be removed or overwritten, every
directory's one-level record, the root record, and the pre-state
manifest text — durable (batch barrier) before the first destructive
operation. After applying, stderr reports:

    prior state stored: <id> (revert: c4 patch -r <id> <dir>)

and that revert command works verbatim. `--no-store` opts out;
`--dry-run` still changes (and captures) nothing; `-s` remains as a
removal-time belt (now redundant in normal use). When no store is
configured, reconcile forms offer to create the default store;
non-interactive runs proceed with a warning. Measured on a 5,050-file
dest: worst case (everything vanishes) 1.0 s → 2.7 s; typical patch
(50 files change) unchanged at 0.4 s.

**Snapshots capture themselves.** `c4 id -s` (and every other ingest
path) now also stores the manifest's canonical text — retrievable by
the manifest's own C4 ID — and the scan root's one-level record
(retrievable by the root directory ID), and reports `stored:
<manifest-id>` on stderr after the durability barrier. Lose the tree
and the `.c4m` file: `c4 cat <manifest-id>` reproduces the manifest
byte-identically and `c4 patch <recovered.c4m> <dir>/` materializes
the tree from the store alone.

- `c4 patch -r` accepts a stored manifest ID as well as a changeset
  file. The changeset form is fixed for nested trees (the OldID names
  the root record, which now exists in the store and expands through
  stored directory records; previously the lookup always failed with
  "pre-patch manifest not found"). A revert target with a missing
  directory record is refused instead of silently truncating the tree.
- Capture rides the same batch durability barrier as every other
  write: the prior state is on stable storage before the first
  destructive operation.

### Store ingest performance: 44x faster snapshots

Snapshotting a 20,050-file / 105 MB tree with `c4 id -s` took ~180 s:
`TreeStore.Put` flushed every object with `F_FULLFSYNC` (6–13 ms per
device flush on darwin), sequentially, plus an O(n) `ReadDir` split
check per Put. See `design/store-ingest-performance.md`.

- New write-durability policy on the store: `store.SyncMode`
  (`SyncEach` / `SyncBatch` / `SyncNone`), selected via
  `(*TreeStore).SetSyncMode`, with `(*TreeStore).Sync` as the batch
  barrier and optional-interface discovery via `store.Syncer`.
  `MultiStore` forwards both. Library default unchanged (`SyncEach`,
  per-object F_FULLFSYNC).
- The CLI uses the batch barrier everywhere, with no flag to change
  it: objects land atomically with a cheap plain `fsync(2)` each, and
  one `F_FULLFSYNC` at command completion makes the whole batch
  durable. Durability is one default behavior — anything `c4` prints
  (IDs, revert commands) refers to durable state.
- `storeManifestContent` stores objects on a bounded worker pool
  (min(GOMAXPROCS, 16)); `TreeStore.Put` is now safe for concurrent
  use (publish step serialized under the store mutex).
- Leaf split accounting is now O(1) amortized via cached per-directory
  counts instead of a `ReadDir` per Put.

Gate benchmark (M-series, APFS, 20k files / 105 MB, fresh store per
run, store contents verified identical and materializing byte-identical
trees): master 178–180 s → batch-barrier default **4.3 s**
(re-measured flagless). Small-repo case (182 files): 1.4 s → 0.05 s.

### Reconcile performance: 43x faster materialization

Materializing a 20,050-file / 105 MB tree via `c4 patch snap.c4m dest/`
took 164.6 s wall against 13.5 s CPU — the process was blocked, not
computing. Root cause: `store.DurableWriter.Close` issues a per-file
`F_FULLFSYNC` (a full device write-cache flush on darwin), serialized by
a fully sequential Apply. See `design/reconcile-performance.md`.

- `reconcile.WithSyncMode` — created-file durability mirroring
  `store.SyncMode`: `SyncEach` (library default, durable per file),
  `SyncBatch` (atomic writes + one device barrier at the end of
  Apply — the CLI's mode for reconcile forms), `SyncNone` (scratch).
- Apply now runs consecutive create operations on a bounded worker pool
  (`reconcile.WithMaxConcurrency`, default min(GOMAXPROCS, 16));
  counters and errors merge in operation order.
- `reconcile.WithTrustedMetadata(true)` — Plan reuses target IDs when
  size+mtime match instead of re-hashing every existing file (the
  guided-scan contract). Enabled by the CLI, matching its guided scan.
- `reconcile.LocalSource` (optional `ContentPath` on sources) —
  Apply copies file-to-file from local stores; `store.DurableWriter`
  gained `ReadFrom` so copies use OS acceleration (`copy_file_range`
  reflinks on Linux CoW filesystems). `store.NewAtomicWriter` is the
  non-fsync writer. `Folder`, `ShardedFolder`, `TreeStore`,
  `MultiStore`, and `DirSource` implement `ContentPath`.

Gate benchmark (M-series, APFS, 20k files / 105 MB, verified
byte-identical): master 171.2 s → per-file durable 129.3 s →
batch-barrier default **4.4 s** (re-measured with the barrier as the
only mode; reference `cp -Rc` whole-tree clone: 2.9 s). No-op
re-apply: 0.74 s → 0.30 s.

## v1.0.13

### Scan performance: quadratic propagation eliminated

Whole-tree metadata propagation (computing directory sizes / timestamps from
children) was `O(directories × entries)` because three separate
implementations — `c4m.propagateMetadata`, `scan.PropagateMetadata`, and the
duplicate in `cmd/c4/internal/scan/metadata.go` — each rescanned the full
entry slice once per null-sized directory. On a 5.14M-entry tree this caused
a 16–20 minute hang at 100% CPU on a fast single-user host.

Replaced all three with one canonical implementation in `c4m` using a
single-pass depth-stack accumulator. New behavior:

- `c4m.PropagateMetadata([]*Entry)` is now an **exported function** — the
  one true implementation that producers of `*c4m.Manifest` should use.
- Algorithm is `O(N)` work, `O(max-depth)` memory.
- Empty directories resolve to `Size = 0` and keep their null `Timestamp`
  (matched explicitly via a `hadChildren` flag — fixes a latent edge case in
  the original sketch where empty-dir timestamps would have rendered as
  `time.Time{}` instead of `NullTimestamp()`).
- Cheap whole-manifest early-out: if no directory has any null Size /
  Timestamp / Mode, the call returns immediately (the common case for
  `ComputeC4ID` re-running `Canonicalize` on an already-resolved manifest).
- Nil-infectious semantics preserved per `c4m/SPECIFICATION.md`: any
  descendant with null Size or Timestamp poisons the parent recursively.
  This **fixes a pre-existing spec violation** in the scan-package
  implementation, which silently skipped null values instead of poisoning.

Removed: `scan.PropagateMetadata`, `scan.CalculateDirectorySize`,
`scan.GetMostRecentModtime`, plus the same set in
`cmd/c4/internal/scan/metadata.go`. The scan packages call
`c4m.PropagateMetadata` directly. `c4m/readiness_test.go` now asserts
PropagateMetadata IS exported (was previously asserted unexported).

Result on the original reproducer (`/Users/joshua/ws/repos`, 6.08M entries,
323 GB): **16–20 min hang → 2 min 24 s** (dominated by filesystem I/O). On
122K entries: walk + assemble 2.63s → 0.72s (3.6×). All canonical bytes
byte-identical pre/post.

### `c4m.Manifest.SortEntries` linearized

`sortSiblingsHierarchically` was `O(N²)` worst case on pathologically
nested chains because each recursive `processLevel` rescanned the
remaining tail. Replaced with a single-pass children-by-parent index
built via depth stack, plus a sort-each-group / depth-first emit phase.

Pathological-chain bench (depth = N, one child each):

| N | Old | New | Speedup |
|---:|---:|---:|---:|
| 1K | 553 µs | 153 µs | 3.6× |
| 10K | 50.9 ms | 1.80 ms | 28× |
| 100K | 10.9 s | 33.3 ms | **327×** |

Normal tree shapes (sqrt-fanout): unchanged ~`O(N log N)`. Dedup-keeps-last
and tail-append-orphans semantics preserved.

### New: `scan.WithProgress`

```go
type ScanStats struct {
    Entries, Bytes, Files, Dirs int64
    Current string
    Elapsed time.Duration
}

scan.WithProgress(func(ScanStats))
```

Fires at most every 1000 entries OR every 250 ms, whichever first, plus
once at end. Zero overhead when not registered. The callback receives a
value-copy `ScanStats` so it's safe even with the parallel walk added
below. Sub-scans triggered for directory C4 ID computation in `ModeFull`
do not report (would double-count).

### New: `scan.WithMaxConcurrency` (bounded parallel subdirectory walk)

```go
scan.WithMaxConcurrency(n)
// n =  0: auto (min(GOMAXPROCS, 16))
// n =  1: purely sequential (preserves pre-parallelism behavior)
// n >  1: explicit cap
```

A single shared buffered-channel semaphore is created in `GenerateFromPath`
and inherited by all `clone()`d sub-scans so the cap is global, not
per-call. `generateDir` does a non-blocking `select` per subdirectory: if
a slot is free it launches a goroutine, otherwise it walks inline. This
guarantees forward progress on any tree shape, never deadlocks, never
oversubscribes the disk. Each subdir's result slice is written to a
pre-allocated slot in source order so the post-walk stitch is
deterministic — **output is byte-identical regardless of `n`**.

Bench on 122K-entry tree, cold cache: **2.53s → 0.45s (5.6×)**. Warm cache:
0.78s → 0.46s (1.7×).

### New: `scan.WithContext` and `scan.WithEntryStream`

```go
scan.WithContext(ctx context.Context)
scan.WithEntryStream(cb func(*c4m.Entry) error)
```

`WithContext` attaches a context whose cancellation is observed at every
directory boundary and between entries within a directory. A partial
manifest is returned on cancel.

`WithEntryStream` installs a per-entry callback that fires once per
discovered entry, before it is added to the manifest. Returning a non-nil
error halts the scan and is returned alongside the partial manifest.
Under parallel walk the callback may be invoked from multiple goroutines
but is serialized by an internal mutex, so callback bodies themselves do
not need to be thread-safe. Order is the discovery order of the worker
pool — pair with `WithMaxConcurrency(1)` if strict walk-order is
required.

When either option is set, `GenerateFromPath` / `Dir` return the partial
manifest alongside the error rather than `nil` — this is opt-in behavior
that preserves the historical nil-on-error contract for existing callers.

The callback is not invoked for entries emitted by internal sub-scans
used to compute directory C4 IDs in `ModeFull` — only top-level walk
entries are streamed.

### New: `c4m.Manifest.WriteCanonical(io.Writer) error`

Streams the canonical form one entry per line, so very large manifests can
be hashed (or otherwise consumed) without materializing the full canonical
text as a single string allocation. `Canonical()` is unchanged in behavior:
it now calls `WriteCanonical` into a `bytes.Buffer` for back-compat.
`ComputeC4ID()` now streams canonical bytes through `io.Pipe` into
`c4.Identify`, dropping bytes/op by ~23% on a 100K-entry manifest. Useful
for the multi-million-entry case where the eliminated allocation would
otherwise be hundreds of MB. C4 IDs are byte-identical to prior versions.

### Progressive scanner: wired `c4m.PropagateMetadata`

`cmd/c4/internal/scan/progressive_scanner.go` previously never called any
propagation pass — directories in its output had null `Size` (-1) and
sometimes null `Timestamp`, diverging from the synchronous scanner. Fixed
by inserting `manifest.SortEntries()` + `c4m.PropagateMetadata(...)`
inside `OutputCurrentState`, after the recursive `addEntriesToManifest`
walk and before encoding. The streaming property of the producer pipeline
is preserved — propagation runs only at emit, not during streaming.

### Breaking changes

None for the c4m wire format or C4 ID values. Canonical bytes are
byte-identical pre/post for any tree that previously had no null
sizes/timestamps (the normal disk-scan case). The pre-existing spec
violation in `scan.PropagateMetadata` (permissive null handling) was
fixed in favor of the spec — manifests with mixed null/known sizes will
now propagate null to the root as the spec requires, where previously
the scan path would silently produce a partial sum.

Removed unexported helpers (`getDirectoryChildren`,
`calculateDirectorySize`, `getMostRecentModtime`, plus their
scan-package siblings) had no external users.

`c4m.PropagateMetadata` was renamed from the unexported
`c4m.propagateMetadata`. The previously-exported
`scan.PropagateMetadata`, `scan.CalculateDirectorySize`, and
`scan.GetMostRecentModtime` are gone — external callers (none were found
in the workspace) should migrate to `c4m.PropagateMetadata` (which is
the One True Implementation).

## v1.0.12

### Bug fixes

- Fix empty directory size reporting: size is 0, not null, when scanned

## v1.0.11

### Bug fixes

- Fix data races in ProgressiveScanner: added mutex for stage field and proper goroutine tracking for progressReporter
- Fix directory size calculation in internal scan: add c4m content size to match the public library behavior

### Code quality

- Fix typos in store package: mathods, cusomizable, contianed, writting

### Documentation

- Add `explain`, `paths`, `intersect` to CLI reference and README command table
- Create ARCHITECTURAL_CATALOG.md documenting package layout, types, and dependencies

## v1.0.10

### New: `c4 explain`, `c4 paths`, `c4 intersect` commands

Three new commands for querying and understanding c4m data:

- `c4 explain id|diff|patch` — human-readable narration of what a command would do, without modifying anything
- `c4 paths` — bidirectional conversion between c4m format and plain path lists
- `c4 intersect id|path` — find common entries between two c4m files, by content identity or by path

### Block link semantics

Patch chain checkpoints replaced with O(1) block links in the decoder and spec. Block links allow the decoder to verify chain integrity without scanning all preceding content.

### Directory size includes c4m content

Directory entry sizes now include the size of the c4m content that describes the directory, giving a more accurate picture of total storage.

### Documentation

- Block link semantics design doc
- Large directory blocks design doc
- c4m identity paradox doc: rationale, edge cases, and open questions

## v1.0.9

### c4m canonical identification

All ID paths now detect c4m content and canonicalize it before identification. This ensures that semantically identical c4m files always produce the same C4 ID regardless of whitespace or field formatting differences.

### Enhanced `c4 cat`

- `-e` flag for ergonomic (pretty-printed) output
- `-r` flag for recursive directory expansion
- File path support: `c4 cat file.c4m` reads from disk, not just from store

### Bug fixes

- `storeDirectoryC4m` now uses Canonicalize+Canonical to match ComputeC4ID

### Documentation

- c4m canonical storage design document
- c4m detection design doc: two-phase check for recognizing c4m content

## v1.0.8

### Bare `c4` shortcuts

- `c4 <path>` works as `c4 id -s`: identify and store in one step
- Bare stdin stores by default; `-x` flag skips storage
- CR (`\r`) bytes rejected in c4m input to enforce Unix line endings

### Auto-sort on read

The decoder now auto-sorts entries to canonical order on decode. If the input was not in canonical order, a note is emitted to stderr.

### Spec clarifications

- Null mode (`-`) vs zero mode (`----------`) are semantically different and must not be conflated

### Bug fixes

- Null mode rendering fixed in pretty-print encoder and paths output

### Documentation

- Design docs: auto-sort-on-read, bare-command-defaults
- Updated progressive-scanning design doc

## v1.0.7

### New commands

- `c4 explain` — human-readable narration for `id`, `diff`, and `patch` operations
- `c4 paths` — convert between c4m format and plain path lists (bidirectional)
- `c4 intersect id|path` — find common entries between two c4m files by content or path

### README and install

- Rewrite of install section with Homebrew, binary downloads, and from-source options
- Toolkit table showing the cross-language C4 ecosystem
- License corrected to Apache 2.0

## v1.0.6

Version bump only (no functional changes).

## v1.0.5

### Scan: nothing is skipped

The scanner no longer hardcodes a `.git` skip or hides hidden files. Everything is included by default. Exclusions are runtime-only via `--exclude`, `--exclude-file`, or the `C4_EXCLUDE_FILE` environment variable.

### License

Switched from MIT to Apache 2.0.

### Documentation

- FAQ: SHA-512 permanence rationale
- FAQ: text vs binary format rationale (2% overhead finding)
- README: merge example added
- FAQ: fix c4py repo URL

## v1.0.4

### New: Multi-target distribution

`reconcile.Distribute` reads a source directory once and writes to
multiple destinations simultaneously — directories, stores, or both.
Content is hashed during the single read pass. This is the library
primitive behind c4sh's multi-destination copy.

```go
result, err := reconcile.Distribute("/mnt/card/",
    reconcile.ToDir("/mnt/shuttle/"),
    reconcile.ToDir("/mnt/backup/"),
    reconcile.ToStore(myStore),
)
```

### Documentation

- Terminology cleanup: "c4m file" used consistently in user-facing docs
- Removed unqualified performance claims ("instant" → specific descriptions)

## v1.0.3

### New: Public scan package

The directory scanner is now a public package (`github.com/Avalanche-io/c4/scan`)
for use by ecosystem tools (c4sh, c4-mcp, etc.):

```go
m, err := scan.Dir("/path/to/dir")
m, err := scan.Dir("/path", scan.WithMode(scan.ModeFull))
```

### New: Quiet mode (`-q`)

`c4 id -q`, `c4 diff -q`, and `c4 patch -q` suppress stdout output
for commands run for their side effects (e.g., `c4 id -sq .` stores
content silently).

### Performance: Guided scan

`c4 diff` and `c4 patch` now use guided scanning when comparing a c4m
file against a directory. Only files with changed size or timestamp are
hashed — unchanged files reuse the C4 ID from the reference manifest.

### Bug fixes

- Directory timestamps now set correctly after reconciliation (deepest first)
- Existing directories get metadata updates, not just newly created ones
- Timestamp comparisons truncated to second precision at the c4m/filesystem
  boundary (prevents spurious diffs from sub-second differences)
- `isTerminal()` checks stdin, not stderr, for interactive prompts
- Store setup prompt appears before scanning, not after
- 12 code quality fixes: unchecked error returns on `Put()`, `Remove()`,
  `MkdirAll()`; TOCTOU race in content sourcing; `os.Lstat` for symlink
  safety; `filepath.Join` for Windows path construction

## v1.0.2

- `-q` (quiet) flag on `id`, `diff`, and `patch` for silent side-effect operations
- Directory metadata (timestamps, permissions) now set on existing directories during reconciliation
- Directory chtimes deferred to post-pass for both new and existing directories

## v1.0.1

- Guided scan optimization for `diff` and `patch` — only hashes files with changed metadata
- Timestamp precision fix at c4m/filesystem boundary (second vs nanosecond)
- Interactive prompt timing fix (before scan, not after)
- `isTerminal()` checks stdin instead of stderr

## v1.0.0

First stable release.

### CLI

Eight commands with consistent semantics — all accept c4m files or directories interchangeably:

- `c4 id` — Identify files, directories, or c4m files
- `c4 cat` — Retrieve content by C4 ID from store
- `c4 diff` — Compare two states, produce patches
- `c4 patch` — Apply target state: reconcile directories, resolve patch chains
- `c4 merge` — Combine two or more filesystem trees
- `c4 log` — List patches in a chain
- `c4 split` — Split a patch chain at a given point
- `c4 version` — Print version

### c4m Format

Formally specified manifest format (SMPTE ST 2114:2017) for describing filesystem trees as plain text. Includes:

- Entry-only format with no directives — works natively with grep, awk, sort, diff
- Patch chains for incremental versioning
- Media file sequence notation (`frame.[0001-0100].exr`)
- Inline range data for self-contained c4m files
- Progressive resolution: structure, metadata, and full scan modes
- Universal Filename Encoding for arbitrary byte sequences in filenames
- Flow links for cross-location data relationships

### Content Store

- Local: adaptive trie-sharded directory with atomic writes
- S3: any S3-compatible object store (AWS, MinIO, Backblaze, Wasabi, Ceph)
- Multi-store: write to first, read from all (`C4_STORE=a,b,c`)
- Zero external dependencies — S3 SigV4 signing implemented with stdlib

### Filesystem Reconciliation

`c4 patch` materializes c4m descriptions to real directories with:

- Pre-flight content availability check — never starts what it can't finish
- Idempotent operations — safe to re-run after interruption
- Move optimization — reuses content already present at different paths
- `-s` stores pre-patch state and removed content for reversal
- `-r` reverts to pre-patch state using stored manifest
- `--dry-run` previews changes without modifying the filesystem

### Platform Support

- macOS, Linux, Windows (amd64, arm64)
- Go 1.16+ minimum
- Zero external dependencies
