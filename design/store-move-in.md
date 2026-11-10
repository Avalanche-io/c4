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
the bytes. Three conditions, each checkable before the rename:

1. **One name.** The file's link count is 1 (`Lstat`, `Sys()` nlink on
   Unix; on Windows the equivalent is `GetFileInformationByHandle`'s
   `NumberOfLinks`). A file with two names has a second path that outlives
   the move and can write through to the stored bytes. Refuse.
2. **A regular file, not a link.** `Lstat` mode is regular. A symlink's
   target is someone else's file. Refuse.
3. **The caller gives the path up.** After the move the path does not
   exist. The call's name says so, and the doc says the caller must hold
   no open writable handle (an open descriptor survives a rename and can
   still write; that is unverifiable from outside, so it is the caller's
   promise, stated).

With those, the bytes have exactly one name, the store's, and the ID the
store computed is the ID of what it serves, forever.

## The call

```go
// Take moves a sealed file into the store and returns its ID. The file
// is hashed in place, then renamed to its hash name when it is on the
// store's volume, else copied and removed. Take refuses a file with more
// than one name (hard links), a symbolic link, or a path the caller
// could not give up; the path does not exist when Take returns. The
// caller must hold no writable handle on the file.
func (s *TreeStore) Take(path string) (c4.ID, error)
```

- Hash in place: one read pass, no second write when on the same volume.
- Rename: into the store root as the temp (`.ingest.<host>.<pid>.*`
  name, so a crash is swept like any other), then the usual publish
  (immediately under `SyncEach`; after the barrier under `SyncBatch`).
  Renaming a file from outside the root into the root's temp name is
  itself the move; the pending map and `Sync` then treat it like a `Put`.
- Different volume: `Rename` fails with `EXDEV`; fall back to copy and
  remove, which is `Put` plus `os.Remove`, and say so in the error type
  or a return flag, since the caller may prefer to refuse a copy of a
  large file.
- Already present: if the store has the ID, remove the source and return
  the ID (the caller gave the file up; keeping it would be a second name).
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
- No ownership or mode preservation: the store's object has the store's
  mode (0444 as today), and the file's own metadata lives in the c4m
  entry, never in the store.

## Open for Joshua

- The name: `Take`, `Adopt` (taken by the operator for the thing this
  replaces), `MoveIn`, `Absorb`.
- Whether `Take` should ever copy (cross-volume) or always refuse and let
  the caller choose `Put` explicitly. Recommendation: refuse by default,
  copy under an option, so a caller never pays a surprise copy.
- Whether the link-count check should be an error or a warning on
  filesystems that do not report it (some network filesystems report 1
  for everything). Recommendation: error when the count is known to be
  greater than one; proceed when it is unknown, and say so.
