# A store call that takes over a sealed file by moving it

Status: design, 2026-11-10, for Joshua. Nothing built. From the storage
project's review (item 13): "`Put` streams, so a large file is written
twice. It has to be a move of a file nothing else can reach: the operator's
`Adopt` hard-links a live one, and the cache served wrong bytes under a C4
ID as a result."

## What exists

- `Put(r io.Reader) (c4.ID, error)`: reads everything, hashes while writing
  a temp file in the store root, publishes by rename (after the batch
  barrier under `SyncBatch`). A file already on disk is read once to hash
  and written once more as the temp: two passes over the bytes and twice
  the space during ingest. For a sealed multi-gigabyte render output
  already on the store's volume that is the whole cost of ingest.
- `Create(id)`: a writer for bytes whose ID the caller already knows; same
  temp-and-rename shape.
- The operator's cache `Adopt`: hard-links a file into the cache under its
  ID. A hard link shares the inode with the original, so a later write to
  the original changes the bytes the cache serves under an ID that no
  longer names them (measured in the operator, fixed there by not
  aliasing live files).

## The contract that makes a move safe

A move is safe exactly when, after it, nothing but the store can reach
the bytes. The checks below came out of the operator's own move-in fix,
where each missing one cost a review (storage project, 2026-11-10).

**Check the file you hash, not the path.** `Lstat`, then hash by path,
then `rename(2)` by path has two gaps: between the stat and the open the
name can become a symlink, so the hash reads the link's target while the
rename moves the link, and the store ends up holding a symlink under the
target's ID; and a parent directory can be swapped the same way. So the
procedure is descriptor-first:

1. Open the path once with `O_NOFOLLOW|O_NONBLOCK` (`O_NONBLOCK` because
   a name that became a FIFO would otherwise block the open forever).
2. `fstat` the descriptor: a regular file (not a link, FIFO, socket or
   device), link count 1 (on Windows `GetFileInformationByHandle`'s
   `NumberOfLinks`), no file flags set, owned by the store's user (below).
   Refuse on any failure.
3. Rename the path into the store root under the temp name
   (`.ingest.<host>.<pid>.*`) **before hashing**: from then on no later
   open by path can reach the bytes, which stops the common accident (a
   tool that reopens its output to append). An open handle survives the
   rename either way, so this closes the path-based cases only.
4. Hash through the descriptor.
5. Confirm the file now at the temp name is the same device and inode as
   the descriptor (`fstat` versus `Lstat` of the temp), then publish as
   `Put` does.

**One name is necessary, not sufficient.** Link count 1 says no second
path exists; it does not say no one holds the inode open. The operator's
live case had link count 1 and was still reachable: an overlay's upper
file, through an overlay mount that was busy and only detached. A process
that already has the file open keeps writing the inode the store now
serves. That cannot be checked from outside, so it is the caller's stated
promise: *nothing holds this file open for writing, and no mount still
reaches it*. A caller who cannot make the promise copies instead (`Put`);
the operator's `Consume` flag is exactly that statement by the caller,
and `Take` should take the same shape: the caller asks for a move, never
gets one by default.

**Flags and extended attributes ride along with a move.** On macOS a
locked file (`uchg`) makes the rename or the store's chmod fail part way;
a transparently compressed file (`UF_COMPRESSED`) reads fine but must
never have its flags cleared (the operator did that to a clone and got an
empty file, no error). Extended attributes (quarantine, Finder info, a
resource fork) would then sit on a store object. Rule: a file with any
flag set is refused (the caller may copy); extended attributes are
stripped from the stored object, since a store object is bytes and
nothing else, and the file's own metadata lives in its c4m entry.

**Ownership.** Anyone who can write both directories can rename another
user's file into the store, after which the store cannot chmod it and its
owner still can. A file the store's user does not own is refused (the
caller may copy).

With those, the bytes have exactly one name, the store's, nothing can
reach them by path, the caller has said nothing reaches them by handle,
and the ID the store computed is the ID of what it serves.

## The call

```go
// Take moves a sealed file into the store and returns its ID. The caller
// states that nothing holds the file open for writing and no mount still
// reaches it; Take checks what can be checked through one descriptor
// (regular file, one name, no file flags, owned by this user), renames
// the file into the store before hashing it, hashes through that
// descriptor, and confirms it moved the file it hashed. It refuses a
// file on another volume (copy it with Put), a file with more than one
// name, a symbolic link, a flagged file, or another user's file; the
// path does not exist when Take returns.
func (s *TreeStore) Take(path string) (c4.ID, error)
```

- One read pass, no second write: the rename is the move, the hash is
  the only pass over the bytes.
- Rename first, into the store root as the temp (`.ingest.<host>.<pid>.*`
  name, so a crash is swept like any other), then hash through the
  descriptor, confirm device and inode, then the usual publish.
  Renaming a file from outside the root into the root's temp name is
  itself the move; the pending map and `Sync` then treat it like a `Put`.
  Both directories involved in the rename are synced (the source's by
  `Take`, the store's by `Sync`, which #86 made fsync the directories it
  renames into).
- Different volume: `Rename` fails with `EXDEV`; `Take` refuses with an
  error that names the volume difference and leaves the file in place.
  A copy is `Put`, which the caller chooses explicitly; a move never
  silently becomes a copy of a large file.
- Already present: if the store already holds the ID, the source is
  removed **only after the same descriptor checks have passed and the
  hash through that descriptor matched**; otherwise `Take` would be a
  way to delete a file through a symlink or to delete someone else's.
- Batch mode and the sweep: a taken file waiting for the barrier is a
  `.ingest.*` temp like any other; a crash before the barrier loses the
  move, and the sweep removes the temp. The caller's file is gone in that
  case, so **under `SyncBatch`, `Take` must `Sync` before returning** or
  the caller must be told the move is not durable until the next `Sync`.
  Recommendation: `Take` syncs its own object (one fsync, as `SyncEach`
  would) and publishes it immediately, regardless of mode; a move is a
  commitment to the file's owner that `Put`'s copy semantics never were.

## Not this

- No hard links into the store, ever (`store-index-and-links.md` keeps
  links for the store's own index, not for ingest). The operator's bug is
  the demonstration.
- No `Take` on a `MultiStore`: a move has one destination; the caller
  names the `TreeStore`.
- No ownership, mode, flag or extended-attribute preservation: the
  store's object has the store's mode (0444 as today) and no attributes,
  and the file's own metadata lives in the c4m entry, never in the store.
- No silent copy: a move that cannot be a move is refused, and the caller
  says `Put` if that is what they want.

## Open for Joshua

- The name: `Take`, `Adopt` (taken by the operator for the thing this
  replaces), `MoveIn`, `Absorb`.
- Whether `Take` should ever copy (cross-volume, flagged, foreign-owned).
  Recommendation in the text: never; the caller says `Put`. An option to
  copy would make the surprise possible again.
- Whether the link-count check should be an error or a warning on
  filesystems that do not report it (some network filesystems report 1
  for everything). Recommendation: error when the count is known to be
  greater than one; proceed when it is unknown, and say so.
