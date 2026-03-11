# DB Persistence Design

## Context

The `db` package is a pure MVCC namespace engine: it maps paths to C4 IDs using an immutable copy-on-write tree. It does not store or manage user content. The question is how to persist the tree state efficiently.

## Current Implementation (v0)

Single-file persistence. On every commit, the entire tree is serialized to c4m entries and written atomically to `state.c4m`. On startup, the file is read and the tree is rebuilt in memory.

**Pros:** Dead simple, correct, easy to reason about.
**Cons:** Every commit writes every entry. O(N) per commit where N is total namespace size, regardless of how small the change is.

This is fine for small-to-medium namespaces (thousands of entries). It becomes a bottleneck as the tree grows.

## In-Memory Structure

The in-memory COW tree is not up for debate — it works well and is orthogonal to the persistence question. It's a trie of sorted children arrays with binary search at each node. Mutations produce new roots via structural sharing. Pointer equality means "unchanged subtree." All MVCC readers hold immutable pointers.

The persistence question is: how do we durably record changes without rewriting the world?

## Option 1: Node-Wise c4m

Each directory node is stored as its own c4m file, identified by C4 ID. The root pointer is the C4 ID of the root directory's c4m.

On commit:
1. Walk from changed leaf up to root
2. For each changed directory, serialize its entries to c4m, store by C4 ID
3. Update parent directories with the new child ID
4. Write the new root ID to a pointer file

**Write amplification:** bounded by tree depth, not tree size. Changing one leaf in a 5-level tree rewrites 5 small c4m files regardless of total namespace size.

**Access pattern argument:** A c4m directory is a natural cache line. If you resolve `projects/main/file`, you load the `projects` directory and now have all its siblings. Users who want a file at some depth likely want all files at that depth. The storage unit matches the access pattern.

**Implementation:** The DB needs a small internal content-addressed store for its own directory c4m files (separate from user content). Something like a ShardedFolder scoped to the DB's directory.

**Pros:**
- Write amplification bounded by depth
- Storage unit matches access pattern
- Uses c4m natively — no translation layer
- Each directory node is independently addressable by C4 ID
- Could enable lazy loading for very large trees (don't load the full tree into memory on startup)

**Cons:**
- Tree shape is determined by the namespace, not balanced
- Deep narrow trees have more write amplification per level than wide shallow ones
- Many small files on disk (one per directory)
- Needs internal blob store for directory nodes
- Old directory c4m files accumulate and need cleanup

## Option 2: Global B+ Tree

A single balanced B+ tree over the entire namespace, keyed by path components (or `parent_id + name`).

This is what modern COW filesystems do:

- **btrfs:** Single global B+ tree for all filesystem metadata. Keys are `(objectid, type, offset)` tuples. Directory entries for all directories live in the same tree.
- **APFS:** Single global B+ tree keyed by `(parent_id, name_hash)`. All directory entries across all directories in one tree.
- **ext4, XFS, NTFS:** Older approach — per-directory B+ trees (or hash trees). Each directory has its own index structure.

The modern COW filesystems chose global B+ trees because snapshots become trivial: a snapshot is just a new pointer to the same root. This is exactly what the MVCC tree in this DB does.

**Key insight:** A global B+ tree keyed by `(parent_id, name)` gets directory-level locality from key ordering — all entries in a directory are adjacent leaves. So you get both balanced logarithmic operations AND "all siblings in one page" access patterns.

**Pros:**
- Balanced: O(log N) reads and writes regardless of namespace shape
- Single data structure — no many-small-files problem
- COW snapshots are natural (same as btrfs/APFS)
- Page-oriented — good I/O characteristics
- No separate cleanup needed (old pages are reclaimed structurally)

**Cons:**
- Significant engineering effort — need a B+ tree implementation
- Custom binary format (not c4m)
- More complex than needed for expected namespace sizes
- Overkill if trees stay shallow and modest

## Option 3: Stay with v0, Optimize Later

Keep the single-file approach. The namespace is likely to be small enough that full serialization per commit is fast. A 10,000-entry tree serializes to maybe 1-2 MB of c4m — that's a few milliseconds to write.

Defer the decision until there's evidence that persistence is actually a bottleneck. The in-memory COW tree handles all MVCC during runtime; persistence is only for durability.

**Pros:**
- Already implemented and working
- Zero additional complexity
- Can always migrate later (the on-disk format is just c4m)

**Cons:**
- O(N) writes on every commit
- Not suitable if namespaces grow to hundreds of thousands of entries

## How Filesystems Decide

| Filesystem | Directory Index | Scope | Why |
|------------|----------------|-------|-----|
| ext4 | HTree (hash B-tree) | Per-directory | Legacy design, simple per-dir scaling |
| XFS | B+ tree | Per-directory | High-performance per-dir operations |
| NTFS | B+ tree | Per-directory (MFT) | Designed for per-dir indexing |
| btrfs | B+ tree | Global | COW snapshots, single structure |
| APFS | B+ tree | Global | COW snapshots, clone/snapshot support |
| ZFS | Hash table (ZAP) | Per-directory | Different design philosophy entirely |

The trend: COW-friendly systems use global trees. Per-directory systems predate COW.

## Recommendation

Start with **Option 1 (node-wise c4m)** as the next step. It's the natural evolution from v0:

- Depth-bounded writes are good enough for realistic namespace shapes
- It uses c4m natively, no new format needed
- It matches the access pattern (directory = cache line)
- It's simpler than a full B+ tree
- It enables future lazy loading if needed

If namespace sizes grow to the point where tree depth creates real performance problems, upgrade to a global B+ tree (Option 2). But that's unlikely for a namespace database where the tree shape is determined by human-organized paths.

## Open Questions

1. **Internal store for directory nodes:** Reuse `store.ShardedFolder` or something simpler? The directory c4m files are small and numerous.
2. **Cleanup of old directory c4m files:** After a commit, old versions of changed directories are unreachable. When/how to reclaim them? Could use the same mark-and-sweep pattern the caller uses for user content.
3. **Lazy loading vs eager loading:** Load the full tree into memory on startup (current approach), or load directory nodes on demand? Eager is simpler; lazy saves memory for very large trees.
4. **Interaction with snapshots:** Held snapshots reference old tree versions. Their directory c4m files must not be cleaned up until the snapshot is released.
