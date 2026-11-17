# A skip rests on a name that is durable

2026-11-16. Requirement S13 of the storage project's collection design
(revision 4, section 7). It matters to every user today, not only to a
future collector: it is the print barrier.

## The fault

A tree store stores an object by writing a temp file, flushing it, and
renaming it to its C4 ID. A rename is a change to a directory, and it
reaches stable storage only when that directory is synced. A writer that
finds the object already there skips its own write. Through 2026-11-16 no
skip synced anything:
- `Put`'s skip-if-present returned at once;
- a batch whose objects were all present left `Sync` with nothing to do
  (`dirty` false, nothing pending), so the barrier returned at its first
  line;
- the batch `Sync`'s own skip, `Create`'s batch writer, and the
  present-means-success rule for a refused rename (c4 #179) also returned
  without syncing;
- a `SyncEach` `Put` or `Create` synced the object's bytes and never its
  name.

`c4 id -s` stores with `SyncBatch`, calls `Sync`, then journals the claim
and prints it. Suppose the name it skipped on was another writer's, not yet
synced: a `SyncNone` writer, a `SyncEach` writer (whose names were never
synced), or a batch writer that died between its renames and its directory
sync. Then a power cut could leave a durable claim whose content no store
finds.

## The rule

Every name a writer relies on, published or skipped, is durable by the
time the writer's claim can be made. `TreeStore.holdName(dir)` does it at
every such place:
- **SyncBatch:** mark the directory touched and the batch dirty. The
  barrier syncs every touched directory and its ancestors to the root,
  once per batch however many objects rest on them, then flushes the
  device.
- **SyncEach:** sync the directory chain, then flush the device, before
  `Put` or `Create`'s `Close` returns.
- **SyncNone:** nothing. Objects stay complete or absent (atomic, not
  durable), by stated choice.

The places are:
- `Put`'s skip;
- `Put`'s publish, including when another writer's publish won the race;
- the batch `Sync`'s skip and its "another writer published it" path;
- `Create`'s batch writer's skip;
- `Create`'s `SyncEach` writer.

The directory is the found copy's, which a racing split may have put
somewhere other than the writer's own target.

## Tests

- **`TestSkippedObjectSurvivesAPowerCut`** (external `store_test` package,
  so it can journal through `c4m`):
  1. A `SyncNone` writer stores X.
  2. A batch store Puts X (a skip), runs `Sync`, and the claim is journaled.
  3. Every object whose directory chain was not synced afterwards is
     removed, as a power cut would lose its entry.
  4. `c4 cat`'s resolution (`OpenStore` with `C4_STORE`, then `Open`) must
     find X.

  On a flat store and on a split one, it fails on the base and passes
  with the fix.
- **`TestTreeStoreHoldsEveryNameItRestsOn`:** eight routes, with the object
  two levels down. Each asserts that the chain was synced before the claim
  could be made. All eight fail on the base.

## Cost

`TestSyncCost` with `C4_SYNC_COST=1`, at load 10 to 18:

| Case | Before | After |
| --- | --- | --- |
| a batch of 3,000 objects, all present: `Sync` | 500 ns | 31.7 ms |
| a batch of 3,000 new objects: `Sync` | 647 ms | 620 ms |
| a `SyncEach` `Put` that publishes | 15.7 ms | 12.3 ms (noise) |
| a `SyncEach` `Put` that skips | 6.0 ms | 9.8 ms |

A `SyncEach` skip already paid a full flush before this change: `Put`
writes and flushes its temp file before it knows the ID.

## Not covered here

The journal's first append on a store creates `<root>/journal` and syncs
the file but not the root's entry for it. It is the same class of gap for
the journal itself, and is left to the lead's decision.
