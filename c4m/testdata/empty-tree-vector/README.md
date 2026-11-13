# Cross-implementation vector: the empty tree (2026-11-11)

The shared conformance fixture for the C4 ID of an empty listing. An empty
tree is known to be empty, not unknown. Its C4 ID is the C4 ID of its
canonical text, and that text is empty, so the ID is the C4 ID of zero
bytes. A directory whose listing is empty carries that ID. A nil (all-ones)
ID would invent an unknown where the value is known. Every c4m
implementation MUST pass this vector.

## Files

| File                    | Made by                                                                       |
| ----------------------- | ----------------------------------------------------------------------------- |
| `empty.c4m`             | the Go encoder, the empty manifest: zero bytes                                |
| `empty-id.txt`          | `ComputeC4ID` of the empty manifest                                           |
| `empty-dir.c4m`         | the Go encoder, canonical form: a tree whose only child is an empty directory |
| `empty-dir-pretty.c4m`  | the same tree, pretty form, timestamps rendered in UTC                        |
| `empty-dir-patch.c4m`   | `EncodePatch` over the empty tree: the empty tree's ID, then the entry        |
| `empty-dir-root-id.txt` | `ComputeC4ID` of the one-directory tree                                       |

No line was written by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestEmptyTreeVector` rewrites the files from the library, and the test fails
if a file differs from what the library writes. The directory is built
through the API with size 0 and the empty listing's ID, as `c4 id` writes an
empty directory it scanned.

## What a conforming implementation MUST do

- Compute the empty manifest's C4 ID as `empty-id.txt`:
  `c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT`,
  the C4 ID of zero bytes. Decoding `empty.c4m` gives that manifest.
- Decode `empty-dir.c4m` and `empty-dir-pretty.c4m` to one directory entry
  with size 0 and the C4 ID in `empty-id.txt`, and resolve both to
  `empty-dir-root-id.txt`.
- Decode `empty-dir-patch.c4m` to one section over the ID in
  `empty-id.txt`. A patch from the empty tree names that ID as its base.
  When an implementation writes such a patch, it writes the same base.

## Found by

A port that returned the nil ID for an empty manifest. Its own patches from
the empty tree began with the nil ID, so no other implementation could match
their base.

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

| File                   | Re-encodes to   |
| ---------------------- | --------------- |
| `empty.c4m`            | itself          |
| `empty-dir.c4m`        | itself          |
| `empty-dir-patch.c4m`  | itself          |
| `empty-dir-pretty.c4m` | `empty-dir.c4m` |
