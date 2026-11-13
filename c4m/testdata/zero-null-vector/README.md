# Cross-implementation vector: known zero against null (2026-11-09)

The shared conformance fixture for the mode and size null rules
(SPECIFICATION.md "Mode", "Null and Zero Values", "Empty and Unread
Directories"; C4M-STANDARD §4.1). Every c4m implementation (Go `c4m`,
c4py, c4ts, c4m-swift, libc4, and the mount and workspace tools built
on them) MUST pass it. Joshua asked for known zero against null to be
sorted correctly everywhere; this is the test of "everywhere".

## Files

| File            | Made by                                                 |
| --------------- | ------------------------------------------------------- |
| `canonical.c4m` | the Go encoder, canonical form (`TestZeroNullVector`)   |
| `pretty.c4m`    | the Go encoder, pretty form, timestamps rendered in UTC |
| `root-id.txt`   | `ComputeC4ID` of the manifest both files describe       |

No line was written by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestZeroNullVector` rewrites the files from the library; the test then
fails if the files differ from what the library writes.

## What a conforming implementation MUST do

Decode either file to one manifest whose root ID is `root-id.txt`, and see:

| Entry          | Mode                                      | Size         | C4 ID            |
| -------------- | ----------------------------------------- | ------------ | ---------------- |
| `zero.txt`     | **known zero**, `----------`, not null    | 4            | present          |
| `nullmode.txt` | **null**, `-`                             | 4            | present          |
| `empty.txt`    | `-rw-r--r--`                              | 0, known     | empty-content ID |
| `emptydir/`    | `drwxr-xr-x`                              | 0, known     | empty-content ID |
| `locked/`      | **`d---------`**: known, zero permissions | 142, derived | present          |
| `inside.txt`   | `-rw-r--r--` (child of `locked/`)         | 6            | present          |
| `unread/`      | **`d---------`**: known, zero permissions | **null**     | **null**         |

Then re-encode: canonical form reproduces `canonical.c4m` byte for byte
(null mode as `-`, zero mode as `----------`, null size as `-`); pretty
form writes the same mode spellings (column layout and local-time
rendering may differ by implementation; the decoded manifest must not).

The legacy exception is the fourth file, `legacy-ten-dash-1.0.21.c4m`: a
real pretty-form file written by c4 1.0.21, whose encoder wrote every
null mode as ten dashes (a copy of `testdata/legacy/ten-dash-null-1.0.21.c4m`).
A conforming decoder reads its **directory** lines (`---------- … d/`) as
**null** mode, because a correct encoder writes a 0000 directory as
`d---------` and only an old encoder produces ten dashes beside a `/`;
it reads its **file** lines as the **known zero**, as ruled, which is the
accepted cost of giving ten dashes a meaning. Re-encoding it writes `-`
for every null, in canonical and pretty form alike; no encoder may write
ten dashes for a null on any line.

The general rule (SPECIFICATION.md "Null and Zero Values"): the name's
trailing `/` says whether an entry is a directory; the type character must
agree. Ten dashes on a directory line is the one disagreement with a
defined meaning (legacy null). The Go decoder accepts other disagreements
unchanged today (measured 2026-11-09); whether they become errors is
Joshua's call.

## What the vector catches

- A decoder that reads `----------` as null (c4 through 1.0.24, c4py,
  c4ts, c4m-swift, libc4 before 2026-11-09).
- A pretty encoder that writes null as `----------`.
- A scanner or propagator that writes `0` for a directory nobody could
  read (`unread/` must keep a null size; c4 master did this until
  2026-11-09).
- A materialiser that applies a null mode as 0000, or a known zero as
  the platform default.

Timestamps are fixed constants; the files are byte-stable. Regenerate
only through the library, never by editing.

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

| File                                   | Re-encodes to                          |
| -------------------------------------- | -------------------------------------- |
| `canonical.c4m`                        | itself                                 |
| `pretty.c4m`                           | `canonical.c4m`                        |
| `legacy-ten-dash-1.0.21.c4m`           | `legacy-ten-dash-1.0.21.canonical.c4m` |
| `legacy-ten-dash-1.0.21.canonical.c4m` | itself                                 |
