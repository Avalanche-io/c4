# c4 explain — Human-Readable Command Narration

## Summary

`c4 explain <command> [args...]` runs any c4 command in a human-readable, explanatory mode. It shows what the command would do with the specific data provided, narrated in plain language. It is both a teaching tool for new users and an ergonomic dry-run for experienced users.

## Motivation

- `c4 diff` outputs c4m patches — correct for scripting, opaque for humans
- `c4 patch --dry-run` requires understanding the patch output format to interpret
- New users need to see what commands do before committing to them
- The HN demo needs commands that narrate themselves

## Design

`explain` is not a flag. It's a prefix that works with any command:

```
c4 explain <command> [args...]    # human-readable narration
c4 <command> [args...]            # machine-readable output
```

Every command keeps its current output format unchanged. `explain` is a parallel mode that targets humans instead of scripts.

## Examples

### c4 explain id

```
$ c4 explain id ./project/
Scanning ./project/: 847 files in 23 directories (14.2 GB)

This produces a c4m file — a text description of every file:
  permissions  timestamp  size  name  identity

Save it:  c4 id ./project/ > project.c4m
Diff it:  c4 diff project.c4m ./project/
```

### c4 explain diff

```
$ c4 explain diff monday.c4m friday.c4m
Comparing monday.c4m (847 files) against friday.c4m (849 files):

  2 files modified
    README.md          5,179 → 5,188 bytes
    src/main.go        2,048 → 2,103 bytes
  1 file added
    src/util.go        847 bytes
  1 file removed
    tmp/scratch.py     312 bytes

845 files unchanged.
```

### c4 explain patch

```
$ c4 explain patch target.c4m ./output/
Reconciling ./output/ to match target.c4m:

  12 files to create (3.2 GB to fetch from store)
   2 files to update
   3 files to remove
 832 files already correct — skipping

Store: /Users/joshua/.c4/store (98% of content available)
Missing: 2 files (41 MB) — not in store

Run without 'explain' to apply.
```

### c4 explain merge

```
$ c4 explain merge base.c4m local.c4m remote.c4m
Three-way merge:
  base:   base.c4m   (200 files)
  local:  local.c4m  (203 files, 5 changed from base)
  remote: remote.c4m (201 files, 3 changed from base)

  3 changes apply cleanly (no overlap with local changes)
  2 local-only changes preserved
  0 conflicts

Run without 'explain' to produce merged c4m.
```

### c4 explain log

```
$ c4 explain log project.c4m
project.c4m contains 4 snapshots:

  1  Mar 15  initial scan (847 files, 14.2 GB)
  2  Mar 18  +3 files, ~2 files modified
  3  Mar 20  -1 file, ~5 files modified
  4  Mar 22  +12 files (new test suite)

Current state: 861 files, 14.8 GB
```

## Behavior

- `explain` does not modify any files or state. It is always safe to run.
- `explain` with no command is equivalent to `c4 --help` but friendlier.
- `explain` should work with all flags the underlying command accepts.
- Output goes to stdout (not stderr) so it can be captured, but is never intended for machine parsing.
- When the underlying command would fail (e.g., missing store content), `explain` reports the problem in human terms instead of erroring.

## Implementation Notes

- Each command implements an `explain` variant alongside its normal execution path.
- The explain path calls the same analysis functions (scan, diff plan, patch plan) but formats results differently.
- No new dependencies. Uses the same c4m package operations.
- The explain output format is not stable and may change between versions — it is explicitly for humans.

## Priority

High — this is a key demo tool for the open source announcement and the most natural onboarding path for new users.
