# Cross-implementation chain vector (grammar erratum, 2026-07-13)

The shared conformance fixture for the chain-grammar erratum
(SPECIFICATION.md "The Closing Validator" / C4M-STANDARD §10.5).
Every c4m implementation (Go `c4m`, c4py, c4ts, c4m-swift, libc4,
and c4sh's vendored decoder) MUST pass all three cases before the
coordinated release that ships the erratum.

## Files

| File                  | A conforming resolving decoder MUST                                    |
|-----------------------|------------------------------------------------------------------------|
| `vector.c4m`          | accept; resolve to exactly the root ID in `resolved-root-id.txt`        |
| `bad-validator.c4m`   | reject (closing validator does not match the resolved manifest)         |
| `bad-checkpoint.c4m`  | reject (interior checkpoint does not match the accumulated state)        |

## Shape under test

```
<base entries>                 one full-form entry (a.txt)
c4<checkpoint>                 = accumulated state after the base
<patch entries>                one addition (b.txt)
c4<closing validator>          = the resolved manifest's ID, at EOF
```

Pinned semantics: checkpoints name the ACCUMULATED manifest state
(never the preceding block's text); a bare C4 ID at EOF is the legal,
mandatory-verified closing validator; a resolving decoder rejects any
mismatch (`ErrPatchIDMismatch`-equivalent). Non-resolving shape
readers MAY skip verification. Timestamps in the fixture are fixed
constants — the files are byte-stable forever; regenerating them is
never necessary and never permitted (they are the contract).
