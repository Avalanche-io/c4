# Large Directory Blocks — Scaling C4M to Millions of Entries

## Problem

A single c4m file for a directory with millions of entries becomes
impractical. Reading, parsing, and hashing a multi-gigabyte text file
for every operation defeats the purpose of lightweight descriptions.

## Design: Patch Chain Blocks

Large directories use the existing patch chain mechanism as a block
chain. Each block contains a defined number of entries, and each
subsequent block references the previous block by its C4 ID.

### Structure

```
# Block 1: first N entries (standalone c4m)
entry1
entry2
...
entryN

# Block 2: references block 1, adds next N entries
c4<id-of-block-1>
entryN+1
entryN+2
...
entry2N

# Block 3: references block 2
c4<id-of-block-2>
entry2N+1
...
entry3N
```

### Why Previous Block, Not Running State

Each bare C4 ID at the top of a patch block references the **previous
block**, not the accumulated state.

- **Previous block is O(1)**: just the ID of what you just wrote.
  No need to read or merge prior blocks.
- **Running state is O(n)**: requires applying all prior patches and
  computing the canonical form of the accumulated result. Expensive
  at every boundary.
- **Chain structure is explicit**: follow the links to reconstruct
  the full directory. Walk once to verify.
- **Streaming friendly**: you can write blocks as you scan, without
  holding the full state in memory.

The first bare C4 ID (at the top of block 2) is the ID of block 1,
which IS the complete state at that point. After that, each bare ID
is just a pointer to the previous block — the running state would
require applying all patches to compute.

### No Fixed Block Size

There is no spec-mandated block size. Producers emit block boundaries
whenever they choose — for memory management, streaming, or transport
reasons. Consumers must accept blocks of any size.

Any patch chain produces a different byte representation than a single
c4m file for the same content. This is already true — patches are a
transport/storage mechanism, not an identity mechanism. The directory's
C4 ID is computed from its canonical one-level form, independent of
how many blocks were used to transmit it.

### Block Identity

Each block's C4 ID is the hash of its canonical text content
(including the bare C4 ID reference line at the top for blocks 2+).
The full directory's C4 ID is the ID of the final block — which
encodes the entire chain by reference.

### Reconstruction

To reconstruct the full directory listing:
1. Start with the final block
2. Follow the bare C4 ID reference to the previous block
3. Repeat until you reach a block with no reference (block 1)
4. Concatenate entries from block 1 through final block

To verify the chain:
1. Hash block 1 — should match the reference in block 2
2. Hash block 2 — should match the reference in block 3
3. Continue through the chain

### Interaction with Directory C4 IDs

The canonical directory C4 ID (used in parent manifests) is computed
from the one-level canonical form — direct children only. For small
directories this is a single block. For large directories, the
canonical form would reference the chain's final block ID.

This needs more design work. The key question: does the parent
manifest store the final-block ID or the canonical-one-level ID?
If the one-level canonical form is itself too large, it needs its
own block chain.

## Open Questions

1. **Should the block limit be configurable?** Per-tool or per-scan?
   Or fixed in the spec?

2. **How does diff work across block chains?** Comparing two chained
   directories requires reconstructing both. Can we diff block-by-block?

3. **Does the block chain affect the directory C4 ID?** If the same
   entries are in one block vs five blocks, should the directory ID
   be the same? Probably yes — the logical content is identical.
   This means the block chain is a transport/storage optimization,
   not part of the identity.

4. **Partial fetch**: can you fetch just the last block to see recent
   entries without reconstructing the whole chain? Useful for
   "what's new in this directory."

## Status

Design draft. Not blocking for announcement — the current implementation
handles directories of practical size. This design is for the "how does
this scale to millions" question that will come up on HN.
