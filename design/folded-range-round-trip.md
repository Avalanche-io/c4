# Folded ranges round-trip through materialisation

Status: implemented (master: restore; release/1.0.27: patch), 2026-11-20.

## The requirement

The maintainer, 2026-11-20: "We do however NEED to have proper round trip
for folded ranges."

**The acceptance test is the round trip:** `c4 id -S -s tree/`, materialise
that description into an empty folder, `c4 id -S` again: the same
description ID. It holds with the list inline and with the list only in the
store, for a range one and two directory levels down, two ranges in one
folder, a range beside ordinary files, padded and unpadded numbering, frames
with mixed modes and times, and a null-dated range. The shared vector
`c4m/testdata/range-roundtrip-vector/` pins every one of these, and every
implementation runs it.

## The rulings it rests on (2026-11-13, the maintainer)

- "A frame id list is a part of the c4m format, and is the same thing as a
  subdirectory being referenced by it's ID on a parents directory line.
  Everything that you would expect of the creation and retention of that
  subdirectory c4m file you would also expect of the frame id list."
  So a materialiser resolves a range's list as it resolves a directory's
  listing: from the description when it is inline, from the store by the
  range's C4 ID when it is not, verified against that ID.
- "You can't expand a sequence if you don't have a description of it's
  contents, this is the same for any content." With neither an inline list
  nor the list in a source, the range is the ordinary missing-content case.
  The message names the list, which is what is missing ("the ID list of
  NAME (ID)"), and nothing is written.
- "A folded sequence is a different thing then the original listing because
  the folded version loses some metadata by applying the range expression
  metadata to every frame." Every frame is written with the range line's
  mode and timestamp. The detector gives a range its frames' newest time and
  most restrictive mode, so a tree of mixed frames comes back with one time
  and one mode on every frame, and its second scan folds to the same line:
  the description round-trips, the original per-frame metadata does not, by
  design (snapshot without `-S` where it must survive).
- "It is an error to drop the frame IDs in any context." Expansion takes
  every frame's ID from the list, and refuses a list that does not name the
  range's frames (count or parse).

A frame's own size is not carried: it comes from its content. Frames are
created under the ordinary null rules (SPECIFICATION "Materializing null
metadata"): a null timestamp sets the epoch, a null mode takes the platform
default.

## How

`reconcile.ExpandRanges(target, src)` returns the target with every range
replaced by its frames (names from the range expression, content by each
frame's ID), using the library's `SequenceExpander`. `Reconciler.Plan`
calls it with its own content sources, so every materialiser built on the
planner (`c4 restore`, `c4 patch` on 1.0.x, c4sh `cp`) writes frames. A
range it cannot write leaves the plan incomplete and empty
(`Plan.Ranges`), with one of three reasons: no list; a list that does not
name the frames; or a range of symbolic links.

`c4 restore` expands the target itself before its pre-flight checks, so the
frames' content is checked like any file's, and verifies the result folded:
the as-built tree is scanned with sequence detection and must reproduce the
folded description.

## Symbolic-link ranges: held

The detector folds a run of symbolic links (measured: `c4 id -S` of the
vector's `links` tree writes `l.[0001-0003].exr -> t.[0001-0003].txt`). A
folded line records the members' targets as one range expression, or `...`
when they do not share one, not one target per member, and this version
does not write links from it.
Until the maintainer rules on whether links fold, a folded link range is
refused with that reason, and the detector is unchanged.

## Windows

A mode the platform cannot hold (permission bits beyond read-only) is
written as the platform can, as for any file. The round trip is measured
from the platform's own scan of the tree there, which is how the CLI test
runs the vector on Windows.
