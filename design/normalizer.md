# The normaliser: `c4 id FILE.c4m` re-derives what the file determines

Status: design, 2026-11-03. Ruled by Joshua (via c4-fleet): "unless the c4
lead comes up with an issue we should have the normalizer do the tree walk
fixes as well." One open edge below is sent back for his answer before any
code.

## Problem

`c4 id FILE.c4m` is the documented normaliser, and today it fixes form only.
Measured on main 2c78b64 (c4-fleet, scratch HOME `/tmp/c4rt.zHMz`):

- **Stale derived values survive.** `nulls.c4m` lists `d/` with a stale size
  (136; the children determine 117) and a stale C4 ID. The normalised output
  keeps both, so its root ID is `c44SU47…`, while the library's own scan of
  the same tree is `c44voXiE…`.
- **Null derived values are not filled.** `derive.c4m` lists `d/` as
  `drwxr-xr-x - - d/ -` with its child beneath it. The output keeps the
  nulls, root ID `c449akn…`: the scan stream's *first* section, before its
  refinement patch.

Everything needed is in the file. A directory whose children are listed has
a derivable size, timestamp and C4 ID, by the same rules the scan stream's
refinement patch applies. A hand edit should normalise to exactly the
library's ID, as `gofmt` turns any spacing into the one canonical form.

## Rule

When `c4 id` is given a c4m file (or `-`), after resolving the chain:

1. **Every directory whose children are listed is re-derived.** Its size,
   timestamp and C4 ID are computed from the listed children by the
   library's rules (below), filling nulls and replacing stale values alike.
   The listed children *are* the directory's definition; a stated aggregate
   that disagrees with them is wrong by construction.
2. **A directory listed without children keeps its stated values.** Nothing
   in the file contradicts them; its ID points at a record in a store. This
   is the collapsed form, and it stays collapsed.
3. **Derived values that would change a stated non-null value are reported
   on stderr**, one line per directory, in every mode. Stdout stays data.
   Verify rather than assert: a stale value is never kept, and never
   replaced silently.
4. **The root ID line** is recomputed from the normalised text, as today.

The derivation rules are the existing ones, used by exactly one
implementation each:

- **Size and timestamp:** `c4m.PropagateMetadata`: size is the sum of
  descendant file sizes plus the byte length of each subdirectory's
  canonical one-level record; timestamp is the most recent among
  descendants; both nil-infectious; an empty directory has size 0 and a null
  timestamp. Today PropagateMetadata fills nulls only ("entries with
  non-null Size/Timestamp are never overwritten"); the normaliser clears the
  aggregates of every directory with listed children first, then calls it,
  so one function derives for both the scan and the normaliser.
- **Directory C4 ID:** the ID of the directory's canonical one-level record
  (direct children at depth 0, canonicalised, sorted), which is what
  `storeDirectoryC4m` stores. That derivation moves into the c4m package as
  `DirectoryRecord(m, dir) (text, id)`; `storeDirectoryC4m` becomes "compute
  with DirectoryRecord, then Put". One implementation per idea.
- **Order:** children before parents, deepest first, so a parent's record
  byte length and ID use the children's resolved lines.

## The open edge: a known value above null children

The derivation is nil-infectious. If `d/` is listed with children that carry
null sizes (a structure-level listing), the derived size of `d/` is null.
Under rule 1 a stated `117` on `d/` is then "stale" and becomes `-`.

That is the one case where normalising replaces a stated value with less
knowledge. Two readings:

- **Re-derive (the rule as stated).** The file cannot support 117 from what
  it lists; keeping it asserts what the file does not show. The value is
  discarded, not invented, and rule 3 reports it. A user who wants 117 kept
  lists the children that produce it, or collapses `d/`.
- **Keep a stated value when the derivation is null.** Nothing in the file
  contradicts 117 either; null is absence of knowledge, not a conflicting
  value. Mixed-level files stay as written.

Recommendation: **re-derive**. A description computes at its own knowledge
level, and a directory line claiming more knowledge than its listed children
is exactly the inconsistency a normaliser exists to remove; with rule 3 it is
never silent. This goes to Joshua before code.

## Checking, gofmt-style

Two flags on `c4 id`, both free today (`-l`, `-d`), both for c4m-file
arguments only:

| Invocation            | Stdout                                        | Exit                               |
| --------------------- | --------------------------------------------- | ---------------------------------- |
| `c4 id FILE.c4m`      | the normalised text, then the root ID         | 0; 2 if the file has null leaves   |
| `c4 id -l FILE...`    | the names of files whose normalised text differs from their bytes | 1 if any name was printed, else 0 |
| `c4 id -d FILE...`    | a unified diff per differing file             | 1 if any diff was printed, else 0  |

`-l` and `-d` compare the file's bytes to its normalised canonical text, so
they catch form (spacing, order, pretty vs canonical) as well as derived
values. The exit code is the pre-commit contract: a hook runs `c4 id -l
*.c4m` and fails on 1, no output parsing needed. Writing back in place is a
shell composition (`c4 id f.c4m > f.tmp && mv f.tmp f.c4m`); no `-w` flag
unless Joshua wants the gofmt idiom, since an in-place rewrite of a file the
command is reading is the one way this could destroy data.

`-q` keeps its meaning: the root ID alone, computed from the normalised
description.

## What does not change

- Chains resolve first (`DecodeResolved`), as today; the output is flat.
- `-m` still projects downward after normalising.
- A directory argument is unaffected: the scan already produces normalised
  output.
- Byte-identical output at any concurrency; the derivation is a pure
  function of the resolved entries.

## Tests

- c4-fleet's three cases, made without hand-rolling c4m: a library scan of a
  two-file tree is the reference (`n.c4m`); the stale and nulled variants are
  produced by decoding it, mutating the `d/` entry through the c4m package,
  and encoding. Normalising each must give the reference's root ID and
  entry lines.
- A collapsed directory (no children listed, stated size and ID) is
  unchanged.
- `-l` and `-d` on an already-normalised file print nothing and exit 0; on a
  stale file print the name or the diff and exit 1.
- The open-edge case, whichever way it is ruled, pinned by a test.
- The stderr report names each changed directory with old and new values.

## Cost

One c4m function (`DirectoryRecord`) extracted, one function
(`Normalize`) added, two flags on one verb. Other implementations gain
nothing to do until they implement a normaliser; the conformance vectors
gain the three cases above.
