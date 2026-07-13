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
