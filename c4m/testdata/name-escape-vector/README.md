# Cross-implementation vector: a name's escapes are never left raw (2026-11-11)

The shared conformance fixture for the characters a written name escapes.
Every c4m implementation MUST pass it. In a written name a space is `\ `, a
double quote is `\"`, and a control byte is encoded by SafeName (`\t`, `\n`,
`\r`, `\0`, or braille between `¤`); no conforming writer leaves one raw
(SPECIFICATION.md, "Universal Filename Encoding" and "c4m field-boundary
escaping"). A raw one is damage. An unescaped space most often means a
damaged C4 ID field, which used to join the name: `real.txt junk` read as one
name and the line's ID was dropped.

Indentation is not damage. Any consistent indent unit reads, because the
libraries' own public APIs can write one (Go `Encoder.SetIndent`, c4ts
`encode(m, {indentWidth})`, c4m-swift `Encoder(indentWidth:)`, and every
per-entry `format(width)`); canonical output always indents by 2. A
validator reports a unit other than 2: a warning in lax mode, an error in
strict mode.

## Files

Every file is made by the Go library (`TestNameEscapeVector`;
`C4M_UPDATE_VECTORS=1 go test ./c4m -run TestNameEscapeVector` rewrites them,
and the test fails if a file differs from what the library and the damage
write). The damaged files are a library line with only its name changed; no
line was written by hand.

| File                        | What it holds                                              | A reader MUST           |
| --------------------------- | ---------------------------------------------------------- | ----------------------- |
| `ok-file.c4m`, `ok-dir.c4m` | the library's plain file and directory lines               | decode                  |
| `ok-escaped.c4m`            | the name `my file "q"<TAB>.txt`, as the library escapes it | decode to exactly that  |
| `ok-indent-3.c4m`           | `d/` holding `x.txt`, written with `SetIndent(3)`          | decode, `x.txt` depth 1 |
| `file-unescaped-space.c4m`  | `real txt`                                                 | refuse, naming line 1   |
| `file-damaged-id-field.c4m` | `real.txt junk` before the C4 ID                           | refuse, naming line 1   |
| `file-raw-quote.c4m`        | `re"al.txt`                                                | refuse, naming line 1   |
| `file-raw-tab.c4m`          | `re<TAB>al.txt`, the tab raw                               | refuse, naming line 1   |
| `file-raw-control.c4m`      | `re<0x01>al.txt`, the byte raw                             | refuse, naming line 1   |
| `dir-unescaped-space.c4m`   | `my dir/`                                                  | refuse, naming line 1   |

**Writer.** An encoder MUST escape these characters in every name it writes.
Through 1.0.26 the Go, c4py and c4m-swift formatters returned the raw name
when an entry was marked as a sequence but held no range (an API-only
path); that path now escapes the name as an ordinary one.

## Known writers of these shapes

The one known writer of raw spaces, quotes and control bytes in names is
libc4's pre-1.0 tag `v0.8.1` (2026-02-28, before SafeName), which no suite
release ever listed. Every 1.0 release of every implementation escapes them
on every CLI and canonical path (measured 2026-11-11 from history and by
running Go and c4py at v1.0.0 and v1.0.25), apart from the sequence fallback
above.

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File                        | Re-encodes to                                                      |
| --------------------------- | ------------------------------------------------------------------ |
| `ok-dir.c4m`                | itself                                                             |
| `ok-escaped.c4m`            | itself                                                             |
| `ok-file.c4m`               | itself                                                             |
| `ok-indent-3.c4m`           | unpaired: written with `SetIndent(3)`; canonical form indents by 2 |
| `dir-unescaped-space.c4m`   | refused                                                            |
| `file-damaged-id-field.c4m` | refused                                                            |
| `file-raw-control.c4m`      | refused                                                            |
| `file-raw-quote.c4m`        | refused                                                            |
| `file-raw-tab.c4m`          | refused                                                            |
| `file-unescaped-space.c4m`  | refused                                                            |
