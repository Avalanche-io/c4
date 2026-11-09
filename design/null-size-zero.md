# A size of 0 beside a C4 ID that is not the empty content's is null

Status: ruled by Joshua 2026-11-08; implemented on branch
`null-size-zero`, not landed.

## The ruling

Joshua, 2026-11-08, on how a null size behaves on the storage mount:

- A file with a null size and no C4 ID can't be fulfilled.
- With a C4 ID, find its size, from a remote source if need be.
- If no source can give a size, 0 is the only value a filesystem can
  show. That would break the round trip, but the C4 ID of empty content
  is known
  (`c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT`),
  so a size of 0 alongside any other C4 ID means null. "So in that
  special case the null can round trip."

## Why it is sound

Only the empty content has size 0. A correct scan that hashes a file's
bytes and records their count can never write `0` beside a C4 ID of
non-empty content; the combination is a contradiction. Reading it as
null therefore changes the meaning of no honest record. It only gives a
meaning to a line that a filesystem surface is forced to write, because
a surface has no `-`.

The same holds for directories: an empty directory's listing is zero
bytes, so its size is 0 and its C4 ID is the empty content's; a
directory with children has a size of at least its listing's byte length.
`0` beside any other ID on a directory line is null too.

Measured on the installed 1.0.24 before writing this (2026-11-08):

```
-rw-r--r-- 2026-11-08T14:46:02Z 6 a.txt c45dkQ…
-rw-r--r-- 2026-11-08T14:46:02Z 0 empty c459dsjfsc…        empty file: 0, empty-content ID
lrwxr-xr-x 2026-11-08T14:46:02Z 5 link -> a.txt c45dkQ…    symlink: its own length, target's ID
drwxr-xr-x 2026-11-08T14:46:02Z 0 emptydir/ c459dsjfsc…    empty dir: 0, empty-content ID
```

Two consequences of that measurement:

- **Symlinks are exempt.** A symlink's size is the link's own and its
  C4 ID is the target's, so `0` and a non-empty ID could in principle
  meet on a link line without contradiction (they do not in practice,
  since a symlink's target is never empty, but the fields do not
  constrain each other). The spec's symlink example showing `0` is
  illustrative, not what the scanner writes.
- **`0` with no C4 ID is a known zero, not null.** c4-fleet asked
  whether it should read as null by the same test, and my first answer
  was yes. That was wrong: a structure-level scan (`c4 id -m s`) records
  every file's size and no C4 ID, so an empty file reads `0 name -`
  legitimately, and the scanner knew it was empty from `stat`. The
  "truly empty files always carry the empty-content ID" argument only
  holds when an ID was computed at all.

## Where the rule lives

One implementation, `Entry.SizeIsNull()` in `c4m/entry.go`:
`Size < 0`, or `Size == 0` beside a non-nil C4 ID that is not
`EmptyContentID` on a non-symlink entry. Everything that asks whether a
size is known asks it:

- **Decoder.** After the entry is assembled, a `0` the rule calls null is
  held as -1, so every consumer (restore's trust check, reconcile's
  projection, `HasNullValues`) sees the one internal value for the one
  meaning.
- **Canonical form and the encoder.** Both write `-` for it. So
  `0 f <ID>` and `- f <ID>` are one description with one ID, and an
  entry built through the API with `Size: 0` beside a real content ID
  (as a scan over a mount would build it) hashes the same as one decoded
  from `-`. Pinned by `TestNullSizeZeroOneID`.
- **PropagateMetadata.** A `0` the rule calls null infects its parent's
  size like any null. Pinned by `TestNullSizeZeroPropagates`.
- **`ListingRecord`** now names `EmptyContentID` rather than recomputing
  it; no behaviour change.
- **The normaliser** inherits all of this: `c4 id FILE.c4m` on a file
  carrying `0 f <ID>` writes `- f <ID>`, and `c4 id -l` lists the file.
- **The scanner** is unchanged. It records `stat`'s size and the hashed
  bytes' ID; on an ordinary filesystem those agree. The one way it can
  disagree is a file that grew between `stat` and `read` with a stat
  size of exactly 0, and the rule turns that from a wrong record into a
  null one. Recording the hashed byte count instead of `stat`'s size
  would close that race for every size, not just 0; proposed separately,
  not done here.
- **The mount** (not in this repo). The capture side writes `-` when it
  knows the size is null, as any c4-aware writer does; the rule exists
  for the surface it serves to clients that have only numbers.

## What a null size does on the mount, end to end

| Entry on the mount                       | Shown      | Opened                                  | Scanned back by a client                                        |
| ---------------------------------------- | ---------- | --------------------------------------- | --------------------------------------------------------------- |
| size known                               | the size   | content by ID                           | round-trips                                                     |
| size null, C4 ID, some source has bytes  | the size the source gives | content by ID               | round-trips with the found size (a refinement, as the primer allows) |
| size null, C4 ID, no source has bytes    | `0`        | refused at the earliest call the protocol has (see below) | `0 name -` plus a read error: partial scan, exit 2 (#64); never a quiet empty file |
| size null, no C4 ID                      | `0`        | refused at the earliest call the protocol has (see below) | same as above                                                   |

The third row is where Joshua's "special case" round trip lives, and it
holds only for a c4-aware writer that knows the ID: the client's scan
cannot recover the ID of bytes it could not read, so it records the read
error instead. That is the right outcome: the client learns that the
file is not there, not that it is empty.

**Where the refusal happens** (amended 2026-11-09 after c4d's review).
An earlier draft of this table said the mount "refuses to open". That
assumes an open call. NFS v3 has none, and any client that sees size 0
skips READ altogether, so a refusal placed at open or at the first read
never fires and `cp` and `cat` would write quiet empty files, exactly
what the ruling forbids. The intent of the ruling is that an
unfulfillable entry is never served as content, so the refusal belongs
at the **earliest call the protocol has**: a surface with no open MUST
NOT present an unfulfillable entry as a size-0 regular file; it refuses
at lookup or getattr with the protocol's "unreadable" error (EIO or its
equivalent), so a listing shows the entry as unreadable and a copy fails
loudly. Hiding the entry from the listing is the other option and is
rejected: it lies about existence, and a client's scan would then record
a deletion.

**Measured by c4d, 2026-11-09** (macOS 27.2 NFS v3 client, c4fs behind
absnfs, a c4m made with the package with README.md's size null, empty
store, c4 master 6aa19d8):

- *Door serving size 0 as a regular file:* `stat` 0, `cat` exit 0 and
  nothing, `cp` a 0-byte copy, `shasum` the empty hash, and
  **`c4 id -q README.md` prints the empty-content ID with exit 0**. No
  open or read of the name reached the server. A scan of the mount
  records `0 README.md c459dsjf…`, and a symlink to it gets the
  empty-content ID too. **So the rule in this note cannot rescue a
  client's scan of an open-less door:** the scan writes `0` beside the
  empty-content ID, which is a known empty file by this very rule. The
  round trip the ruling describes holds only for the c4-aware writer
  that knows the entry's ID; a client that reads bytes records the bytes
  it got. The control (`secret.txt`, size 8, not local) failed properly
  in every tool except `shasum`.
- *Door answering EIO to lstat and open:* every tool is loud, including
  `c4 id -q` (exit 1, "lstat README.md: input/output error"); no quiet
  empty file anywhere. **But the entry vanishes from the listing**, because
  absnfs's READDIR and READDIRPLUS look each name up and skip the ones
  that fail; `ls` omits it, a scan has no line for it (reads as a
  deletion), and a link to it is recorded with a null ID.

So the rule for an open-less door is two-part: fail LOOKUP and GETATTR
for the entry (so every tool is loud), **and keep the name in
READDIR(PLUS)** with no attributes and no handle, so the listing still
says the entry exists. That needs a change in absnfs (c4d has told absfs)
and then a measurement of what `ls -l` and Finder show for such an entry.

**Measured again (absnfs #92, c4d, 2026-11-09 16:14 CST):** with the
name kept in READDIR(PLUS) and no attributes or handle, macOS `ls` and
`find` do not show it at all (the client's bulk-attribute path skips an
entry it cannot describe); `getdirentries` returns it and `stat` gives
EIO. So on NFS v3 with the macOS client the two-part rule is honest to a
program reading the directory raw and invisible to `ls`, Finder and a
`c4 id` scan: the "reads as a deletion" outcome after all. The remaining
option there is to let LOOKUP and GETATTR succeed (size 0, as ruled) and
refuse at ACCESS, the one call the client makes before letting a program
open the file; whether the macOS client honours an ACCESS refusal for a
file its owner may read is not yet measured. On SMB the door has a real
open, and absfs measured that refusal as loud in every tool.

**The rule as it stands, then:** where the door has an open, or an
access check the client honours, refuse there and list the entry with
size 0. Where it has neither, the entry cannot be both listed and safe;
hiding it, with an honest error on any direct access by name, is the
lesser harm, and the capture side must still carry the entry (it knows
the ID), so the hiding is a property of that door, not of the
description. The ACCESS measurement decides which case NFS v3 is in.

## Cost

- **Every implementation** (c4py, c4ts, c4m-swift, libc4) must apply the
  rule in its decoder and canonical form, or its description IDs diverge
  from Go's on any file carrying `0` beside a real content ID. The
  conformance vector for it is the three-line table in
  `TestNullSizeZero`; a fixture file should join `c4m/testdata` when the
  ports begin.
- **No released file changes meaning.** No shipped scanner writes the
  combination, so no existing description reads differently.
- **The spec's symlink example** (`0 link.txt -> target.txt`) remains
  stale about the size the scanner records; left for a separate docs pass.
