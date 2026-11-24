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
  one is never replaced.
- "Computing Directory C4 IDs": the C4 ID of the canonical one-level
  listing.

## The change

- **One derivation.** `rederiveDirectory` (`cmd/c4/derive.go`) re-derives a
  folder from its listed children. It runs PropagateMetadata over the
  subtree, as the store pass always has. The C4 ID is then the ID of the
  one-level listing (`directoryRecord`), and the size is the children's
  sizes plus that listing's bytes, null when a child's size is null. It's
  the store pass's existing derivation, moved out of `rederiveAbove`, which
  now calls it. Nothing new is exported from the c4m package.
- **The tree walk** (`normaliseManifest`, `cmd/c4/id_normalize.go`) runs it
  on every folder whose children are listed, deepest first:
  - a null size or C4 ID the children determine is filled;
  - a stale size or C4 ID is corrected;
  - a stated timestamp is kept;
  - a null timestamp stays null;
  - a folder listed without children keeps its stated values.
- **No folder time from contents.** The maintainer, 2026-11-24: "the mod
  time should NEVER be based on contents, that has bad sideffects."
  - PropagateMetadata fills a null folder time from the newest child, so
    the normaliser calls the derivation with `keepNullTimes`. Every folder
    in the span whose time was null is set null again before the listing
    is read.
  - Restoring the null only after the walk wouldn't be enough: each
    parent's size and C4 ID would have been computed over a child line
    carrying the filled time.
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

## Unchanged, pending the maintainer's order

- **The store pass (`rederiveAbove`, `c4 id -s`) keeps its released time
  handling** by calling the derivation with `fillNullTimes`. Above a file
  it gives up, it re-derives the folder's size (null when the file's size
  is) and fills a null folder time from the newest child, as on 06f5f8f.
- **The scanner does the same fill.** A folder whose own mtime is the epoch,
  which reads as null, is written with its newest child's time. Measured on
  06f5f8f: the scanner runs PropagateMetadata on each folder.
- Both are times derived from contents, which the ruling above rules out,
  but they're released writers. Changing them is the maintainer's to order.
  `TestStorePassFolderTimeIsAsReleased` pins them as they are.
