# Cross-implementation vector: a sequence with a null-size frame (2026-11-13)

The shared conformance fixture for the folded-sequence size rule
(SPECIFICATION.md "Media File Sequences"; `design/sequence-null-size.md`).
Ruled 2026-11-13 by the maintainer: "One cannot use size as an expectation
if there are nulls present, so we must say nothing." A folded sequence's
size is the sum of its frames' sizes, and null if any frame's size is null
(`-`, or a `0` beside a C4 ID other than the empty content's). Every c4m
implementation (Go `c4m`, c4py, c4ts, c4m-swift, libc4) MUST pass it.

## Files

| File                      | Made by                                                                                   |
| ------------------------- | ----------------------------------------------------------------------------------------- |
| `frames.c4m`              | the Go encoder, canonical form: `plate.1001-1004.exr` all known, `shot.1003.exr` null size |
| `frames-zero-spelled.c4m` | `frames.c4m` with the null size field rewritten to `0`, nothing else changed              |
| `folded.c4m`              | the Go encoder on `SequenceDetector` (minimum length 3) applied to `frames.c4m`           |
| `folded-root-id.txt`      | `ComputeC4ID` of the manifest `folded.c4m` describes                                      |

No line is composed by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestSequenceNullSizeVector` rewrites the files; the test then fails if they
differ from what the library writes. On the code before c4 #155 it fails:
the shot range folds to size 14 (5 + 5 - 1 + 5).

## What a conforming implementation MUST do

Decode `folded.c4m` to a manifest whose root ID is `folded-root-id.txt`,
and see:

| Entry                   | Size line | Size is  |
| ----------------------- | --------- | -------- |
| `plate.[1001-1004].exr` | `24`      | known 24 |
| `shot.[1001-1004].exr`  | `-`       | **null** |

Re-encoding it canonically reproduces `folded.c4m` byte for byte.

An implementation that folds sequences MUST also fold either frames file
(minimum length 3) to `folded.c4m` byte for byte: the shot range's size is
written `-`, never a sum, never 0.
