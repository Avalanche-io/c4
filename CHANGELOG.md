# Changelog

## Unreleased — the snapshot loop (BREAKING)

### `cmd/c4/internal/scan` is removed

It was the CLI's scanner at v1.0. On 2026-03-19 every verb moved to the
public `scan` package (616e211) and this one was left behind: nothing has
imported it since, and `go list -deps ./cmd/c4` does not include it. Its
later fixes (a cancel panic, four data races, test deflakes, a
propagation pass) therefore changed no CLI behaviour; their entries
below are corrected to say so. The one capability it had that the CLI
does not offer is status on SIGINFO and SIGUSR1; that would be an
addition to `scan`, not a reason to keep a second scanner. No CLI
behaviour changes. (Decided 2026-11-13 by the release lead, under the maintainer's standing
instruction to decide; reversible.)

### The decoder reads a collapsed directory's listing before a chain deepens it

A self-contained chain whose first section lists a directory as one
collapsed line, and whose next section deepens it, was folded without that
directory's listing: the directory read as empty, and the normaliser
re-derived its ID from the partial listing, so `c4 id -q FILE.c4m` named a
tree that was never there (measured 2026-11-13). The decoder now folds such
a section through `c4m.Deepen`, the fold `ResolveBase`, `c4 patch` and
`c4 log` share: `Decoder.SetSource` and `DecodeResolved` supply the
listing; without a source the chain is refused with `ErrUnresolvedBase`
naming it. `c4 id` never reads the store, so it refuses such a chain
rather than guess; `c4 patch` resolves it from the store. (Decided
2026-11-13 by the release lead, under the maintainer's standing instruction to decide;
reversible.)

### Encoding a description keeps its external base

The encoder never wrote a manifest's `Base`, the first-line bare ID naming
the block a description refines, although the decoder keeps it. So
`Unmarshal` then `Marshal` turned a block on a base into a standalone
description of the block's own entries, silently (found 2026-11-13; c4-swift
had copied the encoder). `Encode`, `Marshal` and `MarshalPretty` now write
it as the first line. `EncodePatch`, whose first line is its `OldID`, does
not write a base the patch manifest carries a second time. A manifest with
no base encodes exactly as before.

### `c4 patch` and `c4 log` fold one-level bases correctly; `c4 patch` verifies checkpoints

Three faults in the chain verbs, found 2026-11-13 while bringing libc4's
chain reader in line with this one, and the file-boundary rule `c4 patch`
now enforces:

- `c4 patch` and `c4 log` applied a file's own sections without reading
  a collapsed directory's listing, the fault the entry below fixed in the
  library fold only. `c4 patch block.c4m`, where the block deepens a
  directory of its one-level base, printed that directory holding only the
  block's children under a line whose ID names them all, exit 0. Both verbs
  now fold through `c4m.FoldSection`, and `c4 log` counts each layer's
  changes against the deepened state before it.
- `c4 patch` never checked a checkpoint: `chain-vector/bad-checkpoint.c4m`
  resolved, exit 0. Every bare ID inside a file is now verified against the
  state the sections before it resolve to.
- At a file boundary in `c4 patch a.c4m b.c4m`, b's first-line bare ID
  must now equal either the state a resolves to (its description ID, the
  form `c4 diff` writes) or a's byte ID (the block form of a shape-B
  chain). Anything else is refused, naming both (ruled 2026-11-13;
  SPECIFICATION.md, "Multiple Patches", wording).
- The shape reader dropped a first-line base that a checkpoint directly
  follows, so `c4 patch` and `c4 log` fetched the checkpoint's ID as the
  base: refused when the store lacked it, and folded onto the wrong record
  when it held it. The base is now kept as an empty first section.

### A block that deepens a directory of its base reads that directory's listing first

A stored base is a one-level record: a subdirectory is one line carrying
its listing's ID. Folding a block onto it treated such a directory as
empty, so a later block that removed a file under it (by restating the
file exactly) added the file instead. The result's root ID came out
right, because it rests on the restated directory lines, while the
entries under them were wrong: `c4 patch` printed a description that
disagreed with its own directory IDs, exit 0 (found 2026-11-12 by an
independent review of shape-B output chains). Now, before each section
applies, every collapsed directory of the accumulated state that the
section restates with children has its listing fetched, verified against
its ID and inlined. A listing the source lacks is an unresolved base,
named in the error; the fold refuses rather than guess.

### `c4 log` follows an external base

On a c4m whose first line names an external base, `c4 log` printed the
file's own first section as "(base)", presenting a block as a base state.
It now resolves the base from the store and lists it as layer 0,
`0  <ID>  (external base <block ID>)`, then the file's sections from 1 as
patches against it, so the indices `c4 split` takes are unchanged. A base
the store cannot supply is an error naming it, as in `c4 patch`.
### A block on an external base is resolved section by section, its checkpoints verified

A c4m whose first line names an external base was resolved by folding the
whole block onto the fetched base as one patch. The decoder, not knowing
the base, had deferred every checkpoint inside the block, and nothing
verified them afterwards, so a block whose checkpoint named the wrong state
resolved without complaint. It had also applied the block's later sections
against its first section alone: a section that restated an entry an
earlier section wrote, removing it, was read against the wrong state, and
the base's entry survived. Resolving now fetches the base, applies each
section in order and verifies every checkpoint against the state so far
(`ErrPatchIDMismatch` on a mismatch).

### A metadata scan records an observed empty directory as empty

`c4 id -m m` computes no directory IDs, so a directory it listed and found
no children in carried no ID. Metadata propagation reads a childless
directory with no ID as unread, so its size went null, and the null spread
to every directory above it: a tree with one empty folder printed `-` for
every ancestor's size. The scan saw that directory empty, so it now records
it as empty, as full mode does: size 0 and the empty listing's C4 ID, a
known constant. Only a directory the scan could not read stays unread, with
a null size and ID. Structure mode, which records no sizes, is unchanged.

### A raw space, quote or control byte in a name is refused

A written name escapes a space as `\ `, a double quote as `\"`, and control
bytes through SafeName. The decoder used to read a raw one as part of the
name. So a damaged C4 ID field joined the name: `real.txt junk` read as one
name, and the line's ID was dropped. It now refuses such a line, naming it.
Pinned by the shared `c4m/testdata/name-escape-vector/`.

Indentation stays readable in any consistent unit, because the library's
own `Encoder.SetIndent` and `Entry.Format` can write one. The Validator
reports a unit other than canonical's 2: a warning in lax mode, an error
with `Strict`.

An entry marked `IsSequence` whose name holds no range was written raw,
with no SafeName: the one writer path that could leave these characters
unescaped. It is now written as an ordinary escaped name.

### The Validator reads through the decoder

`c4m.Validator` split each line with its own parser, and on 18 of the 31
shared vector files its verdict differed from the decoder's:
- It rejected every patch chain and the pretty form's timestamps.
- It read link targets into names.
- It called the library's own canonical order unsorted.
- It accepted damaged mode letters and, in lax mode, torn C4 IDs.

It now decodes with `c4m.Decoder`. A line the decoder refuses is one fatal
error at the decoder's line, so on every vector file the two agree, strict
and lax. On the entries as written, the Validator adds the checks the
decoder does not make:
- duplicate paths, in both modes (the spec says parsers MUST reject them);
- an entry deeper than its parent allows;
- flow targets;
- with `Strict`, canonical order: files before directories, then natural order.

Messages for format faults are the decoder's. The exported API is
unchanged. Lines the decoder reads leniently (unescaped spaces, raw quotes
and control bytes in names, odd indentation) are held in a skipped test
until a decoder rule lands; see
`design/validator-reads-through-decoder.md`.

### A name is never a path: read and write

The decoder cut a name at its first `/` and dropped the rest of the line,
C4 ID included. So `sub/real.txt` on a file line read as a directory `sub/`
with no ID, with exit 0. `restore` then made a directory `sub` with the
file's mode and failed its own verification. The names `.`, `..`, `./`,
`../` and `/` decoded as names. The encoder wrote all of these raw, so the
library broke its own round trip.

Now:
- **The decoder** refuses a line whose name holds a `/` other than a
  directory's trailing one, or is a path component, with an error naming
  the line. The check runs on the decoded name, so an escaped `/` is refused
  too.
- **`Manifest.ComputeC4ID`** gives the nil C4 ID for a manifest holding such
  a name, at any depth, since no conforming text exists to hash. `Canonical`
  gives "", and `WriteCanonical` writes nothing and errs. `Validate` names
  the entry.
- **`Encoder.Encode` and `EncodePatch`** refuse to write such a name, with
  `ErrPathTraversal`, before writing anything.

Pinned by the shared `c4m/testdata/name-rule-vector/`, made by the library.
No CLI path writes such a name, since a filesystem cannot hold one.

### A store made on a case-insensitive volume is readable on a case-sensitive one

TreeStore shards objects into directories named from the ID's leading
characters. On a case-insensitive volume (default APFS, NTFS) two shard
names differing only in case land in one directory, so a store copied to
a case-sensitive volume holds objects under a directory whose case does
not match the lookup, and `c4 cat` said "content not found" for an
object that was there. Lookups that miss at the exact path now search
the shard path case-insensitively. Release 1.0.x has had this since
2026-10-02 (9a40c14); master never took it.

### `c4 restore --force -c` honours the guide as the dry run does

With `-c GUIDE`, the dry run trusted a file whose size and mtime match
the guide (racy-safe) and planned nothing for it, but `--force` planned
from the fully hashed pre-image and rewrote it. Trust is the client's
stated choice, so `--force -c` now plans and verifies from the same
guide-trusted view as the dry run, and leaves such a file as it is. The
pre-image is still hashed in full, as the durable backup, and content
copied from the destination still comes from it. Without `-c` every
file is hashed, as before.

### Names a disk folds together are measured and refused, never paired or deleted

A case-insensitive volume (default APFS, NTFS) takes `README.md` and
`readme.md` for one name, and APFS takes the NFC and NFD forms of `é`
for one. `c4 restore` wrote both such names, so the second write went
through the first's name, leaving one file with the other's bytes. A
description that changed only a file's case did worse: restore wrote the
target's bytes into the existing `Readme.md`, then removed it as
unlisted, leaving the folder empty and failing its own verification
(exit 3).

Each folder's names are now measured on the destination before anything
is written, by identity in a throwaway probe folder, with no rule and no
new dependency. In each folded set the last name in byte order is written
whole, and the others are refused and named (`refused: <path>: this
folder takes it for <name> (measured)`, a directory once with its entry
count); restore writes the rest, verifies it and exits 2. A file whose
name changes only in case is recognised as the same entry, kept and
respelled, exit 0. A dry run writes nothing, not even a probe, and names
case pairs as `may collide: … (not measured)`. `reconcile.Distribute`
measures each source directory's names at every dir target before writing
them, refuses folds per target (`TargetResult.Refused`), and respells
what it wrote. Release 1.0.x has had this for `c4 patch` since #118.

### Restore refuses a name the platform cannot hold, and writes the rest

A c4m name is any bytes but `/` and NUL. On Windows, `c4 restore` turned
a name holding a backslash into a path through `filepath.FromSlash`, so
`a\b.txt` was written as `b.txt` inside a directory `a`; names Windows
reserves or forbids (`CON`, `a:b`, a trailing dot) and, on macOS, names
that are not UTF-8 failed part way. Such a name is now refused: never
renamed, mapped or split. Restore names each refusal on stderr
(`refused: <path>: <reason>`, a directory once with the count of
entries under it), writes the rest, verifies the rest exactly, and exits
2. The rule is git v2.47.0's for Windows (backslash, `: < > " | ? *`,
control bytes, a trailing space or dot, the reserved device names) and
a measurement for macOS (UTF-8 only); Linux refuses nothing. A link's
target is checked on Windows only. Pinned by
`c4m/testdata/name-refusal-vector/`, which c4-operator reads too.

### Restore never writes through a symbolic link in the destination

`c4 restore --force` into a destination holding a symbolic link to a
directory outside it (`dest/sub -> ../outside`) wrote the snapshot's
`sub/payload.txt` through the link into `outside/`, then failed its own
verification with exit 3. Every write is now checked to stay beneath the
destination without passing through a link, and a link standing where
the snapshot records a directory is replaced by the directory. Release
1.0.x has had this since 2026-07-29 (56c4416); master never took it.

### Restore never writes store bytes that do not match their ID

A store object whose bytes no longer hash to its ID (tampered, or
damaged on disk) was copied over the destination file, and restore only
then failed its verification with exit 3, leaving the wrong bytes in
place. Content is now hashed as it is copied and published only when it
matches its ID; otherwise the destination file is left untouched and the
error says which content failed. A copy-on-write clone is attempted only
when there is no ID to check against. Release 1.0.x has had this since
2026-10-02 (5b05207); master never took it.

### A c4m entry that skips its parent directory is an error, not a panic

A c4m whose entry sits at a depth with no parent directory above it (a
corrupt, truncated or hand-edited file) made patch, restore, log, diff,
split, merge, paths and id exit 2 with `panic: runtime error: index out
of range`. Each now exits 1 with `invalid entry: entry "y.txt" at depth 1
has no parent directory at depth 0`. In the library, `Decoder.Decode`
returns that error, and `ApplyPatch` and `ResolvePatchChain` panic with
it (wrapping `ErrInvalidEntry`) instead of a bare index error. Release
1.0.x has had this since 2026-10-02 (59ce7cc, 6cfba2c); master never
took it.

### A folded sequence's ID list is stored, and restore never writes it as a file

`c4 id -s -S` stored a folded sequence's members but not the ID list its
entry names, so `c4 cat <folded ID>` found nothing. And with the list in
the store, `c4 restore --force` of a folded description deleted the real
members and wrote the ID list as a file named `frame.[0001-0003].exr`.
`-s -S` now stores the list (rebuilt from the members and checked
against the entry's ID), and restore refuses a folded entry by name,
exit 1, with nothing touched; a plan that cannot materialise every entry
now refuses before applying anything, where it used to apply nothing and
fail its verification with exit 3. Release 1.0.x has had both since
2026-07-24 and 2026-10-02 (7bac715, 7afd4d8); master never took them.

### A symlink to an ancestor no longer crashes the scan

`c4 id` on a tree holding a link to one of its own ancestors or to its
own directory (`up -> ..`, `here -> .`) died with `fatal error: stack
overflow` and exit 2: the scanner resolves a link to a directory by
scanning it, and that scan met the link again. Release 1.0.x has had the
guard since 2026-07-29 (9092fb1); master never took it. Such a link is
recorded with its target and a nil ID, as on 1.0.x.

### A name holding a backslash is valid

Only `/` separates paths in c4m. A backslash, a colon and every other
byte is a literal character in a name, written `\\` by SafeName where
needed. The scanner, the decoder and `c4 restore` already handled such
names, but the library refused them: `Manifest.Validate` reported a name
like `trail\` as path traversal, and the line Validator rejected any
backslash in a name, so a description of a real tree failed validation.
Both now accept it; `/` and null bytes are still refused.
SPECIFICATION.md's name rules say `/` where they said "path separators".

### A collapsed directory's null size is no longer hashed as 0 ⚠

A directory with no children listed under it and a C4 ID other than the
empty listing's is collapsed: its listing lives under that ID. When its
size was null, `ComputeC4ID` gave it size 0 before hashing, because any
non-nil ID was taken to mean empty. The C4 ID therefore hashed a size
the file does not state and nothing observed. A collapsed directory's
null size now stays null, and only the empty listing's ID gives 0. **C4
IDs change for a description holding a directory known only by its C4
ID with a null size.** No CLI path writes such a line; a description
from the library or another tool can hold one, and `c4 id -q` and the
boundary IDs of `c4 diff` hashed it. `Normalize` now also fills 0 for an
empty-listing directory in a tree with no other directory to re-derive,
where it used to leave null and disagree with `ComputeC4ID`. Pinned by
`c4m/testdata/childless-dir-vector/`; SPECIFICATION.md "Empty and Unread
Directories" names the collapsed case.

### `c4 restore` sets a symlink's own time

Restore recreated a symlink but never set the link's own modification
time, so its verification failed (exit 3, "verification failed: as-built
… != target") on any tree holding a symlink; a link whose target was
already right but whose time differed was left alone. Restore now sets a
link's own time without following it: on Linux with utimensat and
AT_SYMLINK_NOFOLLOW, on darwin by opening the link itself (O_SYMLINK) and
setting its times through the descriptor, both with the standard library
only. A link created from a null time gets the epoch, as a file does.
Where the platform cannot set a link's time (Windows and others, where
os.Chtimes follows the link and would change its target), restore names
each such link on stderr with its kept creation time, and verification
checks the tree at that time instead of failing or skipping it silently.
### A name or link target ending in a backslash reads back as itself

A backslash is a literal in a c4m name (only `/` separates paths), written
`\\` by SafeName. The decoder consumed only the backslash of that escape,
so its second byte started a new escape with the field separator after it:
a real file named `trail\` scanned as `trail\\` and read back as
`trail\ `, a different name, re-encoded `trail\\\ ` with a different C4
ID and exit 0; in a tree with a subdirectory the scan's own checkpoint
then refused the file `c4 id` had just written. A link target ending in a
backslash had the same fault. Every backslash escape is now consumed as a
two-byte unit, in names and in link targets. Pinned by
`c4m/testdata/backslash-name-vector/`.
### A damaged size or mode field is an error, not a different value

The decoder read `6x real.txt` as size 6 and the name `x real.txt`, and a
mode with a character in the wrong place (`-rw-r--r-z`, `-rr-r--r--`,
`-rwtr--r--`) as a clear bit, so a damaged line became a different entry
with no error. A size token now ends at a space, and each permission
position holds its own letter or `-` (with `s`/`S` at the owner- and
group-execute positions and `t`/`T` at the other-execute position); any
other character is an error naming the line. Every encoder in the suite,
the Go CLI at v1.0.21 and v1.0.25 and c4py and c4ts included, writes only
what this accepts (measured on a real tree with setuid and sticky bits).
Pinned for every implementation by `c4m/testdata/damaged-line-vector/`,
which also carries the damaged-C4-ID and impossible-date cases the
decoder already refused.

### A link line reads back as it was written

Two decoder faults, both on lines the encoder writes. **A target that
starts with `c4`** (a C4 ID's text, `<C4 ID>.exr`, `c4studio`), under a
mode that does not say symlink (the null mode views carry links with),
was read as an ungrouped hard link and the rest of the line as its C4
ID, and the line was refused (found by the storage project's git
importer, 2026-11-10). After a link operator a single field is now the
C4 ID field and two fields are a target and the C4 ID field, as the
entry format already implies: the C4 ID is always the last field and a
target escapes its spaces. **The pretty form's padding** before the C4
ID field joined the target of any link on a line with a null timestamp,
was escaped on the next write, and changed the tree's ID with no error;
`c4 id -e` reached it on any tree holding a symlink whose own mtime is
the epoch (measured on 1.0.24 and master). The target and flow-target
parsers now skip the run of spaces before asking whether the C4 ID field
starts. Pinned by `c4m/testdata/link-target-vector/` for every
implementation. Unruled and not pinned: a null-mode link whose target is
shaped like a flow target (`host:path`) still reads as a flow link.

### Every Sink checks a write before it names it (the Sink contract)

The check and the Abort that `TreeStore` gained (below) are now the
`Sink` contract, held by every store in the package rather than by a
wrapper the caller must remember: Folder, MAP, ShardedFolder, RAM, S3
and TreeStore all hash as they write and refuse on Close with
`ErrInvalidID`, naming nothing and leaving nothing, when the bytes do
not hash to the ID; all implement `Aborter`; all report it through the
new `Checked` interface (`ChecksID() bool`), which `Validating` honours
by returning such a writer as it is instead of hashing a second time.
`Validating` still holds the contract on behalf of a Sink from
elsewhere that cannot. `TestSinkContract` runs every Sink and wrapper
through the contract, and `TestEverySinkIsInTheContractTable` parses the
package for `Create` methods so a new Sink cannot be added without a
row. Four older tests that wrote bytes under the ID of other bytes now
write what their IDs name.

### A store write is checked before it is named, and can be given up

`TreeStore.Create(id)` bound whatever bytes arrived to the ID the caller
chose: `Close` named them, so a source that failed part way left a
partial object under a real C4 ID, and the writer offered no other way
out (a caller could publish the partial object or leak the temp file;
`defaultPut` leaked). `Validating` checked only after the store under it
had closed, which is when that store names the object, then removed it
and ignored the error, so wrong bytes sat under the ID from the close to
the remove, and for good if the remove was unsupported or the process
stopped between the two. Both found in the storage project's review,
2026-11-09, as failing tests, kept in `store/review_abandon_test.go`.

Now the bytes are checked, never trusted: `TreeStore`'s writers hash as
they write and `Close` refuses with `ErrInvalidID`, removing the temp
file and naming nothing, when the bytes do not hash to the ID, in every
sync mode. Writers implement the new `store.Aborter` (`Abort() error`):
closes, discards, names nothing, idempotent, and `Close` after it is a
no-op. `DurableWriter` has `Abort` too. `Validating` checks before the
inner writer closes and aborts it where it can, so an inner store that
names on `Close` never names wrong bytes. `defaultPut` aborts its writer
on a copy error. The `Sink` doc states the contract. The cost is one
SHA-512 over each `Create` write, the same price `Put` already pays;
`Create`'s writer no longer forwards `ReadFrom`, since every byte must
pass the hash.

### The store barrier syncs the directories it renames into

`TreeStore.Sync` flushed the device, renamed each pending object to its
hash name, and never synced the directories that received those names or
the new shard directories it made, so a claim's durability leaned on the
journal's own flush carrying unrelated directory entries, which POSIX
does not promise and filesystems handle differently. Found in the storage
project's review, 2026-11-09. Every touched directory and its ancestors
up to the store root now get a plain fsync after the renames, then one
more device flush, so the device flush is paid twice per batch rather
than once per directory. The store root is cleaned on open, and the
ancestor walk is bounded by the root, so no spelling of the root can make
the barrier open directories above the store. `Normalize`'s
documentation gains the rule for writers that update a tree in place:
store every listing the patch restates or the derivation changes.

### `ApplyPatchFrom`: a patch is meaningful only over its own base

Removal is spelled by exact restatement, so what a patch entry means
depends on the state its boundary names. Applying a patch's text to any
other tree reads an addition that meets an identical entry as a removal:
two units writing the same bytes to the same path left the path empty,
and replaying a fold undid it (found in the operator's fold, 2026-11-09).
`c4m.ApplyPatchFrom(target, base, patch)` reads each entry against the
patch's base and does to the target what it means, removing the path or
writing the entry; a directory line with children is context, never a
write, and directory aggregates in the result are derived, never copied.
It equals `ApplyPatch` on its own base (up to those derived values) and is
idempotent. No conflict policy of its own (Merge keeps those).
SPECIFICATION.md "Patch Entry Semantics" states the rule.

A name is one thing. Directory names carry their slash, so a file `x`
written by the patch and a directory `x/` the target held could both
survive in the result (75 of 601 applications with kind swaps did, in the
storage project's review, 2026-11-09). A write of `x` now removes `x/`
and its subtree; a write under `x/`, or of `x/` itself, removes a file
or symlink `x`; the later write wins by kind as it does by bytes.
Pinned as the `kind-conflict` case of `c4m/testdata/apply-from-vector/`.

Three further rules for a patch carried to another tree, each pinned by
a vector case: a removal removes only the kind the base held, so a
directory that replaced a removed file stays
(`removal-meets-replaced-kind`); a directory written by ID alone over
a directory the target holds is refused, `ErrDirByIDOverHeld` naming the
path, unless the target is the patch's own base (`dir-by-id-refused`); and a childless line carrying the empty
listing's ID, which is what a scan writes for a unit that made a
directory and put nothing in it, means the directory exists and removes
nothing under it, so one unit's `mkdir` cannot delete another unit's
files (`empty-dir-over-existing`; before this the empty listing replaced
like any other). `ApplyPatchFrom` now returns `(*Manifest, error)`; it
has not shipped in a release, so no released API changes. (Decided
2026-11-13 by the release lead; reversible.)

### A known-zero mode on a directory name encodes as `d---------`

An entry built in memory with `Mode == c4m.ZeroMode` on a name ending in
`/` encoded as `----------`, which the decoder then read as the legacy
null (ten dashes on a directory line), so a known zero round-tripped to
null (found in c4fs, 2026-11-09). The encoder now writes the
directory type for a known mode on a directory name: `d---------`, in
canonical and pretty form, through one `modeText` used by both. Other
type bits on a directory name are left as given; that is a malformed
entry, not one to repair silently.
### A pretty timestamp with an unknown zone abbreviation is refused

The pretty-form timestamp (`Jan  2 03:04:05 2026 CST`) carries a zone
abbreviation, and Go's parser fabricates a zone with offset zero for any
abbreviation it does not know, so a file written on one machine read
hours wrong on another and claimed success (found by the libc4 and c4py
ports, 2026-11-09). The decoder now accepts UTC, GMT, Z, the local zone's
own abbreviation and numeric offsets, and refuses any other abbreviation
with "unknown time zone abbreviation ...: write UTC or a numeric offset".
Canonical text is unaffected (it is always UTC). What `-e` should write so
its text parses anywhere is with Joshua.

### An unread directory keeps a null size; the zero-against-null vector

A directory the scanner could not open (`d---------`, a chmod 000
directory) was recorded with size `0`: the scanner set the size null,
then `PropagateMetadata` filled 0 because the directory had no children.
Under the size rule, 0 beside no C4 ID is a known empty, so the scan
asserted an emptiness nobody saw (Joshua, 2026-11-09). Now a childless
directory with no C4 ID keeps a null size; an empty directory, whose
listing hashes to the empty-content ID, still gets 0. The scan is partial
(exit 2) either way, as before. SPECIFICATION.md gains "Empty and Unread
Directories". `c4m/testdata/zero-null-vector/` is the shared conformance
vector for known zero against null (ten-dash file, `d---------` with a
child, null mode, empty file and directory, unread directory), made by
the library in canonical and pretty form, with the per-entry results every
implementation must see; `TestZeroNullVector` regenerates and pins it.
### A failed store barrier is fatal; nothing is claimed

`c4 id -s` (and every verb that stores) runs the store's batch barrier
and then journals and prints the claim. When the barrier failed, the
failure was only warned about and the claim went out anyway, naming
bytes that may not be on stable media (found in the storage project's review,
2026-11-09). The barrier's failure is now fatal, before the journal
append and before any ID is printed: "store sync failed; nothing
claimed".

### Incomplete ingests are collected when the store opens

A batch ingest killed before its barrier left its `.ingest.*` temp files
in the store root forever (found by the storage daemon's commit measurements). The rule
(2026-11-08): on boot a system garbage-collects incomplete transactions
that cannot, or should not, be replayed to completion, and opening a
store is its boot. Temps are now named `.ingest.<host>.<pid>.<random>`;
`NewTreeStore` removes those of this host's dead writers, keeps a live
writer's and another host's, sweeps the old unnamed form only after a
day, and never touches an object. `TreeStore.SweepIngest` is exported.
`design/store-ingest-sweep.md`.

### `c4 cat -r` names every listing the store lacks and exits 2

`c4 cat -r` on a snapshot whose store is missing a directory's listing
printed the tree as far as it could and exited 0, silently. That hid a
real bug in a storage-daemon prototype whose check read only the exit status
(2026-11-08). Now the tree still prints as far as the store supplies it,
each listing the store cannot supply is named on stderr as
`partial: <path> listing <ID> <why>` (not in store, does not parse, or no
store configured), and cat exits 2, the partial scan's code. Expanded
listings are now verified against their IDs, as every other cat read
already was; a listing that fails verification is fatal (exit 1) like any
other damaged object, never "partial". `c4 diff` and `c4 restore` given a store
address that does not expand in full now refuse with exit 1 and the same
`partial:` lines, instead of comparing or restoring against a tree with
the store's gaps in it. A directory already listed with its children, or
an empty one, is never expanded or named.

### A c4m file never ends with its own ID; `c4 diff` no longer prints one ⚠

A bare C4 ID at the end of a c4m file is a format error (ruled
2026-11-08; `design/trailing-id.md`). The 2026-07-13 erratum had made it
the verified "closing validator"; that is withdrawn, because a file
cannot contain its own ID and still be the file its ID summons.

- `c4 diff` prints the old state's ID and the entries, and nothing after
  them. Through 1.0.26 it ended with the new state's ID, so every chain
  built with `c4 diff >> FILE.c4m` has the old shape. The restore dry
  run likewise ends on its entries.
- Every reader refuses a file that ends with a bare ID
  (`c4m.ErrTrailingID`), both the resolving decoder and the chain
  reader behind `patch`, `log`, `split` and `explain`, with an error
  that says how to fix it. The 1.0.24 decoder already refused these
  files, as "empty patch section"; only master briefly accepted them.
- The normaliser is the migration: `c4 id FILE.c4m` (or `c4 id -`)
  reads the old shape, verifies the trailing ID as a checkpoint, drops
  it, reports `trailing root ID removed` on stderr and writes the valid
  form. `c4 id -l` lists such files; `c4 id -d` shows the change. Like
  every chain it normalises, the result is one plain c4m of the final
  state (same description ID); to keep a chain's layers, delete its
  final line instead.
  (`Decoder.AllowTrailingID` and `DroppedTrailingID` expose this.)
- Consumers holding immutable old objects (a changeset in a signed
  record) use `c4m.DecodePatchChainMigrating`, which takes the trailing
  ID off and returns it for the caller to verify against the resolved
  state; nothing writes it back. The chain reader also now refuses two
  consecutive checkpoints that disagree (`ErrPatchIDMismatch`) instead
  of keeping the second silently.
- The conformance vectors `chain-vector/{vector,bad-checkpoint}.c4m`
  and `deepen-vector/{deepen,remove}.c4m` lose their final line; their
  root IDs are unchanged. `bad-validator.c4m` now pins both rejections.
  Every other implementation's copies need the same edit.
- `c4m.WriteChainStream` is gone; `WriteChainBody` and `ChainStream`
  write the stream, which ends on its last refined entry.

### `c4 diff`, `c4 restore` and `c4 explain` hash every file; `-c` is the stated way to trust ⚠

A directory side of `c4 diff`, the destination of `c4 restore`, and the
destination `c4 explain restore` describes are hashed in full: a file
whose bytes changed under the same size and mtime is a change. Before,
all three trusted size and mtime against the other side's description,
silently and with no racy rule, so such a change went unreported by
diff, unplanned by restore (whose `--force` then failed its own
verification with the changed bytes still in place), and unmentioned by
explain. Whether to trust metadata is the caller's choice: `-c GUIDE.c4m`
on all three applies exactly `c4 id -c`'s re-scan trust, racy rule and
"reuse:" line included. A restore's plan reuses the pre-image scan it
already took (for files that scan provably hashed: same size, same
mtime, strictly older than the scan start), so the destination is read
once.

### A directory's timestamp is its own (reading A)

A directory's timestamp is its modification time as observed on disk;
the children fill it only when it is null (the spec's "Directory
Timestamps", ruled 2026-11-03). The normaliser re-derives a listed
directory's size and C4 ID and keeps its stated timestamp; it had
re-derived the timestamp from the newest descendant, which made
`c4 restore` fail its own verification on every nested tree whose
directory time differs from its newest child's, and reported a fresh
scan's own output as not normal. A fresh scan is normal.

### Restore sets a directory's recorded time back after changing its contents

Creating, moving, linking or removing an entry bumps its directory's
mtime, so a directory that matched the target when the plan was made no
longer matched after it ran, and a re-run planned the chtimes the first
run should have. The plan now sets the recorded time back on every
directory whose contents it changes, deepest first, after the children.

### One source wanted at two targets

A restore whose target lists one content ID at two paths, onto a folder
holding it at one path to be removed, planned two moves from the one
file; the second rename failed and left the folder half switched. The
first target is a move, later targets are creates copied from the move's
destination.

### `c4 id -s` refuses without a content store

With no store configured and stdin not a terminal (or the create prompt
declined), `-s` silently scanned without storing, printed no "stored:"
line and exited 0. It now refuses before scanning, exit 1, naming
`C4_STORE` and the interactive way to create the default store.

### Patch and diff compare at the format's resolution; a directory symlink stores nothing of its own

`c4m` patch and diff compare timestamps at one-second resolution, so a
scanned tree's sub-second times no longer put unchanged entries in a
patch. `c4 id -s` no longer tries to store a directory symlink's
"content" (the link carries the directory's C4 ID, whose record the
directory's own entry stores).

### `c4 id -q` prints the ID; on 1.0.x it printed nothing ⚠

`-q` changes meaning. On 1.0.x it is "quiet": `c4 id -q DIR` prints
nothing to stdout and exits 0, and the only way to a bare ID is the
`stored: <ID>` line `c4 id -s` narrates on stderr. On 1.1 `-q` means
"the ID only": `c4 id -q DIR` prints the description's ID, `c4 id -s -q`
prints the claim, and the stream is suppressed. A script written for one
line and run on the other gets either silence with exit 0 or an
unexpected ID line; neither fails loudly (found by the filesystem project, 2026-11-09).

Migration: a 1.0.x script that used `-q` to silence `c4 id` should
redirect stdout instead (`c4 id -s DIR >/dev/null`); a script that wants
the ID should use `c4 id -q` on 1.1 and, while it must still run on
1.0.x, read the `stored:` line of `c4 id -s` from stderr. Check
`c4 version` before relying on either. 1.0.25 narrates the old meaning
on stderr when `-q` is given without `-s`, so the silence is no longer
silent.

### A c4m file never ends with its own ID ⚠

`c4 id DIR` no longer closes its output with a bare root-ID line, and
neither do `c4 id FILE.c4m` and `c4 id -`; a chain ends on its last
entry, and bare IDs appear only as boundaries before a patch. A file
written from `c4 id` is therefore the file its own C4 ID names:
`c4 cat` of that ID returns it byte for byte. The description's ID is
`c4 id -q`, the one scripted form; `tail -1` is no longer a contract.
Under `-s` without `-q`, the stored ID is narrated on stderr as before
and recorded in `c4 log`. Identifying or storing stdin, and every file
inside a scanned tree, now means its bytes: a `.c4m` file is no longer
re-rendered before hashing, and `c4 cat ID` returns an object's bytes
unless `-e` or `-r` asks for a rendering. The chain grammar and every
stored record are unchanged. (Joshua, 2026-11-03; design/no-self-id.md.)

### The S3 store: temporary credentials, and the prefix rule ⚠

The S3 store takes the standard AWS credential chain (`store.Ambient`,
standard library only): the environment with `AWS_SESSION_TOKEN`, then a
web identity through STS, then, only with `instance-role=true` on the
URL, the EC2 instance role through IMDSv2; temporary keys renew before
they expire and the session token is sent and signed. Static keys in the
environment work as before. c4-operator imports this chain from here.

An S3 prefix must now end in "/" (or be empty): `s3://bucket/runs` is
refused, naming the fix, because objects are stored as prefix + ID and
that URL put them beside the folder (`runsc4...`), and c4-operator
names the same keys for the same URL. A store filled under a slashless
prefix keeps its objects where they are; the CLI reference has the
one-off `aws s3 mv` recipe that moves them. The URL's region must be a
region name (it becomes part of a host name). `store.NoRedirects` and
`store.ValidRegion` are exported for other S3 clients.

The multipart writer: a completion is a stored object only when its reply
is a whole result (S3 answers 200 and can still report an error in the
body, or the connection can drop first); the start of an upload is never
resent after a lost reply (a second upload would be left open, billed);
every failed exit aborts the upload, panics included, and an abort that
fails names the upload left open for a bucket lifecycle rule.

The snapshot-loop surface (`design/snapshot-loop`, rounds 2–3),
crash-verified end-to-end including streaming and SIGPIPE modes
(`design/snapshot-loop/kill9-crucible.sh`). Breaking changes are
marked ⚠. Round 3 (the crucible-litigated v9 amendment) reshaped the
output surface after round 2; this entry describes the NET result.

### Cancelling a scan no longer panics

The progressive scanner's completion monitor closed the work channels
when the context was cancelled, while a structure worker could still be
queueing a child directory: "send on closed channel", seen on Windows
CI and reachable from Ctrl-C mid-scan. On cancel the monitor now only
signals completion; every worker already leaves on the context.
Regression test: `TestStopDuringStructureScanDoesNotPanic`.

*Correction (2026-11-11):* this fix was to `cmd/c4/internal/scan`, a
package the `c4` CLI has not linked since 2026-03-19, when every verb
moved to the public `scan` package (`go list -deps ./cmd/c4` does not
include it). It was not reachable from Ctrl-C or from any command, and
it changed no behaviour of the CLI. The package is removed below.

### `c4 -S` stores the frames of a nested sequence

Folding put each folded range, then the directories, at the end of the
listing in map order, so a sequence inside a subdirectory came before
its parent and `c4 -S` built the frame paths against the root: "cannot
read sequence member", and no frame stored. Folding now keeps the tree
in order (a range replaces its first frame) and its output is
deterministic.

### `c4 version` and `c4 help` name any other c4 on PATH

A dev build in `~/bin` ahead of Homebrew's c4 was invisible until `c4
farm` did nothing. Both verbs now narrate on stderr every other `c4`
executable on PATH, saying which comes first (and so runs when you type
`c4`) and which comes later. Nothing when this binary is the only one.

### `c4 <name>` runs `c4-<name>`; `c4 farm` and `c4 agent` are reserved

External tools dispatch like git plugins (`design/external-tools.md`):
after the built-in verbs, `farm` and `agent` always run `c4-farm` and
`c4-agent` from PATH (a directory of that name is scanned with `c4
./farm`); any other first argument that names an existing file or
directory keeps the bare shortcut; otherwise `c4 <name>` runs `c4-<name>`
when it is installed. Arguments, stdin, stdout, stderr and the exit code
pass through. `c4 help` lists the installed tools. The standard release
ships `c4-farm` and `c4-agent` from c4-operator, so `brew install
mrjoshuak/tap/c4` ends with a working `c4 farm`.

### Natural order compares digit runs of any length (IDs of rare trees change)

`c4m.NaturalLess` parsed each digit run into an int64, so a run of 20 or
more digits wrapped: two different runs could compare equal, and then
the canonical order, and the tree's ID, depended on input order (one
tree, several IDs across runs), or two runs compared in the wrong
order. Runs now compare as integers of any length with no parse (drop
leading zeros, shorter remainder first, then digit by digit, then the
shorter spelling first). Names with runs of 18 digits or fewer sort
exactly as before, so their IDs are unchanged; a tree with a longer run
now has the one ID the specification always defined, which is the ID
c4py already computed. Conformance vectors:
`c4m/testdata/natural_sort_vectors.json`, shared with c4py and c4ts
(whose double-precision parse lost digits past 16).

### ⚠ Streaming chain output, and `-s`

`c4 id` streams its description as a valid c4m chain: entries in
canonical pre-order with directory lines carrying null ID/size
(bottom-up aggregates, unknown as the line passes), one boundary ID
naming that base state, and one refinement patch filling the
aggregates in. The chain ends on the patch's last entry; a flat tree
is the base alone, with no bare ID at all (see "A c4m file never ends
with its own ID", above). A single regular-file argument emits one
entry line. A killed scan leaves a valid partial description. `-e`
implies buffered aligned output. Without `-q`, id takes ONE path per
invocation. The description's ID is `c4 id -q`; piping the stream
through `c4 id -q -` gives the same ID, and `c4 id DIR | c4` the ID of
exactly the bytes piped.

`c4 id -s` streams the same bytes while snapshotting. The claim is
`c4 id -s -q`'s one ID line, printed only after content is durable and
the journal append is fsynced ("a printed CLAIM never dangles"); the
blessed script capture is `SNAP=$(c4 id -s -q dir/)`. Without `-q`,
`-s` streams the description and narrates `stored: <ID>` on stderr
after the barrier; entry lines are description, never claims, and
`c4 log` holds every claim either way. A dying stdout (closed pipe)
never cancels a snapshot: ingest and journal complete, id exits 1,
`c4 log` holds the claim. Partial scans (unreadable entries, unreadable
directories included) record nulls, continue, and exit 2 with the
claim printed and true.

### ⚠ The store journal is its own format

`<store>/journal` (no longer `log.c4m` — it is not c4m): the magic
first line `@c4 journal 1` (rejected by every conforming c4m parser,
so misinterpretation is structurally impossible), then one line per
claim — scan-start and claim ID, nothing else. Names, sizes, and
filesystem origins are gone from the root record: roots are purely
virtual, and provenance is testimony-layer work. `c4 log` output is
`index scan-start ID` (the ID remains the last field).

### Chain-grammar erratum (all five implementations)

Checkpoints officially name the ACCUMULATED manifest state and a
resolving decoder MUST verify them; a bare C4 ID at EOF is the legal,
mandatory-verified closing validator (`ErrPatchIDMismatch` on
mismatch; `ErrEmptyPatch` retired). This reconciles SPECIFICATION.md
with C4M-STANDARD §10.3/§10.7 and with what shipped tools have always
emitted. The CLI unifies all c4m ingestion on one verifying loader;
`c4 cat <file>` and `c4 paths` error loudly instead of echoing or
garbling non-c4m input. The shared conformance fixture lives at
`c4m/testdata/chain-vector/` — every sibling implementation must pass
it before release.

### The print barrier and the journal

Every store gains a journal, `<store>/log.c4m` — an ordinary c4m
patch chain recording every claim the store makes (snapshots, stdin
blobs, restore pre-images). No ID reaches stdout before its content is
durable AND its journal entry is fsynced: printed ⇒ durable ⇒
recoverable, through `kill -9`, kernel panic, and power loss. Appends
serialize under an OS lock that dies with its holder; a torn tail is
truncated on the next append. `c4 log` (no arguments) lists the
journal: index, then the entry exactly as recorded.

### `c4 restore` — the one tree-writing verb

Make a directory match a description, undo-safely. Dry run by default
(the plan in c4m patch form on stdout); `--force` snapshots the
destination's pre-image — durable and journaled before the first
destructive operation — then reconciles, verifies by recomputation,
and prints exactly two bare-ID lines: the undo handle, then the
as-built ID. Line 1 feeds straight back as a restore target; the undo
prints its own undo. Exit machine: 0 verified / 1 refused untouched /
2 declared omissions / 3 curable.

### ⚠ Identity: full is the default; content is an eval-side projection

`c4 id` describes at FULL fidelity by default (the `ls -l` view), and
`-q` prints the full root ID — equal to the `-s` snapshot ID while
the tree is unchanged. The content projection (`-m c`) — mode and
timestamp null at every level, the exec bit included, so equal bytes
give equal IDs on any machine, clock, or umask — is applied at
EVALUATION time: the scanner always captures full knowledge, and a
null in a saved record now always means genuinely unknown at scan
time. The cross-machine contract line is `CID=$(c4 id -q -m c dir/)`.
`-s` always snapshots at full detail and conflicts with `-m`.

### ⚠ Nothing writes without `-s`

Bare `c4 <path>` is now read-only (`c4 id <path>`); `c4 -s <path>`
snapshots. `echo data | c4` prints the ID without storing;
`echo data | c4 -s` stores. The `-x` flag is gone.

### ⚠ `c4 patch` is text algebra

`patch` resolves chains — c4m text in, c4m text out (`-n N` picks a
section) — and never touches directories; a directory argument exits 1
pointing at restore. The old reconcile/revert/pre-state forms
(`patch <c4m> <dir>`, `-r`, `--dry-run`, `--no-store`, `--source`)
moved behind `restore`'s safety machine.

### ⚠ `c4 diff` slimmed

`c4 diff [-e] <old> <new>`. Sides: directories, c4m files, or store
IDs (ID/path allowed). Directories scan at content level, or at the
other side's level when that side is a description. `-r`, `-s`, `-q`,
`-m` removed.

### Store addresses and ID/path descent

An argument whose first slash-separated component is a C4 ID is a
store address, accepted by `c4 cat`, restore targets, and diff sides:
`<ID>/a/b` descends by recorded entry name, byte-exact, every level
rehash-verified, never following symlinks. `c4 cat` verifies every
read — exit 0 means present AND intact.

### Fast re-scans: `-c` reuse guide and `--verify`

⚠ `-c` is repurposed: it now takes a prior full description as a
*reuse guide* — a file whose path, size, and mtime match (and whose
mtime is strictly older than the guide's scan start; git's racy-index
rule) keeps its recorded ID without being re-read. Summary on stderr:
`reuse: R reused, H rehashed`. The documented trade: a same-size
change under a restored mtime is invisible to a `-c` re-scan;
`--verify` is the audit — full re-hash plus a report of every change
hidden under unchanged metadata. (The old `-c` scan-filter workflow is
retired.) ⚠ `C4_EXCLUDE_FILE` is removed: no environment variable
adds exclusion patterns.

### Reference pages

`c4 --help` prints the one-page contract (identity, scripting rules,
exit codes, durability); every verb's `--help` prints its full
reference page. `c4 explain restore` narrates a restore plan.

### c4m parser fix

The decoder's name-field boundary now looks past aligned padding runs:
column-aligned (ergonomic) listings with null timestamps round-tripped
with padding absorbed into names.

## v1.0.16

Correctness patch: three defects found by adversarial review of the
shipped binary.

### Durability: batch objects publish only after the barrier

In batch mode (the only mode since v1.0.15), objects were renamed to
their final hash names *before* the end-of-command device barrier. A
power cut in that window could leave a torn object at a valid hash
name, which a later run's presence-gated write-skip would adopt
silently. Objects now stay at temp names until `Sync` issues the
barrier, then publish — presence at a hash name always implies bytes
on stable media. During a run, pending objects are still served by
`Has`/`Open`/`ContentPath`, so behavior is unchanged apart from the
crash window. (Stores written by ≤ v1.0.15 cannot be repaired
retroactively; objects verify on read as always.)

### `c4 log`: linear chain resolution

`c4 log` resolved the entire patch chain from scratch for every
section — super-quadratic: 500 sections took ~60 s and 1000 did not
finish. The chain now resolves incrementally (one patch application
per section): 1000 sections in ~5 s including per-step IDs.

### Sequences: `-S` ingest stores member content

With sequence folding, the store pass reconstructed paths from entry
names — a folded pattern (`frame.[0001-0100].exr`) is not a file, so
every member was silently skipped and a "stored" sequence snapshot
held none of its frames. Folded entries now expand to their member
files at store time; unreadable members warn instead of vanishing.

## v1.0.15

Same-day supersede of v1.0.14: identical content plus the Windows
batch-barrier fix and Go 1.16 test compatibility below. v1.0.14's tags
remain for module-proxy immutability; install v1.0.15.

### Windows: batch barrier fixed

v1.0.14's batch durability barrier fsync'd the store root (and, in
reconcile, the destination) as a directory handle, which Windows
refuses (`Access is denied`): four store tests failed and every
`c4 id -s` / `c4 patch` printed spurious warnings on Windows. The
barrier now goes through the platform-conditional `store.SyncDir` —
a no-op on Windows, where per-file `Sync` already reaches stable
storage and NTFS journals rename metadata; unchanged elsewhere
(F_FULLFSYNC device flush on darwin, directory fsync on other
Unixes). No data was ever at risk on Windows. Verified on real
Windows hardware: full suite green, smoke-tested ingest and
materialize clean. The new platform files also restore the paired
legacy `// +build` lines, keeping the Go 1.16 build pledge.

## v1.0.14 (superseded by v1.0.15)

Released and superseded the same day: on Windows the batch barrier
printed spurious "Access is denied" warnings/errors (no data loss) and
four store tests failed. Use v1.0.15.

### Scan correctness: guided-scan directory IDs (action required)

**If you ever ran `c4 id --continue` (guided scan) on v1.0.13 or
earlier, regenerate those c4m files.** A bug caused every *directory*
entry produced by a guided scan to be silently written with the
empty-input C4 ID instead of the directory's real ID. File entries
were unaffected — file IDs are hashed directly from content — so the
damage is confined to directory roll-up IDs, but any tool comparing
directory or tree identity against an affected c4m file gets wrong
answers. A fresh scan (`c4 id <dir>` or `c4 id --continue` on
v1.0.14) produces correct IDs.

Root cause: directory IDs were computed by re-scanning each
directory's subtree with a cloned scanner that incorrectly inherited
the guide filter; root-relative guide paths never match a re-rooted
sub-scan, so every sub-scan came back empty, and the empty manifest's
ID was assigned without error. Two fixes at once:

- **Directory C4 IDs are now computed bottom-up** from the
  already-scanned children — the canonical one-level listing of the
  directory's direct entries, byte-identical to a fresh scan rooted
  there. Directories are never re-scanned, guided scans stay
  root-anchored, and the regression is pinned by tests that assert a
  guided re-scan of an unchanged tree reproduces the original
  manifest exactly.
- **Deep trees now scan in linear time.** The per-directory subtree
  re-scan made full-mode scans O(2^depth): ~93 s at depth 20, never
  finishing at depth 40. Bottom-up computation is O(N): ~0.007 s at
  depth 20.

Cross-implementation audit: the defect is Go-only. `c4py` computes
directory IDs bottom-up in memory and has no guided-scan feature;
`c4ts`, `c4-swift`, and `libc4` do not compute directory entry IDs at
all (parsed or caller-supplied only). No equivalent fix is needed
elsewhere in the suite.

### Diff correctness: entries keyed by full path

`c4m.Diff` keyed entries by bare name, so same-named files in
different directories could collide and misreport. Entries are now
keyed by full path.

### Structure mode: null timestamps render as `-`

Structure-mode scans render unstated timestamps as the null sentinel
`-`, not the zero time `0001-01-01T00:00:00Z`.

### Safety defaults: self-capturing snapshots and patch pre-state

Two gaps between "the store is the safety net" and what the CLI put in
it are closed. See `design/safety-defaults.md`.

**Behavior change — `c4 patch` reconcile forms now store the prior
state by default.** `c4 patch <file.c4m> <dir>`, `c4 patch <dir>
<dir>`, and `c4 patch -r` capture the destination's pre-state before
applying: content that would be removed or overwritten, every
directory's one-level record, the root record, and the pre-state
manifest text — durable (batch barrier) before the first destructive
operation. After applying, stderr reports:

    prior state stored: <id> (revert: c4 patch -r <id> <dir>)

and that revert command works verbatim. `--no-store` opts out;
`--dry-run` still changes (and captures) nothing; `-s` remains as a
removal-time belt (now redundant in normal use). When no store is
configured, reconcile forms offer to create the default store;
non-interactive runs proceed with a warning. Measured on a 5,050-file
dest: worst case (everything vanishes) 1.0 s → 2.7 s; typical patch
(50 files change) unchanged at 0.4 s.

**Snapshots capture themselves.** `c4 id -s` (and every other ingest
path) now also stores the manifest's canonical text — retrievable by
the manifest's own C4 ID — and the scan root's one-level record
(retrievable by the root directory ID), and reports `stored:
<manifest-id>` on stderr after the durability barrier. Lose the tree
and the `.c4m` file: `c4 cat <manifest-id>` reproduces the manifest
byte-identically and `c4 patch <recovered.c4m> <dir>/` materializes
the tree from the store alone.

- `c4 patch -r` accepts a stored manifest ID as well as a changeset
  file. The changeset form is fixed for nested trees (the OldID names
  the root record, which now exists in the store and expands through
  stored directory records; previously the lookup always failed with
  "pre-patch manifest not found"). A revert target with a missing
  directory record is refused instead of silently truncating the tree.
- Capture rides the same batch durability barrier as every other
  write: the prior state is on stable storage before the first
  destructive operation.

### Store ingest performance: 44x faster snapshots

Snapshotting a 20,050-file / 105 MB tree with `c4 id -s` took ~180 s:
`TreeStore.Put` flushed every object with `F_FULLFSYNC` (6–13 ms per
device flush on darwin), sequentially, plus an O(n) `ReadDir` split
check per Put. See `design/store-ingest-performance.md`.

- New write-durability policy on the store: `store.SyncMode`
  (`SyncEach` / `SyncBatch` / `SyncNone`), selected via
  `(*TreeStore).SetSyncMode`, with `(*TreeStore).Sync` as the batch
  barrier and optional-interface discovery via `store.Syncer`.
  `MultiStore` forwards both. Library default unchanged (`SyncEach`,
  per-object F_FULLFSYNC).
- The CLI uses the batch barrier everywhere, with no flag to change
  it: objects land atomically with a cheap plain `fsync(2)` each, and
  one `F_FULLFSYNC` at command completion makes the whole batch
  durable. Durability is one default behavior — anything `c4` prints
  (IDs, revert commands) refers to durable state.
- `storeManifestContent` stores objects on a bounded worker pool
  (min(GOMAXPROCS, 16)); `TreeStore.Put` is now safe for concurrent
  use (publish step serialized under the store mutex).
- Leaf split accounting is now O(1) amortized via cached per-directory
  counts instead of a `ReadDir` per Put.

Gate benchmark (M-series, APFS, 20k files / 105 MB, fresh store per
run, store contents verified identical and materializing byte-identical
trees): master 178–180 s → batch-barrier default **4.3 s**
(re-measured flagless). Small-repo case (182 files): 1.4 s → 0.05 s.

### Reconcile performance: 43x faster materialization

Materializing a 20,050-file / 105 MB tree via `c4 patch snap.c4m dest/`
took 164.6 s wall against 13.5 s CPU — the process was blocked, not
computing. Root cause: `store.DurableWriter.Close` issues a per-file
`F_FULLFSYNC` (a full device write-cache flush on darwin), serialized by
a fully sequential Apply. See `design/reconcile-performance.md`.

- `reconcile.WithSyncMode` — created-file durability mirroring
  `store.SyncMode`: `SyncEach` (library default, durable per file),
  `SyncBatch` (atomic writes + one device barrier at the end of
  Apply — the CLI's mode for reconcile forms), `SyncNone` (scratch).
- Apply now runs consecutive create operations on a bounded worker pool
  (`reconcile.WithMaxConcurrency`, default min(GOMAXPROCS, 16));
  counters and errors merge in operation order.
- `reconcile.WithTrustedMetadata(true)` — Plan reuses target IDs when
  size+mtime match instead of re-hashing every existing file (the
  guided-scan contract). Enabled by the CLI, matching its guided scan.
- `reconcile.LocalSource` (optional `ContentPath` on sources) —
  Apply copies file-to-file from local stores; `store.DurableWriter`
  gained `ReadFrom` so copies use OS acceleration (`copy_file_range`
  reflinks on Linux CoW filesystems). `store.NewAtomicWriter` is the
  non-fsync writer. `Folder`, `ShardedFolder`, `TreeStore`,
  `MultiStore`, and `DirSource` implement `ContentPath`.

Gate benchmark (M-series, APFS, 20k files / 105 MB, verified
byte-identical): master 171.2 s → per-file durable 129.3 s →
batch-barrier default **4.4 s** (re-measured with the barrier as the
only mode; reference `cp -Rc` whole-tree clone: 2.9 s). No-op
re-apply: 0.74 s → 0.30 s.

## v1.0.13

### Scan performance: quadratic propagation eliminated

Whole-tree metadata propagation (computing directory sizes / timestamps from
children) was `O(directories × entries)` because three separate
implementations — `c4m.propagateMetadata`, `scan.PropagateMetadata`, and the
duplicate in `cmd/c4/internal/scan/metadata.go` — each rescanned the full
entry slice once per null-sized directory. On a 5.14M-entry tree this caused
a 16–20 minute hang at 100% CPU on a fast single-user host.

Replaced all three with one canonical implementation in `c4m` using a
single-pass depth-stack accumulator. New behavior:

- `c4m.PropagateMetadata([]*Entry)` is now an **exported function** — the
  one true implementation that producers of `*c4m.Manifest` should use.
- Algorithm is `O(N)` work, `O(max-depth)` memory.
- Empty directories resolve to `Size = 0` and keep their null `Timestamp`
  (matched explicitly via a `hadChildren` flag — fixes a latent edge case in
  the original sketch where empty-dir timestamps would have rendered as
  `time.Time{}` instead of `NullTimestamp()`).
- Cheap whole-manifest early-out: if no directory has any null Size /
  Timestamp / Mode, the call returns immediately (the common case for
  `ComputeC4ID` re-running `Canonicalize` on an already-resolved manifest).
- Nil-infectious semantics preserved per `c4m/SPECIFICATION.md`: any
  descendant with null Size or Timestamp poisons the parent recursively.
  This **fixes a pre-existing spec violation** in the scan-package
  implementation, which silently skipped null values instead of poisoning.

Removed: `scan.PropagateMetadata`, `scan.CalculateDirectorySize`,
`scan.GetMostRecentModtime`, plus the same set in
`cmd/c4/internal/scan/metadata.go`. The scan packages call
`c4m.PropagateMetadata` directly. `c4m/readiness_test.go` now asserts
PropagateMetadata IS exported (was previously asserted unexported).

Result on the original reproducer (`/Users/joshua/ws/repos`, 6.08M entries,
323 GB): **16–20 min hang → 2 min 24 s** (dominated by filesystem I/O). On
122K entries: walk + assemble 2.63s → 0.72s (3.6×). All canonical bytes
byte-identical pre/post.

### `c4m.Manifest.SortEntries` linearized

`sortSiblingsHierarchically` was `O(N²)` worst case on pathologically
nested chains because each recursive `processLevel` rescanned the
remaining tail. Replaced with a single-pass children-by-parent index
built via depth stack, plus a sort-each-group / depth-first emit phase.

Pathological-chain bench (depth = N, one child each):

|    N |     Old |     New |  Speedup |
| ---: | ------: | ------: | -------: |
|   1K |  553 µs |  153 µs |     3.6× |
|  10K | 50.9 ms | 1.80 ms |      28× |
| 100K |  10.9 s | 33.3 ms | **327×** |

Normal tree shapes (sqrt-fanout): unchanged ~`O(N log N)`. Dedup-keeps-last
and tail-append-orphans semantics preserved.

### New: `scan.WithProgress`

```go
type ScanStats struct {
    Entries, Bytes, Files, Dirs int64
    Current string
    Elapsed time.Duration
}

scan.WithProgress(func(ScanStats))
```

Fires at most every 1000 entries OR every 250 ms, whichever first, plus
once at end. Zero overhead when not registered. The callback receives a
value-copy `ScanStats` so it's safe even with the parallel walk added
below. Sub-scans triggered for directory C4 ID computation in `ModeFull`
do not report (would double-count).

### New: `scan.WithMaxConcurrency` (bounded parallel subdirectory walk)

```go
scan.WithMaxConcurrency(n)
// n =  0: auto (min(GOMAXPROCS, 16))
// n =  1: purely sequential (preserves pre-parallelism behavior)
// n >  1: explicit cap
```

A single shared buffered-channel semaphore is created in `GenerateFromPath`
and inherited by all `clone()`d sub-scans so the cap is global, not
per-call. `generateDir` does a non-blocking `select` per subdirectory: if
a slot is free it launches a goroutine, otherwise it walks inline. This
guarantees forward progress on any tree shape, never deadlocks, never
oversubscribes the disk. Each subdir's result slice is written to a
pre-allocated slot in source order so the post-walk stitch is
deterministic — **output is byte-identical regardless of `n`**.

Bench on 122K-entry tree, cold cache: **2.53s → 0.45s (5.6×)**. Warm cache:
0.78s → 0.46s (1.7×).

### New: `scan.WithContext` and `scan.WithEntryStream`

```go
scan.WithContext(ctx context.Context)
scan.WithEntryStream(cb func(*c4m.Entry) error)
```

`WithContext` attaches a context whose cancellation is observed at every
directory boundary and between entries within a directory. A partial
manifest is returned on cancel.

`WithEntryStream` installs a per-entry callback that fires once per
discovered entry, before it is added to the manifest. Returning a non-nil
error halts the scan and is returned alongside the partial manifest.
Under parallel walk the callback may be invoked from multiple goroutines
but is serialized by an internal mutex, so callback bodies themselves do
not need to be thread-safe. Order is the discovery order of the worker
pool — pair with `WithMaxConcurrency(1)` if strict walk-order is
required.

When either option is set, `GenerateFromPath` / `Dir` return the partial
manifest alongside the error rather than `nil` — this is opt-in behavior
that preserves the historical nil-on-error contract for existing callers.

The callback is not invoked for entries emitted by internal sub-scans
used to compute directory C4 IDs in `ModeFull` — only top-level walk
entries are streamed.

### New: `c4m.Manifest.WriteCanonical(io.Writer) error`

Streams the canonical form one entry per line, so very large manifests can
be hashed (or otherwise consumed) without materializing the full canonical
text as a single string allocation. `Canonical()` is unchanged in behavior:
it now calls `WriteCanonical` into a `bytes.Buffer` for back-compat.
`ComputeC4ID()` now streams canonical bytes through `io.Pipe` into
`c4.Identify`, dropping bytes/op by ~23% on a 100K-entry manifest. Useful
for the multi-million-entry case where the eliminated allocation would
otherwise be hundreds of MB. C4 IDs are byte-identical to prior versions.

### Progressive scanner: wired `c4m.PropagateMetadata`

`cmd/c4/internal/scan/progressive_scanner.go` previously never called any
propagation pass — directories in its output had null `Size` (-1) and
sometimes null `Timestamp`, diverging from the synchronous scanner. Fixed
by inserting `manifest.SortEntries()` + `c4m.PropagateMetadata(...)`
inside `OutputCurrentState`, after the recursive `addEntriesToManifest`
walk and before encoding. The streaming property of the producer pipeline
is preserved — propagation runs only at emit, not during streaming.

*Correction (2026-11-11):* `cmd/c4/internal/scan` has not been linked by
the `c4` CLI since 2026-03-19, so this changed no output of any command.
The package was removed in the Unreleased section.

### Breaking changes

None for the c4m wire format or C4 ID values. Canonical bytes are
byte-identical pre/post for any tree that previously had no null
sizes/timestamps (the normal disk-scan case). The pre-existing spec
violation in `scan.PropagateMetadata` (permissive null handling) was
fixed in favor of the spec — manifests with mixed null/known sizes will
now propagate null to the root as the spec requires, where previously
the scan path would silently produce a partial sum.

Removed unexported helpers (`getDirectoryChildren`,
`calculateDirectorySize`, `getMostRecentModtime`, plus their
scan-package siblings) had no external users.

`c4m.PropagateMetadata` was renamed from the unexported
`c4m.propagateMetadata`. The previously-exported
`scan.PropagateMetadata`, `scan.CalculateDirectorySize`, and
`scan.GetMostRecentModtime` are gone — external callers (none were found
in the workspace) should migrate to `c4m.PropagateMetadata` (which is
the One True Implementation).

## v1.0.12

### Bug fixes

- Fix empty directory size reporting: size is 0, not null, when scanned

## v1.0.11

### Bug fixes

- Fix data races in ProgressiveScanner: added mutex for stage field and proper goroutine tracking for progressReporter (*correction, 2026-11-11:* in `cmd/c4/internal/scan`, which the CLI does not link; no CLI behaviour changed)
- Fix directory size calculation in internal scan: add c4m content size to match the public library behavior

### Code quality

- Fix typos in store package: mathods, cusomizable, contianed, writting

### Documentation

- Add `explain`, `paths`, `intersect` to CLI reference and README command table
- Create ARCHITECTURAL_CATALOG.md documenting package layout, types, and dependencies

## v1.0.10

### New: `c4 explain`, `c4 paths`, `c4 intersect` commands

Three new commands for querying and understanding c4m data:

- `c4 explain id|diff|patch` — human-readable narration of what a command would do, without modifying anything
- `c4 paths` — bidirectional conversion between c4m format and plain path lists
- `c4 intersect id|path` — find common entries between two c4m files, by content identity or by path

### Block link semantics

Patch chain checkpoints replaced with O(1) block links in the decoder and spec. Block links allow the decoder to verify chain integrity without scanning all preceding content.

### Directory size includes c4m content

Directory entry sizes now include the size of the c4m content that describes the directory, giving a more accurate picture of total storage.

### Documentation

- Block link semantics design doc
- Large directory blocks design doc
- c4m identity paradox doc: rationale, edge cases, and open questions

## v1.0.9

### c4m canonical identification

All ID paths now detect c4m content and canonicalize it before identification. This ensures that semantically identical c4m files always produce the same C4 ID regardless of whitespace or field formatting differences.

### Enhanced `c4 cat`

- `-e` flag for ergonomic (pretty-printed) output
- `-r` flag for recursive directory expansion
- File path support: `c4 cat file.c4m` reads from disk, not just from store

### Bug fixes

- `storeDirectoryC4m` now uses Canonicalize+Canonical to match ComputeC4ID

### Documentation

- c4m canonical storage design document
- c4m detection design doc: two-phase check for recognizing c4m content

## v1.0.8

### Bare `c4` shortcuts

- `c4 <path>` works as `c4 id -s`: identify and store in one step
- Bare stdin stores by default; `-x` flag skips storage
- CR (`\r`) bytes rejected in c4m input to enforce Unix line endings

### Auto-sort on read

The decoder now auto-sorts entries to canonical order on decode. If the input was not in canonical order, a note is emitted to stderr.

### Spec clarifications

- Null mode (`-`) vs zero mode (`----------`) are semantically different and must not be conflated

### Bug fixes

- Null mode rendering fixed in pretty-print encoder and paths output

### Documentation

- Design docs: auto-sort-on-read, bare-command-defaults
- Updated progressive-scanning design doc

## v1.0.7

### New commands

- `c4 explain` — human-readable narration for `id`, `diff`, and `patch` operations
- `c4 paths` — convert between c4m format and plain path lists (bidirectional)
- `c4 intersect id|path` — find common entries between two c4m files by content or path

### README and install

- Rewrite of install section with Homebrew, binary downloads, and from-source options
- Toolkit table showing the cross-language C4 ecosystem
- License corrected to Apache 2.0

## v1.0.6

Version bump only (no functional changes).

## v1.0.5

### Scan: nothing is skipped

The scanner no longer hardcodes a `.git` skip or hides hidden files. Everything is included by default. Exclusions are runtime-only via `--exclude`, `--exclude-file`, or the `C4_EXCLUDE_FILE` environment variable.

### License

Switched from MIT to Apache 2.0.

### Documentation

- FAQ: SHA-512 permanence rationale
- FAQ: text vs binary format rationale (2% overhead finding)
- README: merge example added
- FAQ: fix c4py repo URL

## v1.0.4

### New: Multi-target distribution

`reconcile.Distribute` reads a source directory once and writes to
multiple destinations simultaneously — directories, stores, or both.
Content is hashed during the single read pass. This is the library
primitive behind c4sh's multi-destination copy.

```go
result, err := reconcile.Distribute("/mnt/card/",
    reconcile.ToDir("/mnt/shuttle/"),
    reconcile.ToDir("/mnt/backup/"),
    reconcile.ToStore(myStore),
)
```

### Documentation

- Terminology cleanup: "c4m file" used consistently in user-facing docs
- Removed unqualified performance claims ("instant" → specific descriptions)

## v1.0.3

### New: Public scan package

The directory scanner is now a public package (`github.com/Avalanche-io/c4/scan`)
for use by ecosystem tools (c4sh, c4-mcp, etc.):

```go
m, err := scan.Dir("/path/to/dir")
m, err := scan.Dir("/path", scan.WithMode(scan.ModeFull))
```

### New: Quiet mode (`-q`)

`c4 id -q`, `c4 diff -q`, and `c4 patch -q` suppress stdout output
for commands run for their side effects (e.g., `c4 id -sq .` stores
content silently).

### Performance: Guided scan

`c4 diff` and `c4 patch` now use guided scanning when comparing a c4m
file against a directory. Only files with changed size or timestamp are
hashed — unchanged files reuse the C4 ID from the reference manifest.

### Bug fixes

- Directory timestamps now set correctly after reconciliation (deepest first)
- Existing directories get metadata updates, not just newly created ones
- Timestamp comparisons truncated to second precision at the c4m/filesystem
  boundary (prevents spurious diffs from sub-second differences)
- `isTerminal()` checks stdin, not stderr, for interactive prompts
- Store setup prompt appears before scanning, not after
- 12 code quality fixes: unchecked error returns on `Put()`, `Remove()`,
  `MkdirAll()`; TOCTOU race in content sourcing; `os.Lstat` for symlink
  safety; `filepath.Join` for Windows path construction

## v1.0.2

- `-q` (quiet) flag on `id`, `diff`, and `patch` for silent side-effect operations
- Directory metadata (timestamps, permissions) now set on existing directories during reconciliation
- Directory chtimes deferred to post-pass for both new and existing directories

## v1.0.1

- Guided scan optimization for `diff` and `patch` — only hashes files with changed metadata
- Timestamp precision fix at c4m/filesystem boundary (second vs nanosecond)
- Interactive prompt timing fix (before scan, not after)
- `isTerminal()` checks stdin instead of stderr

## v1.0.0

First stable release.

### CLI

Eight commands with consistent semantics — all accept c4m files or directories interchangeably:

- `c4 id` — Identify files, directories, or c4m files
- `c4 cat` — Retrieve content by C4 ID from store
- `c4 diff` — Compare two states, produce patches
- `c4 patch` — Apply target state: reconcile directories, resolve patch chains
- `c4 merge` — Combine two or more filesystem trees
- `c4 log` — List patches in a chain
- `c4 split` — Split a patch chain at a given point
- `c4 version` — Print version

### c4m Format

Formally specified manifest format (SMPTE ST 2114:2017) for describing filesystem trees as plain text. Includes:

- Entry-only format with no directives — works natively with grep, awk, sort, diff
- Patch chains for incremental versioning
- Media file sequence notation (`frame.[0001-0100].exr`)
- Inline range data for self-contained c4m files
- Progressive resolution: structure, metadata, and full scan modes
- Universal Filename Encoding for arbitrary byte sequences in filenames
- Flow links for cross-location data relationships

### Content Store

- Local: adaptive trie-sharded directory with atomic writes
- S3: any S3-compatible object store (AWS, MinIO, Backblaze, Wasabi, Ceph)
- Multi-store: write to first, read from all (`C4_STORE=a,b,c`)
- Zero external dependencies — S3 SigV4 signing implemented with stdlib

### Filesystem Reconciliation

`c4 patch` materializes c4m descriptions to real directories with:

- Pre-flight content availability check — never starts what it can't finish
- Idempotent operations — safe to re-run after interruption
- Move optimization — reuses content already present at different paths
- `-s` stores pre-patch state and removed content for reversal
- `-r` reverts to pre-patch state using stored manifest
- `--dry-run` previews changes without modifying the filesystem

### Platform Support

- macOS, Linux, Windows (amd64, arm64)
- Go 1.16+ minimum
- Zero external dependencies
