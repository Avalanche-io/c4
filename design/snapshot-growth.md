# Snapshot-loop growth: full listings versus change sets

Measured 2026-10-14 on c4 1.0.18, following the retention study's critic
("shrink growth instead of choosing what to delete") and Joshua's
"directories are always streams" (2026-10-03). Script and artifacts:
`/tmp/growth-test/` (volatile).

## Setup

A tree of 10,000 files in 50 directories. 200 snapshots, each after 3
small edits to existing files (about 75 bytes per edited file). Two forms
recorded side by side:

- **Full form**, what the store does today: `c4 <tree>` stores the content
  objects and a complete listing object for every directory on the path.
- **Change-set form**: `c4 diff <previous snapshot> <this snapshot>`, the
  c4m patch grammar, appended to a chain.

## Result

| | Per snapshot | 200 snapshots |
|---|---|---|
| Content that actually changed | ~0.2 KB | 45 KB (600 edited files) |
| Full form (store growth) | ~1,659 KB | 333 MB, 11,644 objects; 289 MB of it listing objects |
| Change-set form (description growth) | ~1 KB | 195 KB of change sets + a 1.35 MB base |

- The store grew about 7,400 times faster than the content changed, and
  87% of that growth is directory listings rewritten whole.
- The change-set form is about 1,700 times smaller per snapshot.
- The chain of base + 200 change sets resolved (`c4 patch`) to a listing
  byte-identical to the 200th full snapshot.

## What it means

In an agent-undo or sync loop, "nothing expires" is unaffordable only
because the store keeps each directory as a fresh full listing per
snapshot. Stored as the stream the c4m grammar already defines, the
knowledge of 200 states costs about as much as one state. The retention
question then narrows to content, where a few large files hold most bytes
(1% of objects held 77% of bytes in the default store).

## Caveats

- Reading state N means resolving the chain from its base; at 10,000
  entries a section costs about 146 ms to fold on master (critic's
  measurement), so periodic full listings (keyframes) are still needed.
- The July chain-resolution fixes (dropped external base, deepen-as-delete,
  O(K×N) fold) are prerequisites and are unpushed on local master.
- This measured description growth only; it did not change the store. How
  the store would hold a chain (one object per section, verifying the
  directory ID at each step) is undesigned.
