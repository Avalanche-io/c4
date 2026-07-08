# Store Ingest Performance

Requirements and design for making store ingest (`c4 id -s`,
`c4 patch <dir>`, `c4 diff -s`) bounded by hashing and file I/O, not by
per-file device flushes. Companion to `reconcile-performance.md`, which
fixed the same disease on the materialize (read-side) path.

## Motivation (measured)

Snapshotting a 20,050-file / 105 MB tree into a local TreeStore on APFS
(M-series Mac):

| Operation | Wall |
|---|---:|
| `c4 id -s` (v1.0.13) | 178–210 s across sessions |
| clonefile fork of the same tree | 0.10 s |

Root causes, in order of impact:

1. **Per-object F_FULLFSYNC.** `TreeStore.Put` calls `os.File.Sync` on
   every ingested object before rename — on darwin that issues
   `fcntl(F_FULLFSYNC)`, a full device write-cache flush. Measured on
   this machine: 6.4 ms per flush quiet, 12.6 ms under device
   contention → 20k objects ≈ 130–250 s of pure waiting.
2. **Sequential ingest.** `storeManifestContent` stores objects one at
   a time, so every per-file latency is serialized.
3. **O(n) split check per Put.** `TreeStore.maybeSplit` re-reads the
   whole leaf directory (`os.ReadDir`) after every Put to count files.
   While a leaf grows toward the 4096 split threshold the check averages
   ~2k entries per Put — an O(n²) tax that surfaces once the fsyncs are
   gone.

Reference probe (this machine, 5 KB temp-write+rename cycles, quiet
device): no fsync 112 µs/file, plain `fsync(2)` 126 µs/file,
`F_FULLFSYNC` 6.4 ms/file, one directory-fd `F_FULLFSYNC` barrier
5.4 ms total. Plain fsync costs 14 µs over no-fsync — the batch
barrier's per-object step is effectively free.

## The default question

The store is the safety net — snapshots and `c4 patch -s` pre-state
backups live there — so the materialize answer (no-fsync opt-in via
`--no-fsync`, durable by default) cannot simply be inverted to
no-fsync-by-default here. The moment `c4 id -s` reports success, the
user may delete the source tree; a completed ingest must be durable.

But per-object durability is stronger than needed. While the command
runs, the source tree still exists and the store copy is redundant.
The real requirement is **durable at completion**, not durable per
object.

## Options weighed

- **(a) Plain fsync per object instead of F_FULLFSYNC.** ~10–100x
  cheaper; data reaches the device but may sit in its volatile cache
  indefinitely. Alone, the command can report success with nothing
  guaranteed on stable storage — no better a guarantee than (c)'s
  no-fsync, just a smaller window. Rejected as a complete answer, but
  it is the right per-object step inside (b).
- **(b) Batch barrier (chosen default).** Per object: write temp,
  plain `fsync(2)`, atomic rename. At the end of the whole ingest: one
  `F_FULLFSYNC` (11 ms total, size-independent). The per-object fsync
  is required for the barrier to be sound: `F_FULLFSYNC` flushes the
  device cache plus the pages of the fd it is issued on — it does not
  write back other files' dirty pages from the OS page cache, so each
  object must already have been handed to the device. It also orders
  data before the rename that publishes it, preserving the
  complete-or-absent guarantee. Crash mid-command: some objects lost,
  none partial, command never reported success, source intact. Crash
  after the barrier: everything durable.
- **(c) No fsync at all (`--no-fsync`).** Fastest; after a power
  failure objects may be lost even though the command succeeded. Right
  for scratch stores and re-runnable ingests, wrong as a default for
  the safety net. Kept as an explicit opt-out, consistent with
  `c4 patch --no-fsync`.
- **(d) Async fsync pool (parallel F_FULLFSYNC).** Full-cache flushes
  serialize at the device — reconcile measured only 171 s → 129 s from
  parallelizing them. Complexity without the win. Rejected.
- **Default: (b)**, with `--durable` selecting per-object F_FULLFSYNC
  and `--no-fsync` selecting (c). **Library default is unchanged**
  (`SyncEach`, per-object F_FULLFSYNC): only the CLI, which owns a
  clear batch lifecycle and can place the barrier, opts into
  `SyncBatch`. Library users who never call `Sync()` keep full
  durability without reading release notes.

### Parallelism and the split check

- Ingest Puts are independent; `storeManifestContent` runs them on a
  bounded pool (min(GOMAXPROCS, 16), the reconcile/scan precedent).
  Hashing, temp writes, and plain fsyncs overlap; `TreeStore.Put`
  serializes only the cheap publish step (exists-check, rename, split
  accounting) under its existing mutex, making Put safe for concurrent
  use. Directory c4m objects are stored after the pool drains, so
  entry ID updates never race.
- The split check becomes O(1) amortized: `TreeStore` keeps a
  per-leaf-directory file count (seeded by one `ReadDir` on first
  touch, incremented per Put, invalidated on split). The cache may
  drift if an external process mutates the store concurrently — the
  only consequence is a slightly early or late split, which affects
  layout, never correctness.

## Chosen design — API additions

| Addition | Purpose |
|---|---|
| `store.SyncMode` (`SyncEach`, `SyncBatch`, `SyncNone`) | Write-durability policy. `SyncEach` is the default and matches v1.0.13 behavior. |
| `(*TreeStore).SetSyncMode(SyncMode)` | Select the policy (set before writing, not concurrently with writes). |
| `(*TreeStore).Sync() error` | The batch barrier: makes every object written so far durable. No-op under `SyncEach` (already durable), `SyncNone` (waived), or when nothing was written. |
| `store.Syncer` interface | Optional-interface discovery of `Sync`, like `Walker` / `LocalSource`. |
| `MultiStore.SetSyncMode` / `MultiStore.Sync` | Forward both to member stores that support them. |
| `c4 id --durable / --no-fsync`, `c4 diff --durable / --no-fsync`, `c4 patch --durable` | CLI policy selection for ingest; batch is the default. |

`DurableWriter` generalizes its internal no-sync flag to the three
modes; `NewDurableWriter` and `NewAtomicWriter` are unchanged, and
`TreeStore.Create` hands out writers that follow the store's mode.
`Folder` and `ShardedFolder` stay durable-only — `OpenStore` never
returns them and they are not on the ingest path.

CLI wiring: the bulk-ingest store (`getOrSetupStore`) gets the
flag-selected mode (default `SyncBatch`) and every ingest path issues
the barrier at completion — `storeManifestContent` after its last Put
(so in `c4 patch <dir> <dir>` the source content is durable in the
store *before* the destination is mutated), and the single-Put paths
(`id -s <file>`, stdin, c4m normalize) right after their Put. The
reconcile safety-net store in `c4 patch` (`-s` pre-state manifests and
`WithStoreRemovals` content) keeps the durable default regardless of
flags: it ingests content that is about to be destroyed, where
per-object durability is the point.

## Failure modes and trade-offs

- `SyncBatch`: a power failure before the barrier may lose recently
  ingested objects (never partial ones — data is fsynced before the
  rename publishes it). The command has not reported success at that
  point and the source still exists; re-running the ingest heals the
  store. On non-darwin platforms plain fsync and `os.File.Sync` are the
  same call, so `SyncBatch` costs one extra directory fsync and gives
  full per-object durability.
- `SyncNone`: after a power failure a *published* object may be empty
  or missing even though the command succeeded. Scratch use only.
- The end barrier flushes the device write cache and forces the APFS
  transaction containing the renames to commit; the residual exposure
  is filesystems that lie about cache flushes, which no fsync strategy
  survives.
- Concurrent `Put` publishes under one mutex; `Has`/`Open` racing a
  concurrent split may transiently miss an object mid-redistribution —
  the same exposure an external writer has always had. The CLI's
  read-then-ingest phases do not overlap.

## Measured results (gate benchmark)

Synthetic 20,050-file / ~105 MB tree (200 dirs x 100 files + 50 root
files, 5240 B each), fresh TreeStore per run, `c4 id -s -q`, APFS,
M-series Mac (macOS 26.6). Both runs shown where two were taken.

| Run | Wall | Notes |
|---|---:|---|
| master v1.0.13 | 178.4 s / 180.2 s | per-object F_FULLFSYNC |
| branch default (batch barrier) | **4.11 s / 4.02 s** | **~44x; target met** |
| branch `--durable` | 163.1 s | per-object F_FULLFSYNC kept; parallelism gains little (flushes serialize at the device — confirms rejection of (d)) |
| branch `--no-fsync` | 3.94 s / 3.87 s | the barrier + per-object fsyncs cost ~0.15 s total |
| clonefile fork reference | 0.10 s | lower bound, no hashing |

Every run (master and branch) produced the identical store: 20,250
objects, same du footprint. Completeness of the batch-barrier store
verified end-to-end: `c4 gc snap.c4m` dry-run reports the full object
set reachable and zero garbage, and `c4 patch snap.c4m dest/`
materializes a tree byte-identical to the source (`diff -rq`).

Small-repo case (the c4 repo tree sans .git, 182 files / ~20 MB,
fresh store): master 1.45 s / 1.33 s → branch 0.06 s / 0.05 s.
