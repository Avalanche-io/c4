# Block Link Semantics — Replacing Checkpoint IDs

## Summary

Bare C4 IDs in c4m streams change from checkpoint verification (O(n))
to block links (O(1)). A bare C4 ID is always the ID of the previous
block, not a computed hash of accumulated state.

## Current Behavior (Being Replaced)

The spec says a bare C4 ID after entries must match the canonical C4 ID
of all accumulated content above it. This requires hashing all prior
entries at every boundary — O(n) work that doesn't scale.

## New Behavior

A bare C4 ID in a c4m stream is always the C4 ID of the block
immediately above it. "Block" means the text content between the
previous bare C4 ID (or start of file) and this bare C4 ID.

### Producing a block boundary

1. Write entries
2. Hash the bytes you just wrote (the block)
3. Write the block's C4 ID as a bare line
4. Continue with next block

O(1) at each boundary. No accumulated state needed.

### Verifying a chain

1. Read block 1 (everything before first bare ID)
2. Hash block 1 — should match the bare ID that follows it
3. Read block 2 (everything between first and second bare IDs)
4. Hash block 2 — should match the bare ID that follows it
5. Continue through the chain

Each block verifies the previous. Walk once to verify everything.

### Reconstructing content

For patches (applying changes):
1. Start with block 1 entries
2. Each subsequent block's entries are applied as a patch:
   - New entry = addition
   - Identical entry = removal
   - Modified entry = replacement
3. The final state is the result of applying all patches in order

For large directory blocks (concatenation):
1. Collect all entries from all blocks
2. The full entry list is the concatenation in chain order
3. No patch semantics — just more entries

The distinction between "patch chain" and "directory blocks" is in
how the entries relate to each other, not in the block link mechanism.

### First bare C4 ID

The first bare C4 ID in a stream (before any entries) is an external
base reference — it points to a complete manifest stored elsewhere.
This is unchanged from current behavior. It's a link to content you
need to fetch to know the starting state.

After the first block of entries, all subsequent bare C4 IDs are
block links to the immediately preceding block.

## What Changes

| Aspect | Old (checkpoint) | New (block link) |
|--------|-----------------|------------------|
| Bare ID means | hash of all accumulated content | hash of previous block |
| Cost to produce | O(n) — hash everything so far | O(1) — hash last block |
| Cost to verify | O(1) per checkpoint (if you trust prior) | O(1) per block |
| Full verification | already done at each checkpoint | walk chain once |
| Scaling | breaks at millions of entries | constant cost per block |

## Impact

- Spec change: SPECIFICATION.md section on patch boundaries
- Decoder change: don't verify bare IDs against accumulated state
- Encoder change: emit block ID instead of accumulated state ID
- Test vector change: the manifest_vectors entry may need updating
- All language implementations: Go, Python, TypeScript, Swift, C

## Compatibility

Old c4m files with checkpoint IDs will still parse — the decoder
just won't try to verify them against accumulated state. The bare
IDs become block links that happen to also be valid checkpoints
(because in a two-block file, the first block IS the accumulated
state). Multi-block files created under the old semantics would
need reverification, but in practice these are rare.

## Status

Design complete. Implementation pending — this touches the decoder
in every language.
