# Cross-implementation vector: a damaged field is an error, never a value (2026-11-10)

The shared conformance fixture for reading a c4m line whose fields have
been damaged, as a torn write, a bad copy or a hand edit damages them.
Every c4m implementation MUST pass it. The rule under test: a field that
does not parse is an error naming the line. It never becomes a value: not
a null C4 ID, not a clear permission bit, not a size with the rest of its
token moved into the name.

## Files

`ok.c4m` is one line written by the Go library (`TestDamagedLineVector`).
Each other file is that line with one field damaged, made by the same
test; no line was written by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m
-run TestDamagedLineVector` rewrites them, and the test fails if a file
differs from what the library and the damage write.

## What a conforming implementation MUST do

Decode `ok.c4m`. Refuse every other file with an error that names line 1.

| File                               | Damage                                           | Go decoder through f87a2d5   |
| ---------------------------------- | ------------------------------------------------ | ---------------------------- |
| `id-89.c4m`                        | C4 ID cut by one character (a torn write)        | refused                      |
| `id-91.c4m`                        | C4 ID one character too long                     | refused                      |
| `id-bad-char.c4m`                  | C4 ID with a character outside base58 (`0`)      | refused                      |
| `size-junk.c4m`                    | `6x`: a size followed by junk                    | size 6, name `x real.txt`    |
| `size-null-junk.c4m`               | `-x`: a null size followed by junk               | size null, name `x real.txt` |
| `mode-bad-char.c4m`                | `-rw-r--r-z`: a character no encoder writes      | read as `-rw-r--r--`         |
| `mode-wrong-letter.c4m`            | `-rr-r--r--`: an `r` in the owner-write position | read as `-r--r--r--`         |
| `mode-sticky-letter-misplaced.c4m` | `-rwtr--r--`: `t` in the owner-execute position  | read as `-rw-r--r--`         |
| `time-impossible.c4m`              | month 13, day 45                                 | refused                      |

## What each permission position may hold

Exactly what every encoder in the suite writes: the position's own letter
(`r`, `w`, `x`) or `-`; at the owner- and group-execute positions also `s`
or `S` (setuid, setgid); at the other-execute position also `t` or `T`
(sticky). A size token ends at a space.

Before pinning this, files written by every encoder in the suite were read
with the stricter decoder and all decoded: the Go CLI at v1.0.21, v1.0.25
and master, c4py at v1.0.21 and main, and c4ts main, each in canonical and
pretty form, on a real tree holding setuid and sticky bits with and without
execute (`-rwsr-xr-x`, `-rwSr--r--`, `drwxrwxrwt`, `drwxrwxrwT`), a
known-zero mode and a 1.2 MB file. c4m-swift and libc4 write the same
character set (their mode formatters were read).

## Round trip

Each file decoded and then encoded canonically. Go's
`TestVectorRoundTrip` reads this table, and every port runs the same
check, comparing SHA-512 of the bytes:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `ID in` a file: a chain; the resolved state has that C4 ID.
- `refused`: decoding fails.
- `unpaired`: it decodes; no file holds its canonical form, for the reason given.

| File                               | Re-encodes to |
| ---------------------------------- | ------------- |
| `ok.c4m`                           | itself        |
| `id-89.c4m`                        | refused       |
| `id-91.c4m`                        | refused       |
| `id-bad-char.c4m`                  | refused       |
| `mode-bad-char.c4m`                | refused       |
| `mode-sticky-letter-misplaced.c4m` | refused       |
| `mode-wrong-letter.c4m`            | refused       |
| `size-junk.c4m`                    | refused       |
| `size-null-junk.c4m`               | refused       |
| `time-impossible.c4m`              | refused       |
