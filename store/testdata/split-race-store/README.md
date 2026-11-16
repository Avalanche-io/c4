# Fixture: a split tree store with one object a level too high (2026-11-16)

The shared fixture for the tree store split race (`design/store-split-race.md`).
A tree store keeps objects in a trie of two-character directories, and a
leaf that passes its threshold splits, moving its files one level down.
Through 2026-11-16, a write that raced another process's split could rename
its object into the old level after the split had made the deeper
directory. A lookup that went straight to the deepest directory that exists
then never found it again. Every store reader (Go `store.TreeStore`, c4ts's
store layout, c4py's FSStore) MUST pass it.

## Files

| File            | Made by                                                                                                                       |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `store/`        | Go's `TreeStore` at split threshold 4: `Put` of `object 1`, `object 2`, … (see below), then one object moved up a level      |
| `objects.txt`   | one line per object: its C4 ID, a space, its exact bytes                                                                      |
| `misplaced.txt` | the C4 ID of the object one level too high                                                                                    |

Nothing is placed by hand.
- **The contents:** `object N`, taken in order of N. Any content whose first shard segment (characters 3 and 4 of its ID) equals another's under case folding is skipped, so the tree is the same on case-insensitive and case-sensitive filesystems. At least twelve are taken, and they keep being taken until two objects share a shard directory.
- **The misplaced object:** the first object by ID that shares its directory with another. It is moved from `c4/XX/<ID>` to `c4/<ID>`, the inverse of the move `TreeStore.Settle` makes, and the place the race was measured to leave such an object. Its directory keeps the other object, so the directory survives in git and the deeper level still exists.
- **Regenerating:** `C4M_UPDATE_VECTORS=1 go test ./store -run TestSplitRaceStoreFixture` rewrites the files. The test then fails if they differ from what the library writes.

## What a conforming reader MUST do

Open `store/` as a tree store, read-only or on a copy.

- **Find every object.** Every ID in `objects.txt` is present: `Has`/`exists` true, and reading it returns exactly the listed bytes, which hash to the ID. That includes the one in `misplaced.txt`, which is at `c4/<ID>` although `c4/XX/` exists for its segment.
- **Look at every level on the way down.** A lookup that misses at the deepest existing directory looks for the file at each level from the root down, checking each level before descending into the next. Here, a reader that looks only at the deepest directory misses exactly the misplaced object, and every other object reads there.
- **Settle, if implemented,** finds exactly one object above its level (the misplaced one) and moves it to `c4/XX/<ID>`. After that, the exact-path reader finds it too.
- **Write nothing** to the fixture while reading it.

Go runs all of this in `TestSplitRaceStoreFixture`.
