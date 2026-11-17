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

A lookup that returns a path is a look, not a hold: the file can move down
between the look and the use. `Open` therefore searches again when opening
the path it found says "no such file", and `Remove` does too. Each retry
follows a file that moved deeper. CI caught `Open` failing once during a
split after the lookup fix and before this
(`TestTreeStoreOpenFollowsAMove` forces it). `ContentPath` returns a path as
it stands; a caller that opens it later can get "no such file" if a split
moved it, and should look again.

**A crash in the middle of a split** leaves some files in the old leaf and
some below, never a file in two places or none. Every one is found by (3).
The leaf is now an interior node, so it never splits again, and the files
left in it stay there. They are found, and `Settle` moves them.

**Already-damaged stores need no repair to read correctly** from this
version on. Measured on the 300 stores the publish reproducer damaged:
master's code missed 180 of 32,400 object files, and this code misses none.

## The premises, checked against the code (2026-11-16)

**An object only ever moves deeper.** Every `os.Rename` in store/ and
cmd/c4:

| Where                            | What it renames                                   | Direction                        |
| -------------------------------- | ------------------------------------------------- | -------------------------------- |
| `split`                          | `dir/<ID>` to `dir/XX/<ID>`                        | one level down                   |
| `Settle`                         | a misplaced object to `path(id)`                   | down only (`deeper` is checked)  |
| `Put`, `Sync`                    | a temp file to the resolved path                   | publishes a new object; no move  |
| `DurableWriter.Close` (`Create`) | a temp file to the path resolved at Create        | publishes a new object; no move  |

cmd/c4 renames nothing inside a store.

**No shard directory is ever removed.** There is no `os.RemoveAll` in
store/. Every `os.Remove` in store/ names a temp file or an object file:
- the `DurableWriter`, `pendingWriter` and `Put` temp clean-ups;
- `SweepIngest`, which removes only `.ingest.*` temps in the root;
- `Remove(id)`, which removes `path(id)` or what `foldFind` returns, both
  ending in the ID.

cmd/c4 removes nothing inside a store; reconcile's removes act on the
restore target. `TestTreeStoreSplitLeavesItsLevel` checks both premises
over 400 Puts of real splits. The code states them beside `foldFind`.

## What lives in a store root, and what a split did to it

| File                                    | Before 2026-11-16                                 | After                    |
| --------------------------------------- | ------------------------------------------------- | ------------------------ |
| objects (`c4…`)                         | moved to `c4/`                                    | moved to `c4/` (as meant) |
| `journal` (c4m.JournalName; locks itself) | **moved to `jo/journal`**; claims dropped out    | stays                    |
| `.ingest.<host>.<pid>.*` (Put, Create temps) | skipped (dot file)                           | skipped                  |
| `.tmp.*` (DurableWriter temps, in leaves) | skipped (dot file)                              | skipped                  |
| `config`, if the store is rooted at `~/.c4` | moved to `co/config`; read where it lies (corrected 2026-11-16, below) | stays |
| any other file a user leaves there      | moved into a two-character directory              | stays                    |

The journal locks its own file, so the root holds no separate lock file.
`SweepIngest` writes nothing. A split now moves only files whose names
parse as C4 IDs (`TestTreeStoreSplitMovesOnlyObjects`).

**A journal an earlier split displaced is read where it lies.** `Claims`
reads `jo/journal` first, if it begins with the journal magic, then the
root's. A root split puts it there, and no object name starts `jo`, so
nothing else is filed there. Every claim before the split is in the
displaced file and every claim after it in the root's, so the two read in
that order are the history in order. Putting it back was rejected: the
root journal has been restarted by then, so putting it back means merging
two files under the lock and renaming over a file other processes may
hold open. A crash, or a writer with the old file open, could lose a claim
that way. Reading in place writes nothing.

## A writer at a stale level, after the fix

It can still happen. `Put` resolves its path under its own process's lock,
and another process's split can come between that and the rename. In
`TestTreeStoreStaleLevelWriters` (8 processes, threshold 16), 12 to 19 of
40 rounds per run left an object above its level. What it leaves is one
of two things:
- an object at an interior level, which every lookup finds;
- a second copy of an ID already stored. Both copies are complete and
  hash to the ID, because a name is only given after the hash check.

Two processes publishing the same new ID at once, at different levels,
make the second case. None arose in 120 rounds, since each Put's dedup
check found the first copy; `TestTreeStoreSettle` makes one directly.

Nothing is lost or corrupted:
- every lookup reads either copy;
- `Settle` never moves over an object already at its place;
- `Remove` now deletes every copy on the ID's path. Through 2026-11-16 it
  deleted the first it found, and `Has` went on finding the other.

The test checks every ID's bytes after the writers and after `Settle`, and
that `Remove` leaves nothing `Has` finds.

## The shared fixture

`store/testdata/split-race-store/` is a store Go split at threshold 4,
with one object moved one level up. That is the inverse of `Settle`'s
move, and the place the race was measured to leave one. Its README says
what a conforming reader must find; the ports (c4ts, c4py) read it.
`TestSplitRaceStoreFixture` regenerates it from the library and checks it.

## The cost of a true miss

A hit costs what it did: the fast path, one stat of the deepest path,
finds it. A miss (a Has for an absent ID, as every Put's dedup check and
every diff against a store makes) now also stats the file name at each
interior level on the way down: one extra stat per level. Counted
through the store's stat seam, 20,000 absent IDs each, on APFS
(case-insensitive, so the case-fold variants are probed too):

| Depth of the leaf | Stats per miss before | After |
| ----------------- | --------------------- | ----- |
| 1 (`c4/`)         | 7.8                   | 8.8   |
| 2 (`c4/XX/`)      | 12.2                  | 14.2  |
| 3 (`c4/XX/YY/`)   | 17.0                  | 20.1  |

Depth 2 is every store from about 4,098 objects to about 1.16 million. On
such a store (4,296 objects, default threshold), 100k absent-ID Has calls
took 3.9 to 4.0 s before and 4.7 to 5.1 s after, about 40 µs to 47 to 51 µs
each, two runs each. That is about 1 s per 100k new objects, beside a
hash, a write and (by default) a flush for each one. It does not matter
next to what a Put costs. The extra look cannot be skipped: it is what finds
an object one level up.

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

Back-ported to release/1.0.27. The lookup code there is master's, so the
lookup, `Remove`, `Open` and the objects-only split are the same change.
`Settle` there has no batch-barrier bookkeeping, which the line doesn't
have. Measured on release/1.0.27 eb33848 before the change:
- during one split, 4,383 to 4,753 of 12,288 lookups of present objects
  missed;
- 18 to 28 of 60 publish rounds lost 40 to 60 objects.

After: none. On the 300 damaged stores, this line's code misses none
without repair, and `Settle` moves exactly the 180.

This line has no journal, so the journal fault and the displaced-journal
read are master-only. Its store root holds objects and `.ingest.*` temps
(`.tmp.*` temps live in the leaves); a split skipped the dot files. The
objects-only rule is still taken here, because the old split moved any
other non-dot file: `~/.c4/config` in a store rooted at `~/.c4`, or a file
a user left in a leaf. This line has no `SweepIngest`, so the root's only
removes are temp clean-ups.

**What reads `~/.c4/config`, corrected 2026-11-16.** The table above said
the moved config "read nothing". That holds only while `C4_STORE` is set:
the config is read by `configuredEndpoints` alone (its `store` lines), and
only when `C4_STORE` is unset; `AutoStoreEnabled` reads `store_auto` but
nothing calls it. A config that itself says `store = ~/.c4`, with
`C4_STORE` unset, is read, and the root split lost it. Measured
(`HOME` a scratch directory, 4,200 objects stored with `c4 id -s`):
released 1.0.26 and release/1.0.27 before this change both leave
`~/.c4/co/config`, and the next `c4 cat` fails with "no content store
configured"; in a terminal `c4 id -s` would then offer to create
`~/.c4/store` and write a new config naming it, so later content goes to a
second store. With this change the config stays, and `c4 cat` finds the
object. The default store is `~/.c4/store`, so only a hand-written
`store = ~/.c4` reaches this. A config already at `co/config` is read
where it lies when `~/.c4/config` does not exist (`configReadPath`). It is
never written, moved back or removed, and when both exist the root's is
read: it is the file c4 writes, so the newer. Decided 2026-11-16 by the
release lead (`TestConfigDisplacedBySplitIsReadWhereItLies`).

## Same-ID publishers on Windows

Not this change's fault and not its fix. Two writers storing the same
bytes can both miss the look and both rename; on Windows the second rename
can fail ("Access is denied"). It shows in this change's tests because they
run several writers on one store, and it is on release/1.0.27 without them
(measured in c4 #181: 160 to 240 of 5,760 same-ID Puts failed on every
Windows leg, none elsewhere). The fix is its own PR, c4 #184 (master's is
#179), and this change is stacked on it.
