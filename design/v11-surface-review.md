# v1.1 surface review — does it subscribe to the philosophy?

Status: review in progress (2026-07-25). Opened because Joshua's
suspicion that the proposed v1.1 API and formats "do not fully
subscribe to the c4 core design philosophy" needs testing against the
shipped surface, not against the design docs.

**The standard being applied**, in his words:

> Look at the c4m and c4 id core designs. Note how ergonomic and terse
> they are. Note how a c4m can lead from a single full description to a
> stream, or how c4 id's are simple self-identifying strings that stand
> without any other structure around them. We want any new work we do
> to follow in this same pattern. The core idea is discoverability, and
> useability.

---

## Finding 1 (CRITICAL) — a directory ID no longer stands alone

**The founding property.** A C4 ID of a *file* is self-sufficient. The
question it answers — "what are these bytes?" — is implicit, universal,
and needs no context. That is what lets a C4 ID travel in an email, a
delivery spec, or a SMPTE document and still mean exactly one thing.

**What v1.1 does.** A C4 ID of a *directory* is the hash of a canonical
listing, and that listing is a **function of scan-time choices**:

1. the projection (`-m c` content vs the default full/snapshot),
2. whether `-S` sequence folding was on,
3. the exclusion set (`--exclude`, `--exclude-file`).

The ID records none of them. The help says so outright:

> "An ID never says which kind it is: compare like with like."
> "…the snapshot records what was scanned, never what was omitted."

**Demonstrated.** One unchanged directory, four ways to ask for "its
ID" — four different, indistinguishable 90-character C4 IDs:

```
full/snapshot (default):      c41nNyZNNee9G4nuzfATDZPmDPaHaMv4JZn…
content (-m c):               c45t4fUBZ35bKmtEJRAkdJdmcjDVK4yJ8DN…
content + folding (-m c -S):  c44ABn7ERU5ZwuUyDGjShpviQ7JgHobVswx…
content + exclude '*.log':    c45VDa6f16gxf5jMyDxwnjJMRW1s5THmvz5…
```

**Both failure directions are reachable, and both are silent.**

*False negative* — two engineers, byte-identical trees, both believing
they computed "the content ID," one with `-S` and one without:

```
engineer 1 (no -S):   c41fcTQbVMxMga7LDUcckj98rsPRbxq2R6aodMAKoGS…
engineer 2 (with -S): c43F2A4EiCjeoVdcXzirwXLXu7qYhJywxeViuzYvgDB…
→ same bytes, different IDs → "the content differs." Wrong.
```

*False positive* — two genuinely different deliveries, both scanned
with a reasonable exclusion:

```
A and B differ only in build.log
$ c4 id -q -m c --exclude '*.log' A   →  c41YLjF3cFHEok5wsf2HcS1iJbRhDDgRhLd…
$ c4 id -q -m c --exclude '*.log' B   →  c41YLjF3cFHEok5wsf2HcS1iJbRhDDgRhLd…
→ identical IDs → "these deliveries are identical." They are not.
```

The false positive is arguably "working as specified" — you asked for
the tree minus logs, and those are equal. That is exactly the point:
**the ID answers a question but does not carry the question.** A
recipient cannot tell an exclusion-scoped ID from a whole-tree ID, and
nothing about the string invites suspicion.

**Why this is worse here than elsewhere.** git's tree hash is also a
hash of a listing — but git has exactly one canonical projection and no
flags, so everyone computes it identically. v1.1 introduces
**flag-dependent identity** into a system whose entire proposition is
that identity is intrinsic to content. Against the philosophy:

- *"Verification over assertion. Claims must be recomputable. IDs are
  never trusted, only checked — a stale record must be falsifiable in
  one command."* A mismatched-projection comparison is **not**
  falsifiable in one command: the two IDs simply differ, and nothing
  reveals *why*.
- *"Determinism everywhere. IDs must reproduce across machines,
  umasks, and clocks."* They reproduce across machines only if the
  flags also match — which is a fourth axis the doctrine never names.

**One-way door.** Once directory IDs are in delivery specs, archives,
and third-party manifests, their meaning is fixed forever. This should
be settled before a v1.1 freeze.

### A candidate fix worth attacking: make `-S` identity-neutral

Of the three parameters, folding is the one that looks spurious.

Exclusions genuinely produce a different tree, so a different ID is
*correct* (the honest framing is "you computed the ID of a subset," and
the surface should probably refuse to call that "the directory's ID").
The projection is a real semantic choice between two questions.

But **folding is an encoding, not a fact about the tree.** A folded and
an unfolded description name the same members with the same bytes. In
the *content* projection, where modes and timestamps are nulled
entirely, folding appears to be strictly information-preserving — the
folded entry records the ID of the member ID-list, and the per-member
mode/mtime it would otherwise collapse have already been nulled.

If that holds, then **in content projection `-S` should not change the
ID**, and today's behavior is a defect of the same family as
[#41](https://github.com/Avalanche-io/c4/issues/41). Fixing it removes
an entire class of silent false negatives and drops the out-of-band
parameter count from three to two.

(In *full* projection folding does lose information — shared mode,
newest mtime — so identity may legitimately differ there, or folding
should be refused in full projection. That is the design question.)

**Status: proposed, not accepted.** Under adversarial review — the
snapshot-loop rounds may have already settled this deliberately.

---

## Finding 2 (to be confirmed) — `c4 id` may be several concepts in one name

`c4 id` now carries: describe, snapshot-and-write (`-s`), stream, parse
stdin (`-`), guided re-scan (`-c`), audit (`--verify`), four detail
levels (`-m s|m|f|c`), sequence folding (`-S`), exclusions, and quiet
(`-q`) — whose meaning changes with multiple paths.

The philosophy says *"Fewest concepts, fully load-bearing"* and *"Safe
by default, fast by flag, destructive by long-form-explicit."* The
open question is whether a single short flag (`-s`) turning a read verb
into a write verb is discoverable, and whether four detail levels are
all load-bearing. Both sides argued under review.

---

## Finding 3 (to be confirmed) — format count is growing

Shipped: c4m (frozen) and the journal (`@c4 journal 1`). Designed but
unbuilt: `@c4 receipt 1`, `@c4 process 1`, and shape-A testimony
predicate lines. That is four to five plain-text formats where the
philosophy asks for the fewest fully load-bearing concepts, and each
costs five reimplementations plus conformance vectors.

The journal itself is a good citizen — two fields, self-identifying,
appendable, streamable. The question is whether receipt/process/
testimony can converge into one line-oriented records grammar rather
than three.

---

## Resolution for the c4m transformation (Joshua, 2026-07-25)

> "So perhaps we now never transform c4m files but rather log their
> existence in the store with a canonical id that points to
> non-canonical c4m files?"

**Yes — and the strongest version of this adds no new concept at all,
because the relationship is already a derivation record.**

### The reframe

Canonicalization is a **deterministic, pure function of bytes**:
`C = canonicalize(R)`. So the fact we want to keep is not a
transformation applied *to* the stored object — it is an ordinary
`exact` derivation edge *about* it:

```
<canonical-id> produced-from canonicalize inputs=<raw-id> class=exact
```

That is Layer 1 of `derivation-records.md`, verbatim, and c4m
canonicalization is the ideal first process for it: stdlib-only,
deterministic, and already implemented (`Canonicalize`,
`WriteCanonical`). Layer 2's read-side rule then handles retrieval
without any special case:

> `c4 cat <canonical-id>` → not present in store → consult the
> derivation record → the `exact` route's input resolves → run the
> process → **hash the result against the requested ID** → serve.

Verification is preserved end to end: a lying record is caught by
identity, exactly as that design specifies.

### What this buys

| Property | Today (transform on ingest) | Proposed (record the relation) |
|---|---|---|
| Restore round-trips a tree with a `.c4m` in it | **no** (data loss) | **yes** — raw bytes are what's stored |
| Entry line's size and ID describe the same bytes | **no** | **yes** |
| `c4 id -q dir/` == `c4 id -s -q dir/` | **no** | **yes** |
| Two differently-formatted c4m → same identity | yes | **yes** (compare canonical IDs) |
| "One exception to raw-byte identification" | required | **the exception disappears** |
| New concepts introduced | 1 (a hidden transform) | **0** (an existing record kind) |

The March 2026 goal survives intact — it just becomes a **true
statement about the bytes** instead of a **substitution of the bytes**.
That is the difference between testimony and forgery, and it is the
system's own founding distinction applied to itself.

### Two design questions this opens

**1. Eager or lazy?** "Log their existence" implies computing the
canonical ID for every c4m-parseable file at ingest. That costs a
parse + canonicalize + hash on the hot path, in five implementations,
for every file that passes phase-1 detection — and phase 1 has real
false positives (a markdown list or a diff both start with `-`), so an
eager index would accrue junk edges for files that merely *look* like
c4m.

Recommended: **lazy.** Store raw bytes; compute the canonical ID only
when asked. Nothing is lost, because a pure function of bytes you
already hold is always recomputable — *"partial knowledge is a valid
state"* and don't precompute what verification can re-derive. An index
can be added later, off every destructive path (`store-index-and-links.md`,
`gc-requirements.md`), if a real query demands it.

**2. Does anything need recording at launch? Probably not.** The
argument path already answers the canonical question for any c4m you
hold — verified today:

```
$ c4 id -q canon.c4m    → c434sqNgqNNZfrzJKYwKeEiqvnAGiZ…
$ c4 id -q pretty.c4m   → c434sqNgqNNZfrzJKYwKeEiqvnAGiZ…   (identical)
```

So the *only* question the record layer would newly answer is the
reverse one: **"which stored objects canonicalize to this description?"**
That is a genuine index question, and it is not release-blocking.

### The practical consequence: the halves are separable

This matters for the freeze:

- **Now (release-blocking, [#43](https://github.com/Avalanche-io/c4/issues/43)):**
  stop transforming. Files inside scanned trees are raw bytes, always —
  which is what `help.go:141-142` already promises and what four sibling
  implementations already do.
- **Later (not release-blocking):** the `canonicalize` derivation edge,
  as an ordinary record when that layer ships. No format change is
  needed to add it, because it is a sidecar record about an ID — the
  sidecar doctrine working as designed.

Nothing is lost in the interval. **Ship the deletion; the relationship
can be recorded whenever the record layer earns its way in.**

### One constraint to write down now

The canonical ID must **never** appear in a c4m entry field. The
grammar is frozen and an entry line describes bytes; the canonical
identity is a fact *about* an object, so it lives on a record line —
the same rule already adopted for assigned/`nom:` IDs.

## What is emphatically right (do not churn these)

- **The journal.** `@c4 journal 1` + `<RFC3339> <ID>` per line. Terse,
  self-identifying, appendable, greppable, readable whole or streamed.
  This is the idiom done correctly.
- **The streaming chain.** A c4m that resolves from a stream of
  entries + one refinement patch + a final root ID is exactly the
  "single full description that can also be a stream" property.
- **stdout/stderr discipline** and the claim/description distinction.
- **`c4 explain`** as a separate verb rather than prose mixed into
  machine output.
