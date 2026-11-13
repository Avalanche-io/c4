# A folded range's size counts its ID list

Status: implemented on branch `sequence-size-includes-list` (decided
2026-11-13 by the release lead; reversible). Shared fixture:
`c4m/testdata/sequence-size-vector/`.

## The rulings

The maintainer, 2026-11-13:

> A folded sequence is a different thing then the original listing because
> the folded version loses some metadata by applying the range expression
> metadata to every frame. This is like a folder in which the files in it
> are renamed, it is indeed a different folder.

and:

> the file id list is just a variation on the c4m file format specific to
> folded sequences. People have a choice to use folding or not if they
> need to retain per item metadata. The size is the size of every frame and
> the frame id list. Note that the frame id list has a compleately
> deducable size based on the item count alone since c4 id's have a fixed
> length. So one can easily figure out what the actual file data size is as
> well by subtracting the deduced file id list size.

## The rule

- **A range's size** is the sum of its frames' sizes plus the byte length
  of its ID list, count × 90: the bare concatenation of member C4 IDs, no
  newline. A null frame size still makes the range's size null. The
  frames' own total is the size less count × 90.
- **A directory holding a range** follows the existing rule
  (`directory-size-includes-c4m.md`) with the range as one child: its
  children's sizes plus the byte length of its listing as written, the
  folded one. That listing is also what its C4 ID hashes; this was already
  the behaviour, and the spec now states it.
- **Every path agrees.** `SequenceDetector` writes the range size; the
  scanner's `-S` walk sizes each directory from the folded children it
  hashes (`listedChildren`), where through 2026-11-13 it sized from the
  unfolded children; `PropagateMetadata` and the normaliser take the range
  as one child. A `-S` scan is now normal.

## Measured

Three 11-byte frames in `sub/`, on this branch's parent (2608be9) and on
this branch:

| Path                     | Before: range | Before: `sub/` | After: range | After: `sub/` |
| ------------------------ | ------------- | -------------- | ------------ | ------------- |
| `c4 id -S DIR` (scanner) | 33            | 441            | 303          | 446           |
| normaliser (`c4 id`)     | 33            | 175            | 303          | 446           |

The folded listing grows from 142 to 143 bytes, because "303" is one digit
longer than "33".

## Reach

Range sizes change, and so do the size and C4 ID of every directory above
a range, up to the root. Description IDs of trees without folded ranges
do not change.

`SequenceDetector` shares the entries it does not fold with its input, so
normalising a fold also rewrites those entries in the input manifest
(found writing the vector). That predates this change and is not changed
here.
