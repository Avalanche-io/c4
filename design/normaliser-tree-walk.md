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
  line. They check against the canonical form (one normal form, as gofmt
  has), so a file in the pretty (`-e`) form is listed.

## The two edge cases

**A stated size over a child with a null size is re-derived to null,** and
the replacement is reported on stderr like any other (`size 117 -> -`). The
maintainer ruled it on 2026-11-03 (15:03:45Z), answering the question "a
directory line states size 117, but its listed children have null sizes
(a structure-only listing), so re-deriving gives null. Should the
normaliser replace the stated 117 with `-`?" with "Re-derive to null
(Recommended)", the option the question, as framed, marked recommended.
In the same dialog, on a gofmt-style `-w`: "No -w (Recommended)". So there
is no `-w`; the normaliser writes to stdout and never rewrites a file.

Reading A, a stated folder timestamp kept, was also a dialog answer
(2026-11-04, 01:20:57Z): "A: own mtime (Recommended)".

**A folder over a child with a null C4 ID** is the one edge still open. The
derivation computes it; the normaliser keeps the stated C4 ID, as
`c4 id FILE.c4m` always has. Its size
  still follows "Directory Sizes": the listing holds `-` where the ID was,
  so the size is re-derived from that listing.

The store pass is unchanged. Above a file it gives up, it re-derives the
folder's size, and that size is null when the file's size is.
