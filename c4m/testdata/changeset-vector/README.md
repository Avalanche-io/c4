# Cross-implementation vector: a changeset, as `c4 diff` writes it (2026-11-16)

The shared conformance fixture for patch writers and chain readers
(SPECIFICATION.md, "Patch Format"). A changeset is the old state's C4 ID
on its own line, then the patch's entries in canonical order, then the ID
lists of any ranges it adds or changes. Nothing follows: a c4m file never
ends with a bare C4 ID. Ruled 2026-11-08 by the maintainer: "A c4 id at
the end of a file is no different than a generic data file that happens
to contain c4 ids... If we are talking about a c4id at the end of a c4m
file, then this is an error in format." Appending a changeset to the c4m
of its old state (`c4 diff >>`) makes a chain that resolves to the new
state.

Measured 2026-11-16 on the suite's 1.0.27 lines with these files: libc4's
`EncodePatch` still ended every changeset with the new state's bare C4
ID; libc4's `Manifest::Parse` concatenated a chain's sections instead of
resolving them; and c4py's `patch_diff` wrote a directory before its
sibling files. The patch section is canonical c4m text: files before
directories, then natural sort within each group.

## Files

| File               | Made by                                                                                                 |
| ------------------ | ------------------------------------------------------------------------------------------------------- |
| `base.c4m`         | the Go encoder on a tree built through the API: `top.txt`, `sub/` holding two files and `deep/leaf.txt` |
| `v2.c4m`           | the same tree with `sub/deep/leaf.txt` changed, `sub/gone.txt` removed and `sub/new.txt` added          |
| `changeset.c4m`    | `PatchDiff(base, v2)` written as `c4 diff` writes it: the old state's ID, then the patch                |
| `chain.c4m`        | `base.c4m` then `changeset.c4m`, byte for byte (`c4 diff >>`)                                           |
| `v3.c4m`           | `v2.c4m` with the directory `sub/deep/`, and the file in it, removed                                    |
| `changeset-v3.c4m` | `PatchDiff(v2, v3)` written as `c4 diff` writes it                                                      |
| `chain-v3.c4m`     | `v2.c4m` then `changeset-v3.c4m`, byte for byte                                                         |

No line is composed by hand. `C4M_UPDATE_VECTORS=1 go test ./c4m -run
TestChangesetVector` rewrites the files; the test then fails if they
differ from what the library writes. The files are byte-identical to what
core's CLI writes for the same tree on disk (`c4 id` on each state and
`c4 diff base.c4m v2.c4m`, release/1.0.27 eb33848).

## What a conforming implementation MUST do

| Operation                                         | Output                            |
| ------------------------------------------------- | --------------------------------- |
| every patch writer, given `base.c4m` and `v2.c4m` | `changeset.c4m`, byte for byte    |
| decode `chain.c4m` and encode canonically         | `v2.c4m`, byte for byte           |
| apply `changeset.c4m`'s patch to `base.c4m`       | `v2.c4m`, byte for byte           |
| every patch writer, given `v2.c4m` and `v3.c4m`   | `changeset-v3.c4m`, byte for byte |
| decode `chain-v3.c4m` and encode canonically      | `v3.c4m`, byte for byte           |
| apply `changeset-v3.c4m`'s patch to `v2.c4m`      | `v3.c4m`, byte for byte           |

In particular:

- The changeset's last line is an entry, never a bare C4 ID.
- Within `sub/`, the patch lists `gone.txt` (a removal: restated exactly)
  and `new.txt` (an addition) before the directory `deep/`, files before
  directories, as every canonical listing does. Its first line is the C4
  ID of `base.c4m`'s description.
- A chain's sections are resolved, never concatenated: `chain.c4m` holds
  no `gone.txt` and one `leaf.txt`.
- A removed directory is its own line alone, restated exactly, with
  nothing under it: `changeset-v3.c4m` restates `deep/` and not
  `leaf.txt`. A directory restated with children under it is deepened,
  never removed (SPECIFICATION.md, "Patch Entry Semantics"), so a writer
  that restates the removed directory's children too writes a patch that
  removes only the children and keeps `deep/`. Added 2026-11-16: c4ts's
  `patchDiff` wrote that shape.

A port with no patch writer has no such operation for the first row; it
still runs the other two. Go runs every row in `TestChangesetVector`.

## Round trip

Each file decoded and then encoded canonically, comparing SHA-512 of the
bytes. On master Go's `TestVectorRoundTrip` reads this table; on the 1.0.x
line, which has no such test, `TestChangesetVector` checks the same rows.
Every port runs the same check:

- `itself`: the output is the file, byte for byte.
- A file name: the output is that file, byte for byte.
- `itself without its base line`: the output is the file with its first
  line, the base C4 ID, removed, and otherwise byte for byte.

On the 1.0.x line a decode keeps a changeset's first-line base C4 ID, and
an encode drops it; writing it back is c4 #145, on the 1.1 line only. So
on 1.0.x each changeset re-encodes to its patch entries alone, and only
`c4 diff` writes the base line, printing the old state's C4 ID before the
encoded patch. Measured 2026-11-16 on core release/1.0.27 9175b56 and on
c4ts release/1.0.27: `changeset.c4m` decoded and encoded starts with
`sub/`'s line. On master both changesets re-encode to themselves.

| File               | Re-encodes to                |
| ------------------ | ---------------------------- |
| `base.c4m`         | itself                       |
| `v2.c4m`           | itself                       |
| `changeset.c4m`    | itself without its base line |
| `chain.c4m`        | `v2.c4m`                     |
| `v3.c4m`           | itself                       |
| `changeset-v3.c4m` | itself without its base line |
| `chain-v3.c4m`     | `v3.c4m`                     |
