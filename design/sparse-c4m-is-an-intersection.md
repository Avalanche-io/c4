# A sparse c4m is an intersection

**Status:** proposal, 2026-11-24. Nothing is built. It needs the maintainer's
answers to the four questions at the end.

## The point

A mount (or any consumer) that wants part of a tree needs no selection
language of its own. The part is itself a c4m, with its own C4 ID: the source
c4m intersected, by path, with a set of names. Core already has both halves:

- **a set of names is a c4m**: SPECIFICATION.md's null-values use cases name
  "Boolean set operations: Create manifests with only names to define sets",
  and `c4 paths` turns a path list into exactly that;
- **an intersection by path:** `c4 intersect path <a> <b>`, documented as
  giving "a valid c4m from the second argument's perspective".

So the sparse c4m needs no new verb, flag or format:

    c4 paths FULL.c4m | grep -E '…' | c4 paths | c4 intersect path - FULL.c4m

Patterns stay the caller's business (grep here, a controller's own matching
elsewhere). C4 gains no pattern language to specify, port and write vectors
for. Nothing is lost by keeping patterns out: a pattern is an intention, and
its expansion against FULL is the fact that the result depends on.

What is missing is that `intersect path` doesn't yet give the right answer.

## What it gives today (measured, master f1bbcda)

The source is a scanned tree: `docs/readme.txt` and
`shots/{s010/{a,b}.exr, s020/c.exr}`, with `shots/` at `drwxr-x---` and each
folder's time distinct.

| selection | result |
| --- | --- |
| `shots/s010/a.exr` | a.exr with FULL's metadata, under `- - - shots/ -` and `- - - s010/ -` |
| the folder line `shots/s010/` alone | nothing (exit 0, empty output) |
| a path FULL lacks | nothing (exit 0) |
| no names at all | an empty c4m (its ID is the empty content's) |

In every case `c4 id -l` says the output would change, so it isn't
normaliser-clean. The cause is in `cmd/c4/intersect.go`:

- both loops skip directories, so only files are matched;
- `ensureParentDirs` makes each ancestor up with a null mode, size and time,
  although FULL's own line for that folder is in hand.

## The change, on master

1. **Ancestors come from FULL as written.** Each ancestor of a matched entry
   is FULL's line for that folder, stated mode and timestamp included. That's
   SPECIFICATION.md "Directory Timestamps": "Directory timestamps come from the
   filesystem's modification time for the directory itself." The normaliser
   already keeps a stated directory timestamp, and never derives a mode.
2. **A folder named with none of its contents is matched as one line**
   (question 1): FULL's line for it, with its stated C4 ID and no children
   listed. The normaliser already keeps a childless folder's stated values ("A
   directory listed without children keeps its stated values"), and that ID
   names the whole subtree, as in one-level form. So the whole subtree is
   selected by reference, with no rule added. A folder named together with
   some of its contents is just their ancestor, re-derived to what is kept.
3. **Folders are re-derived** through `c4m.Normalize`, so each kept folder's
   size and C4 ID describe what the result holds. The output is
   normaliser-clean, and its description ID is the sparse set's identity.
4. **The work moves from `cmd/c4` into the c4m package,** as one function the
   verb calls, so a Go caller (the operator) uses the same code as the shell.
   This is a move, not a new concept.
5. **The result is idempotent:** intersecting FULL again with the result's own
   names gives the result back. This is a test, and it's what makes the link
   below checkable.

A name FULL lacks is simply not in an intersection, as today. That's the
verb's meaning when comparing two versions (`design/intersect-command.md`), so
no refusal is added. A caller that needs every name present compares the two
path lists.

## The two Filter functions

`Manifest.FilterByPath` and `FilterByPrefix` (c4m/operations.go) return
matched entries with no ancestors, so the result isn't a valid c4m. The
implementations don't agree on what they match:

| implementation | FilterByPath matches | FilterByPrefix matches |
| --- | --- | --- |
| Go | the bare name | the bare name |
| c4py | the full path | the full path |
| c4ts | the full path | the full path |
| libc4 | the bare name | the full path |

Measured on Go: `FilterByPath("shots/s010/*.exr")` returns nothing, while
`FilterByPath("*.exr")` returns the two files at depth 2 with no parents.

Nothing outside their own packages calls them (searched: c4-operator, c4sh,
c4git, c4d and each port). Their only other users are each port's copy of the
shared range-data vector.

**Recommendation: remove them on master and on each port's main,** replacing
their range-data vector cases with the intersection. Leave the release lines
as they are: removing a public function is not a patch-release change.

## Where the link to the source is kept

The sparse c4m doesn't say what it was taken from, and shouldn't (the sidecar
doctrine). The link is two C4 IDs, joined outside the c4m: FULL and RESULT.
Nothing else needs keeping. Because the intersection is idempotent, RESULT is
its own selection, so the claim "RESULT was taken from FULL" is recomputable
in one command:

    c4 intersect path RESULT.c4m FULL.c4m | c4 id -q -    # equals RESULT's ID

The patterns, or the names, that produced RESULT are the caller's history, not
part of the fact.

**For the operator** (its lead's decision, noted here only so it can be
weighed):
- Its signed resolution record keeps `source`, `include` (patterns) and
  `view` today. With RESULT being the mount spec, `include` is no longer part
  of what is mounted.
- Its own `internal/view/filter.go` says "Filters compose by intersection",
  which is how a project's grant limits a Materialization. Stated directly, a
  grant is a c4m too, and the narrowing is this same `intersect path`.

## What it costs

- **Core:** `intersect path`'s output changes on master. Ancestors become as
  FULL states them instead of null, folders are re-derived, and a folder named
  alone is matched. Anyone diffing that output sees new values, so it's a
  behaviour change for master's next release. The release lines are untouched.
- **Ports:** c4py, c4ts and libc4 have no intersect. c4-swift has
  `intersect(_ manifests:)` (Sources/C4M/Operations.swift), which would follow.
- **Vectors:** one shared case of the intersection replaces the Filter
  functions' range-data cases.

## Not decided here: the maintainer's

1. **A folder named alone:** is it the whole subtree by reference (its line,
   with its ID), as proposed, or only the folder's own line, with nothing
   below it?

   This has an edge, measured on 2026-11-24 with the fix built as this note
   writes it. A is a full scan with an empty folder `x/`, which A records
   with size 0 and the empty listing's C4 ID; B is the same tree with `x/`
   filled. `c4 intersect path A B` returns B's `x/` by reference, at size 258
   with B's filled ID. A names-only set naming `x/` (its ID null) gives the
   same. Two readings, both following the existing null rule:
   - **(a) As this note is written:** any folder named alone takes B's line
     and its subtree by reference, so A's stated-empty `x/` becomes B's filled
     one.
   - **(b) The null rule applied:** a folder line whose C4 ID is null says
     nothing about what is under it (partial knowledge), so B's line and its
     subtree come by reference. A folder line with a stated ID is a statement
     about its listing, so a folder A states as empty stays empty, with B's
     own mode and time, and its size and ID re-derived. No separate "matches
     B's" clause is needed.
2. **A folded range named in part:** a set naming some frames of a range.
   Options:
   - expand the range from its ID list and keep those frames (the 11-13
     parity: a list is handled as a listing is), with no list meaning the
     range stays folded and unmatched;
   - keep the whole range;
   - refuse.
3. **Selecting nothing:** the empty c4m (as today, a valid state, with the
   empty content's ID), or an error? A mount of nothing may be the caller's to
   refuse.
4. **The behaviour change to `intersect path` on master** (above): acceptable
   as the way to get the sparse c4m, or should the corrected form be a
   separate function, leaving intersect's current output alone?
