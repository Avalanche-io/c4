# A sparse c4m is an intersection

**Status:** proposal, 2026-11-24. Questions 1 and 4 are answered (see "The
ruling" below); 2 and 3 are still open. The change is being built on the
release line, not on master.

**Read this first.** Several sections below were written before the ruling,
and described a change on master. They are kept as the record. Where they
disagree with "The ruling", the ruling wins. In particular, a folder named
alone is no longer taken by reference.

## The ruling (the maintainer, 2026-11-24)

> "I think c4 intersect should do what we'd expect such an operation to do on
> a filesystem comparison. Point is you want a new filesystem that represents
> the intersection, so nececarily you need the correct new system's full
> directory structure. And how directories are represented is already defined
> for a filesystem so that should simply comply to the spec. If a folder
> remains, but it has no files, then the size should be zero not null and not
> the previous size as one might have from a nieve line copy."

What follows from it, as built:
- The result is the c4m of the new filesystem, and every folder above a kept
  entry is present.
- A folder's mode and timestamp are its own, as the second argument states
  them. SPECIFICATION.md "Directory Timestamps": "Directory timestamps come
  from the filesystem's modification time for the directory itself."
- Its size and C4 ID are derived from what the result holds, by the spec's
  rules ("Directory Sizes"; "Computing Directory C4 IDs"). They're never
  copied from the second argument, and never null where the result's
  children determine them.
- A folder that remains with nothing in it has size 0 and the empty listing's
  C4 ID.
- **Question 1, by the release lead's reading of the ruling** (it's been put
  to the maintainer, who can object): a folder named alone does NOT bring the
  second argument's subtree; it remains as an empty folder. To take a subtree,
  the caller names its paths. That's neither reading (a) nor (b) below, and
  it's simpler than both.
- **Question 4:** the change is made in place, on the release line (the
  maintainer: "should simply comply to the spec").
- `intersect id` uses the same builder.
- The test that states the ruling: the output's description ID equals a scan
  of a real folder holding exactly the kept files, with each folder's mode
  and time set as the second argument states them.

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
matched entries with no ancestors, so the result isn't a valid c4m.

**Corrected 2026-11-24.** This section's table said c4py and c4ts both have
FilterByPath and FilterByPrefix, matching the full path. That was wrong; it
came from reading, not running. Measured on each release line:

| implementation | path pattern | prefix |
| --- | --- | --- |
| Go core | `FilterByPath`, the bare name | `FilterByPrefix`, the bare name |
| libc4 | `FilterByPath`, the bare name | `FilterByPrefix`, the full path |
| c4py | `filter(glob or callable)`, the full path, where `*` also matches `/` | none |
| c4ts | `filter(predicate)` only, with no glob | none |
| c4-swift | none | none |

The mains of c4py and c4ts have neither function either.

Measured on Go core release/1.0.27 (the source is a scanned tree:
`top.exr`, `docs/readme.txt`, and `shots/` holding `s010/{a,b}.exr` and
`s020/c.exr`):
- `FilterByPath("shots/s010/*.exr")` returns nothing.
- `FilterByPath("*.exr")` returns `top.exr` at depth 0 and three files at
  depth 2, with no parents. `Manifest.Validate()` accepts it, while
  `NewValidator(true)` refuses its encoded text ("invalid depth jump from 0
  to 2"). Encoded and decoded again, the three files come back at depth 1.
- `FilterByPrefix("shots/")` returns the one line `shots/`.

So the function disagrees with itself (one validator accepts, the other
refuses), its result doesn't survive its own encoder and decoder, and for
one call the implementations give three different answers.

Nothing outside their own packages calls them (searched: c4-operator, c4sh,
c4git, c4d and each port). Their only other users are each port's copy of the
shared range-data vector.

**Recommendation: remove them on master and on each port's main,** replacing
their range-data vector cases with the intersection. Leave the release lines
as they are: removing a public function is not a patch-release change.

**Superseded 2026-11-24.** The master line is to be retired, so a change
made only there isn't a fix. The functions exist on the release lines of
core and libc4 only (table above). What to do with them there is the
maintainer's call, since removing a public function isn't a patch-release
change.

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

1. **Answered 2026-11-24, see "The ruling".** The question as it was asked:
   **A folder named alone:** is it the whole subtree by reference (its line,
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
4. **Answered 2026-11-24, see "The ruling": in place, on the release line.**
   The question as it was asked: **The behaviour change to `intersect path`
   on master** (above): acceptable
   as the way to get the sparse c4m, or should the corrected form be a
   separate function, leaving intersect's current output alone?
