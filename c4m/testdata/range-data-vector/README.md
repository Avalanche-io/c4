# Cross-implementation vector: ID lists through merge, diff and patch (2026-11-13)

The shared conformance fixture for inline range data through operations
(SPECIFICATION.md "Inline Range Data"; `design/range-data-through-operations.md`).
A folded sequence's C4 ID is the ID of its ID list, the frames' C4 IDs
bare-concatenated. With no store the list travels inline, one line after
the entries of the section that adds the range. Through 2026-11-13 Go's
merge, diff and patch dropped every list: the output described the same
tree (same description ID) but its bytes and byte ID changed, and the
frames' IDs were gone. That is data loss. Every c4m implementation that
merges, diffs or patches (Go `c4m`, c4py, c4ts, c4m-swift, libc4) MUST
pass it.

Ruled 2026-11-13 by the maintainer: "A frame id list is a part of the
c4m format, and is the same thing as a subdirectory being referenced by
it's ID on a parents directory line. Everything that you would expect of
the creation and retention of that subdirectory c4m file you would also
expect of the frame id list." The list's format does not change: it is
"a linked parseable text file that has formatting specific to frame
ranges, which doesn't need the per file metadata", the bare
concatenation of member C4 IDs. Only its handling matches a listing's.

The rule: an output carries the ID list of every range entry it
contains, and no list for a range it does not. A list the inputs lack
stays absent; none is invented. A patch section carries the lists of the
ranges it adds or changes, after its entries; a removal line takes none.
A range line whose list is not inline is valid, as a directory line in
one-level form is; a conforming writer writes it as is.

## Files

| File               | Made by                                                                                                                           |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `folded.c4m`       | the Go encoder on `SequenceDetector` (minimum length 3): `plate.[1001-1004].exr` at the root, `sub/shot.[001-003].exr`, two lists |
| `plain.c4m`        | the Go encoder: `other.txt` and `readme.txt`, no ranges                                                                           |
| `no-plate.c4m`     | `folded.c4m` with the plate range and its list removed through the API, directory aggregates re-derived                           |
| `merged.c4m`       | `Merge(nil, folded, plain)`, encoded                                                                                              |
| `add.c4m`          | `PatchDiff(plain, folded)` written as `c4 diff` writes it: the old state's ID, then the patch                                     |
| `remove.c4m`       | `PatchDiff(folded, no-plate)` written the same way                                                                                |
| `add-chain.c4m`    | `plain.c4m` then `add.c4m`, byte for byte (`c4 diff >>`)                                                                          |
| `remove-chain.c4m` | `folded.c4m` then `remove.c4m`, byte for byte                                                                                     |

No line is composed by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestRangeDataVector` rewrites the files; the test then fails if they
differ from what the library writes. On the code before this vector,
`merged.c4m` and `add.c4m` come out with no ID-list lines, `c4 merge`,
`c4 diff` and `c4 patch` write none, and `remove-chain.c4m` re-encodes
with the plate list still in it, for a range it no longer holds.

The two chains place lists in both positions the spec allows: in
`remove-chain.c4m` they follow the base section's entries, before the
checkpoint; in `add-chain.c4m` they follow the patch section's entries.

## What a conforming implementation MUST do

Each operation's output, encoded canonically, is the named file byte for
byte. Ports without a sequence detector start from `folded.c4m`.

| Operation                           | Output         |
| ----------------------------------- | -------------- |
| merge `folded.c4m` with `plain.c4m` | `merged.c4m`   |
| merge `plain.c4m` with `folded.c4m` | `merged.c4m`   |
| merge `folded.c4m` with itself      | `folded.c4m`   |
| diff `plain.c4m` to `folded.c4m`    | `add.c4m`      |
| diff `folded.c4m` to `no-plate.c4m` | `remove.c4m`   |
| patch (resolve) `add-chain.c4m`     | `folded.c4m`   |
| patch `plain.c4m` with `add.c4m`    | `folded.c4m`   |
| patch (resolve) `remove-chain.c4m`  | `no-plate.c4m` |

In particular:

- `merged.c4m` carries both lists; `no-plate.c4m` and the resolution of
  `remove-chain.c4m` carry only the shot list, because the plate range is
  gone.
- `add.c4m` carries both lists after its entries; `remove.c4m` carries
  none, because its only line removes a range.
- The lists never change a description ID: each of `folded.c4m`,
  `merged.c4m` and `no-plate.c4m` has the same description ID with its
  lists as without them. Only the bytes, and so the byte ID, differ.

Go runs the operations in `TestRangeDataVector` (library) and
`TestRangeDataVectorCLI` (`c4 merge`, `c4 diff`, `c4 patch`).

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

| File               | Re-encodes to  |
| ------------------ | -------------- |
| `folded.c4m`       | itself         |
| `plain.c4m`        | itself         |
| `no-plate.c4m`     | itself         |
| `merged.c4m`       | itself         |
| `add.c4m`          | itself         |
| `remove.c4m`       | itself         |
| `add-chain.c4m`    | `folded.c4m`   |
| `remove-chain.c4m` | `no-plate.c4m` |
