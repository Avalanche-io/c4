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
c4<closing validator>          = the resolved manifest's ID, at EOF
```

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
resolving it to `remove.c4m`'s answer. Note that a decoder which
verifies the closing validator will surface this as a mismatch, while
a non-verifying chain reader will simply return the wrong manifest —
so conformance MUST be checked on the resolved entry set, not only on
the error.

Timestamps in the fixtures are fixed constants — the files are
byte-stable forever; regenerating them is never necessary and never
permitted (they are the contract).
