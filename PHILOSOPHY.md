# C4 Philosophy

This document is incomplete. It is note-keeping, not doctrine — a collection of
philosophical positions discovered over the course of building C4, gathered here
so they can eventually inform a proper philosophy section in the documentation.

These positions are being discovered incrementally, much as C4 itself discovers
data: partial knowledge first, refined over time, each stage valid in its own
right. This document is itself a partial resolution.

---

## On Identity

**A thing is what it contains, not where it lives or what you call it.**
A file's identity is the cryptographic hash of its content. Names, paths, and
locations are labels — useful, but not fundamental. Two files with the same
content are the same file, regardless of what anyone named them or where they
stored them. This is not a design choice so much as a recognition of what
identity actually means for digital content.

**Identity is the only fact that travels perfectly.**
You can copy a file incorrectly. You can lose metadata in transit. You can
rename it, move it, corrupt it. But if you have the content, you can
recompute the identity, and it will match — or it won't, and you'll know
something went wrong. Identity is self-verifying in a way that no other
property of a file is.

---

## On Observation

**You cannot observe a large system atomically.**
A filesystem with millions of files and petabytes of data cannot be apprehended
in a single instant. Learning what it contains is necessarily incremental —
names before sizes, sizes before hashes, nearby before distant. Any system that
pretends otherwise is hiding the latency, not eliminating it.

**Partial knowledge is not an error state.**
Most systems treat incomplete information as a failure — a transfer either
succeeded or it didn't. C4 formalizes partial knowledge as a valid state.
A manifest with names but no hashes is not broken; it is a less-resolved
view of the same reality. Each level of resolution adds a stronger guarantee
without invalidating what came before.

**Every intermediate state is a complete state.**
A partially scanned filesystem is not "in progress" — it is a version. A less
complete version, but a version nonetheless, with its own identity and its own
integrity. The progression from incomplete to complete is a sequence of valid
snapshots, not a transition from invalid to valid.

---

## On Separation

**Knowing about data and having data are different things.**
The description of a project — its structure, its files, their identities — is
lightweight and portable. The content itself may be terabytes. Separating these
two concerns means you can work with the description immediately and let the
content follow asynchronously. Most systems conflate knowing with having. The
manifest is the missing abstraction between them.

**Intent and data move differently.**
Push intent, pull data. A manifest declares what should exist. The receiver
compares it against what they already have and pulls only what's missing. This
inverts the traditional model where the sender decides what to transmit. The
sender declares; the receiver decides.

---

## On Simplicity

**Simplicity is the goal, not a tradeoff.**
The minimalist aesthetic of C4 is not a constraint imposed by limited resources
or an early-stage compromise. It is the design objective. Every abstraction,
configuration option, and line of code is a burden. The ideal system has the
fewest concepts that can express the full range of necessary operations.

**Every layer of abstraction must justify its existence.**
If a concept can be expressed with existing primitives, the new abstraction is
noise. Standards over invention. Defaults over configuration. Deletion over
addition. The best optimization is often removing something.

**Explicit is better than implicit.**
When a value is unknown, say so — don't guess, don't default, don't hide it.
C4M uses null values rather than zero values. A missing hash means "not yet
computed," which is different from "computed and found to be zero." Clarity about
what you don't know is as important as clarity about what you do.

---

## On Trust

**Trust should be mathematical, not social.**
Content addressing means verification is inherent. You don't need to trust the
transport, the sender, or the storage medium. Recompute the hash; it either
matches or it doesn't. This eliminates whole categories of problems — not by
handling them gracefully, but by making them structurally impossible.

**Consistency is more important than convenience.**
UTC-only timestamps. Canonical sort order. Byte-exact matching. These choices
make C4M less friendly to casual human use but ensure that the same filesystem
always produces the same identity. Reproducibility is not negotiable.

---

## On Interruption

**Interruption is normal, not exceptional.**
Networks drop. Processes get killed. Disks fill up. A system that corrupts or
loses progress on interruption is not robust — it is fragile in the most common
way possible. C4's content-addressed state transitions mean that interruption
at any point leaves the system in a valid, resumable state. Not because
interruption is handled as a special case, but because there is no special case
to handle — every written state is already valid.

**Progress is never lost.**
If you've scanned 10,000 files and the process dies, you have 10,000 scanned
files. If you've transmitted 7 of 10 manifest pages and the connection drops,
you have 7 pages. The work already done is not contingent on the work yet to
come. Content addressing makes this automatic — the identity of what you have
is independent of what you don't have yet.

---

## On Making the Case

While writing an argument for a specific design decision (push intent, pull
content), we noticed something about how the argument should be structured. The
worst version of a design justification says "here's why our approach is great."
The best version says "here's the analysis we did; here's where we think the
alternatives fall short; here's where reasonable people might disagree; here's
where we landed." If someone reads it and concludes the approach isn't justified
for their use case, that's a valid outcome — and the document should make that
easy to determine rather than trying to prevent it.

This may not apply generally. It arose in a specific context — justifying a
layer of abstraction against the principle that every layer must earn its
existence. But it's worth thinking about whether this says something broader
about how C4 should explain itself.

---

*This document will grow as positions are discovered. Like the data it
describes, it resolves progressively.*
