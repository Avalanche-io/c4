# The tree store's split race

Status: fixed 2026-11-16 (decided 2026-11-16 by the release lead;
reversible). Found as F2 of `proposals/storage/GC-REFUTATION-2026-11-16.md`.

## The fault

`TreeStore` keeps objects in a trie of two-character directories. A leaf
that passes the threshold (4,096 files) splits. `split` makes a
subdirectory for each next segment and renames each file into it, one
rename at a time. `path` descends into any subdirectory that exists, and
through 2026-11-16 a lookup looked nowhere else. The store's mutex is per
value, so per process. Processes sharing one store (several coding agents
snapshotting into `~/.c4/store`, two git commands on one c4git `.c4/store`)
saw two faults:

- **(a) Transient.** From a subdirectory's mkdir until a file's own rename,
  that file was invisible to every other process: `Has` false, `Open` "no
  such file". Measured on master b82ec07, at the default threshold: during
  one split, 4,173 to 5,077 of 12,288 `Has` calls on objects present
  throughout returned false, in every run.
- **(b) Permanent.** A writer that resolved its target before another
  process's split renamed into the old level, where no lookup looked again:
  `Put` returned an ID no store could find. At threshold 16, 8 writers of
  12 puts each: 87 of 300 rounds left 180 such objects.

A split cut short by a crash left the same shape as (b) for every file it
had not moved yet.

## The fix

A lookup that misses at the deepest path searches from the root down,
**looking for the file at each level before descending** (`foldFind`, which
already walked case-fold variants of each segment). The fast path, one stat
of the deepest path, is unchanged, so a hit costs what it did.

Why this is enough, with no lock across processes:

1. A file only ever moves deeper. `split` renames from a leaf into a
   subdirectory of it; `Settle` moves a file to the deeper place the trie
   resolves; nothing moves one up.
2. Shard directories are never removed.
3. So if the file exists while the lookup runs, consider the level the
   lookup is looking at. Either the file is there, and it is found, or it is
   already deeper. Then the next directory on its path exists, it still
   exists when the lookup looks for it, and the lookup descends into it. The
   lookup cannot reach the bottom without passing the file.

Looking below first and then at the level breaks this: the file can move
down between the two looks. `TestTreeStoreLookupOrderBeatsASplitMove` forces
that interleaving.

**A crash in the middle of a split** leaves some files in the old leaf and
some below, never a file in two places or none. Every one is found by (3).
The leaf is now an interior node, so it never splits again, and the files
left in it stay there. They are found, and `Settle` moves them.

**Already-damaged stores need no repair to read correctly** from this
version on. Measured on the 300 stores the publish reproducer damaged:
master's code missed 180 of 32,400 object files, and this code misses none.

## Rejected: making a split's files visible at their new place first

One could build each subdirectory under a temporary name, hard-link the
files in, rename it into place, then remove the originals. That closes (a)
but not (b): a writer that resolved the old level still lands there after
the split. It also needs hard links, which some filesystems a store lives on
lack. The lookup change closes both, and it reads stores that are already
damaged.

## Settle: a check for older readers

`TreeStore.Settle(move)` walks the store and reports every object file
above the level `path` resolves its ID to. With move set it renames each
one down, never over an object already at its place (reported as a
duplicate, nothing removed). This version doesn't need it. Readers built
before the fix (c4 1.0.26 and earlier, and tools built on that library)
still miss misplaced files, so a store shared with them is worth settling.
On the 300 damaged stores it found exactly the 180 and moved them, and
master's code then missed none. `go run ./hack/storesettle [-move] DIR` runs
it. It is not a `c4` verb: none of the existing verbs is a store check, and
a new one needs the maintainer's yes.

## How often a split happens

This replays the store's own rule over 3 million random C4 IDs at the
default threshold. The root splits into `c4/` at 4,097 objects, and `c4/`
splits into about 297 leaves right after, since every ID starts with `c4`.
The next splits come at about 1.16 million objects, all 297 leaves between
1.16M and 1.5M.

A first snapshot of a repo stores about one object per file and per
directory: about 410 for c4, about 2,300 for c4-operator. Each later
snapshot adds the changed files and the listings on their paths, about 15
for a handful of edits. So a store makes its early pair of splits once,
after something like 120 to 250 snapshots of one repo, sooner when several
repos share the store. Nearly every store in use has passed it. The
exposure was one window per store, of roughly the time it takes to rename
4,096 files, open to any other process using the store at that moment.
After it, the next split is of the order of 77,000 snapshots away.

## On the 1.0.x line

Back-ported to release/1.0.27. That line's lookup code is master's, so
the change is the same; Settle there has no batch-barrier bookkeeping,
which the line does not have. Measured on release/1.0.27 eb33848 before the
change: during one split, 4,383 to 4,753 of 12,288 lookups of present
objects missed; 18 to 28 of 60 publish rounds lost 40 to 60 objects. After:
none. On the 300 damaged stores, this line's code misses none without
repair, and Settle moves exactly the 180.
