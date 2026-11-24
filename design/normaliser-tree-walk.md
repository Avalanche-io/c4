# `c4 id FILE.c4m` does the tree walk (release line, 2026-11-24)

## What was asked

The maintainer, 2026-11-03: "Isn't there a way to use the cli to get a hand
edtied c4m file to be cononical? Like a gofmt inspired tool?", then "Yes
unless the c4 lead comes up with an issue we should have the normalizer do
the tree walk fixes as well." The same day he ruled reading A of
`directory-timestamp.md`: a stated directory timestamp is the directory's
own mtime and is kept.

## What the line did (measured on 06f5f8f)

`c4 id FILE.c4m` decoded and re-encoded, so it fixed form only. A folder
with a null size, timestamp or C4 ID, or a stale size or C4 ID, built
through the c4m package from a real scan, came out exactly as it went in.

## The rule (SPECIFICATION.md on this line)

- "Directory Sizes": the recursive sum of file sizes plus the byte length
  of the canonical one-level listing of the directory and of each
  subdirectory in it; a null child makes the size null.
- "Directory Timestamps": the directory's own modification time. A stated
  one is never replaced; a null one is filled from the children.
- "Computing Directory C4 IDs": the C4 ID of the canonical one-level
  listing.

## The change

- **One derivation.** `rederiveDirectory` (`cmd/c4/derive.go`) re-derives a
  folder from its listed children: PropagateMetadata over its subtree, then
  `directoryRecord` for the ID. It's the store pass's existing derivation,
  moved out of `rederiveAbove`, which now calls it. Nothing new is exported
  from the c4m package.
- **The tree walk** (`normaliseManifest`, `cmd/c4/id_normalize.go`) runs it
  on every folder whose children are listed, deepest first:
  - a null value the children determine is filled;
  - a stale size or C4 ID is corrected;
  - a stated timestamp is kept;
  - a folder listed without children keeps its stated values.
- **Each stated value replaced is named on stderr** (`normalized: FILE: PATH:
  field OLD -> NEW`). Stdout stays the c4m. Filled nulls aren't narrated.
- **`-l` and `-d`** check, gofmt-style: they list the files whose bytes
  differ from their normalised text, or show the diff, and exit 1 when any
  would change. Files are never written. These are new flags on a patch
  line.

## Left as the line had it (not ruled)

The derivation computes both of these cases; the normaliser keeps what the
c4m states, as `c4 id FILE.c4m` always has:

- **A stated size over a child with a null size** isn't re-derived to null.
- **A folder over a child with a null C4 ID** keeps its stated C4 ID. Its size
  still follows "Directory Sizes": the listing holds `-` where the ID was,
  so the size is re-derived from that listing.

The store pass is unchanged. Above a file it gives up, it re-derives the
folder's size, and that size is null when the file's size is.
