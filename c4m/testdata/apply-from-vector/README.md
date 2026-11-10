# Cross-implementation vector: applying a patch from its own base (2026-11-09)

The shared conformance fixture for `ApplyPatchFrom(target, base, patch)`
(SPECIFICATION.md "Patch Entry Semantics": a patch is meaningful only over
its own base). One folder per case:

| File                   | Content                                                         |
| ---------------------- | --------------------------------------------------------------- |
| `base.c4m`             | the state the patch was diffed from                             |
| `target.c4m`           | the tree the patch is carried to                                |
| `patch.c4m`            | the patch, as `EncodePatch` writes it: the base's ID, then entries |
| `expected.c4m`         | what a conforming implementation produces                       |
| `expected-root-id.txt` | its description ID                                              |

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

## Cases, and what each broke

| Case                         | What happened before the rule                                           |
| ---------------------------- | ----------------------------------------------------------------------- |
| `same-add`                   | plain `ApplyPatch` on the target read the second add of the same bytes as a removal; the path vanished |
| `removal-under-removed-dir`  | the restated `d/` line was written, recreating a directory the target had removed |
| `emptied-dir-differing-line` | the target's `weights/` line, with a stale ID, survived the removal of its last child and read as a directory "known by ID" |
| `later-write-wins`           | the operator's documented rule; pinned so both forms agree               |
| `own-base`                   | equality with `ApplyPatch` across addition, modification, removal and deepening |
| `dir-own-line-kept`          | a directory's observed time in the target is kept (reading A); size and ID derive from f, g and x |

The operator's record-tree form was cross-checked against the library on
1,000 steps over 300 random two-unit histories with 0 differences before
this vector was pinned (2026-11-09). Each case here is a shape that broke
one of the two during that work.
