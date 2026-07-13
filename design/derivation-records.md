# Derivation records — generalized delta compression via predicate links

Status: idea-stage design note (2026-07-13, Joshua). Connects the
predicate link system (`store-index-and-links.md`), the realization
record (`testimony-record.md`, substrate map #3), and the storage
appendix's one real gap (`store-records-architecture.md` appendix:
whole-file dedup loses 20–75× to CDC systems on mutating files).

## The idea, as stated

Source-repo diff compression can be generalized. A **derivation
predicate** states: item A can be derived by applying process B to
input set C — where, at the first tier, C is a structured collection
of an original document plus a patch document (text diff). Later tiers
extend the same predicate to compression algorithms, binary-delta
processes, images, and other domain-specific derivations.

## Why this slots in cleanly (analysis)

- **It IS the realization record.** `A produced-from (process B,
  inputs C)` with determinism class `exact` is precisely substrate
  map #3 / the testimony record's realization form. No new record kind
  — a new *use* of one already designed.
- **Identity is untouched.** A's ID remains the hash of A's bytes.
  The derivation is testimony about how those bytes can be
  re-obtained; content addressing stays pure. This is what makes it
  generalize where git's packfile deltas cannot: git's deltas are a
  closed, format-internal encoding invisible to everything else; these
  are first-class, inspectable, *falsifiable* records — run process B
  on C and the output either hashes to A or the record is proven
  false. Verification over assertion, applied to compression.
- **Process B is content-addressed too** (predicate-as-ID refinement):
  B names a tool/action description by C4 ID, so the derivation
  vocabulary needs no registry and pins exact semantics.
- **Determinism classes gate what derivations may do.** Only `exact`
  derivations can ever license storage elision (below). `validated` /
  `volatile` realizations remain provenance only.
- **The resolver already prices it.** Fetch vs recompute gains a third
  arm: local-derive (usually milliseconds for base+patch). The
  resolver lab's decision-record shape applies unchanged.

## Storage elision — the delta-compression payoff

With derivation records, the store can drop A's bytes while keeping A
*resolvable*: keep base + patch + the record; re-derive on demand.
This closes the borg/ZFS gap (their chunk/delta stores) with zero
format changes — deltas become a storage *policy*, not a format.

**The rooting tension and its resolution.** Accepted decision #4 says
links never pin bytes — permanent, for federation-DoS reasons. Elision
seems to need the opposite (dropping A requires its inputs to
survive). Resolution: **elision is a store-level act with its own
warrant, not a link-layer implication.** Eliding A is a deliberate,
recorded operation (a disposal class beside trash/scrub — working
name `elide`) whose act explicitly keep-pins an ID-list {B, C…} in the
retention record and leaves a stub/tombstone stating how A recovers.
The link record informs; the retention act pins. Both principles
survive: links still never pin; the *elision warrant* pins its inputs
the same way every other destructive act is written down.

## Tiers (same predicate throughout)

1. **Text diff**: B = unified-diff apply (or c4m patch algebra for
   descriptions — already shipped); C = {base ID, patch ID}.
2. **Chunk maps**: B = concatenate; C = CDC chunk ID list (subsumes
   the perf appendix's chunk-map sidecar).
3. **Compression wrappers**: B = zstd/xz decode; C = {compressed
   object ID}.
4. **Domain processes**: image/video/audio codecs and transforms —
   the resolver lab's FFmpeg realization was tier 4 all along.

## Honest constraints

- Elision trades read latency for space (derive-on-read); the
  resolver's cost model chooses, per object, measurably.
- Derivation chains need a depth/cycle rule and re-derivation
  amplification limits (a chain of 1000 diffs = git's own pathology;
  policy caps, measured).
- `exact` means exact: any platform- or version-sensitivity in B
  demotes the class to `validated`, which forbids elision (the
  resolver-lab x264 lesson).
- gc's closure walk stays link-free (no index on any destructive
  path): elision safety rides the retention record's explicit keep
  lines, never link traversal.

## Next steps (not scheduled)

Requirements doc after the v8 build + testimony-record shape freeze;
the elide disposal class belongs to a future gc design round; tier-1
prototype (text diff over store objects) is a natural first
measurement.

## Generalization: the dependency graph itself (2026-07-13, Joshua)

The predicate generalizes past compression into a full derivation DAG:

- **Having A and computing A are fungible.** Materialized bytes, an
  exact delta route, and a recompute route are interchangeable ways to
  resolve one ID, chosen per context by cost (the resolver's decision
  record, now over N routes instead of fetch-vs-rebuild).
- **Removal is priced against reconstruction.** The gc bill can state,
  per condemned object, whether a derivation route survives and its
  estimated cost — so the ladder of loss is explicit: materialized →
  elided (exact route pinned) → droppable-but-rerunnable (validated /
  volatile route) → unrecoverable. Deletion decisions become economic,
  not binary.
- **One predicate spans tool classes.** There is no structural
  difference between "Render_V002.0001.exr = binary-image-diff applied
  to Render_V001.0001.exr" and "Render_V002.0001.exr = render(scene-v2,
  frame 1)" — two edges to the same target ID, different processes,
  different determinism classes. And the inputs recurse: scene-v2 is
  itself patch(scene-v1, scene-diff). The DAG extends arbitrarily deep.
- **Any subset is supersedable.** Because identity is intrinsic (the
  target ID verifies the result regardless of route), edges are
  competing *testimonies about routes*, not load-bearing structure. A
  new direct edge (one combined patch; one render op) can shortcut a
  thousand-edge chain; old edges simply lose the cost race and their
  exclusive inputs become condemnable through normal retention. This
  is the inversion that Bazel/Nix/Ray cannot make: their action graph
  IS the truth inside one tool's lifetime; here bytes are the truth
  and routes are open, falsifiable, cross-tool, and durable.

Honest edge from the resolver lab, restated: renders are rarely
bit-exact (x264 lesson), so frame-from-scene edges are usually
`validated`/`volatile` — good for reconstruction-with-acceptance,
never for elision. The binary-diff edge to the same frame is `exact`
and can license elision. Both classes coexisting on one target is the
system working, not a conflict; the class travels with the edge and
the economics stay honest.

## Integration path — bolting onto the existing store (2026-07-13)

The store changes not at all at first; the system arrives in four
additive layers, each independently shippable:

**Layer 0 — nothing changes.** IDs, c4m grammar, store layout, ingest,
cat, patch, restore: byte-identical behavior. A store containing zero
derivation records is today's store. (Sidecar doctrine.)

**Layer 1 — records only.** Derivation edges are testimony lines
(shape A): `<A> produced-from <actor> process=<B> inputs=<C-list>
class=exact`. Plain text, writable today by any tool, storable as
ordinary objects. Deliverable: pin `produced-from` + the class
vocabulary in the testimony freeze. Zero c4 code.

**Layer 2 — read-side resolution (first behavior change, opt-in).**
The resolution path (cat/restore), on a missing ID, may consult a
configured derivation-record source for an `exact` route whose inputs
resolve, execute the process, and serve the result — ALWAYS hashed
against A first (store invariant; a lying record is caught by
identity). Optionally cache the materialization back as a normal
journaled ingest. Core executes only built-in, stdlib-only processes:
c4m patch algebra, concatenate (chunk maps), flate. External processes
(bsdiff, ffmpeg) belong to sidecar/zone executors, never core.
Cost policy v1 is trivial (derive if local, else fetch); measurement
refines it.

**Layer 3 — write-side generation (opt-in porcelain).** Something must
mint deltas: a `derive` porcelain (or ingest policy) that, when
storing version N with a known predecessor, computes the patch object
and appends the record. Never silently on the hot ingest path.
This is docket #220's tier-1 prototype.

**Layer 4 — space realization (`elide`, future gc round).** A new
disposal class: pre-flight derives A via the claimed route and
verifies the hash BEFORE any byte dies (never trust a record for a
destructive act), keep-pins the inputs list in the retention record,
drops A's object with a drop line carrying the route reference. The
closure walk is unchanged (it walks the pinned inputs root normally);
layer 2 makes A servable again on demand; the gc bill prints
reconstruction cost per elided object.

Invariants preserved throughout: no unverified bytes ever served or
stored; presence-at-hash-name semantics untouched; links never pin
(the elide warrant pins); no index on any destructive path (elide
verifies by execution, not record trust); c4m grammar frozen; zero
core dependencies (tier-1 processes are stdlib-only).
