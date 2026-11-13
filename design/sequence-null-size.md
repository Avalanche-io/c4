# A folded sequence's size when a frame's size is null

Status: ruled 2026-11-13, implemented in c4 #155; the ports follow with the
same fixture.

## The ruling

Ruled 2026-11-13 by the maintainer: "One cannot use size as an expectation
if there are nulls present, so we must say nothing."

A folded sequence (`c4 -S`, `SequenceDetector`) states the sum of its
frames' sizes. If any frame's size is null (`-1`, or a `0` beside a C4 ID
other than the empty content's, `SizeIsNull`), the sequence's size is null.
Frames with no C4 ID still prevent the fold, as before.

## What it fixes

Measured on master 5ee54b6: four 6-byte frames, one with an unknown size,
folded to `shot.[1001-1004].exr` with size 17 (6 + 6 - 1 + 6). The `-1` was
added in, so the range claimed a known size that was wrong. c4-swift's
`Sequence.swift` had the same sum.

## Reach

C4 IDs change for a description holding such a sequence. A scan never
writes one: a frame it cannot read has no C4 ID and is not folded.

SPECIFICATION.md ("Media File Sequences") and C4M-STANDARD.md state the
rule as a one-line erratum quoting the ruling.
