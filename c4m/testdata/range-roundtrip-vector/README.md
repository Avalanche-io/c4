# Folded-range round-trip vector

Every implementation that materialises a description runs this vector. The
rule it pins (`design/folded-range-round-trip.md`): scan a tree with
sequence detection, materialise that description into an empty folder,
scan it again with sequence detection, and get the same description ID.

## Files

| File              | What it is                                                                                     |
| ----------------- | ---------------------------------------------------------------------------------------------- |
| `trees.txt`       | The trees, as plain data (not c4m): one line per path with its kind, mode, mtime and content. |
| `<case>.c4m`      | The folded description of each tree, ID lists inline, as Go's library writes it.              |
| `ids.txt`         | Each case's description ID: the C4 ID of the root's one-level listing.                        |
| `lists/<C4 ID>`   | Every range's frame ID list, named by the range's C4 ID, for the list-only-in-a-store case.   |

Every `.c4m` file, `ids.txt` and `lists/` is written by the library:
`scan`'s `TestRangeRoundtripVectorIsTheLibrarys` builds each tree from
`trees.txt`, scans it (`scan.Dir` with `WithSequenceDetection`, as
`c4 id -S` scans), encodes it, and compares every byte with these files
(`C4_UPDATE_VECTOR=1` rewrites them). No c4m here was written by hand.

## Cases

| Case       | What it holds                                                                    |
| ---------- | -------------------------------------------------------------------------------- |
| `one`      | One three-frame range.                                                           |
| `down1`    | A range one directory down, beside a file.                                       |
| `down2`    | A range two directories down.                                                    |
| `two`      | Two ranges in one folder (three and four frames).                                |
| `beside`   | A range beside ordinary files of other modes and times.                          |
| `gaps`     | Frames 1-3 and 7-9: the detector folds them as two ranges.                       |
| `unpadded` | Unpadded numbering, `f.[1-4].exr`.                                               |
| `mixed`    | Frames of mixed modes (600, 644, 640) and times: the range takes the newest time and the most restrictive mode, and every frame is written with them. |
| `nulltime` | Frames at the Unix epoch: a null-dated range, created at the epoch again.        |
| `links`    | Three symbolic links that fold. Not written as a vector file (a link's own time cannot be set portably); the materialiser must refuse it, naming the reason. |

## What a materialiser must do

For each case except `links`:
1. With the list inline: materialise `<case>.c4m` into an empty folder,
   taking frame content from a store that holds the tree's files. A scan of
   the result with sequence detection has the description ID in `ids.txt`.
2. With the list only in a store: materialise the same description without
   its inline lines (for example the stored one-level listing), with
   `lists/<C4 ID>` in the store. The same ID results.
3. With no list anywhere: the materialiser writes nothing and names the
   range's list as the missing content.

A frame takes the range line's mode and timestamp; its size comes from its
content. A frame created from a null timestamp is set to the epoch.

Ranges the detector never writes (a stepped range, `[0001-0007:2]`, or one
line naming several runs, `[0001-0002,0005-0006]`) must expand too; the Go
library tests them with ranges built through its API (`reconcile`'s
`TestExpandRangesWritesEveryRangeForm`).

On Windows a mode the platform cannot hold is written as the platform can,
and the round trip is measured from the platform's own scan of the tree.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.

| File    | Encodes to |
| ------- | ---------- |
| `*.c4m` | itself     |
