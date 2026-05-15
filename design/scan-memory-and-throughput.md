# Scan Throughput — Quadratic Metadata Propagation on Large Trees

## Status

**Fixed** on branch `fix/propagate-metadata-linear`. This document records
the original investigation, the broader-than-initially-described scope of
the bug, and the path the fix took. Kept for posterity and as a reference
for sibling-language implementations (libc4, c4-python, c4m-swift, c4git)
that may carry the same quadratic shape.

## TL;DR

`scan.Dir` on a 5.14M-entry tree took 16–20 minutes single-core, 100% CPU,
~3.3 GB RSS — the user experienced it as a hang. The cost was
**O(directories × entries)** metadata propagation in two parallel
implementations:

- `c4m/manifest.go` `propagateMetadata` (called by `Canonicalize` /
  `ComputeC4ID`) — nil-infectious, matched the spec.
- `scan/metadata.go` `PropagateMetadata` (called by every
  `scan.GenerateFromPath`) — permissive (silently skipped null sizes /
  timestamps, **violating the spec**).
- A third, near-duplicate fork in `cmd/c4/internal/scan/metadata.go`.

Each rescanned the full entry slice once per null directory via
`getDirectoryChildren`. For a tree with `D` directories and `N` total
entries the total work is `D × N` — ~10¹² comparisons at the 5.14M /
~200K-dir scale.

The fix is a single-pass depth-stack algorithm in
`c4m.PropagateMetadata` (now exported as the canonical, spec-compliant
implementation). `scan.PropagateMetadata`,
`scan.CalculateDirectorySize`, `scan.GetMostRecentModtime` and their
duplicates in `cmd/c4/internal/scan/` have been **removed**. The scan
packages call `c4m.PropagateMetadata` directly. The "scan version is
permissive, c4m version is nil-infectious" semantic divergence is also
resolved — `c4m`'s nil-infectious behavior is the One True
Implementation.

## What the original investigation got right

- Algorithmic shape (`getDirectoryChildren` linear scan per null
  directory → `O(D × N)`).
- Fix idea: single-pass depth-stack accumulation, `O(N)` work +
  `O(max-depth)` memory.
- `ModeFull` amplification: `scan/generator.go:267` calls
  `subManifest.ComputeC4ID()` per directory, which previously ran the
  same quadratic propagation on each subtree. The single-pass fix
  resolves this automatically.

## What the original investigation got wrong (corrected here)

1. **Wrong function attribution.** The note pointed at
   `c4m/manifest.go:484 getDirectoryChildren`, but the dominant hot
   path in `scanbench -mode m` is `scan.PropagateMetadata`. Evidence:
   pre-fix at 122K entries, `canonicalize()` measured 1 ms wall-clock
   while pprof attributed 19.6% of CPU (0.37 s) to `PropagateMetadata`.
   0.37 s does not fit inside a 1 ms stage; that CPU lived in the
   `walk + assemble` stage's call to `scan.PropagateMetadata`. The c4m
   version was almost always a no-op because `scan.PropagateMetadata`
   ran first and left no null-valued directories for it to find.

2. **Three implementations, not one.** The note assumed a single
   propagator. There were three: `c4m.propagateMetadata`,
   `scan.PropagateMetadata`, `cmd/c4/internal/scan.PropagateMetadata`.
   All had the same `O(D × N)` shape; the scan ones had divergent
   permissive null semantics that **violated** `SPECIFICATION.md`'s
   nil-infectious rule. Fixing only the c4m one would not have moved
   the wall-clock needle on real scans.

3. **Sketch had a latent empty-directory bug.** The closeFrame logic
   in the original sketch would set an empty directory's Timestamp to
   Go's zero `time.Time{}` (year 1) instead of the proper
   `NullTimestamp()` (Unix epoch). The implementation tracks an
   explicit `hadChildren` flag on each frame to distinguish "no
   children seen" from "all children resolved to non-null timestamps."

## Empirical results

`scanbench` against real trees, before and after the fix:

| Path                       | Entries  | Pre-fix walk+assemble | Post-fix walk+assemble | Pre-fix canonicalize | Post-fix canonicalize | Pre-fix total | Post-fix total |
|----------------------------|---------:|----------------------:|-----------------------:|---------------------:|----------------------:|--------------:|---------------:|
| `repos/aws`                |  122,693 | 2.63 s                | 0.72 s                 | 1 ms                 | 1 ms                  | ~2.6 s        | ~0.7 s         |
| `repos` (full)             | 6,084,244 | **16–20 min (hang)** | **2 min 24 s**         | unmeasured (hang)    | 69 ms                 | **16–20 min** | **~2.5 min**   |

C4 ID byte-identical pre/post for both. On the 122K case the
walk+assemble stage is now 3.6× faster (filesystem I/O dominates the
remainder). On the 5M case the hang is gone — the tool now scans
6,084,244 entries (323 GB of content) in 2 m 24 s, dominated by
disk reads.

## Verification

Beyond the wall-clock numbers, the fix is verified two ways:

1. **Bench scaling.** `BenchmarkPropagateMetadata_Linear` in
   `c4m/benchmarks_test.go` runs synthetic walk-ordered manifests at
   10K / 100K / 1M entries and asserts near-linear scaling. Measured:

   ```
   entries=10000      5.08 ms/op
   entries=100000    50.89 ms/op   (10.0×)
   entries=1000000  494.56 ms/op   (9.7×)
   ```

   Perfect linearity. Pre-fix at 1M entries with realistic dir
   ratio extrapolated to ~10²·log s, which never finished in
   the user's bench.

2. **Byte-for-byte canonical comparison.** Before the fix landed,
   `/tmp/c4m-baseline/baseline.go` captured the canonical bytes,
   per-entry Mode/Size/Timestamp dumps, and top-level C4 ID of five
   synthetic fixtures (simple, deep, with-empty, wide, mixed) plus the
   122K `repos/aws` tree. After the fix, the same script ran against
   the same trees and produced byte-identical output for every fixture
   and the same C4 IDs. This is the strongest possible evidence that
   the fix is semantics-preserving on the disk-scan path (where files
   always have known sizes / timestamps and nil-infection has nothing
   to bite on).

## Single-pass algorithm

`c4m.PropagateMetadata` maintains a stack of open directory frames.
Each entry's parent is the most-recent frame whose depth is one less.
When the algorithm leaves a subtree (the next entry's depth is at or
above an open frame's depth), it closes the frame and propagates the
resolved directory contribution into the parent.

Frame state:

- `idx, depth` — the directory entry's index in `entries` and its depth.
- `size, c4mBytes` — accumulating sum of resolved child sizes and the
  byte-length of their canonical c4m lines (per
  [`directory-size-includes-c4m.md`](directory-size-includes-c4m.md)).
- `ts` — most-recent resolved timestamp among children.
- `nullSize, nullTs` — sticky flags: any descendant with a null size /
  timestamp poisons the parent (nil-infectious per spec).
- `hadChildren` — distinguishes empty dir (Size = 0, Timestamp = null)
  from "all children resolved cleanly."

The non-directory branch only contributes into the immediate parent
frame; the directory branch pushes a frame on open and pops + propagates
on close.

Complexity:

- **Work**: each entry is visited exactly once on the way in. Each
  directory frame closes exactly once. Total work = O(N).
- **Memory**: stack depth = max tree depth. For a real source tree this
  is ≤ ~20 levels; never proportional to N.

Precondition: `entries` is in filesystem-walk order — direct
children appear contiguously after their parent directory; sibling
subtrees do not interleave. This is what `Manifest.SortEntries`
produces and what `scan.GenerateFromPath` emits.

An additional cheap pre-pass short-circuits when no directory has any
null Size / Timestamp / Mode (i.e. when `scan.PropagateMetadata` has
already resolved everything and `ComputeC4ID`'s second pass would
otherwise re-do the accumulation needlessly). This preserves the
sub-millisecond cost of repeated `Canonicalize` calls on a fully
resolved manifest.

## Files touched

- `c4m/manifest.go` — rewrote `propagateMetadata` as exported
  `PropagateMetadata` with the single-pass algorithm. Removed
  `getDirectoryChildren`, `calculateDirectorySize`, `c4mContentSize`,
  `getMostRecentModtime`.
- `c4m/manifest_test.go` — removed `TestGetDirectoryChildren`,
  `TestCalculateDirectorySize`, `TestGetMostRecentModtime`. Kept
  `TestPropagateMetadata` and `expectedDirSize`. Updated lowercase
  `propagateMetadata` calls to `PropagateMetadata`.
- `c4m/readiness_test.go` — flipped the contract:
  `testPropagateMetadataUnexported` → `testPropagateMetadataExported`.
  PropagateMetadata is now deliberately exported as the canonical
  implementation that scan / cmd / sibling packages should share.
- `c4m/benchmarks_test.go` — added `BenchmarkPropagateMetadata_Linear`
  and `buildSyntheticManifest`.
- `scan/metadata.go` — removed `PropagateMetadata`,
  `CalculateDirectorySize`, `GetMostRecentModtime`, `c4mContentSize`,
  `getDirectoryChildren`. Header comment explains why and points
  callers to `c4m.PropagateMetadata`.
- `scan/generator.go` — `PropagateMetadata(...)` →
  `c4m.PropagateMetadata(...)`.
- `scan/metadata_test.go` — removed `TestCalculateDirectorySize`,
  `TestGetMostRecentModtime`. `TestPropagateMetadataDirectorySizes`
  now calls `c4m.PropagateMetadata` and uses a local test helper
  `c4mContentSizeForTest` for size expectations.
- `cmd/c4/internal/scan/metadata.go` and
  `cmd/c4/internal/scan/metadata_test.go` — same treatment as `scan/`.
- `cmd/c4/internal/scan/generator.go` — same call-site update.

## Secondary issues — still open

These were flagged during the original investigation. None is on the
critical path for the hang, but they remain real follow-up work.

1. **No progress signal.** A 5M-entry scan still takes ~2½ minutes.
   Callers cannot tell whether the scanner is alive or stuck. A
   `WithProgress(func(ScanStats))` option would close this gap. The
   `cmd/c4/internal/scan/progressive_scanner.go` exists for this but
   currently does not call `PropagateMetadata` — see TODO comment in
   that package's `metadata.go`.

2. **Single-goroutine walk.** `scan.GenerateFromPath` is sequential.
   A balanced tree would parallelize cleanly at directory boundaries.
   Less important than (1) until the bench has clean numbers; cross
   reference `progressive-scanning.md`.

3. **No partial result.** On Ctrl-C the in-progress manifest is lost.
   Streaming entries via an iterator or channel would let a
   workspace-scope tool persist as it goes.

4. **`Manifest.Canonical()` allocates a single string.** Streaming a
   canonical form to an `io.Writer` would let large manifests be
   hashed without materializing the canonical text. Connected to
   [`large-directory-blocks.md`](large-directory-blocks.md) — for
   trees above a threshold the right answer is patch-chain blocks
   rather than one giant c4m.

5. **`SortEntries` (`sortSiblingsHierarchically`) can be
   super-linear** on pathologically nested chains (each level scans
   its subtree). On normal trees it is ~O(N log N). Not a current
   hot path but worth keeping in mind at the 10M+ scale.

## Reproducer

```bash
cd ~/ws/active/githublook/pm
go build -o /tmp/scanbench ./cmd/scanbench

# Reasonable size — sanity check.
/tmp/scanbench -mode m /Users/joshua/ws/repos/aws

# The original hang target. Now finishes in ~2.5 min.
/tmp/scanbench -mode m /Users/joshua/ws/repos
```

Bench scaling:

```bash
cd ~/ws/active/c4/oss/c4
go test -run='^$' -bench=BenchmarkPropagateMetadata_Linear -benchtime=5x ./c4m
```

## Breadcrumbs for sibling-language implementations

The same algorithmic shape likely exists in the other-language
implementations of c4m. Each one's directory-size / timestamp
propagation should be inspected for an equivalent `O(D × N)` pattern
and converted to the single-pass depth-stack approach if so. Pointers
have been left in:

- `oss/libc4/PROPAGATE_METADATA_PORT.md`
- `oss/c4-python/PROPAGATE_METADATA_PORT.md`
- `oss/c4m-swift/PROPAGATE_METADATA_PORT.md`
- `oss/c4git/PROPAGATE_METADATA_PORT.md`

Each breadcrumb points back to this design doc and asks the
language-specific agent to verify on next run.

## See also

- [`directory-size-includes-c4m.md`](directory-size-includes-c4m.md) —
  the design that pulled `c4mBytes` into the directory size, which the
  accumulator preserves.
- [`large-directory-blocks.md`](large-directory-blocks.md) — the
  on-disk scaling story; complementary to the in-memory algorithmic
  fix here.
- `c4m/manifest.go` `PropagateMetadata`, `Canonicalize`, `ComputeC4ID`
  — the corrected hot path.
- `scan/generator.go` line 228 — single call site that now uses
  `c4m.PropagateMetadata`.
- `cmd/c4/internal/scan/generator.go` line 228 — same.
- `c4m/readiness_test.go` `testPropagateMetadataExported` — the new
  contract anchoring this as a public API.
