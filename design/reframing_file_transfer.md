# Reframing File Transfer: An Algorithmic Argument for Persistent State Descriptions

## Origin

This document arose from a rigorous examination of C4M's push-intent-pull-content
model. While developing that argument, we noticed that the value of the manifest
layer is invisible when analyzing single transfers but obvious when analyzing
sequences of operations. This suggested the real contribution isn't an
optimization within the existing problem frame — it's a reframing of the problem
itself.

The structure of this argument follows a well-known pattern in algorithm design:
prove a lower bound on the conventional problem, then show that a different
problem formulation sidesteps the bound entirely.

---

## The Conventional Problem

**"Transfer files from A to B."**

The lower bound on this problem is the size of the data that needs to move.
You can compress, delta-encode, and deduplicate — rsync does all of these —
but you are bounded by the bytes that must traverse the channel. For the
general case, rsync is essentially optimal within this frame.

This is not a trivial result. rsync's algorithm (rolling checksums for
block-level delta detection, file-list comparison for change detection) is
genuinely good. Decades of engineering have refined it. For any single
point-to-point transfer, it's hard to do meaningfully better.

---

## The Lower Bound on Stateless Synchronization

Consider a synchronization protocol that maintains no persistent state between
invocations. To determine what has changed, it must examine the current state of
both endpoints on every sync. This examination has a cost:

**Without persistent state descriptions, the per-synchronization cost is
Ω(n) where n is the number of files, regardless of how few files changed.**

This is because the protocol has no memory of what the previous state was. It
must re-derive the current state from scratch to determine what differs. rsync
does exactly this: it builds a complete file list on every invocation, transmits
it, and compares. The file-list phase scales with n, the total number of files,
not with Δ, the number that changed.

For small filesystems this cost is negligible. For a filesystem with millions
of files where a handful changed, the scanning and negotiation phase dominates
the actual transfer time. The protocol spends most of its time rediscovering
what it already knew last time.

---

## The Reframing

**"Reconcile filesystem state between parties."**

This is a different problem. The relevant quantities are not the total data
size but the *size of the state description* and the *size of the state
difference*:

- A state description is O(n) in the number of files but **independent of
  file sizes**. A 23 KB manifest describes 8 TB of content. The description
  is not a compressed version of the content — it is a different kind of
  object entirely, capturing structure and identity without carrying payload.

- A state difference is O(Δ) in changed files, not O(total). Computing this
  difference requires comparing two state descriptions, not scanning two
  filesystems.

**With a persistent state description, the per-synchronization cost drops to
O(Δ) where Δ is the number of files that changed.**

The old description is already in hand. The new description is computed (or
updated incrementally). The diff between them identifies exactly what changed.
Only the changes need to transfer.

---

## The Cost Accounting

Over k synchronization operations on a filesystem with n files, where each
sync involves Δ changed files (Δ << n):

**Without persistent state (rsync model):**

    Scan + negotiate:  k × O(n)
    Transfer changes:  k × O(Δ_bytes)
    Total:             O(kn) + O(k × Δ_bytes)

**With persistent state (manifest model):**

    Build manifest:    O(n) + O(total_bytes) for hashing — paid once
    Per-sync diff:     k × O(Δ)
    Transfer changes:  k × O(Δ_bytes)
    Total:             O(n + total_bytes) + O(kΔ) + O(k × Δ_bytes)

The difference is in the scan/negotiate term: O(kn) versus O(n + kΔ).

When Δ << n and k is large — many syncs of a mostly-stable filesystem — the
manifest model wins by a factor of n/Δ per operation on the metadata
comparison alone. This is not a constant-factor improvement. It is a change
in how cost scales.

---

## Preprocessing: The Algorithmic Pattern

This maps directly to a well-known pattern: **preprocessing for efficient
queries.**

| Domain | Preprocess | Query without | Query with |
|--------|-----------|---------------|------------|
| Search | Build index: O(n log n) | O(n) | O(log n) |
| Substring | Build suffix array: O(n) | O(nm) | O(m log n) |
| Filesystem sync | Build manifest: O(n + bytes) | O(n) per sync | O(Δ) per sync |

The manifest is preprocessing. It pays an upfront cost to enable fundamentally
cheaper subsequent operations. The insight is not novel — preprocessing is one
of the oldest ideas in computer science. What is notable is that the dominant
file synchronization tools don't do it. They re-preprocess on every invocation.

---

## Why rsync Doesn't Persist State

rsync builds a file list (effectively a manifest) on every run. It examines
timestamps and sizes to detect changes, computes rolling checksums for delta
transfer, and transmits only differences. It has all the machinery. It just
rebuilds it from scratch every time and discards it when done.

This is like sorting an array before every binary search. The algorithm is
correct. Each individual operation is well-optimized. But the sequence of
operations pays a cost that could be avoided by persisting the sorted structure.

There are practical reasons rsync doesn't persist state. It was designed as a
stateless tool — no daemon, no database, no metadata to manage. This is a
virtue: rsync works anywhere, requires no setup, and leaves no residue.
Introducing persistent state adds complexity, failure modes, and the problem of
state staleness. These are real costs, and for many use cases they outweigh the
benefit of faster subsequent syncs.

The C4M argument is that for sufficiently large filesystems synchronized
sufficiently often, the balance tips. And that the persistent state (the
manifest) has value beyond sync performance — as a diffable document, a
shareable description, a verifiable snapshot — which shifts the balance further.

---

## The Hash Computation Cost

The most expensive part of building a manifest is computing content hashes:
O(total_bytes). This appears to make the preprocessing cost prohibitive for
large filesystems.

Two observations weaken this objection:

**1. Marginal cost during existing I/O.**

In a production pipeline, files are written by renders, copied during ingest,
read during transcodes. The bytes are already flowing through I/O operations.
Computing a SHA-512 hash during an existing read adds negligible marginal cost —
hash throughput on modern CPUs far exceeds storage I/O bandwidth. The
preprocessing cost approaches O(0) when integrated into existing data flow.

This is analogous to maintaining a balanced binary search tree during insertions
rather than sorting after the fact. The per-insertion overhead is small; the
alternative (a separate O(n log n) sort pass) is avoided entirely.

**2. Incremental updates.**

Once a manifest exists, changed files are re-hashed individually. The
O(total_bytes) cost is paid once; subsequent updates are O(Δ_bytes). The hash
computation follows the same amortization pattern as the manifest itself.

---

## The Online/Offline Distinction

There is a deeper framing available here. The conventional file transfer model
treats each sync as an **independent event** — an online algorithm processing
requests one at a time with no shared state between them. C4M treats syncs as
**operations on a persistent data structure** — an offline algorithm processing
a sequence of requests with shared context.

In online algorithm analysis, you evaluate each operation independently. In
offline (or amortized) analysis, you evaluate the cost over a sequence. These
yield different bounds for the same underlying operations.

A skeptic who asks "show me the benefit on *this* transfer" is demanding an
online analysis. The answer is: in the online frame, the manifest is overhead
on any single transfer. Its value is invisible. Switch to the offline frame —
analyze the sequence of syncs, diffs, shares, and resumptions over the
lifecycle of a project — and the manifest's value becomes the dominant term.

This is not a rhetorical trick. It is a genuine difference in what is being
measured. Both analyses are correct. They answer different questions:

- **Online:** "What is the cheapest way to do this one transfer?"
  Answer: push content directly (rsync, scp).

- **Offline:** "What is the cheapest way to keep these systems synchronized
  over time?" Answer: build and maintain a persistent state description.

Neither answer invalidates the other. They apply to different workloads. A
one-time transfer to a single recipient is an online problem. Ongoing
synchronization of large, mostly-stable filesystems across multiple parties
is an offline problem.

---

## What This Means for C4M

C4M's manifest is not an optimization of file transfer. It is a reframing of
the problem from transfer to state reconciliation. Within the transfer frame,
the manifest is overhead. Within the reconciliation frame, the manifest is
preprocessing that converts an O(n)-per-operation problem to an
O(Δ)-per-operation problem.

The reframing is valid when:

- The filesystem is large enough that O(n) scanning is expensive
- Syncs are frequent enough that the preprocessing cost amortizes
- Changes between syncs are small relative to total state (Δ << n)
- The state description has uses beyond sync (diff, share, browse, verify)

The reframing is not valid when:

- The filesystem is small (scanning is cheap, preprocessing is overhead)
- The transfer is one-time (no amortization opportunity)
- The entire filesystem changes between syncs (Δ ≈ n, no advantage)

The honest conclusion is not "C4M is better." It is: **C4M solves a different
problem.** For workloads that match the reconciliation frame — large, mostly
stable, frequently synchronized, multi-party — it achieves fundamentally better
scaling. For workloads that match the transfer frame — small, one-time,
point-to-point — it is unnecessary overhead.

The contribution is identifying that the reconciliation frame exists and that
persistent state descriptions are the preprocessing structure that makes it
efficient.

---

## Relation to the Push-Intent-Pull-Content Model

Push-intent-pull-content is the *protocol* that follows from the reconciliation
frame. If you have persistent state descriptions:

- Pushing intent = sharing the state description (cheap, O(n) in files)
- Pulling content = acting on the state difference (O(Δ_bytes))
- The receiver can compute the difference locally (no negotiation)
- Multiple receivers can use the same description independently
- The description persists for future diffs, audits, and resumption

Push-content is the protocol that follows from the transfer frame: move bytes
from A to B, negotiate if needed, repeat from scratch next time.

The choice of protocol follows from the choice of problem frame. Arguing about
the protocol without addressing the frame is arguing about the wrong thing.

---

## Open Questions

This analysis treats the manifest as a monolithic preprocessing step. C4M's
progressive resolution complicates this — the manifest can exist at multiple
resolution levels, each with different preprocessing costs and different utility.
A formal analysis of progressive resolution as a family of preprocessing
tradeoffs (more expensive preprocessing enables cheaper queries) is worth
developing.

The paged @base chain adds another dimension: preprocessing can itself be
incremental and resumable. The preprocessing structure (the manifest) has the
same interrupt-tolerance properties as the operations it enables. Whether this
is a necessary property or a convenient one is worth examining.

The relationship between content addressing and preprocessing deserves formal
treatment. Content addressing makes the preprocessing result (the manifest)
self-verifying — you can detect stale or corrupt preprocessing without
re-running it. This is unusual for preprocessing structures and may have
implications for the trust model of distributed synchronization.
