# Progressive Scanning — Explicit Exclusion vs Incomplete Scan

## Problem

When `c4 id -c manifest.c4m ./dir/` continues a scan, files exist on disk that are not in the manifest. There are two reasons this can happen:

1. **Interrupted scan** — the process died. The manifest is incomplete. On continuation, those files SHOULD be added.

2. **Deliberate exclusion** — the user edited the manifest and removed entries. On continuation, those files should NOT be re-added.

Both cases look identical in the c4m file: a file exists on disk but not in the manifest. The intent is opposite.

## Current Behavior

`c4 id -c` only fills in metadata (hashes, sizes, timestamps) for entries already in the manifest. It does not add new entries. This makes it safe for case 2 but broken for case 1.

## Proposed Solution

Add a completeness marker to the c4m file. When present, it means "this manifest intentionally represents the full set of files. Anything not listed is deliberately excluded."

When absent, the manifest may be incomplete.

### Possible approaches:

**A. A sentinel entry or comment**
Not viable — c4m has no comment syntax and sentinels pollute the entry space.

**B. A flag on `c4 id -c`**
- `c4 id -c manifest.c4m ./dir/` — current behavior, fill metadata only
- `c4 id -c --add-missing manifest.c4m ./dir/` — fill metadata AND add new files found on disk

**C. Separate commands for the two cases**
- `c4 id -c manifest.c4m ./dir/` — fill metadata for existing entries (deliberate exclusion respected)
- `c4 id manifest.c4m ./dir/` — rescan, using manifest as a guide for what's already hashed (interrupted scan recovery)

## Related

- [auto-sort-on-read.md](auto-sort-on-read.md) — tools should auto-sort malformed c4m input
- [explain-command.md](explain-command.md) — human-readable command narration (implemented)
- [paths-command.md](paths-command.md) — bidirectional path/c4m converter (implemented)
- [intersect-command.md](intersect-command.md) — set operations on c4m files (implemented)

## Status

Deferred. This design needs more thought. Filed for post-announcement work.
