# ID lists through merge, diff and patch

Status: fixed on branch `range-data` (decided 2026-11-13 by the release
lead; reversible). Shared fixture: `c4m/testdata/range-data-vector/`.

## The fault

A folded sequence (`shot.[1001-1004].exr`) has a C4 ID that is the ID of
its ID list, the frames' C4 IDs bare-concatenated. With no store the list
travels inline, one line after the entries of the section that adds the
range (SPECIFICATION.md, "Inline Range Data"). The lists are not part of
a description's identity, but they are part of the file: drop them and
the bytes and byte ID change, and the frames' IDs are gone wherever the
list existed nowhere else. That is data loss.

Measured on master 8a52087 (2026-11-13), with HOME, USERPROFILE and
C4_STORE set to a fresh directory:

| Function or verb                           | Before                                 | After              |
| ------------------------------------------ | -------------------------------------- | ------------------ |
| `c4 id FILE.c4m`                           | keeps                                  | keeps              |
| `c4 merge` / `Merge`                       | drops every list                       | keeps              |
| `c4 diff` / `PatchDiff`                    | the patch carries no list              | keeps              |
| `c4 patch` (chain, two files, plain)       | drops every list                       | keeps              |
| `c4 split`                                 | drops every list                       | keeps              |
| `DecodePatchChain`                         | skips list lines                       | keeps, per section |
| `ResolvePatchChain` (even one section)     | drops                                  | keeps              |
| `FoldSection`                              | drops                                  | keeps              |
| `ApplyPatch`, `ApplyPatchFrom`             | keep, and keep removed ranges' lists   | keeps              |
| `ResolveBase`, `DecodeResolved`            | keep, and keep removed ranges' lists   | keeps              |
| `Decoder.Decode` (list in a patch section) | keeps, and keeps removed ranges' lists | keeps              |
| `FilterByPath`, `FilterByPrefix`           | drop                                   | keeps              |
| `ExtractSubtree`, `InjectSubtree`          | drop                                   | keeps              |
| `WriteChainBody`                           | drops                                  | keeps              |

"Keeps" means the rule below holds exactly. No store was written by any
of them, before or after.

## The rule

An output carries the ID list of every range entry it contains, and no
list for a range it does not. A list the inputs lack stays absent; none is
invented. A patch section carries the lists of the ranges it adds or
changes, after its entries; a removal line takes none. Lists are keyed by
their own C4 ID, so the same key from two inputs is the same list.

One helper (`rangeDataFor` in `c4m/rangedata.go`) states it, and every
operation calls it on its output. `PatchSection` gains `RangeData`, the
lists written in that section. The encoder and `WriteChainBody` share one
writer for the lines, sorted by ID; `WriteChainBody` writes them after the
base section's entries, before the checkpoint, where the spec places them.

## Reach

Byte IDs change for every merge, diff and patch output that holds a
folded sequence whose list an input carried: the output now has the
lines it lost. Description IDs do not change; the lists are never part of
one. The encoder still writes whatever a manifest's `RangeData` holds, so
`c4 id FILE.c4m` keeps a list even for a range the file lacks: a
normaliser of the file's form, not an operation.

## Open

Expanding a range whose list cannot be found still gives every frame the
range's own C4 ID (`expandSequenceEntry`), an ID that names the list and
no frame; what it gives instead is a null-semantics question for the
maintainer, and is not changed here.

Also measured and not changed here: `c4 id -s FILE.c4m` stores the
description's listings but not its inline lists, and `c4 id -S DIR`
without `-s` folds sequences but writes no list, inline or stored (the
scanner keeps the folded entries and drops the detector's lists). Both
are the spec's store-or-inline lifecycle, a question for the maintainer.
