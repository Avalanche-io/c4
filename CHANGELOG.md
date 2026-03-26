# Changelog

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
