# Cross-implementation vector: applying a patch from its own base (2026-11-09)

The shared conformance fixture for `ApplyPatchFrom(target, base, patch)`
(SPECIFICATION.md "Patch Entry Semantics": a patch is meaningful only over
its own base). One folder per case:

| File                   | Content                                                            |
| ---------------------- | ------------------------------------------------------------------ |
| `base.c4m`             | the state the patch was diffed from                                |
| `target.c4m`           | the tree the patch is carried to                                   |
| `patch.c4m`            | the patch, as `EncodePatch` writes it: the base's ID, then entries |
| `expected.c4m`         | what a conforming implementation produces                          |
| `expected-root-id.txt` | its description ID                                                 |

Every file is made by the library (`TestApplyFromVector`;
`C4M_UPDATE_VECTORS=1 go test ./c4m -run TestApplyFromVector` rewrites
them and the test then fails if a file differs from what the library
writes). Timestamps are fixed constants. No line was written by hand.

## The rule under test

Read each patch entry against **base**: an entry identical to the base's
with no children listed means *removed*; anything else means *the path now
holds this*. Then do that to **target**: remove the path, or write the
entry, overwriting what target holds (no conflict policy here). A
directory line with children under it is context for those children, not
a write: it creates a directory in target only when a write lands under
it, and a directory the walk entered has its size and ID re-derived from
the children that remain (the emptied directory becomes the empty one),
while a directory the target holds keeps its own line unless that line is
still the base's. With target equal to base the result is `ApplyPatch`'s.

A name is one thing: writing file `x` removes directory `x/` and its
subtree; a write under `x/`, or of `x/` itself, removes a file or
symlink `x`; the later write wins by kind as it does by bytes. A
removal removes only the kind the base held. A directory written by ID
alone replaces the target's listing, except the empty listing, which
means the directory exists and removes nothing under it.

## Cases, and what each broke

| Case                          | What happened before the rule                                                                                                                                                                                                                                                                  |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `same-add`                    | plain `ApplyPatch` on the target read the second add of the same bytes as a removal; the path vanished                                                                                                                                                                                         |
| `removal-under-removed-dir`   | the restated `d/` line was written, recreating a directory the target had removed                                                                                                                                                                                                              |
| `emptied-dir-differing-line`  | the target's `weights/` line, with a stale ID, survived the removal of its last child and read as a directory "known by ID"                                                                                                                                                                    |
| `later-write-wins`            | the operator's documented rule; pinned so both forms agree                                                                                                                                                                                                                                     |
| `own-base`                    | equality with `ApplyPatch` across addition, modification, removal and deepening                                                                                                                                                                                                                |
| `dir-own-line-kept`           | a directory's observed time in the target is kept (reading A); size and ID derive from f, g and x                                                                                                                                                                                              |
| `kind-conflict`               | the result held both a file `x` and a directory `x/` (75 of 601 applications with kind swaps); the later write wins by kind                                                                                                                                                                    |
| `metadata-only`               | a restatement with the same C4 ID and size but a different mode, time or both is a write ("all fields identical" is the removal rule); a reader comparing only ID and mode removed `notes.txt`, whose only change is its time. The target is the base, so this is also plain patch application |
| `metadata-only-other-target`  | the same patch carried to a target that is not its base; the target's own `extra.txt` stays                                                                                                                                                                                                    |
| `removal-meets-replaced-kind` | removing file `x` where the target replaced it with `x/`: the directory stays (pinned so both forms agree)                                                                                                                                                                                     |
| `dir-by-id-refused`           | a directory written by ID alone over a directory the target holds is refused (`ErrDirByIDOverHeld`): replacing would drop the target's subtree, keeping its children would discard the written ID (decided 2026-11-13 by the release lead; reversible)                                         |
| `empty-dir-over-existing`     | a unit that only ran mkdir: its childless empty-listing line over another unit's `logs/b.log` kept b.log; the same line over nothing creates the empty directory                                                                                                                               |

The operator's record-tree form was cross-checked against the library on
1,000 steps over 300 random two-unit histories with 0 differences before
this vector was pinned (2026-11-09). Each case here is a shape that broke
one of the two during that work.

`metadata-only` was found by a c4m consumer (2026-11-11): its view reader
treated a mode- or time-only restatement as a removal. Measured the same
day, plain application of `base.c4m` then `patch.c4m` as one chain: Go
(master and 1.0.25), c4py, c4m-swift and libc4 (main and release/1.0.x)
reach `expected-root-id.txt`; c4ts (main and release/1.0.x) removes
`notes.txt`. c4ts's own `patchDiff` also leaves `notes.txt` out of the
patch it writes, so its patch does not reach the new state either.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.

A file that is not canonical by design re-encodes to a canonical partner
the library wrote; `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestVectorRoundTrip` rewrites the partners.

| File      | Re-encodes to |
| --------- | ------------- |
| `*/*.c4m` | itself        |
