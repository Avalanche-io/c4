# Cross-implementation vector: a directory with no listed children (2026-11-11)

The shared conformance fixture for deriving the size of a directory that
has no children listed under it and a null size. What it derives to depends
on its C4 ID alone (SPECIFICATION.md, "Empty and Unread Directories"). Every
c4m implementation MUST pass it.

| Directory    | C4 ID                  | Derived size | Why                                                   |
| ------------ | ---------------------- | ------------ | ----------------------------------------------------- |
| `empty/`     | the empty listing's ID | `0`          | its listing is known to be empty, so it holds nothing |
| `collapsed/` | any other ID           | null         | its listing lives under that ID, not in this file     |
| `unread/`    | none                   | null         | nobody listed it                                      |

## Files

| File             | Made by                                                                  |
| ---------------- | ------------------------------------------------------------------------ |
| `stated.c4m`     | the Go encoder: the three directories, each with a null size             |
| `normalized.c4m` | `Normalize` of the same manifest: `empty/` takes 0, the others stay null |
| `root-id.txt`    | `ComputeC4ID` of the manifest                                            |

No line was written by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestChildlessDirVector` rewrites the files from the library, and the test
fails if a file differs from what the library writes.

## What a conforming implementation MUST do

- Resolve `stated.c4m` and `normalized.c4m` to `root-id.txt`. They are the
  same description: the ID hashes the derived sizes.
- When it normalises or propagates metadata, give `empty/` size 0 and leave
  `collapsed/` and `unread/` null.
- The C4 ID of `normalized.c4m`'s own bytes is `root-id.txt`: the file is
  canonical, and its bytes are the description.

## Found by

Every implementation in the suite, through 2026-11-11, gave `collapsed/` size
0 when computing the C4 ID: any non-nil ID was taken to mean empty. The ID
therefore hashed a size the file does not state and nothing observed. The
Go library's `Normalize` went the other way and left `empty/` null, so the
normaliser and the ID disagreed.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File             | Re-encodes to |
| ---------------- | ------------- |
| `normalized.c4m` | itself        |
| `stated.c4m`     | itself        |
