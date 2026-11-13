# Cross-implementation vector: a size of 0 beside a C4 ID (2026-11-13)

The shared conformance fixture for the null-size rule (SPECIFICATION.md
"Size" and the null table; C4M-STANDARD §4.3): a size of 0 beside a C4 ID
other than the empty content's is a null size. A filesystem surface has no
`-`, so a null size shows there as 0, and only the empty content truly has
size 0. The rule was ruled 2026-11-08 and landed 2026-11-13 (decided by the
release lead; reversible). Every c4m implementation (Go `c4m`, c4py, c4ts,
c4m-swift, libc4) MUST pass it.

## Files

| File               | Made by                                                                                      |
| ------------------ | -------------------------------------------------------------------------------------------- |
| `canonical.c4m`    | the Go encoder, canonical form (`TestNullSizeVector`); null sizes written as `-`             |
| `zero-spelled.c4m` | `canonical.c4m` with the size field of every `-` line rewritten to `0`, nothing else changed |
| `root-id.txt`      | `ComputeC4ID` of the manifest both files describe                                            |

No line is composed by hand. `zero-spelled.c4m` is the one transformation
of the library's own output that this rule is about.
`C4M_UPDATE_VECTORS=1 go test ./c4m -run TestNullSizeVector` rewrites the
files; the test then fails if they differ from what the library writes.

## What a conforming implementation MUST do

Decode either file to one manifest whose root ID is `root-id.txt`, and see:

| Entry           | Size line  | C4 ID               | Size is  |
| --------------- | ---------- | ------------------- | -------- |
| `null.txt`      | `-` or `0` | content of `hello`  | **null** |
| `collapsed/`    | `-` or `0` | a non-empty listing | **null** |
| `empty.txt`     | `0`        | empty-content ID    | known 0  |
| `emptydir/`     | `0`        | empty-content ID    | known 0  |
| `structure.txt` | `0`        | none                | known 0  |
| `link`          | `0`        | none (symlink)      | known 0  |
| `known.txt`     | `5`        | content of `hello`  | known 5  |

Then re-encode either file canonically: both reproduce `canonical.c4m`
byte for byte, every null size written as `-`.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File               | Re-encodes to   |
| ------------------ | --------------- |
| `canonical.c4m`    | itself          |
| `zero-spelled.c4m` | `canonical.c4m` |
