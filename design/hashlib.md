# hashlib — A Content-Addressed Data Structure Library

## Why Build This?

Most general-purpose data structure libraries are designed around an implicit assumption: **keys are arbitrary, and randomness is expensive**. They spend significant engineering effort on hash functions, random number generators, and probabilistic balancing schemes — all to manufacture the uniform distribution that efficient data structures require.

Content-addressed systems break this assumption entirely.

When your keys are cryptographic hashes — SHA-512 in particular — you arrive with 512 bits of high-quality, uniformly distributed, computationally irreducible randomness already in hand. Every lookup key is, by construction, the output of a process specifically engineered to produce unpredictable, avalanche-behaved bit strings. The randomness infrastructure that general libraries work hard to provide is simply... already there.

This creates an unusual opportunity. Every data structure that secretly needs randomness to guarantee its performance properties can be reimplemented to exploit pre-existing hash keys directly:

- **Hash maps** can skip hashing entirely, reducing the hot path to a bitmask and a memory read
- **Cuckoo structures** can use non-overlapping byte windows as genuinely independent hash functions, achieving worst-case O(1) lookup that is normally only theoretical
- **Treaps** can derive their balancing priority from the key itself, eliminating the RNG and producing perfect expected balance by construction
- **Skip lists** can derive level assignments from leading zero counts, turning geometric distribution into a bit operation
- **Bloom filters and sketches** need k independent hash functions — with SHA-512 keys, those are k pointer reads into the key itself
- **Patricia tries** over a uniform keyspace are perfectly balanced without any rotation logic, because the bit distribution guarantees it

The result is a library where the common case — computing over content-addressed data — is **measurably faster** than what a general-purpose library can offer, while also being **simpler to implement correctly** because the randomness preconditions are guaranteed by the key type rather than approximated at runtime.

Beyond raw performance, content-addressed systems have a second structural property worth exploiting: **keys are stable identity**. A hash is not just an index — it is a fingerprint of content, valid forever, comparable across nodes, and immune to renaming or mutation. Data structures that are aware of this can provide operations that general maps cannot: XOR-distance routing, set reconciliation without full enumeration, structural sharing across snapshots, and Merkle-style integrity verification at any subtree.

This library is built for systems where:
- Data is identified by cryptographic hash
- Nodes need to efficiently route, replicate, and sync content
- Set membership and approximate counting are on the critical path
- Graph structure over hash-linked content needs to be traversed and diffed
- Snapshots, rollback, and changelog are first-class operations

The goal is not to replace general-purpose collections. It is to provide a suite of structures and primitives that are **aware of what SHA-512 keys actually are**, and exploit that awareness at every layer.

---

## Core Data Structures

### Hash-Optimized

These structures exploit SHA-512 keys directly, treating the key as a pre-computed, high-quality hash rather than input to be hashed.

| Structure           | Description                                                                                                                                      |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `HAMTMap[V]`        | Hash array mapped trie. Hash -> value store with structural sharing and copy-on-write semantics.                                                 |
| `HAMTSet`           | Set variant of HAMT. Efficient existence checks, set operations, and persistent snapshots.                                                       |
| `CuckooFilter`      | Probabilistic membership structure supporting deletion. Compact per-node existence summaries for gossip.                                         |
| `CuckooMap[V]`      | Full key-value map using cuckoo hashing. Worst-case O(1) lookup using non-overlapping byte windows as independent hash functions.                |
| `CountingBloom`     | Bloom filter with per-element counters. Membership plus approximate access frequency.                                                            |
| `PatriciaTrie`      | Binary trie over hash space. XOR-nearest queries, prefix routing, range scans. Perfectly balanced by construction on uniform keys.               |
| `Treap[V]`          | Ordered map with hash-derived priorities. No RNG required. Identical structure for identical sets, enabling structural set reconciliation.       |
| `ExtendibleHash[V]` | Dynamically sharded hash table. Splits by reading one more bit of the key prefix — perfectly halves load every time due to uniform distribution. |

### Probabilistic / Sketch

Structures for approximate answers at very low memory cost. With SHA-512 keys, all multi-hash variants use non-overlapping byte windows — no hash computation required.

| Structure        | Description                                                                                                      |
| ---------------- | ---------------------------------------------------------------------------------------------------------------- |
| `BloomFilter`    | Fast set membership. No deletion. Optimal for deduplication during ingest.                                       |
| `HyperLogLog`    | Cardinality estimation. "How many unique items does this node hold?"                                             |
| `CountMinSketch` | Frequency estimation. "How many times has this hash been requested?"                                             |
| `TopK`           | Heavy hitters. Built on CountMin. "What are the most accessed items?"                                            |
| `MinHash`        | Set similarity estimation via Jaccard coefficient. "How similar are these two nodes' content sets?"              |
| `SimHash`        | Near-duplicate detection over hash metadata.                                                                     |
| `XorFilter`      | Smaller and faster than Bloom for read-only use cases. Suitable for static indexes.                              |
| `IBF`            | Invertible Bloom Filter. Set difference without full set exchange — the right primitive for efficient node sync. |

### Graph / Link Structures

Structures for the link graph itself — forward links, reverse links, DAG traversal, and integrity verification.

| Structure             | Description                                                                                |
| --------------------- | ------------------------------------------------------------------------------------------ |
| `DAG`                 | Directed acyclic graph over hash-identified nodes. Core representation for linked content. |
| `MerkleTree`          | Subtree integrity and partial verification.                                                |
| `MerkleDAG`           | IPFS-style deduplicating DAG. Structural sharing across content with common subgraphs.     |
| `AdjacencyHAMT`       | Forward and reverse edge maps, both HAMT-backed. Efficient bidirectional link traversal.   |
| `TopologicalIterator` | Walk a DAG in dependency order.                                                            |
| `CycleDetector`       | For graphs that may not be pure DAGs.                                                      |

### Ordered / Range

Structures supporting ordered access and range queries over metadata indexes.

| Structure      | Description                                                                                                         |
| -------------- | ------------------------------------------------------------------------------------------------------------------- |
| `SkipList[V]`  | Ordered map with range scan support. Level assignment from hash bits — no RNG. Lock-friendly for concurrent access. |
| `BTreeMap[V]`  | Cache-friendly ordered map for dense metadata indexes.                                                              |
| `IntervalTree` | Overlap queries on ranges. Useful for time-range and size-range metadata queries.                                   |
| `RadixTree`    | Prefix queries over hash prefixes.                                                                                  |

### Sets

| Structure       | Description                                |
| --------------- | ------------------------------------------ |
| `HashSet`       | Basic set built on CuckooMap.              |
| `RoaringBitmap` | Compressed integer sets for index bitmaps. |

---

## Functional Primitives

### Collection Operations

```go
Map(set, fn)           -> set
Filter(set, pred)      -> set
Reduce(set, fn, init)  -> value
FlatMap(set, fn)       -> set        // fn returns set
Partition(set, pred)   -> (set, set)
GroupBy(set, fn)       -> map[K]set
Zip(set, set)          -> set[pair]
```

### Hash-Specific

```go
Nearest(id, k)         -> []ID       // k nearest by XOR distance
Between(lo, hi)        -> iter[ID]   // range in hash space
Slice(id, bits)        -> uint64     // extract bit window
Shard(id, n)           -> int        // which of n shards owns id
CommonPrefix(a, b)     -> int        // bits of shared prefix
XorDist(a, b)          -> ID         // Kademlia XOR distance
```

### Set Reconciliation

```go
Diff(a, b)             -> (onlyA, onlyB)
Intersect(a, b)        -> set
Union(a, b)            -> set        // HAMT structural merge
Sync(local, remote)    -> delta      // treap-walk based
EstimateDiff(a, b)     -> int        // IBF-based, no full exchange
```

### Graph Traversal

```go
BFS(root, adj)         -> iter[ID]
DFS(root, adj)         -> iter[ID]
TopoSort(roots)        -> []ID
Ancestors(id)          -> set
Descendants(id)        -> set
Closure(id, rel)       -> set        // transitive closure of relation
Subgraph(roots)        -> DAG        // induced subgraph
```

### Persistence / Snapshot

```go
Snapshot(root)         -> Token      // O(1), structural sharing
Diff(snap1, snap2)     -> changeset
Rollback(token)        -> root
Changelog(snap1, snap2) -> iter[op]
```

### Routing / Distribution

```go
Route(id, nodes)           -> nodeID      // nearest responsible node
Replicas(id, nodes, k)     -> []nodeID    // k responsible nodes
Rebalance(old, new)        -> migrations  // extendible hash split plan
Gossip(filter)             -> delta       // cuckoo filter exchange
```

### Iterator / Lazy

```go
Take(iter, n)          -> iter
Drop(iter, n)          -> iter
Batch(iter, n)         -> iter[[]T]
Merge(iters...)        -> iter       // sorted merge
Dedupe(iter)           -> iter       // stateless via bloom
Window(iter, n)        -> iter[[]T]  // sliding window
```

---

## Core Interfaces

```go
type Hashable interface {
    ID() ID
}

type Resolver interface {
    Get(ID) ([]byte, error)   // content fetch
    Has(ID) bool              // local existence
    Put([]byte) (ID, error)   // store and return hash
}

type Router interface {
    Nearest(ID, k int) []NodeID
    Responsible(ID) NodeID
}

type Filter interface {
    Add(ID)
    Has(ID) bool
    MarshalBinary() []byte    // for gossip
    Merge(Filter) Filter
}

type SyncableSet interface {
    Add(ID)
    Remove(ID)
    Diff(SyncableSet) ([]ID, []ID) // (have, want)
    Fingerprint() uint64           // fast equality check
}
```

---

## Implementation Priority

### Tier 1 — Foundation
`HAMTMap`, `HAMTSet`, `CuckooFilter`, `PatriciaTrie`, core functional primitives

These are required before anything else is useful. The HAMT pair is the primary storage substrate. CuckooFilter and PatriciaTrie unlock the network layer. Core functional ops make everything composable.

### Tier 2 — Network Layer
`IBF`, `HyperLogLog`, `CountMinSketch`, routing primitives

IBF is the key primitive for efficient sync — two nodes can compute their symmetric difference by exchanging a fixed-size structure, with no need to enumerate either full set. HyperLogLog and CountMin give you capacity planning and cache prioritization at near-zero cost.

### Tier 3 — Query Layer
`SkipList`, `MerkleDAG`, `TopK`, graph traversal, snapshot/diff

Once data is stored and nodes can sync, you need to query it. SkipList handles ordered metadata indexes and range scans. MerkleDAG gives you the linked content graph. Snapshot and diff round out the persistence model.

### Tier 4 — Optimization
`XorFilter`, `RoaringBitmap`, `SimHash`, `MinHash`

Drop-in replacements and additions that improve performance or enable new approximate query patterns once the core system is working.
