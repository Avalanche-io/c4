# A directory's timestamp: its own mtime, or its newest descendant's?

Status: format question for Joshua, 2026-11-03. Two readings exist in the
spec and the code, and they disagree on master today. This note names the
scope in which each is right, which code follows which, what breaks, and
the minimal fix under each reading. It picks nothing.

## The defect that exposed it

Measured on master `c75eb94`, scratch HOME, library-built c4m
(`/tmp/c4repro/repro1.sh`): a tree `src/a/x.txt` with `x.txt` at
16:00 and the directory `a/` itself touched to 17:00.

- `c4 id -s src/` writes `a/` at `23:00:00Z` (17:00 local, the
  directory's own mtime) in both the base section and the refinement
  patch, and stores the description as `c467DuVw…`.
- `c4 id -d src.c4m` reports the scan's own output as not normal: it would
  rewrite `a/` to `22:00:00Z`, the newest child's time.
- `c4 restore --force src.c4m dest/` into an empty folder materialises
  the tree correctly, then fails its own check: "verification failed:
  as-built `c441QXkY…` != target `c467DuVw…`", exit 3. A rescan of
  `dest/` gives exactly the source's description, so the folder is right
  and the verdict is wrong.

Cause: `cmd/c4/restore.go` `projectToKnowledge` calls `c4m.Normalize` on
the as-built scan, and the normaliser (`c4m/normalize.go`, 71d2cd3, today)
clears every listed directory's timestamp and re-derives it from its
children. The target was scanned, so it carries the directory's own
mtime; the projection carries the newest child's; the IDs differ. The
scouts' report: every nested tree whose directory mtime differs from its
newest child's to the second, which is the normal case for media written
over more than a second or re-rendered in place.

## Reading A: the directory's own modification time

`c4m/SPECIFICATION.md`, "Directory Timestamps": "Directory timestamps come
from the filesystem's modification time for the directory itself."

Code that follows it:

- The scan: `cmd/c4/id.go:488` and `cmd/c4/internal/scan/metadata.go:111`
  set every entry's timestamp, directories included, from `ModTime()`.
- `c4m.PropagateMetadata` (`c4m/manifest.go`): "entries with non-null
  Size/Timestamp are never overwritten". It fills a *null* directory
  timestamp with the newest descendant's; it never replaces a stated one.
- Every released c4 (1.0.x) and every stored description made by one.

Under A a directory's timestamp is a fact observed on disk, like a
file's. Its children cannot derive it: a rename or deletion inside the
directory bumps the directory's mtime and touches no child. It is
information the description carries and the children do not.

## Reading B: the newest descendant's time

`c4m.PropagateMetadata`'s doc comment: "Directory Timestamp is the most
recent modification time among all descendants." The normaliser's rule
(`design/normalizer.md`, "Rule" 1): size, timestamp and C4 ID are "computed
from the listed children … filling nulls and replacing stale values
alike".

Code that follows it:

- `c4m.Normalize` (today): clears and re-derives the timestamp of every
  directory with listed children.
- `projectToKnowledge` in restore, through Normalize.
- Sequence folding (`cmd/c4/help.go:134`): a folded entry's "mtime = the
  newest member's" (a folded range is not a directory, but it is the same
  idea: a derived time for an aggregate).
- c4-operator's views null every timestamp, so they take no side.

Under B a directory's timestamp is an aggregate, like its size: a function
of its children, recomputable by anyone from the listing, and a hand edit
or a stale value can be corrected by the normaliser. The price is the
information A keeps: a directory whose contents were renamed or deleted
reads as unchanged, and a scan must derive rather than observe, so an
empty directory has a null time.

## Where each is right

- **A answers "what did the filesystem say about this directory?"** That
  is what a scan records and what a restore must reproduce, since
  `chtimes` on the directory is how the as-built tree gets the same ID
  back. The round-trip test (scan, materialise, scan again, same ID) only
  holds under A if the directory's own time is restored, which the
  reconciler already does (`applyChtimes`, deferred for directories).
- **B answers "what do these children determine?"** That is the
  normaliser's question. Size and the directory's C4 ID are determined by
  the children; the timestamp is determined only when it was null, and
  only as a fill, which is what `PropagateMetadata` has always done.

The two are not in conflict once the scopes are named: the timestamp is
*observed* when the scan can observe it and *filled from the children*
when nobody observed it. The conflict was introduced today by making the
normaliser treat a fill rule as a derivation rule. The doc comment on
`PropagateMetadata` ("most recent modification time among all
descendants") describes the fill, and reads as a definition; that
sentence is where B came from.

## The minimal fix under each reading

**Under A (spec unchanged):** the normaliser re-derives size and C4 ID,
and fills a null directory timestamp from the children, but never
replaces a stated one. `c4m/normalize.go`: stop clearing `e.Timestamp`
before `propagateMetadata`; the "timestamp" `Change` can then only come
from a fill, so it is dropped (filled nulls are not changes). The
`PropagateMetadata` comment is reworded to say "fills a null directory
timestamp with…". `design/normalizer.md` rule 1 is corrected in place,
dated. Restore's false failure disappears because the projection keeps
the as-built directory times. One test: a scanned nested tree is already
normal (`c4 id -l` prints nothing), and restore of it verifies.

**Under B (spec changes):** SPECIFICATION.md "Directory Timestamps"
becomes "the most recent modification time among the directory's
descendants; an empty directory's timestamp is null"; the scan derives
directory times instead of reading them (`id.go`, `scan/metadata.go`),
and the reconciler stops restoring directory mtimes (they are implied).
Every stored description of a nested tree made by 1.0.x gets a new ID
under re-scan, so this is a format change for the 1.1 line, with a
conformance vector, and it is multiplied across c4py, c4ts, c4m-swift and
libc4, whose scanners all read the directory's own mtime today.

Either way, `projectToKnowledge` should project the target's *knowledge*
(which fields are null) and nothing else; today it also re-derives, which
is only correct if derivation is total, which under A it is not.

## What is not in question

- Directory **size** is derived from the children (sum of file sizes plus
  each subdirectory's record length) and the normaliser is right to
  replace a stale one.
- Directory **C4 ID** is the ID of its one-level record and the normaliser
  is right to replace a stale one.
- **Null fill** from the newest descendant is the behaviour of every
  implementation and is not changed by either reading.
