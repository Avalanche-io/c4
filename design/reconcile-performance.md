# Reconcile Performance

Requirements and design for making `reconcile.Apply` and `reconcile.Plan`
fast enough that materializing a tree from a c4m file is bounded by the
filesystem, not by the reconcile package.

## Motivation (measured)

Materializing a 20,050-file / 105 MB tree from a c4m file on APFS
(M-series Mac, local TreeStore):

| Operation | Wall | CPU | Real disk |
|---|---|---|---|
| `c4 patch snap.c4m dest/` (v1.0.13) | 164.6 s | 13.5 s | 158 MB |
| `cp -Rc` (APFS clonefile) of the same tree | 3.08 s | — | 5.9 MB |
| whole-dir clonefile of a 13,837-file node_modules | 0.26 s | — | — |

The 12x gap between wall and CPU time shows the process is almost
entirely blocked, not computing. Root causes, in order of impact:

1. **Per-file F_FULLFSYNC.** `store.DurableWriter.Close` calls
   `os.File.Sync`, which on darwin issues `fcntl(F_FULLFSYNC)` — a full
   device write-cache flush, ~5–10 ms each. `applyCreate` routes every
   created file through `store.NewDurableWriter`, so a 20k-file
   materialization performs 20k device flushes: ~160 s of pure waiting.
2. **Fully sequential Apply.** Operations execute one at a time, so
   every per-file latency (fsync, open, close) is serialized.
3. **Plan re-hashes every existing destination file.** `Plan` walks the
   destination and computes the C4 ID of every regular file, even when
   size and mtime match the target entry exactly. A no-op re-apply of a
   large tree pays a full hashing pass. (The CLI already performs a
   guided scan of the destination that trusts size+mtime — Plan then
   redundantly re-hashes what the guided scan just skipped.)
4. **Byte copies instead of copy-on-write clones.** Content is streamed
   byte-by-byte from the store into the destination. On CoW filesystems
   (APFS, btrfs, XFS) the same result is achievable with a clone that
   costs ~0.15 ms per file and shares extents (5.9 MB vs 158 MB of new
   disk for the reference tree).

## Requirements

- Materialize the 20k-file tree in single-digit seconds when the caller
  opts out of per-file durability.
- Defaults preserve v1.0.13 semantics: writes remain durable
  (fsync-before-rename) unless the caller opts out.
- Zero new dependencies. The module has none; that is a feature.
- Minimal API surface, consistent with existing option patterns
  (`scan.WithMaxConcurrency` precedent).
- Every fast path falls back transparently to the existing slow path.

## Options weighed

### Durability

- **Keep per-file F_FULLFSYNC always** — correct but unusable for
  scratch materialization; rejected as the only mode.
- **Batch fsync (sync once at the end)** — an fsync of each file is
  still required for the rename to be durable; deferring them helps
  little on darwin where each is a device flush, and complicates the
  writer contract. Rejected.
- **F_BARRIERFSYNC on darwin** — cheaper than F_FULLFSYNC but requires
  a raw `fcntl` constant not exposed by the stdlib, and changes
  durability semantics subtly. Rejected (same fragility class as raw
  syscalls, see CoW below).
- **Opt-out option (chosen).** New `store.NewAtomicWriter`: identical
  temp-file + atomic-rename behavior (readers never observe a partial
  file) but no fsync — a crash may lose content, which is acceptable
  because the content remains pullable from the store by C4 ID.
  `reconcile.WithSync(false)` selects it for Apply. Default is
  unchanged (durable).

### Parallelism

- **Whole-plan parallelism (DAG scheduling)** — mkdir/create/remove
  dependencies would need explicit modeling. Over-engineered; rejected.
- **Parallel create batches (chosen).** Plan already orders operations
  as [mkdirs, moves, creates, symlinks, chmods, chtimes, removes,
  rmdirs]. Creates are independent of each other once their parent
  directories exist, so Apply runs consecutive OpCreate runs on a
  bounded worker pool and flushes the batch before any other operation
  type. Per-operation results and errors are collected into
  index-addressed slices and merged in operation order, so output is
  deterministic regardless of completion order.
  `reconcile.WithMaxConcurrency(n)` mirrors the scan package: 0 = auto
  (min(GOMAXPROCS, 16)), 1 = sequential, n > 1 explicit. Content
  sources must tolerate concurrent `Open` when concurrency > 1; all
  in-module sources do.

### Plan fast path

- **Always trust size+mtime** — silently changes Plan's detection
  contract; rejected as a default.
- **Opt-in trusted metadata (chosen).** `reconcile.WithTrustedMetadata
  (true)`: during the destination walk, when a regular file's size and
  mtime (second precision) match the target entry at the same path,
  Plan reuses the entry's C4 ID instead of hashing the file. This is
  exactly the guided-scan contract that already exists in `scan`
  (`WithGuide`) and that the CLI already applies to the same directory
  in the same command — so the CLI enables it, making Plan consistent
  with the diff it prints. Entries with null timestamps, symlinks, and
  directories never match; files failing the check are hashed as
  before.

### Copy-on-write clones

Reference: cloning replaces a ~2 ms byte copy (5 KB file: open, read,
write, close) with a ~0.15 ms metadata operation and shares extents.
Options for per-file CoW with zero new dependencies:

- **(a) `exec /bin/cp -c` per file** — process spawn is ~2–10 ms per
  file, i.e. slower than the byte copy it replaces for small files.
  Measured on this machine: see benchmark notes. Rejected.
- **(b) Build-tagged raw syscall on darwin** — Go on darwin calls
  through libSystem trampolines; raw syscall numbers via `syscall.
  Syscall` ride the deprecated `syscall(2)` shim and Apple has signaled
  its removal. Fragile; rejected.
- **(c) `golang.org/x/sys/unix.Clonefile`** — the correct wrapper, but
  it would be the module's first dependency. Not added without
  sign-off; the measured benefit is recorded in the benchmark so the
  decision can be made with numbers.
- **(d) Hardlink store blobs into the destination** — instant and
  zero-copy, but the destination then shares inodes with the store: a
  later in-place edit or chmod of a destination file corrupts the
  store. Unsafe for a general tool; rejected.
- **Interface + stdlib fallback (chosen).** A new optional interface:

  ```go
  // LocalSource is a ContentSource whose content lives in the local
  // filesystem.
  type LocalSource interface {
      ContentPath(id c4.ID) (string, bool)
  }
  ```

  `DirSource`, `store.Folder`, `store.ShardedFolder`, and
  `store.TreeStore` implement it. Apply prefers a local path when one
  exists: it attempts a platform clone (`cloneFile`, currently
  unsupported everywhere in-module) and falls back to a direct
  file-to-file copy. `store.DurableWriter` gains a `ReadFrom` method
  delegating to the underlying temp file, so byte copies through it use
  the stdlib's OS acceleration — on Linux `copy_file_range`, which
  *is* a CoW reflink on btrfs/XFS. Darwin gets true per-file CoW the
  day `cloneFile` is backed by `unix.Clonefile`; everything else is
  already in place.

## Chosen design — API additions

| Addition | Purpose |
|---|---|
| `store.NewAtomicWriter(final)` | Temp + atomic rename, no fsync. |
| `(*store.DurableWriter).ReadFrom` | OS-accelerated file-to-file copies. |
| `reconcile.WithSync(bool)` | Default true. False selects atomic (non-fsync) writes in Apply. |
| `reconcile.WithMaxConcurrency(int)` | Worker cap for parallel creates. 0 = auto, 1 = sequential. |
| `reconcile.WithTrustedMetadata(bool)` | Default false. True lets Plan reuse target IDs on size+mtime match. |
| `reconcile.LocalSource` | Optional interface: local path for content, enabling clone/direct copy. |
| `ContentPath` on `DirSource`, `Folder`, `ShardedFolder`, `TreeStore` | LocalSource implementations. |
| `c4 patch --no-fsync` | CLI opt-out of per-file fsync for reconcile paths. |

CLI behavior: `c4 patch` enables `WithTrustedMetadata(true)` (matching
its existing guided scan of the same directory) and auto concurrency.
Durability remains the CLI default; `--no-fsync` opts out.

## Measured results (gate benchmark)

Reproduced on the same machine class (M-series, APFS, macOS 26.6):
synthetic 20,050-file / ~105 MB tree, local TreeStore, `c4 patch
snap.c4m dest/` into an empty destination. All outputs verified
byte-identical to the source tree (`diff -rq`).

| Run | Wall | CPU (user+sys) | Notes |
|---|---:|---:|---|
| master v1.0.13 | 171.2 s | 13.1 s | baseline reproduced (164.6 s originally) |
| branch, durable default | 129.3 s | 26.4 s | parallel fsync; F_FULLFSYNC serializes at the device |
| branch `--no-fsync` | **4.0 s** | 42.5 s | **43x; target met** |
| `cp -Rc` reference | 2.9 s | 2.7 s | whole-tree clonefile |
| no-op re-apply, master | 0.74 s | 1.2 s | Plan hashes all 20k files |
| no-op re-apply, branch | 0.20 s | 0.5 s | trusted-metadata plan |

CoW option measurements:

- **`exec cp -c` per file**: 2.11 ms/file measured → 42 s sequential
  for 20k files; even parallelized it cannot beat the 4.0 s byte-copy
  path. Confirms rejection.
- **`x/sys unix.Clonefile` per file** (experiment outside the module,
  16 workers): 20,050 clones in **2.48 s** (0.123 ms/file), real disk
  allocation **6.4 MB** (df delta) vs ~157 MB for byte copies. Adopting
  x/sys would cut materialization wall time roughly in half again and
  reduce disk allocation ~25x — this is the measured case for the
  dependency decision.

## Failure modes and trade-offs

- `WithSync(false)`: after a power failure, recently created files may
  be empty or missing (never partial — the rename is still atomic).
  Recovery is re-running the patch; content is still in the store.
- `WithTrustedMetadata(true)`: a file whose content changed while
  preserving size and mtime is treated as unchanged — the same
  exposure the guided scan has always had.
- Parallel apply: errors keep operation order; counts are exact.
  Custom `ContentSource` implementations must tolerate concurrent
  `Open` calls when concurrency > 1.
