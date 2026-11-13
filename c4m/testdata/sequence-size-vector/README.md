# Cross-implementation vector: a range's size counts its ID list (2026-11-13)

The shared conformance fixture for the folded-sequence size rule
(SPECIFICATION.md "Media File Sequences" and "Computing Directory C4 IDs";
`design/sequence-size-includes-list.md`). Ruled 2026-11-13 by the
maintainer: "the file id list is just a variation on the c4m file format
specific to folded sequences. People have a choice to use folding or not
if they need to retain per item metadata. The size is the size of every
frame and the frame id list. Note that the frame id list has a
compleately deducable size based on the item count alone since c4 id's
have a fixed length. So one can easily figure out what the actual file
data size is as well by subtracting the deduced file id list size."

A range's size is the sum of its frames' sizes plus its ID list's length,
count × 90 (the bare concatenation of member C4 IDs, no newline); null if
any frame's size is null. A directory holding a range follows the
existing rule with the range as one child: its children's sizes plus its
listing as written, the folded one. Every c4m implementation that folds
sequences or derives directory sizes (Go `c4m`, c4py, c4ts, c4m-swift,
libc4) MUST pass it.

## Files

| File           | Made by                                                                              |
| -------------- | ------------------------------------------------------------------------------------ |
| `frames.c4m`   | the Go encoder: `sub/` holding `f.001.exr` to `f.003.exr`, 11 bytes each, normalised |
| `folded.c4m`   | `SequenceDetector` (minimum length 3) on `frames.c4m`, normalised, encoded           |
| `expanded.c4m` | `SequenceExpander` (embedded) on `folded.c4m`: the range line, then its three frames |

No line is composed by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestSequenceSizeIncludesList` rewrites the files; the test then fails if
they differ from what the library writes.

## What a conforming implementation MUST do

| Entry in `folded.c4m` | Size | Why                                                   |
| --------------------- | ---- | ----------------------------------------------------- |
| `f.[001-003].exr`     | 303  | 33 bytes of frames + 3 × 90 bytes of ID list          |
| `sub/`                | 446  | 303 + 143, the byte length of `sub/`'s folded listing |

- Folding `frames.c4m` (minimum length 3) and deriving directory sizes
  writes `folded.c4m` byte for byte.
- Normalising `folded.c4m` changes nothing.
- A scan that folds sequences writes the same sizes as the normaliser.
  Through 2026-11-13 Go's `c4 id -S` sized the directory from its unfolded
  children (441 here) while hashing the folded listing, and the
  normaliser derived 175.
- If any frame's size is null, the range's size is null, and so is `sub/`'s.
- Expanding `folded.c4m` writes `expanded.c4m`: the range line keeps its
  size, 303, which counts the list, and each frame comes out with a null
  size (`-`), because the list names the frames' C4 IDs and carries no
  per-frame metadata.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.

| File           | Re-encodes to |
| -------------- | ------------- |
| `frames.c4m`   | itself        |
| `folded.c4m`   | itself        |
| `expanded.c4m` | itself        |
