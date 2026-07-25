# Listings — the general format

Status: exploration note (2026-07-25). Prompted by Joshua: index every
captured directory at both projections; the virtual root then becomes
"just another entity ID"; and *"this potentially leads us towards a
general format."* This note is the think-through, not a requirements
doc. Nothing here is settled.

---

## 1. Index every directory, not just the root

**The proposal.** Every directory ID a c4m carries gets indexed, at
both its canonical (full) ID and its nil-metadata ID — not only the
root.

**Why it is more than bookkeeping.** A c4m of a deep tree *already*
contains an ID for every directory in it, because a directory's ID is
the hash of its one-level listing. Indexing them costs a lookup table
over data already computed — and it converts a tree from one
addressable thing into *n* addressable things.

What that makes answerable, none of which is answerable today:

- **Do I already have this subtree?** — for every subtree, not just
  roots. Partial-have becomes a real query.
- **How much of this delivery do I already hold?** — computed by
  intersecting directory IDs, which is the tree-level generalization
  of `c4 intersect`. This is the PRODUCT.md persona claim ("you already
  have 60% of this project from the last show") becoming computable
  rather than aspirational.
- **Which captures share this subtree?** — reverse lookup; dedup and
  provenance both fall out.
- **Skip re-describing an unchanged subtree** — a reuse path keyed on
  *identity* rather than on size+mtime, i.e. strictly stronger than
  `-c`'s racy rule wherever an ID is already known.

**The one that matters commercially — structure classes.** A
nil-metadata ID is not just a second key; it is an **equivalence-class
identifier**. Every directory with the same shape hashes to the same
nil-metadata ID, so:

> A delivery specification can be expressed as a nil-metadata ID, and
> "does this package conform?" becomes an ID comparison.

That is the media wedge's actual product — package conformance —
reduced to the system's cheapest operation, with no new verb.

## 2. The virtual parent dissolves

Joshua: *"this then makes the top level virtual folder a moot issue as
it simply is a new entity id, the virtual root with additional context
or the root of the directory captured."*

Correct, and it is worth stating why the fork was false. Once every
directory is an indexed entity, a virtual root is **not a special
construct** — it is a directory that happens to contain the captured
tree plus a context file, and it is indexed like any other. Both
entities exist; both have IDs; both are findable:

- given the virtual root's ID → the captured tree's ID is one of its
  child entries;
- given the captured tree's ID → the reverse index finds virtual roots
  that contain it.

So "should scope be inside the identity or beside it?" stops being a
choice. **It is both, simultaneously, at two different IDs** — and a
caller picks the one that answers their question. Nothing has to be
decided in advance, which is the tell that the model is right.

## 3. Toward the general format

Here is the shape I think this is pointing at.

### The observation

Every addressable thing in c4 is one of exactly two things:

1. **opaque bytes** (a file), or
2. **a listing** — an ordered set of lines whose hash is its ID.

A directory is already a listing (its ID is the hash of its one-level
canonical listing). A c4m is a listing. The journal is a listing. A
receipt is a listing of edges. An index is a listing of pairs. A
delivery spec is a listing. None of these needed inventing — they are
all the same shape, and c4 has been building them one at a time without
naming the pattern.

### The format

```
@c4 <kind> <version>          # optional for the frozen kind
<line>
<line>
…
```

- **kind** fixes the line grammar and the canonical order.
- **The listing's ID is the hash of its canonical text.** One identity
  rule, already true for directories.
- **A line may reference other IDs**, so listings compose — which makes
  the whole store a Merkle DAG of listings. It already is one; this
  names it.
- **Read whole or streamed**, and a stream that stops is a valid
  partial listing. This is c4m's existing property, generalized —
  *"partial data is data."*

Today's c4m is this with the header elided. Keeping the header optional
for that one kind preserves the frozen grammar byte-for-byte and keeps
the common case terse.

### What it buys

- **One canonicalization rule, one identity rule, one streaming rule,
  one partial-data rule** — instead of re-deciding each per format.
- **One framing parser per implementation**, with small per-kind line
  grammars, instead of N independent formats × 5 languages ×
  conformance vectors. This is the direct answer to the format-economy
  finding in `v11-surface-review.md`.
- **The diff/patch algebra generalizes.** c4m already has one; if
  listings share a framing, patching a journal, an index, or a receipt
  is the same operation, not four.
- **New kinds become cheap and bounded** — a new capability costs a
  line grammar, not a format.

### The convention that must be settled first

**ID position.** c4m entry lines put the ID **last**; the journal puts
it last; shape-A testimony puts it **first**. Under a general format
this must be one rule, and ID-last wins on two grounds: it is the
frozen grammar's choice, and `awk '{print $NF}'` then works on every
listing in the system. Shape A is the one that diverges and should
move.

## 4. Edge cases and honest costs

1. **Which projections, exactly?** "Nil metadata" is ambiguous and the
   two readings answer different questions:
   - **content** (`-m c`) nulls mode and time, **keeps** child IDs →
     "same bytes, arranged the same."
   - **structure** (`-m s`) keeps names only → "same shape, whatever
     the contents."

   Conformance-to-a-delivery-spec wants **structure**; dedup and
   partial-have want **content**. They are not substitutes.
   Recommendation: index **all three** (full, content, structure). The
   marginal cost is two hashes per directory over data already in
   memory at scan time, and it removes the ambiguity permanently.
2. **The index must be derived, never authoritative.** It has to be
   rebuildable from stored listings alone, so a lost or corrupt index
   costs time and nothing else. This keeps *verification over
   assertion* intact and keeps the index off every destructive path
   (`store-index-and-links.md`, `gc-requirements.md`).
3. **Folding is part of the key**, per the 2026-07-25 ruling — a folded
   and an unfolded description are different descriptions at every
   projection.
4. **Cost is small but not zero.** A 20k-file tree holds roughly 2k
   directories; 50 snapshots ≈ 10⁵ entries of a few IDs each. Trivial
   on disk. The real cost is two extra hashes per directory at scan
   time, over listings already held in memory.
5. **This does not fix the recipient problem.** An ID handed to someone
   else still does not say which projection produced it. The index
   disambiguates *within a store*; it cannot travel with a bare ID.
   That residue of `v11-surface-review.md` Finding 1 stands, and the
   virtual root is the only idea so far that addresses it.

## 5. What I would do about it

Nothing yet, and deliberately: this is a *general format* proposal
arriving while three release-blocking defects are open
([#42](https://github.com/Avalanche-io/c4/issues/42),
[#43](https://github.com/Avalanche-io/c4/issues/43),
[#44](https://github.com/Avalanche-io/c4/issues/44)) and the v1.1
surface is unfrozen. The right order is:

1. Fix the three blockers; they are pure subtraction and they stop
   active data loss.
2. Freeze the v1.1 surface, unchanged by this note.
3. Then take the listing format through the crucible — it is exactly
   the class of decision (a format, a frozen grammar, a core primitive)
   that the crucible exists for, and exactly the class that is
   expensive to get wrong.

The reason to write it down now is that it changes what the record
layer should look like *before* anyone builds it — the receipt,
process, and testimony kinds are still unbuilt, and if listings are the
right frame, those three collapse into line grammars rather than
formats.
