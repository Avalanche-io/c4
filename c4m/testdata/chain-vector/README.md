# Cross-implementation chain vector (grammar erratum, 2026-07-13; amended 2026-11-08)

The shared conformance fixture for the chain grammar
(SPECIFICATION.md "No Closing Validator" / C4M-STANDARD §10.5).
Every c4m implementation (Go `c4m`, c4py, c4ts, c4m-swift, libc4,
and c4sh's vendored decoder) MUST pass all three cases before the
coordinated release that ships the erratum.

## Files

| File                  | A conforming resolving decoder MUST                                      |
|-----------------------|--------------------------------------------------------------------------|
| `vector.c4m`          | accept; resolve to exactly the root ID in `resolved-root-id.txt`          |
| `bad-validator.c4m`   | reject (the bare ID at EOF mismatches the accumulated state, and ends the file) |
| `bad-checkpoint.c4m`  | reject (interior checkpoint does not match the accumulated state)          |

## Shape under test

```
<base entries>                 one full-form entry (a.txt)
c4<checkpoint>                 = accumulated state after the base
<patch entries>                one addition (b.txt); the stream ends here
```

Pinned semantics: checkpoints name the ACCUMULATED manifest state
(never the preceding block's text); a resolving decoder rejects any
mismatch (`ErrPatchIDMismatch`-equivalent); a stream ends on an entry
line, and a bare C4 ID at EOF is a format error (`ErrTrailingID`-
equivalent). Non-resolving shape readers MAY skip verification but
MUST still reject a trailing ID. Timestamps in the fixture are fixed
constants — the files are byte-stable forever.

## 2026-11-08 amendment

Until 2026-11-08 `vector.c4m` and `bad-checkpoint.c4m` ended with a
fourth block, the resolved manifest's ID on a bare final line (the
"closing validator" of the 2026-07-13 erratum). Joshua ruled that a
bare C4 ID at the end of a c4m file is a format error: no file can
contain its own ID. That final line was removed from both files; the
resolved root ID in `resolved-root-id.txt` is unchanged, because the
line never contributed to the state. `bad-validator.c4m` keeps its
trailing (wrong) ID: it is now the fixture for both rejections. The
"never regenerate" rule stands for the entries and checkpoint; this
one deletion is the only edit the files have had.
