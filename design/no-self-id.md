# No file contains its own ID: the stream's closing line, the claim, and c4m-aware hashing

Status: design, 2026-11-03, for Joshua's review. Nothing
built. Supersedes draft-v9 rule 4 and part of its section 3 (see §5).

## The ruling

Joshua, 2026-11-03: "No file can contain its own ID." And: "One MUST be
able to use a file's c4 id to summon that file; if the id of the file is
appended to the file, that now is a second file with the first file's id
at the end. These do not close the loop."

Measured on main (scratch HOME `/tmp/c4rt.zHMz`): a description's
ID `c44voXiE…` is the byte ID of its stored canonical record, so it is a
byte ID like any other; the chain file `c4 id dir` writes, which ends with
that line, is `c42Q2h7j…` as bytes. Appending the ID made a different file,
and that file's own ID is what `c4 cat` would need to return it.

The rule that follows: **a c4m file never ends with a bare ID of its own
resolved state. Bare IDs appear only as boundaries, naming the state of
the sections before them, where a patch follows.** The spec's own chain
grammar already reads this way (Patch Format, "Multiple Patches": the
example stream ends on patch-2's entries); only draft-v9 added the closing
line.

A second, related finding while checking the first: in five places the
CLI hashes or stores a *re-rendered* form of c4m text rather than its
bytes, so a c4m file's ID is not the ID of the file:

| Site                          | What it does today                                                    |
| ----------------------------- | --------------------------------------------------------------------- |
| `c4 id < file` (`doStdin`)    | detects c4m, canonicalises, hashes the canonical text                 |
| `c4 id -s < file`             | same, and stores the canonical text                                   |
| `identifyC4mFile`             | a `.c4m` path, or bytes that look like c4m, hashed canonical          |
| `storeC4mAware`               | same, stored canonical                                                |
| `storeFileEntry` (tree scan)  | a `.c4m` file *inside a scanned tree* is stored as its canonical text |

The last is the sharpest: `c4 id -s project/` records `notes.c4m` with an
entry ID that `c4 cat` answers with a different byte sequence than the
file on disk, and `c4 restore` then writes that other sequence back. The
same ruling retires all five: **identify means the bytes.**

## 1. Where the claim and the root ID go

- **`c4 id DIR` stdout is the chain body and nothing after it**: the base
  listing, then, only when a directory needed refinement, one boundary ID
  (the base state) and the refinement patch, ending on the patch's last
  entry. A flat tree is the base alone, with no bare ID at all.
  `c4 id dir > x.c4m` is then a file whose own byte ID summons it, and
  whose resolved description ID any reader computes.
- **The description's ID is composition**: `c4 id DIR | c4 id -q -` (Joshua,
  2026-11-03); `-q` is the convenience that prints the same ID directly.
  `c4 id DIR | c4` is a different ID: that of exactly the bytes piped,
  which summons that exact text. Both are documented in id(1) and pinned
  by the round-trip tests. `tail -1` is retired as a contract; the help
  text that promises it goes.
- **The `-s` claim.** Printed claims are exactly the `-q` lines, printed
  only after the store barrier and the journal append: "a printed claim
  never dangles" survives with one fewer kind of claim. Without `-q`,
  `c4 id -s DIR` streams the body (description, not a claim; it may reach
  the terminal before the barrier, as today) and narrates `stored: <ID>`
  on stderr after the barrier, as it already does. stdout is data, stderr
  is narration; the ID a script wants is `-q`. The journal (`c4 log`)
  holds every claim either way.
- **`c4 id FILE.c4m` and `c4 id -`** already end on the last entry
  (branch `normalizer`, 272360c).

## 2. What replaces the closing validator

Nothing replaces it in the file. The boundary before the refinement patch
still verifies the base; the final patch is verified by what it names.

What goes undetected at load time, and whether it matters:

- **Truncation of the final patch** (a killed stream). The reader resolves
  the lines it has: a description with some directory aggregates still
  null. That is a valid partial description ("partial data is data"), and
  the resolved ID differs from the complete one, so nothing false is
  asserted; what is lost is the ability to tell "killed" from "complete"
  from the file alone. Under `-s` the journal has the complete ID, and the
  complete record is in the store under it.
- **Corruption inside the final patch** (a flipped byte in a restated
  directory line). A wrong directory ID is caught when the record is
  fetched by it; a wrong size is caught by the normaliser, which
  re-derives every listed directory and reports the disagreement, and by
  `c4 id -l` in a hook. A wrong byte in the base section is caught by the
  boundary ID as today.
- **Corruption of the whole file** is covered by the file's own byte ID
  wherever the file travels by ID, which is the point of the ruling.

Content is verified by ID at use; the closing line only moved one class of
check from use time to load time, at the cost of making every file a
different file from the one its ID names. Accept.

## 3. Identify means the bytes

- `c4 id < file` and `c4 id -s < file` hash and store stdin's bytes. No
  c4m detection. `c4 id -` keeps parsing stdin as a description (that is
  what `-` says).
- A `.c4m` path argument to `c4 id` is the normaliser (a description), by
  Joshua's decision today; the bytes identity of a c4m file is
  `c4 id < f.c4m`. Any other path argument, and every file inside a
  scanned tree, `.c4m` or not, is its bytes: `identifyC4mFile`,
  `storeC4mAware`, `storeFileEntry` lose their canonicalising branch and
  `looksLikeC4m` has no caller left (delete it).
- Consequence worth stating: a project's own `.c4m` files inside its tree
  get byte IDs, restore writes them back byte for byte, and two
  differently-formatted files with the same resolved description are two
  files. That is the ruling.

## 4. Release notes (1.1) and the design record

Release-notes paragraph:

> **A c4m file never ends with its own ID.** `c4 id DIR` no longer closes
> its output with a bare root-ID line, and neither do `c4 id FILE.c4m`
> and `c4 id -`; a chain ends on its last entry, and bare IDs appear only
> as boundaries before a patch. A file written from `c4 id` is therefore
> the file its own C4 ID names: `c4 cat` of that ID returns it byte for
> byte. The description's ID is `c4 id -q`, the one scripted form;
> `tail -1` is no longer a contract. Under `-s` without `-q`, the stored ID
> is narrated on stderr as before and recorded in `c4 log`. Identifying or
> storing stdin, and every file inside a scanned tree, now means its
> bytes: a `.c4m` file is no longer re-rendered before hashing. The chain
> grammar and every stored record are unchanged; only what is printed
> after the chain, and how c4m-looking bytes are hashed, change.

Design record: `design/snapshot-loop/round-3/draft-v9.md` gets a dated
correction block under rule 4 and under section 3, in place, stating the
ruling and pointing here; the original text stays visible.

Spec: `c4m/SPECIFICATION.md` Patch Format gains one sentence after
"Multiple Patches": a chain ends on its last section's entries and never
on a bare ID of its own resolved state. The decoder is unchanged by
this; a reader that meets a bare ID at end of input treats it as today
(an empty patch section is an error), which is also what 1.0.x does, so
the 1.0.21 skew closes with no backport.

## 5. Tests

- For every form the CLI writes to a file (`c4 id DIR > f`,
  `c4 id -e DIR > f`, `c4 id FILE.c4m > g`, `c4 id - < f > g`), store the
  file's bytes with `c4 id -s < f` and check `c4 cat <that ID>` returns
  exactly the file.
- A `.c4m` file inside a scanned tree: its entry ID equals the ID of its
  bytes, and `c4 restore` writes it back byte for byte.
- `c4 id DIR` output never ends with a bare ID; a flat tree yields no
  bare ID at all; a tree with a refined directory yields exactly one,
  before the patch.
- `c4 id -s -q DIR` prints one ID, after the barrier, equal to
  `c4 id -q DIR`; `c4 id -s DIR` prints the body on stdout and
  `stored:` on stderr, nothing after the body.
- The help text no longer promises `tail -1`.

## Cost

Removes a concept (the closing line) and a branch in five functions
(`looksLikeC4m` and `canonicalizeC4mBytes` callers), adds no flag.
Other implementations: the chain grammar is unchanged; any that print a
closing ID in their own CLIs drop it, and the conformance vectors gain
the "file written by the CLI is the file its ID names" case.
