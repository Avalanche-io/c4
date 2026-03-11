# Progressive Filesystem Scan

## Problem

Creating a c4m of a real filesystem is expensive. For 25M files, a full
content scan (computing C4 IDs) takes hours. The current scanner is
all-or-nothing: it blocks until complete, or skips C4 IDs entirely. There is
no way to get a useful answer during the scan, resume after interruption, or
avoid re-scanning unchanged files.

## Design Principle

**The c4m file is the scan state.** There is no side database, no progress
file, no separate index. The null fields in the c4m tell you exactly what work
remains. Crash recovery is: read the file, find the dashes, keep going.

This follows directly from the C4 philosophy: "Partial knowledge is a valid
state, not an error. Every intermediate state is a complete, valid state."

## Resolution Levels

A c4m entry progresses through three resolution levels. Each level is a valid
c4m entry; higher levels add more information.

### Level 0 -- Structure (readdir)

`os.ReadDir` returns `fs.DirEntry` which provides name and type bits without
calling stat. This is the cheapest possible scan.

```
d--------- -                    -               src/
---------- -                    -               main.go
---------- -                    -               go.mod
l--------- -                    -               link.txt
```

Mode has type bits set but permissions unknown (`d---------` not `drwxr-xr-x`).
Timestamp is null (`-`) padded with spaces to 20 chars. Size is null (`-`)
padded with spaces to 15 chars. C4 ID is null (`-`) padded with spaces to
90 chars. The name field length is already final.

### Level 1 -- Metadata (stat)

After `os.Stat`, permissions, size, and mtime are known:

```
drwxr-xr-x 2026-03-09T01:00:00Z -               src/                                                                                           -
-rw-r--r-- 2026-03-08T14:30:00Z 4521            main.go                                                                                        -
-rw-r--r-- 2026-03-01T10:00:00Z 89              go.mod                                                                                         -
lrwxr-xr-x 2026-03-05T08:00:00Z 12              link.txt -> ../other.txt                                                                       -
```

Directory sizes remain null until child propagation. File sizes fill in.
Symlink targets appear. C4 IDs still padded null.

### Level 2 -- Identity (content hash)

After reading the full file content and computing SHA-512:

```
drwxr-xr-x 2026-03-09T01:00:00Z 4610            src/ c45Xk9abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijk
-rw-r--r-- 2026-03-08T14:30:00Z 4521            main.go c41bRzabcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijk
-rw-r--r-- 2026-03-01T10:00:00Z 89              go.mod c43Qwzabcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890abcdefghijk
lrwxr-xr-x 2026-03-05T08:00:00Z 12              link.txt -> ../other.txt
```

## Fixed-Width Line Property

The key insight: **with proper padding, lines never change width across
resolution levels.** Every phase update is an in-place byte overwrite. No
shifting. No cascading writes.

Field width analysis:

| Field | Phase 0 | Phase 1 | Phase 2 | Width |
|---|---|---|---|---|
| Mode | `d---------` | `drwxr-xr-x` | same | 10 (always) |
| Timestamp | `-` + 19 spaces | `2026-03-09T01:00:00Z` | same | 20 (always) |
| Size | `-` + 14 spaces | `4521` + 11 spaces | same | 15 (padded) |
| Name | `main.go` | same | same | varies per entry |
| C4 ID | `-` + 89 spaces | same | `c45Xk9...` (90 chars) | 90 (always) |

Total line width is constant: `10 + 1 + 20 + 1 + 15 + name_len + 1 + 90 + 1`.

The only variable is the size field. With 15 chars of padding, file sizes up
to ~100 TB fit without width change. For the rare case where a size exceeds
the padding (>100 TB), the line grows by a few bytes -- this is the only
situation requiring gap consumption, and it's vanishingly rare.

**Consequence**: Phase 1 and 2 are purely in-place `pwrite` operations. Gaps
are only consumed by structural changes (new files on rescan, the rare
oversized file). This drastically reduces the complexity of the gapped array.

### Spec Note

The c4m specification currently requires null values to be represented as a
single `-` character. The padded format (`-` followed by spaces) must be
documented as a valid alternative for working files. The parser already
handles this correctly (it trims whitespace and checks for `-`), but the
spec should explicitly bless it.

The final compaction pass normalizes all whitespace, producing a canonical
c4m with standard field spacing.

## CLI Interface

Two modes of operation:

### Foreground (streaming)

```
c4 scan ~/ws                       # stdout, blocks
c4 scan ~/ws | c4 diff old.c4m -   # pipe to diff
c4 scan ~/ws --level 1             # stop after metadata
```

Writes the c4m to stdout as it progresses. Useful for piping or quick
inspection. Blocks until the requested level completes.

### Background (detached)

```
c4 scan ~/ws -o workspace.c4m      # returns immediately
c4 scan --status                    # query active scans
c4 scan --stop workspace.c4m       # stop a running scan
```

CLI sends the scan request to c4d and exits. c4d manages the scan as a
background job, progressively writing to the output file. The file is always
a valid c4m, readable at any point.

## File Format: Gapped Array

The c4m file uses blank lines as gap space between directory chunks. The c4m
parser already silently skips blank lines, so this is invisible to readers.

### Layout

Entries in sorted order within each directory chunk. Blank lines between
chunks serve as gap space for structural changes (new files on rescan).
Since field updates are in-place (fixed-width lines), gaps are NOT consumed
by phase 1 or phase 2 updates.

```
d--------- -                    -               ./
---------- -                    -               Makefile
---------- -                    -               README.md
d--------- -                    -               src/

  d--------- -                    -               pkg/
  ---------- -                    -               lib.go

  ---------- -                    -               main.go
  ---------- -                    -               util.go

d--------- -                    -               docs/
  ---------- -                    -               guide.md

```

### Gap Purpose

With fixed-width lines, gaps serve a narrow but important role:

1. **New files on rescan**: A file created since the last scan needs a new
   entry inserted into the correct sorted position within its directory chunk.
   Gap space absorbs this without rewriting the rest of the file.

2. **Deleted files on rescan**: Removing an entry (overwriting with spaces)
   creates new gap space naturally.

3. **Symlink target addition**: When phase 1 discovers a symlink's target,
   the entry line grows by the target path length. This is the one metadata
   update that can change line width.

### Gap Management

When a directory chunk outgrows its gap (new files on rescan):

1. **Local redistribute**: spread entries + gaps across the current region
   and neighboring gap space.
2. **Section rewrite**: if neighbors are also full, rewrite a larger section
   with fresh gaps.
3. **Global expand**: if the file is too tight overall, rewrite to a new file
   with 2x gap space (amortized doubling). Atomic write-to-temp + rename.

Deletions create gap space naturally. Over time, the file self-balances.

### In-Memory Index

The scanner maintains a lightweight index during operation:

```go
type dirIndex struct {
    offset   int64  // byte offset of first entry in c4m file
    size     int    // bytes used by entries in this chunk
    gapSize  int    // bytes of blank-line gap after chunk
    entries  int    // number of entries in chunk
    level    int    // current resolution level (0, 1, 2)
    mtime    int64  // directory mtime (for prioritization, 0 if unknown)
}
```

For 500K directories at ~60 bytes each = ~30 MB. Trivial.

The index is reconstructed from the c4m file on startup/resume by a
sequential scan that notes directory boundaries and gaps.

## Multi-Part c4m Files

For very large trees (billions of files), a single c4m file becomes unwieldy
(150+ GB). At a configurable size threshold, the scanner splits into
numbered parts:

```
workspace.0001.c4m    # first chunk of the tree
workspace.0002.c4m    # continuation; first line is C4 ID of part 0001
workspace.0003.c4m    # continuation; first line is C4 ID of part 0002
```

Each continuation file's first line is the C4 ID of the preceding file,
creating a verified chain. Within each file, entries follow the standard
c4m format with gaps.

This reuses the existing patch/streaming format. Additions and removals are
inline within each part. A reader consuming the full tree reads the chain
in order. The final compaction can optionally merge parts into a single file
or keep them split.

The threshold is generous (e.g., 4 GB per part) so that developer-scale trees
(25M files) stay in a single file. Splitting only kicks in at data-center
scale.

## Three-Phase Pipeline

### Phase 0: Structure Discovery

**Single-threaded depth-first walk.** Readdir is fast and depth-first order
gives the correct c4m tree ordering naturally.

For each directory:
1. `os.ReadDir(path)` -- returns names + types, no stat
2. Sort entries (files before directories, natural sort)
3. Write sorted chunk to c4m file with fixed-width padding
4. Leave gap space after chunk (proportional to entry count)
5. Record directory in index

**Output**: Complete tree structure. Every file and directory has a name and
type. All other fields are padded null.

**Cost**: ~30-60 seconds for 25M files (dominated by readdir syscalls).

### Phase 1: Metadata Resolution

**Multi-threaded, prioritized by directory mtime.**

1. Stat directories first (one stat per directory, cheap). This gives us
   directory mtimes for prioritization.
2. Sort directories by mtime descending (most recently modified first).
3. Fan out workers, each takes a directory:
   a. Stat all children in the directory
   b. `pwrite` each entry's metadata fields in place (fixed-width overwrite)
   c. Workers operate on non-overlapping file regions -- no locking needed

Entries for recently modified directories resolve first, so the most
interesting content gets attention earliest.

**Output**: Full metadata for all entries. C4 IDs still padded null.

**Cost**: ~2-5 minutes for 25M files (dominated by stat syscalls).

### Phase 2: Content Identification

**Heavily threaded with balanced worker pools.**

Two worker pools to balance I/O patterns:

- **Small-file pool** (many workers): files below a size threshold (e.g.,
  64 KB). High per-file syscall overhead. Batch many files per worker.
  Throughput limited by IOPS.
- **Large-file pool** (few workers): files above the threshold. One file per
  worker. Throughput limited by sequential read bandwidth.

Results are batched by directory when practical, but **time-limited to 60
seconds**. If a directory batch isn't complete after 60 seconds, flush
whatever C4 IDs are available. This ensures:
- Progress is visible to readers (other tools, the Navigator)
- Crash loses at most 60 seconds of work
- Large directories with huge files don't block progress reporting

Each C4 ID fill-in is a fixed-width `pwrite` at the known byte offset
(entry offset + mode + timestamp + size + name fields = C4 ID position).
No gap consumption needed.

**Output**: Fully resolved c4m with all C4 IDs.

**Cost**: Hours for 25M files (dominated by sequential read + SHA-512).
Disk bandwidth is the binding constraint (~500 MB/s on modern SSDs, so
2.5 TB of content takes ~83 minutes minimum).

### Final: Compaction

Strip all gap space and normalize whitespace. Write canonical sorted c4m
via atomic temp + rename. This is the clean final output.

Optional: can be skipped if the user is fine with the gapped file (it's
valid c4m regardless).

## Rescan (Incremental Update)

When rescanning an existing c4m against the live filesystem:

| Filesystem state | c4m state | Action |
|---|---|---|
| File exists, same size + mtime | Entry with C4 ID | **Keep** (cache hit) |
| File exists, size or mtime changed | Entry with C4 ID | Update metadata, **wipe C4 ID to `-`** |
| File exists | Not in c4m | **Insert** into gap |
| Does not exist | In c4m | **Remove** (entry becomes gap space) |

After the diff pass, queue all entries with `-` C4 IDs for phase 2.

Second scan of 25M files where 100 changed: re-hash 100 files. Minutes, not
hours. The c4m is its own cache.

### Mtime Trust Model

Size + mtime is the cache key. If both match, the C4 ID is presumed valid.
This is the same heuristic used by `rsync`, `make`, and every build system.
It is not cryptographically safe (mtimes can be forged), but it is practical.

A `--force` flag can bypass the cache and recompute all C4 IDs.

## Scale Analysis

### 25 Million Files (Developer Laptop)

| Metric | Value |
|---|---|
| Phase 0 (readdir) | 30-60 seconds |
| Phase 1 (stat) | 2-5 minutes |
| Phase 2 (C4 IDs) | 1-4 hours |
| C4m file (gapped, during scan) | ~4-5 GB |
| C4m file (compacted) | ~3.7 GB |
| In-memory index | ~30 MB |
| Rescan (100 changes) | ~1 minute |

### 1 Billion Files (Production Server)

| Metric | Value |
|---|---|
| Phase 0 (readdir) | 20-60 minutes |
| Phase 1 (stat) | 1-4 hours |
| Phase 2 (C4 IDs) | Days (depends on storage) |
| C4m file (compacted) | ~150 GB (multi-part) |
| In-memory index | ~1 GB |

At this scale, multi-part c4m files keep individual files manageable. The
index fits in RAM. Phase 2 is the bottleneck and benefits most from
parallelism.

## Implementation Components

### ProgressiveWriter (new, in c4m package)

Manages a c4m file as a gapped array with fixed-width lines and an in-memory
directory index.

```
Open(path string) (*ProgressiveWriter, error)     // open existing or create new
WriteChunk(dirPath string, entries []*Entry)       // write sorted directory chunk with gaps
OverwriteField(offset int64, field, value)         // in-place pwrite of a single field
InsertEntry(dirPath string, entry *Entry)          // insert into gap (rescan)
RemoveEntry(dirPath string, name string)           // blank out entry (rescan)
Compact(w io.Writer)                               // emit canonical c4m
RebuildIndex() error                               // scan file, reconstruct index
DirIndex(dirPath string) (dirIndex, bool)          // lookup directory in index
```

### Progressive Scanner (new, in c4d)

Orchestrates the three-phase pipeline with c4d's background job system.

```
Scan(root string, output string, opts ScanOptions) (jobID, error)
Status(jobID) ScanStatus
Stop(jobID)
Resume(output string)  // reads c4m, infers progress, continues
```

### Scanner Pipeline (refactor existing scan package)

The existing `scan.Generator` and progressive scanner in `cmd/c4/internal/scan`
provide the foundation. Refactor to:
1. Separate the three phases explicitly
2. Support writing to a `ProgressiveWriter` instead of building in-memory
3. Support resume from an existing partially-resolved c4m

## Open Questions

1. **fsevents / inotify integration**: For long-running scans, the filesystem
   changes during the scan. Should we watch for changes and invalidate entries
   as we go? Or just accept that the c4m is a point-in-time snapshot that was
   "taken over several hours"?

2. **C4 ID computation for directories**: Currently, a directory's C4 ID is
   the hash of its canonical c4m representation. This requires all children to
   be fully resolved first. Should directory C4 IDs be deferred until all
   children have IDs? (Probably yes -- they naturally fall out of phase 2
   completion.)

3. **Concurrent pwrite safety**: Multiple threads writing to different file
   regions via pwrite. Need to verify this is safe on macOS/Linux for
   non-overlapping regions (it should be -- pwrite is atomic for
   non-overlapping ranges on local filesystems).

4. **Multi-part threshold**: What's the right size to split? 4 GB per part
   keeps things manageable while ensuring single-file for typical developer
   trees. Needs benchmarking.

5. **Symlink target width**: Symlink entries gain a target path in phase 1,
   which changes line width. Options: (a) pre-allocate generous target padding
   in phase 0 (wasteful for non-symlinks), (b) accept the line-width change
   for the small number of symlink entries and consume gap space, or (c) defer
   symlink target discovery to phase 1 and write the full padded line then.
   Option (c) seems cleanest -- symlinks are uncommon enough that the gap
   cost is negligible.
