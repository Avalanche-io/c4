# Two IDs: the file ID and the description ID, and how the CLI keeps them apart

Status: design, 2026-11-03, for Joshua. Nothing built.
Follows design/no-self-id.md; precedes the "one ID, any starting point"
page and its conformance test, which this note makes possible to write.

## The two IDs

A c4m file has two C4 IDs, and both are real:

- **The file ID**: the ID of the file's bytes, as saved: nested, aligned,
  a chain, whatever the author wrote. `c4 cat <file ID>` returns that
  exact file. This is what "no file contains its own ID" protects.
- **The description ID**: the ID of what the file describes, which is the
  byte ID of the description's canonical one-level root record, the thing
  the store holds and `c4 cat <description ID>` returns. Every form of the
  same description converges on it: the scan, the flat file, the pretty
  file, the chain, the hand edit after normalising.

They differ by design for every form except the canonical one-level
record itself. Joshua: "It's still going to confuse people and agents
that use the commandline tools if we're not careful."

## The traps, as built on branch no-self-id

| Route                                   | ID today        | Why                                              |
| --------------------------------------- | --------------- | ------------------------------------------------ |
| `c4 id -q notes.c4m`, `c4 notes.c4m`    | description     | a `.c4m` path argument is the normaliser         |
| `cat notes.c4m \| c4`                   | file            | stdin without `-` is bytes                       |
| `cat notes.c4m \| c4 id -q -`           | description     | `-` parses a description                         |
| `c4 id -s -q proj/` with notes.c4m in it | file (entry)    | a file inside a tree is its bytes                |
| `c4 cat <description ID>`               | the root record | not the nested file the user saved               |

The bare `c4 <path>` shortcut is `c4 id <path>` exactly (main.go), so it
has `id`'s rule; it is not a third route. The sharp edge is the first row
against the fourth: the same path means the description as an argument
and the file inside a tree, and the only thing switching the meaning is
where the path appears.

## Evaluation of the proposal

**(1) Two names, used everywhere.** Yes. "File ID" for the bytes and
"description ID" for what a description computes to, in help, errors,
docs, `c4 explain`, and c4-mcp. Never "the ID" of a c4m without saying
which. The spec already distinguishes "the ID of a description is the ID
of its canonical text"; the names make it sayable in one breath.

**(2) stderr names both whenever they differ.** Yes, with one bound. When
`c4 id` reads a `.c4m` *path* whose file ID differs from its description
ID, one stderr line: `notes.c4m: file ID c42Q…, describes c44v…`. Stdout
stays data; nothing is silent; a script author who printed the wrong one
sees the other beside it the first time they run it. Not for stdin: a pipe
has no file to name, and the verb already says which ID it computes (`-`
parses; no `-` is bytes). Not for files inside a scanned tree: their entry
line already carries the file ID and nothing describes them.

**(3) One consistent default, one flag for the other.** The consistent
rule I recommend is by *verb*, not by input route, because it is the
rule the verbs already have and it is one sentence:

> **`c4 id` describes; bare stdin identifies.** `c4 id X` and `c4 X`
> print the description of X and `-q` its description ID: a directory's
> scan, a `.c4m` file's description, a plain file's one-line description
> (whose only ID is its file ID). `... | c4` prints the file ID of
> exactly the bytes piped; `... | c4 id -` parses them as a description.

Under this rule the `.c4m` suffix does switch what `c4 id` does with a
path, and that is Joshua's own decision today (the normaliser is
`c4 id FILE.c4m`). The escape to the other ID is composition, not a flag:
`cat notes.c4m | c4` is the file ID. I recommend **no flag**: a `--bytes`
on `id` would be a second way to say what a pipe already says, and a
`--describe` on the bare form would be a second way to say `-`.

The alternative, for Joshua to weigh: **paths are always bytes, `-` is
always a description.** `c4 id notes.c4m` would print one entry line with
the file ID, like any file, and the normaliser would be
`c4 id - < hand.c4m`, with `-l`/`-d` keeping their path arguments as the
explicit checking forms. It removes the suffix switch entirely, at the
cost of the gofmt-like `c4 id FILE.c4m` he asked for this morning. I do
not recommend it, but it is the only shape with no suffix rule.

**(4) c4-mcp returns both, labelled.** Yes: every c4-mcp result that
names a c4m carries `fileId` and `descriptionId`, never a bare `id`,
and its tool descriptions use the two names. c4-mcp is its own
repository; this is a requirement on it, written here.

## The agent case

An agent writing a script from the docs alone must get the ID it expects.
The doc rule, in the CLI reference, the README's identity section and
`c4 explain id`:

> To get the description ID of anything, `c4 id -q <path>`, or pipe a
> description through `c4 id -q -`. To get the file ID of a file, pipe its
> bytes through `c4`. `c4 id` tells you on stderr when a `.c4m` file's
> two IDs differ. `c4 cat <description ID>` returns the description's
> record; `c4 cat <file ID>` returns the file.

And one more line in `c4 cat`'s narration: when the object it returns is
a description record, stderr says `description record; -r expands it`,
so a user who saved a nested file and kept "its ID" learns which ID they
kept.

## What this does not change

The identities themselves, the chain grammar, the store's records, the
no-self-id rule, and the normaliser. It adds two names, one stderr line
in `id`, one in `cat`, doc text, and a c4-mcp requirement.

## Tests

- `c4 id notes.c4m` (nested, aligned, chain) prints the stderr line with
  both IDs; a canonical one-level file prints none (its two IDs are one).
- `c4 cat <description ID>` narrates that it is a record.
- The "one ID, any starting point" conformance test (next) asserts the
  rule above for every form the CLI writes.

## Joshua's model: identification and storage as distinct steps

Joshua: "Is the problem here that we simply need to make identification and storage distinct steps?" The model, evaluated:

- **Every ID is a file ID.** True, and it is the fact that dissolves the confusion. The description ID is the file ID of the description's canonical one-level root record: `c44voXiE…` is the byte ID of the stored listing. A directory has no bytes of its own, so its identity is its record's; the content projection (`-m c`) and a patch chain's resolved state are records too. Nothing in the system has an ID that is not the ID of some bytes.
- **Three steps.** Canonicalise (a folder or any c4m form in, the canonical root record out: the only step that interprets); identify (bytes in, ID out, always); store (bytes in, the same ID out, never interprets). This holds exactly, with one word made precise: the bytes that canonicalise emits are the **root record**, one level, with each subdirectory line carrying that subdirectory's own record ID. That is what the store holds and what the description ID names. The nested flat text the normaliser prints is a different artifact, the resolved *view* of the description, with its own file ID; it is what `c4 cat -r` expands a record into. The model does not break there; it needs the word.
- **So description ID = identify(canonicalise(x))**, and `c4 id -q x` is sugar for exactly that. A snapshot (`c4 id -s dir`) is canonicalise applied to every directory plus store applied to every file's bytes and every record: many stores, each bytes-only, no interpretation inside store.
- **What goes away.** Any route that decides by extension or by input route. The bare `c4 FILE` shortcut is today `c4 id FILE`, which reads a `.c4m` as a description; under the model bare `c4 FILE` is identify (and with `-s`, store) of the file's bytes, whatever its name, printing one bare file ID, the same as `cat FILE | c4`. Bare `c4 DIR` stays the snapshot shortcut, because a directory has no bytes to identify. The one place a `.c4m` is read as a description is `c4 id`, the verb whose job is to describe, and `c4 id -s x.c4m` is the explicit way to snapshot a description held in a file. This replaces my recommendation under (3) above: the rule is now **`c4 id` describes; `c4` identifies and stores bytes; a directory snapshots.** No suffix switch anywhere but inside the describing verb.

### The gap: the normaliser emits the nested view

`c4 id x | c4 id - | c4` hashes the nested resolved text, so it equals the description ID only for a flat tree. Two ways to close it:

- **A. Canonicalise as a command that emits the root record.** Then identify(canonicalise(x)) is literally a pipe, `c4 canon x | c4`, and store is `| c4 -s`; `c4 id -q x` is documented as that pipe's sugar. Name and shape for Joshua: a verb `c4 canon <dir|file.c4m|->` printing the root record (one level, resolved), or a flag on `id`. A verb says what the step is; a flag hides it in the describing verb again. The record is also what `c4 cat <description ID>` returns from a store, so the output is not a new format, only a new way to get it without storing first.
- **B. Document `| c4 id -q -` as the one pipe** and leave the nested view as `id`'s only output. Cheaper, but then identification of a description is sugar with no composed form, which is the gap Joshua's question points at.

Recommendation: **A**, as a verb, proposed not added. It is the step the model names, it makes the three-step story true at the shell, and it costs one verb with one output that already exists as a stored object.

> **Withdrawn 2026-11-03, same day (Joshua's direction, below):** the record is a store artefact that people rarely handle as a file, and `c4 cat <record ID>` already writes it when they must. No `c4 canon`.

### Retrieval under the model

A user who stored a nested file (`c4 -s notes.c4m`, or the file inside a snapshotted tree) gets that exact file back with `c4 cat <file ID>`. A user who snapshotted a description (`c4 id -s dir`, `c4 id -s notes.c4m`) gets the root record with `c4 cat <description ID>` and the nested view with `c4 cat -r <description ID>`. Each ID returns the bytes it names, and the stderr line in `c4 id` tells a user holding a `.c4m` file which of its two IDs they are looking at.

## Revision: the record is a store artefact (Joshua's direction, 2026-11-03)

Joshua: "maybe we need an explicit file creation step that people just quickly learn not to use. The idea is you'd only use the compiled c4m format in a store most likely."

**The model, restated with that.** The canonical one-level root record is the store's form of a description. Its byte ID is the description ID. People normally never hold it as a file: the store does. Every human-facing c4m file (the nested resolved listing, the aligned `-e` form, a scan's chain, a `c4 diff` chain, a hand edit) is a *view* of a record, with a file ID of its own. The three steps stand: `c4 id` compiles whatever it is given to the record and reports the record's ID (`-q`), and with `-s` puts the record, every directory's record and every file's bytes in the store; bare `c4` identifies, and with `-s` stores, bytes, whatever their name; a directory given to bare `c4` snapshots, since it has no bytes.

**The explicit file-creation step already exists.** `c4 cat <description ID> > record.c4m` writes the root record from a store, byte for byte, and `c4 cat -r` writes its expansion. It needs the description to be stored first, which is the point: the record is a store artefact, and a user who wants it as a file has stored it. No `c4 canon`; the proposal above is withdrawn. The one case a store-less canonicalise would serve, comparing record bytes across implementations, is served by a temporary store in the conformance test, not by a verb.

**One term everywhere.** Two candidates: *compiled* (Joshua's word: it says you do not write it by hand) and *record* (the word the store, `c4 cat -r`, the spec and the code already use for exactly this object). Recommendation: **record**, introduced once as "the record is the compiled c4m: the store's form of a description, one level, never written by hand", then *record* everywhere: help, errors, docs, `c4 explain`, c4-mcp. Two names for one object is the confusion we are removing; *record* is the one already in the codebase.

**The two names, final.** *File ID*: the ID of a file's bytes. *Record ID* (the description ID): the ID of the record a description compiles to. The spec's "the ID of a description is the ID of its canonical text" becomes "is its record's ID".

**The stderr line.** When `c4 id` reads a `.c4m` path whose file ID differs from its record ID, one line on stderr:

    notes.c4m: file ID c42Q…; its record is c44v…

A canonical one-level file prints nothing: its file ID is its record ID. Stdin and files inside a scanned tree print nothing (the verb, or the entry line, already says which ID).

**How the "one ID, any starting point" page then reads.** One rule at the top: *`c4 id -q <anything>` is the record ID; `c4 <file>` is the file ID.* Then the table, one row per starting form, each with the command for the record ID and, for file forms, the command for the file ID and what `c4 cat` of each returns:

| Starting form | Record ID | File ID | `c4 cat` of the record ID returns |
| --- | --- | --- | --- |
| a directory on disk | `c4 id -q DIR` | (none: no bytes) | the root record |
| a stored snapshot | the ID itself | the ID itself | the root record; `-r` the whole tree |
| the record as a file (from `c4 cat ID > f`) | `c4 id -q f` = `c4 f` | `c4 f` | the same bytes |
| a nested or aligned c4m file | `c4 id -q f` | `c4 f` | the root record, not this file |
| a scan's chain or a `c4 diff` chain | `c4 id -q f` | `c4 f` | the record of the resolved state |
| a hand-edited c4m | `c4 id -q f` (normalised first) | `c4 f` | the record of what it describes |
| a c4m on stdin | `… \| c4 id -q -` | `… \| c4` | the record |
| a c4m inside a scanned tree | `c4 id -q path` | its entry's ID | the record |
| an S3 store listing | `c4 cat ID` with the store configured | the ID itself | the root record |

Every row's record ID is the same ID for the same description; the file IDs differ by design and each summons its own file. The conformance test asserts exactly that table, form by form, with every form made by the CLI.

**The agent sentence.** "To name a description, use `c4 id -q`; to name a file, use `c4 FILE`. If you hold a `.c4m` file, `c4 id` tells you both on stderr. `c4 cat` returns what the ID you give it names: the record for a record ID, the file for a file ID."
