# Legacy c4m files

Real files written by released versions of c4, kept to pin how the
current decoder reads them. Never regenerate them.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File                       | Re-encodes to                                                                 |
| -------------------------- | ----------------------------------------------------------------------------- |
| `ten-dash-null-1.0.21.c4m` | unpaired: written by c4 1.0.21, whose encoder wrote a null mode as ten dashes |
| `diff-chain-1.0.24.c4m`    | refused                                                                       |
