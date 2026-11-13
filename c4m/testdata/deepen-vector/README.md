# Cross-implementation deepen-vs-remove vector (erratum, 2026-07-31)

The shared conformance fixture for the removal-rule narrowing
(SPECIFICATION.md "Patch Entry Semantics" / C4M-STANDARD §10.4).
Every c4m implementation (Go `c4m`, c4py, c4ts, c4m-swift, libc4,
and c4sh's vendored decoder) MUST pass both cases before the
coordinated release that ships the erratum.

## Files

| File         | A conforming resolving decoder MUST                            |
| ------------ | -------------------------------------------------------------- |
| `deepen.c4m` | accept; resolve to exactly the root ID in `deepen-root-id.txt` |
| `remove.c4m` | accept; resolve to exactly the root ID in `remove-root-id.txt` |

## Shape under test

Both files share a byte-identical base and restate the SAME directory
line byte-identically. They differ only in whether a child follows it:

```
-rw-r--r-- … a.txt …           base: a file
drwxr-xr-x … docs/ …                 a directory, resolved
  -rw-r--r-- … readme.md …           holding one file
c4<checkpoint>                 = accumulated state after the base

drwxr-xr-x … docs/ …           restated BYTE-IDENTICALLY in both files
  -rw-r--r-- … guide.md …      deepen.c4m ONLY: a child follows
```

Both files end on their last entry line. Until 2026-11-08 each ended
with the resolved manifest's ID on a bare final line; that line was
removed when a trailing bare ID was ruled a format error (chain-vector
README, "2026-11-08 amendment"). The root IDs in the `*-root-id.txt`
files are unchanged.

- `deepen.c4m` resolves to **four** entries: `a.txt`, `docs/`,
  `readme.md`, `guide.md`. A section that declares a child cannot be
  removing that child's parent.
- `remove.c4m` resolves to **one** entry: `a.txt`. Restating a
  directory alone still deletes its whole subtree — the narrowing does
  not disable removal.

The two root IDs therefore differ. That is the entire contract: the
same restated line means opposite things depending on whether children
follow it.

## Why this vector exists

Before the erratum, `deepen.c4m` resolved to `a.txt` alone — identical
to `remove.c4m`. The directory, the child the section had just
declared, AND `readme.md` (which the patch never mentioned) were all
silently discarded, at exit 0, with no diagnostic. Progressive
breadth-first scanning makes this the common shape rather than an
exotic one: a scan learns a directory's children before it learns
anything about the directory itself, so its parent line is necessarily
restated unchanged.

An implementation carrying the old rule fails `deepen.c4m` by
resolving it to `remove.c4m`'s answer, at exit 0: nothing in the file
verifies the final state, so conformance MUST be checked on the
resolved entry set against `deepen-root-id.txt`, never on an error.

Timestamps in the fixtures are fixed constants — the files are
byte-stable forever; apart from the 2026-11-08 removal of the trailing
ID line, regenerating them is never necessary and never permitted
(they are the contract).

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

| File         | Re-encodes to              |
| ------------ | -------------------------- |
| `deepen.c4m` | ID in `deepen-root-id.txt` |
| `remove.c4m` | ID in `remove-root-id.txt` |
