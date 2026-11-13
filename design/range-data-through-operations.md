# A frame ID list is handled the way a subdirectory listing is

Status: fixed on branch `range-data` (decided 2026-11-13 by the release
lead; reversible). Shared fixture: `c4m/testdata/range-data-vector/`.

## The rulings

The maintainer, 2026-11-13:

> You can't expand a sequence if you don't have a description of it's
> contents, this is the same for any content. However, it is an error to
> drop the frame IDs in any context. Just like it would be an error to
> create a c4m tree and drop the subdirectories. No tool should make this
> easy to do.

and, the same day:

> A frame id list is a part of the c4m format, and is the same thing as a
> subdirectory being referenced by it's ID on a parents directory line.
> Everything that you would expect of the creation and retention of that
> subdirectory c4m file you would also expect of the frame id list.

The list is not literally a listing. It is "a linked parseable text file
that has formatting specific to frame ranges, which doesn't need the per
file metadata": the spec's bare concatenation of member C4 IDs
(SPECIFICATION.md, "Inline Range Data"). **No list format changes.** What
a frame ID list shares with a subdirectory listing is its handling: it is
linked by ID from its parent line, and it is created, stored and carried
the way a listing is.

## The rule: parity with subdirectory listings

- **One-level form.** A folded range line whose list is not inline is
  valid, exactly like a directory line that carries its ID with its
  children not listed. The encoder writes it as is; there is no refusal
  and no flag. A file an older c4 wrote without its lists stays valid and
  processable.
- **Creation.** Folding makes the list, as a scan makes a subdirectory's
  listing. `c4 id -S DIR` writes each range's list inline, as nested
  listings are written inline, with or without `-s`.
- **Retention in storage.** Wherever a listing is stored, the list is
  stored beside it, as an object under its own C4 ID, which is the range's
  ID (SPECIFICATION.md: "With store: ID lists are stored as regular
  objects"). `storeManifestSelf`, which every `-s` path goes through
  (`c4 id -s FILE.c4m`, `c4 id -s -`, `c4 id -S -s DIR`, restore's
  pre-image), stores the lists of the ranges it holds. `c4 id -s FILE.c4m`
  keeps them inline in its output too.
- **Retention through operations.** Wherever an operation carries a
  subdirectory's entries, it carries the range's list. An output carries
  the ID list of every range it contains and none for a range it does
  not; a list the inputs lack stays absent, never invented. A patch
  section carries the lists of the ranges it adds or changes, after its
  entries; a removal line takes none. Dropping a list for a range the
  output does not hold discards nothing it describes, and is allowed.
- **Expansion** is descent: a range is expanded only from its list, as a
  directory is descended into only from its listing (see below).

One helper, `rangeDataFor` / `keepRangeData` (`c4m/rangedata.go`), states
the carry, and every library operation that builds a fresh manifest
calls it: Merge, PatchDiff, ResolvePatchChain and FoldSection, ApplyPatch
and ApplyPatchFrom, the base resolver, FilterByPath and FilterByPrefix,
ExtractSubtree and InjectSubtree, and the sequence detector. The scanner's
`-S` fold keeps the detector's lists, and a content projection keeps the
lists it is given. `PatchSection` gains `RangeData`, the lists written in
that section. `WriteChainBody` writes the lists of the ranges it holds
after the base section's entries, before the checkpoint, where the spec
places them. The encoder is unchanged: it writes the lists a manifest
carries.

`TestRangeListParityWithSubdirectories` (library) and
`TestRangeListParityCLI` (merge, diff, patch, split, `id`, `id -s`,
`id -S`) run a tree holding a subdirectory and a tree holding a folded
range through each operation side by side; both keep their content by ID
the same way.

## Expansion: no list, no expansion

The maintainer, 2026-11-13: "You can't expand a sequence if you don't
have a description of it's contents, this is the same for any content."

- `SequenceExpander` finds a range's list where content is found: inline
  in the manifest, or in the source given with `SetSource` (a
  `store.Source`, as `ResolveBase` takes). A list read from the source
  must hash to the range's C4 ID and be a bare concatenation of C4 IDs;
  anything else is not available.
- A range whose list is not available stays one folded entry, with no
  frames, as a directory whose listing is not available stays one line.
  No frame is given a null C4 ID, and none the range's own C4 ID, which
  names the list and no frame. Through 2026-11-13 every frame took the
  range's own ID; that fallback is deleted. A range with no C4 ID names
  no list and stays folded.
- `ExpandRange(m, entry)` expands one range. When its list is not
  available it returns what any unavailable content returns: an
  `*os.PathError` naming the list's C4 ID, with
  `errors.Is(err, os.ErrNotExist)`, the form a store's `Open` gives.
- The expansion carries the list of every range it keeps, including one
  read from the source.
- **Frame metadata, beside directory descent.** Descending a directory
  in one-level form gives each child the size, mode and timestamp its
  listing states, because a listing carries per-file metadata (measured:
  three 7-byte files come back with size 7). A frame list carries none:
  it "doesn't need the per file metadata". So a frame takes its C4 ID from
  the list, its mode and timestamp from the range line, which the fold
  applied to every frame, and a null size (-1), because the range states
  only the total. Both descents give exactly what the linked file and
  the parent line state, and invent nothing.

## The fault, measured

Measured on master 8a52087 (2026-11-13), with HOME, USERPROFILE and
C4_STORE set to a fresh directory. The 1.0.x column is the 1.0.27 release
binary (its `c4 version` prints 1.0.25) and `origin/release/1.0.x`
ac75240, which gave the same results.

| Function or verb                           | 1.0.x                     | master (before)                        | After              |
| ------------------------------------------ | ------------------------- | -------------------------------------- | ------------------ |
| `c4 id FILE.c4m`                           | keeps                     | keeps                                  | keeps              |
| `c4 merge` / `Merge`                       | drops every list          | drops every list                       | keeps              |
| `c4 diff` → chain → `c4 patch`             | drops every list          | drops every list                       | keeps              |
| `c4 patch FILE.c4m` (plain)                | drops every list          | drops every list                       | keeps              |
| `c4 split`                                 | not measured              | drops every list                       | keeps              |
| `c4 id -S DIR` (no `-s`)                   | no list, inline or stored | no list, inline or stored              | inline             |
| `c4 id -S -s DIR`                          | stored, none inline       | stored, none inline                    | stored and inline  |
| `c4 id -s FILE.c4m`                        | not measured              | lists not stored (2 objects)           | stored (4 objects) |
| `DecodePatchChain`                         |                           | skips list lines                       | keeps, per section |
| `ResolvePatchChain` (even one section)     |                           | drops                                  | keeps              |
| `FoldSection`                              |                           | drops                                  | keeps              |
| `ApplyPatch`, `ApplyPatchFrom`             |                           | keep, and keep removed ranges' lists   | keeps              |
| `ResolveBase`, `DecodeResolved`            |                           | keep, and keep removed ranges' lists   | keeps              |
| `Decoder.Decode` (list in a patch section) |                           | keeps, and keeps removed ranges' lists | keeps              |
| `FilterByPath`, `FilterByPrefix`           |                           | drop                                   | keeps              |
| `ExtractSubtree`, `InjectSubtree`          |                           | drop                                   | keeps              |
| `DetectSequences` on ranges already folded |                           | drops their lists                      | keeps              |
| `WriteChainBody`                           |                           | drops                                  | keeps              |

## Reach

Byte IDs change for every output that holds a folded range whose list an
input carried: the output now has the lines it lost. `c4 id -S DIR` now
writes its lists inline. Description IDs do not change; the lists are
never part of one. A c4m that arrived without its lists (an older c4's
merge, diff or patch output) stays valid: its range lines are in
one-level form, and a rescan regenerates the lists where the frames still
exist.

## Later refinement

The spec's lifecycle says "With store: ID lists are stored as regular
objects. No inline lines in the c4m." Verbs that write c4m from c4m keep
the lists inline even with a store configured, and `-s` writes them
inline as well as storing them; inlining is safe, as nested listings are
written inline. Dropping the inline lines when a store holds them is a
possible later refinement.
