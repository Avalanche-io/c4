# Cross-implementation vector: names that are not ASCII (2026-11-11)

The shared conformance fixture for reading names, a symlink target and a
flow-target path whose bytes are not ASCII. Every c4m implementation MUST
pass it.

## Files

Every file is written by the Go library (`TestUnicodeNameVector`;
`C4M_UPDATE_VECTORS=1 go test ./c4m -run TestUnicodeNameVector` rewrites
them, and the test fails if a file differs from what the library writes).
No line was written by hand. Every timestamp is null: the pretty form writes
a stated time in the local zone, so stated times would make the file depend
on the machine that wrote it.

| File                        | Content                                                                |
| --------------------------- | ---------------------------------------------------------------------- |
| `canonical.c4m`             | the tree in canonical form                                             |
| `pretty.c4m`                | the same tree in pretty form (aligned columns)                         |
| `names.txt`                 | every entry's name, one per line, indented by depth                    |
| `root-id.txt`               | the tree's description C4 ID                                           |
| `nfc-nfd-chain.c4m`         | a base holding `café.txt` in NFC, then a patch adding the NFD spelling |
| `nfc-nfd-chain-root-id.txt` | the chain's resolved C4 ID: both spellings                             |

The tree holds:
- `café.txt` twice, once in NFC (`é` is U+00E9) and once in NFD (`e` then U+0301). They are two distinct names, by bytes.
- `日本.txt`.
- `😀.txt`, a code point outside the Basic Multilingual Plane.
- A directory `Ünïcödé/` holding `naïve.txt`.
- A symlink `链接 -> 目标.txt`.
- A directory `发送/` with a flow link `-> nas:データ/`.

## What a conforming implementation MUST do

Decode `canonical.c4m` and `pretty.c4m`. Each MUST give exactly the names
in `names.txt` (byte for byte, normalisation untouched), the link's target
`目标.txt`, the flow target `nas:データ/`, and the root ID in `root-id.txt`.
It MUST resolve `nfc-nfd-chain.c4m` to two entries, both spellings, with
the root ID in `nfc-nfd-chain-root-id.txt`: names are bytes, and a reader
that compares canonically equivalent text as equal merges them.
Re-encoding `canonical.c4m` MUST give back its bytes.

## What it caught

Through 1.0.25, c4m-swift refused every line whose name was not ASCII. Its
field parsers walked the line's UTF-8 bytes but sliced the line by Character
offsets, so the C4 ID field lost one position per extra byte of the name:
`café.txt … c41dF…` was refused as "invalid C4 ID '41dF…'". Behind that,
its name parser built the name one byte per Character, so `é` would have
read as `Ã©`. Measured 2026-11-11 on c4m-swift main and release/1.0.x, with
lines written by the Go binary from real files. The c4m-swift encoder wrote
the same names byte-identical to Go, with matching C4 IDs.

The fix also exposed a second c4m-swift fault: Swift `String` equality,
ordering and hashing use canonical equivalence, so wherever it keyed names
as `String` the NFC and NFD spellings were one name. Its encoder dropped one
of the two `café.txt` entries, and the chain above resolved to one entry.

## On release lines before 1.0.27

`root-id.txt` is master's. The childless directory `发送/` has a null size
and no ID, and before the collapsed-directory size rule (1.0.27) a release
line canonicalises that null size as `0`. So on release/1.0.x the tree's ID
is `c411tr2n8AwGyvHsERmffCE38dcjqPaY92KjizyxW9iafuxh3eJ53R9upuWH5MprvogtoM9nNm8j4Fde7L3vCZtj81`,
measured with Go release/1.0.x's library (2026-11-11). Every other
expectation is the same on both lines.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File                | Re-encodes to                     |
| ------------------- | --------------------------------- |
| `canonical.c4m`     | itself                            |
| `pretty.c4m`        | `canonical.c4m`                   |
| `nfc-nfd-chain.c4m` | ID in `nfc-nfd-chain-root-id.txt` |
