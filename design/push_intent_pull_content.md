# Push Intent, Pull Content

## What This Document Is

An argument for C4's core transfer model: the sender pushes a description of
what exists (a manifest); the receiver pulls the content they need.

This is not obvious. Most file transfer works by pushing content directly.
rsync, scp, Dropbox, FedEx'd hard drives — the sender decides what to send and
sends it. Adding a manifest layer between sender and receiver is an additional
step, and every additional step must justify itself against the simplicity of
not existing.

We take that challenge seriously. This document presents the case, engages with
genuine counterarguments, and tries to be honest about where the case is strong,
where it's weaker, and where reasonable people might disagree.

---

## The Core Claim

Push-intent-pull-content is superior to push-content when the description of
data is substantially cheaper than the data itself, and when the receiver is
better positioned than the sender to decide what needs to move.

[The core claim might be conflating two separable things: the directionality of data flow (push vs pull) and the persistence of the description (ephemeral vs durable).]
---

## The Arguments

### 1. Knowledge Lives on the Receiver's Side

The sender doesn't know what the receiver already has. If you push content, you
either push everything (wasteful) or negotiate what's missing (complex,
synchronous, requires both parties online). But the receiver already knows what
they have. Pushing intent lets the decision happen where the knowledge is.

**Counterargument:** rsync already handles this. It negotiates what's missing
via a two-party protocol and transfers only deltas. No manifest required.

**Response:** rsync's negotiation is ephemeral. It builds an implicit manifest,
uses it once, and discards it. If you transfer the same dataset to a second
receiver, the entire negotiation repeats. If the transfer fails and you retry,
the negotiation repeats. The manifest *persists* the description. It's
negotiation you do once and reuse indefinitely.

**Counterargument:** But most transfers are point-to-point. You rarely send the
same dataset to multiple receivers.

**Response:** That's true in a world where sending a dataset is expensive. When
sending a description costs 23KB and an email, the behavior changes. You share
descriptions the way you currently share links — casually, to anyone who might
benefit. The frequency of multi-recipient sharing is low *because* the cost is
high. In media production specifically, the same plates routinely go to
compositing, lighting, editorial, and the client — the multi-recipient case is
already common even under current constraints.

More broadly: many objections of the form "we don't do that" are framed in a
universe where the thing in question is expensive or impossible. They measure
demand under current constraints, not demand with the constraint removed. We
should be careful here — "if you build it, they will come" is not always true —
but we can observe that in media production, the pain of multi-recipient
delivery, delivery reconciliation, and cross-site coordination is well
documented and actively worked around. The latent demand is visible in the
workarounds.

---

### 2. Source Independence

The receiver, knowing what they need, can pull from any available source — the
original sender, a peer who already has the data, a local cache, a hard drive
that arrived in the mail. Intent describes *what*; the receiver chooses *where
from*.

**Counterargument:** Multi-source sounds good architecturally, but in practice
you know where the data is and you pull from there. Most transfers are
single-source.

**Response:** The most common "second source" is yourself from the past. Last
week's delivery shares 80% of the same content as this week's. With
content-addressed pull, the receiver discovers they already have most of the
data and pulls nothing for those files. The deduplication happens *before* the
transfer, automatically, without anyone thinking about it.

**Counterargument:** I can dedup after receipt.

**Response:** After receipt, the bandwidth is spent. Content-addressed pull
means the transfer that doesn't need to happen simply doesn't happen. It's not
an optimization applied after the fact; it's the absence of unnecessary work.

---

### 3. Intent is Cheap, Content is Expensive

A 23KB manifest describes 8TB of media. Sharing intent is essentially free.
This means you can share descriptions speculatively — "here's what I have, take
what you want" — without committing resources.

**Counterargument:** Generating that manifest requires reading every byte to
compute hashes. You haven't eliminated the cost; you've front-loaded it onto
the sender.

**Response:** In most real workflows, someone is already reading those bytes.
A render writes them. An ingest copies them from camera media. A transcode
reads and re-encodes them. The bytes are already flowing through a pipeline.

Computing a C4 hash during an existing read is nearly free. SHA-512 throughput
on modern CPUs vastly exceeds storage I/O bandwidth. The hash computation isn't
a new operation; it's a tap on a pipe that's already flowing. A C4-aware
pipeline computes identity as a side effect of every operation that touches
content. By the time you want a manifest, the hashes already exist.

The expensive version of hashing — going back and reading files solely to
compute hashes — is only necessary when the data was written by a tool that
isn't C4-aware. This is real for initial adoption, but diminishes as the
pipeline integrates.

**Counterargument:** Even without hashes, you can share a file listing. You
don't need a formal manifest format for "here's what I have."

**Response:** A file listing tells you names and sizes. It can't answer "is this
the same file I already have from the last delivery?" because names aren't
identity. Two files named `render_001.exr` at two different vendors are probably
different files. Two files with the same C4 ID are provably the same content
regardless of what anyone named them.

But you're right that a file listing is useful without hashes. C4M supports
this explicitly — a manifest with names and sizes but no hashes is a valid
manifest. Progressive resolution means you don't pay for hashes until you need
them. The format accommodates the spectrum from "quick file listing" to "fully
verified inventory" as a single document at different resolution levels.

---

### 4. Timing Decoupling

The sender pushes intent and walks away. The receiver pulls content later — five
minutes, five days, or never. Neither party waits for the other.

**Counterargument:** Just upload to object storage and send a link. Same
decoupling, no manifest.

**Response:** That *is* push-intent-pull-content — implemented informally. The
link is a degenerate manifest: it says "content exists here." C4M formalizes
what you're already doing and adds structure (full project tree, not a flat
bucket), verification (content identity, not trust-the-URL), and source
independence (not locked to one storage endpoint).

**Counterargument:** Maybe the informal version is good enough. Maybe the
formalization isn't worth the complexity.

**Response:** It's good enough until you need to operate on the descriptions
themselves. You can't diff two S3 URLs to see what changed between deliveries.
You can't merge three links to see the union of three vendors' work. You can't
ask "do I already have 60% of this from the last project?" A file listing is
useful for humans to read; a manifest is useful for machines to reason about.

Whether that matters depends on your workflow. For a single delivery to a single
recipient, an S3 link is simpler and sufficient. For ongoing coordination across
multiple parties with overlapping data, the formal manifest pays for itself.

---

### 5. Failure and Resumption

If a pull fails, the receiver knows exactly what they have and what they still
need. No coordination with the sender required.

**Counterargument:** rsync handles retry fine. Run it again and it figures out
what's missing.

**Response:** rsync re-negotiates from scratch on every retry. For large
projects, the negotiation phase itself is significant. With content-addressed
storage, the receiver's "what do I still need" check is a local operation — no
network, no sender involvement. The sender could be offline.

**Counterargument:** How often does this actually matter?

**Response:** Interruption during large transfers is not exceptional; it's
routine. Networks drop. Processes get killed. Disks fill. The question isn't
whether it happens — it's what recovery costs. Current tools generally *do*
support resumption, and when that support is reliable and automatic, people
benefit from it constantly without noticing. The problem is that support is
often unreliable, partial, or requires manual intervention. rsync resumes well.
Many other tools don't, or resume only under specific conditions.

C4's content-addressed model makes resumption structural rather than
feature-level. It's not a "resume" capability that was added; it's a
consequence of how state is represented. Every written page is independently
valid. There is no incomplete state to recover from because there is no
incomplete state — only less complete state, which is itself valid.

---

### 6. Receiver Autonomy

The receiver decides what to pull, in what order, at what priority. They might
want the whole project. They might want one shot. The architecture doesn't
force the choice.

**Counterargument:** This is just selective sync. Git sparse checkout, cloud
storage selective sync, and others already support this without manifests.

**Response:** Those are special modes bolted onto push-content systems. You
configure sparse checkout patterns; you toggle selective sync in a settings
panel. In pull-content, selectivity is the default behavior. You always pull
what you need, whether that's everything or a subset. There's no mode to enter.

**Counterargument:** The default should be "give me everything." Forcing users
to choose is overhead.

**Response:** The default *is* everything. Pull all content referenced by the
manifest — same result as push-content, same user experience. The difference is
that the option to be selective exists without special configuration. A
compositor pulls only their shots. An editor pulls only editorial resolution.
The system doesn't need to know these are different use cases; the receiver just
requests what they need.

---

## When Push-Content Wins

Push-intent-pull-content is not universally superior. Push-content is simpler
when:

- **The payload is small.** Generating and transmitting a manifest for a single
  config file is overhead. `scp file.txt server:` will always be simpler.
- **The sender knows exactly what the receiver needs.** A build system pushing
  an artifact to a deploy target doesn't benefit from receiver-side decisions.
- **Real-time streaming.** Live video, log tailing, event streams — the full
  intent isn't known upfront.
- **One sender, one receiver, one time.** If the data goes to one place once
  and you never need to describe, diff, or repeat the transfer, the manifest
  provides nothing you'll use.

The model pays for itself when datasets are large, recipients are multiple or
repeated, integrity matters, or the descriptions themselves are useful objects.
In media production, all of these tend to be true simultaneously.

---

## The Layer Justification

Push-intent-pull-content adds a layer: the manifest sits between sender and
receiver. Every layer must justify its existence. Here is the justification:

The manifest layer cannot be eliminated without losing one of:
- **Persistent description** (rsync's negotiation is ephemeral)
- **Source independence** (push-content locks you to the sender)
- **Receiver-side decisions** (push-content locks you to sender's choices)
- **Deduplication before transfer** (push-content pays bandwidth first)
- **Offline operation** (push-content requires both parties online)
- **Composable descriptions** (ad-hoc links can't be diffed or merged)

Whether all of these matter for a given use case is a decision for the user, not
for the system. The document `DESIGN_ANALYSIS_PLAN.md` outlines a more rigorous
analysis we intend to conduct — genuinely attempting to eliminate the manifest
layer and seeing what, if anything, survives.

---

## Summary

The sender describes. The receiver decides. Content follows demand.

This is one additional step compared to "sender sends files." That step costs
23KB and an email. It buys persistent descriptions, source independence,
receiver autonomy, deduplication before transfer, asynchronous operation, and
composable project state.

Whether that trade is worth it depends on your constraints. For small,
one-time, point-to-point transfers, it isn't. For large-scale media production
with multiple parties, overlapping data, and ongoing coordination, it is.
