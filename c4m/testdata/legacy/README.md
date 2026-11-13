# Legacy c4m files

Real files written by released versions of c4, kept to pin how the
current decoder reads them. Never regenerate them. Each `.canonical.c4m`
beside one is its canonical partner, written by the current library.

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

| File                                 | Re-encodes to                        |
| ------------------------------------ | ------------------------------------ |
| `ten-dash-null-1.0.21.c4m`           | `ten-dash-null-1.0.21.canonical.c4m` |
| `ten-dash-null-1.0.21.canonical.c4m` | itself                               |
| `diff-chain-1.0.24.c4m`              | refused                              |
