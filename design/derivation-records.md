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

## Call-granularity derivation — memoized tool/MCP calls (2026-07-16, Joshua)

Cross-ref: `receipt-record.md` "The two axes every edge carries" folds
the two axes below into the receipt design at session granularity — a
receipt edge is the coarse tier of this same graph. This section is the
call-granularity refinement.

The same predicate applies one level below the filesystem snapshot: a
**single tool or MCP call is a derivation edge** — `output
produced-from <tool> inputs=<arg+state closure> class=<...>`. If the
call is `exact` and a prior run's edge with the same input-closure ID
already has its output materialized, the resolver serves the cached
output and **skips the call**. Have-vs-compute (the fungibility
principle) at call granularity: re-running a workflow after a small
change pays only for the calls whose inputs actually changed. This is
the receipt design's tool-call edge (`receipt-record.md`) turned into
an incremental-recompute cache; it is the resolver lab's have-vs-compute
(the embedding-index "fetch the cached result" beat from the original
demo) at tool granularity. Same records, queried forward
(process+inputs → output) instead of backward (target → route).

**Why it slots in with zero new machinery.** The memo key is
`H(tool-object-ID, input-closure-ID)`; the memo value is the output ID;
that key→value fact IS a derivation edge, stored and resolved by Layer
2 exactly as written above. Cross-tool (any tool that declares its call
shape), falsifiable (re-run and re-hash — the record is proven true or
false), durable and portable (the cache is the store), and priced by
the resolver. This is the Bazel/Nix/Unison capability without the
single-tool lock-in the DAG section already contrasts — an action cache
where bytes are the truth and routes are open, not one build system's
internal graph.

**The entire difficulty is the input closure, and it is not free.** A
call is cacheable only as a PURE function of its *fully content-
addressed* inputs. This is load-bearing and unforgiving:

- **The closure must be complete.** `read_file(path)` is not a function
  of `path` — it is a function of the file's content ID. `run_tests()`
  is a function of the entire relevant source+test+dep+env closure, not
  of nothing. Miss one input (an env var, a config byte, a clock read)
  and the cache serves a stale result **silently**. This is the classic
  underspecified-action / cache-poisoning failure of every build cache;
  content addressing does not exempt us — it only makes a wrong hit
  *catchable on verification*, never *prevented at declaration*.
- **The class is testimony the tool makes, and C4 cannot infer it.**
  `llm_call`, `web_search`, `bash("date")` are `volatile` — never
  cacheable-as-exact (this is precisely why a session edge is volatile:
  it contains an llm call). `bash("sort f")` is `exact` iff `f`'s
  content is in the closure. A tool (or a thin wrapper) must declare its
  input closure and class; a wrong `exact` is falsifiable but not
  blocked. Invariant carried from elision: **eliding a call trades
  re-execution for trust in the closure declaration** — so the resolver
  must always retain the option to verify by re-execution, and any
  platform/version sensitivity demotes `exact`→`validated` (which may
  reconstruct-with-acceptance but never silently serve).
- **`validated` is opt-in policy, never a default.** Serving a
  `validated` cached result (a render, a flaky test's last green) is a
  choice the user makes per context, not an automatic hit.

**Interaction with the volatile-poisons-upward rule.** A session edge
is `volatile` because it contains a `volatile` sub-call, exactly as a
null descendant poisons a directory's rollup. But the `exact` sub-edges
*under* a volatile parent remain individually reusable — you cannot
re-derive the session, yet you can re-use its deterministic sub-steps
when you replay it by hand. That is the incremental payoff and the
honest limit in one sentence: **the graph memoizes the pure steps; the
impure ones (the model, the network, the clock) always re-run.**

## Resolution intent — the second axis (2026-07-16, Joshua)

The determinism class is a property of the EDGE: "would a fresh run
reproduce this output?" There is a second, orthogonal property that
belongs to the REQUEST, not the edge: **what is the consumer actually
asking for?** Every cache on the web conflates the two, and that
conflation is the "Confirm Form Resubmission" horror — the browser
refuses the recorded response and demands a re-POST because it assumes
the only thing you could want is a *fresh* result. You did not. You
wanted to see what the page said when you loaded it. Those are two
different questions, and the recorded answer is an EXACT answer to the
second one no matter how volatile the call was.

Name the axis **freshness intent** on the request:

- **`current`** (default) — "I want the value a fresh execution would
  produce now." Honored by re-running, or by an `exact` cache hit whose
  inputs are unchanged. A `volatile` call must re-run; a `validated`
  hit is served only by opt-in.
- **`recorded`** (a.k.a. `as-of`) — "I want the value this call
  *produced*, as a historical fact." Served straight from the edge,
  **class-independent** — because you are not asking for a fresh run,
  you are asking for a byte-fact that already happened. A content-
  addressed output is an exact answer to "what did it produce?" even
  when it is a useless answer to "what would it produce now?"

The truth table the whole thing collapses to:

| intent | class | inputs | resolution |
|---|---|---|---|
| current | exact | unchanged | serve cache (equals fresh) |
| current | exact | changed | MISS → re-derive (cheap) |
| current | validated | unchanged | serve only on opt-in |
| current | volatile | any | must re-run |
| recorded | ANY | ANY | **serve the recorded output** |

**Why `recorded` is always honest — and why the web's version is not.**
The sin the browser commits is not caching; it is offering only
"resubmit" or nothing, with no vocabulary for "show me the recording."
And when systems *do* serve a cached thing they hide that it is cached,
so a stale value masquerades as fresh. C4 does neither: a `recorded`
result is served WITH its provenance edge — its inputs, its process,
its timestamp, its class — so it is self-labeling as the T-recording,
never a silent stale swap. Verification over assertion: the value
carries the proof of exactly what it is. Disclosed recording is honest;
undisclosed staleness is the lie. C4 can offer the first because the
response is content-addressed and bound to its request by an edge —
which is precisely the record HTTP never had.

**`recorded` is frequently the CORRECT semantics, not merely the
faster one — because of side effects.** The reason the browser insists
on re-POSTing is that a POST *mutates server state*; effect-freedom is
a third property, distinct from reproducibility (worth its own edge
label: `pure` vs `effectful`). `send_email`, `charge_card`, `POST
/order` are `volatile` AND `effectful`. When you undo an agent workflow
and replay it, you almost never want those re-fired — you want the
recorded outcome so the replay is *consistent* without re-committing
the effect. So `recorded` intent means: serve the record, suppress
re-execution, and therefore suppress its side effects. That is the safe
default for replaying any effectful step, and it is the exact thing no
build cache and no browser will do for you. (The agent-safety framing:
replay-without-re-sending is a feature the entire field lacks.)

**Which recording?** A volatile+effectful call may have many recorded
outputs, each a journaled edge with a scan-start time. `recorded`
therefore takes a selector — `latest` (default), `as-of <time>`, or a
specific run/session ID — answered directly from the journal's per-edge
timestamps. "Show me what the page said when I loaded it Tuesday" is a
real, cheap query over the receipt graph.

This axis is a request-side flag over the same records — **no new
record machinery**, just the resolver honoring an intent it currently
cannot express. It stays in the same platform-tail drawer as
call-granularity memoization, but it is the cheaper, sharper half:
serving a recorded byte-fact needs only Layer 1 (the edge exists) and a
read path that discloses provenance; it does not even need the
input-closure-declaration convention that gates `current`-intent
memoization. It could ship first.

## The identity-basis axis — assigned vs content IDs (2026-07-16, Joshua)

A third first-class axis, deeper than the other two because it is a
property not of the edge or the request but of **every node's identity
itself**. So far every node is a *content* ID — the hash of its bytes,
checkable by recomputation, universal, reproducible across machines.
There is a real, honest regime where that is unavailable or undesired,
and a node is identified instead by an **assigned** (nominal) ID — a
name in some authority's namespace, not derived from content.

**Three rungs of identity knowledge** (partial knowledge is a valid
state, applied to identity):

- `null` — no handle yet. Already shipped: the streaming chain emits
  directory lines with null IDs, then binds them (draft-v9). "I do not
  even have a name."
- `assigned` / nominal — a name, but no verified bytes behind it. "Call
  this node X; I have not (or cannot) hash its content."
- `content` — the verified hash. "I have the bytes and they check."

Unknown → named → verified. An assigned ID is the middle rung: more
than null (it participates in the graph, carries edges, can be planned
over) and less than content (it cannot be checked by recomputation).

**Why this must be first-class AND labeled — the integrity boundary.**
This looks like it contradicts C4's founding claim ("unlike a UUID,
assigned arbitrarily, a C4 ID is discovered from the data itself"). It
completes it. You can only *honestly* prefer content identity if the
system can represent assigned identity explicitly and show exactly where
it fell back to it. The whole aesthetic — verification over assertion,
IDs never trusted only checked — survives on one rule: **no verified
operation may ever treat an assigned ID as a content ID.** That requires
the grammar to make them distinguishable (an assigned ID is visibly not
a 90-char c4 ID — a namespace-qualified tag, e.g. `nom:<authority>/<name>`),
and every read/verify path, on meeting an assigned ID, to either resolve
it through its authority or report "asserted here, not verifiable,"
never to fall through to a hash-mismatch. Labeled assignment is honest;
silent assignment-as-content is the one lie the system must be
structurally unable to tell.

**Containment rule (2026-07-16, binding) — assigned IDs never enter c4m
entry fields.** The c4m grammar is frozen, and its ID field admits
exactly two states: a content ID or null. An assigned ID (`nom:...`)
may appear ONLY on edge/testimony lines — never inside a c4m entry. A
`nom:` tag in a c4m is a parse ERROR in every implementation, not a
third state to handle. Consequences, all load-bearing: (1) all six c4m
parsers remain untouched by the assigned regime — the sidecar doctrine
holds at the grammar level, not just in spirit; (2) the streaming/null
rung already covers "no content ID yet" WITHIN a c4m, so nothing is
lost; (3) a c4m is by definition a description of content — a node the
graph knows only nominally has no c4m yet, and its place in the graph
is held by the assigned ID on its edges until promotion. The graph may
be partially nominal; a c4m file never is.

**Promotion — the "moving toward content-addressed" case.** An assigned
node is *promoted* when its bytes are materialized and hashed, recorded
by a **binding**: `nom:auth/x resolves-to <content-id>` — a shape-A
testimony line, the same machinery as every other edge. A graph is
therefore *partially content-addressed*, and becomes "more true" as
nodes promote — identical in shape to the streaming refinement (null dir
ID → real ID), one rung higher. The binding is testimony ("x was these
bytes"), falsifiable only where enough was kept to re-check; promotion
restores verification from that point forward.

**Value in itself (not only a transitional state).**
- **Cheap structural snapshots.** Capture the graph/workflow *shape* of
  a terabyte working set now, at near-zero cost, by assigning IDs to the
  big nodes and hashing lazily or never. You get the dependency
  structure and the receipt immediately; content verification is
  deferred or forgone — honestly labeled as unverified.
- **Representing the un-hashable.** Running services, live streams,
  remote resources behind an API, an effectful call's outcome you never
  materialized — things with identity but no fixed local bytes. Assigned
  IDs let the graph be *complete* (the user's word): it can represent
  the whole workflow including its external, mutable, un-hashable parts,
  instead of only the local-bytes subset.
- **Forward references / build-graph-then-execute.** Name a node that
  will be produced, wire the DAG, materialize later (the DAG-first
  pattern ThruWire uses; a promise/future as a first-class node).
- **Privacy stand-ins.** An assigned handle that *deliberately never
  resolves* — "a node exists here and these edges touch it" without
  holding or disclosing its content.
- **Resolver planning.** Operate on structure (assigned) for routing and
  cost estimates; force content-addressing only when verification is
  actually demanded.

**Honest constraints (the price of the regime).**
- **Non-portable / non-federating.** A content ID resolves in any copy
  of any store; an assigned ID is meaningful only relative to its
  assigning authority. The "copy a store anywhere and every ID resolves,
  roots are virtual" property is *forfeited* for the assigned subgraph
  until promotion. Assigned IDs must carry their namespace or collide.
- **Asserted, not verified.** No recomputation can catch a lie about an
  assigned node — there is nothing to recompute against. Trust rests
  entirely on the assigning authority's testimony. The only thing
  keeping the system honest is the label.
- **An edge is only as verifiable as its least-content-addressed node.**
  An `exact` process whose inputs are assigned cannot be checked by
  recomputation until those inputs promote — so the interesting cell is
  `exact` process + assigned inputs = *deterministic but unverifiable
  here*, upgradable to fully verifiable on promotion. The class still
  describes the process; checkability tracks the node basis.

**Orthogonality.** Node identity-basis (node property) is independent of
reproducibility class (edge property) and freshness intent (request
property). Every combination is meaningful: a `volatile` edge can have
`content`-addressed nodes (we hashed the outputs though we cannot
reproduce them — the launch receipt is exactly this); an `exact` edge
can have `assigned` nodes (pure process, un-hashed inputs).

**Prior art for the duality (brief).** Nix input-addressed vs
content-addressed (CA) derivations is this exact axis — and
`ca-derivations` is literally Nix "moving toward content." git separates
assigned refs (mutable names) from content-addressed objects; IPFS
separates IPNS (assigned, mutable) from CIDs. Each keeps the two poles
cleanly distinct and resolves one to the other — the model to match.
None treats the basis as a *labeled per-node honesty property that
degrades and promotes*; that framing is the C4-native contribution.

**Scheduling.** Platform-tail with the rest of the records substrate,
but weightier: it touches the ID grammar (assigned IDs must be
syntactically un-mistakable for content IDs), so its freeze belongs
pinned at the records/testimony freeze, not bolted on later. The `null`
rung already ships (streaming refinement); the `assigned` rung and the
`resolves-to` binding are the new first-class citizens.

## Prior art and where the moat actually is (2026-07-16 landscape scan)

The five properties that must co-occur: (a) content-addressed
materialized outputs; (b) a portable, cross-tool re-execution graph;
(c) a determinism CLASS that *labels* non-determinism (exact/validated/
volatile) rather than forbidding it; (d) a per-request choice between
recompute-fresh and serve-recorded; (e) side-effect-suppressing replay.
A dated scan (classics + 2024–2026 entrants) found **no system that
unifies all five**, and located the moat precisely at (c) and (d).

- **Durable-execution family — the (e) cluster, requires-not-labels
  determinism.** Temporal, and 2025 newcomers DBOS, Restate, Inngest,
  Resonate: a per-run journal replays recorded step results on recovery
  so effectful steps don't re-fire (genuine (e), in production). All
  drop (a)(b)(c)(d): the store is keyed by workflow/step ID in a DB not
  a content hash; the journal is per-run not a portable artifact DAG;
  they *require* determinism and make you wrap non-determinism in a
  step (the anti-(c)); replay is recovery-only, never a user toggle
  (Resonate's own docs frame determinism as a constraint to satisfy).
- **Content-addressed ML builds — the (a) cluster.** Flox/Nix-for-ML
  (2025): genuinely content-addressed weights ("same hash = same
  weights"), partial (b) as a build-time dep graph. But Nix *enforces*
  purity — it forbids non-determinism (anti-(c)) and side effects
  (anti-(e)), and serves by hash with no recompute-vs-recorded toggle
  (anti-(d)).
- **Agent observability (LangSmith/Langfuse/Braintrust/Phoenix) and
  LLM VCR libs (vcr-langchain, pytest-recording).** Record-only traces;
  "replay" re-runs live (no (e) unless you mock); cassettes are
  per-test YAML, not a durable content-addressed store. Records, not a
  substrate.
- **The one independent hit on (d).** AI21's 2025 essay "Reproducing
  Variance: Caching in Agentic LLM Pipelines" argues explicitly that
  agentic pipelines need BOTH serve-cached AND recompute, *selectable*
  within one pipeline — axis (d) stated as a felt need by a serious lab.
  But it is a cache-KEY design essay: no first-class content-addressed
  store, no cross-tool graph, no determinism-class taxonomy, no side-
  effect suppression. The need is articulated; nobody has productized it.
- **Research converging on (a)+(b) — arXiv IDs verified; the closest
  one read at full text (copies + verification in
  `plans/gtm-2026-07/prior-art-papers/`).** "From Agent Loops to
  Deterministic Graphs" (ThruWire, 2605.06365, May 2026) is the closest
  published work: a DAG of artifact-producing nodes with identity-based
  replay AND **selective recompute** (recompute only descendants of a
  changed artifact, preserve unrelated branches). It genuinely validates
  the (a)+(b)+incremental-recompute direction — so that foundation is
  NOT ours alone. But (verified from full text) it treats
  non-determinism as a *defect to structure away*, not a served-able
  class (drops (c)); its replay-vs-recompute is *change-driven
  incremental for the CURRENT value* — the axis-(d)-current half every
  build system has — with no per-request recorded-intent choice (drops
  the sharp half of (d)); it does not address side-effect suppression
  (drops (e)); and "execution identity" is a runtime key inside their
  framework, not a portable content-addressed byte store. "Portable
  Agent Memory" (Microsoft, 2605.11032) is the closest to C4's
  store+signed-testimony SHAPE (content-addressed Merkle-DAG + signed
  provenance edges) but is a memory-transfer protocol, not a recompute
  route (drops (c)(d)(e)). "From Agent Traces to Trust" (2606.04990) is
  the best survey — descriptive provenance, not executable.

**Reading.** (a) and (b) and even *incremental recompute for the current
value* are validated-and-converging — ThruWire has built and published
the graph-of-identity-keyed-nodes-with-selective-recompute core, and the
build-system lineage predates all of it. So that foundation is right and
not ours alone. The unclaimed square is therefore NARROWER than "the
whole derivation graph" and it holds precisely: **(c) labeling
non-determinism as a served class + the RECORDED half of (d) (serve the
historical byte-fact by intent, regardless of freshness) + (e)
side-effect-suppressed replay — unified on a portable content-addressed
byte substrate where bytes are the truth.** The
durable differentiation is (c)+(d)+(e-done-as-choice): labeling
non-determinism as a served-able class instead of a bug, and making
freshness a per-request intent instead of the system's decision. Both
are genuinely unclaimed; (d) has one dated external signal that the need
is felt (AI21, 2025) and (e) is being assembled separately by the
durable-exec vendors. The pieces are being built in isolation; the
unification — one content-addressing scheme where bytes are the truth
and routes/records/classes/intents are all first-class and cross-tool —
is the unoccupied square. The window is real, not infinite. (Confidence:
product claims solid/2025; arXiv items reported at abstract level; SEO
explainer sources discarded.)

## The graph IS winnable — as a standard, not as internals (2026-07-16, Joshua)

Correction to the "not ours alone" reading above: it measured the wrong
kind of winning. ThruWire's graph is *internal to ThruWire's runtime*;
Bazel's is internal to Bazel; Temporal's event log is internal to
Temporal; Nix's derivations are internal to Nix. **Every existing
derivation/lineage graph is a single-system truth** — true inside one
tool's lifetime and one tool's namespace (this doc's own DAG section
says exactly that: "their action graph IS the truth inside one tool's
lifetime; here bytes are the truth and routes are open, falsifiable,
cross-tool, and durable"). A vendor's internal graph structurally cannot
become the neutral interchange — it is a conflict of interest (Temporal
will not adopt ThruWire's format; each is the internals of a product).

The empty, defensible position is the one none of them can occupy: **an
open, discoverable, plain-text, content-addressed derivation record that
is the interchange BETWEEN the closed runtimes, not the internals of
any one.** The thing every ThruWire/Bazel/Temporal/Claude-Code and every
DIT's ingest tool *emits and reads*, the way ASC MHL is the thing every
media ingest tool emits rather than any one vendor's internal hash log.
The proliferation of graph-emitters (ThruWire, the agentic-caching
cluster, the 2026 provenance preprints) is therefore the *precondition*
for a neutral standard, not a threat to it — a standard needs multiple
implementations to matter.

**This is a repeatable, decade-proven play, not a hope — and it is
literally C4's own play.** Media production learned, over a decade-plus,
that both content identity AND asset relationships must be open
standards, not tool internals, because the pipeline is irreducibly
multi-vendor (camera → DIT → editorial → VFX houses → color → deliver →
archive): content identity became a standard (C4 → SMPTE ST 2114; ASC
MHL, which already carries C4 digests; Netflix-mandated MHL ingest), and
the *derivation/relationship graph* is being standardized right now,
vendor-neutral (MovieLabs 2030 Vision + the Ontology for Media Creation;
C2PA content credentials; OpenTimelineIO for editorial structure). The
agentic derivation graph is the same shape as the media derivation
graph. The same primitive (content ID as an open standard) and, where it
matters, the same author (the person who authored ST 2114) can win the
agentic graph the same way the media graph is being won — as an open
standard and a completely open discoverable system.

**What this changes in the moat.** The durable moat is not only the
narrow (c)+(d)+(e) combination — it is the **open cross-vendor,
content-addressed, plain-text derivation standard itself**, which the
closed-runtime builders cannot be. The (c)+(d)+(e) axes and the
identity-basis axis are what make that standard *complete and honest*
(it can represent non-determinism, recorded intent, side effects, and
assigned-vs-content identity — the things a closed runtime hard-codes or
omits); openness and standards-authority are what make it *win* and what
defend it against platform absorption (a platform can ship a feature; it
cannot author and cede a neutral standard — its version is, by
construction, one more internal graph).

**Launch discipline, unchanged.** Standards win by adoption, slowly, not
by assertion — so "we are the standard" is never launch copy, exactly as
the media wedge is date-gated. What a solo founder CAN do now is the
thing C4 already does on cccc.io: author and publish the record grammar
as an open, plain-text spec with a reference implementation in the OSS
suite, so the graph is discoverable and emittable by anyone from day
one. The receipt/derivation records ARE that spec extending ST 2114
upward from identity to lineage. Publish open; let adoption earn the
word "standard."

## Creation is a first-class edge — class is verification, never worth (2026-07-16, Joshua)

A correction to how the class axis is READ, binding on every doc, UI,
and launch sentence that touches it.

**The mathematical definition stands; the valence goes.** `volatile`
means exactly one thing: a fresh deterministic run would not reproduce
this output. It is a statement about what a VERIFIER can recompute —
and nothing else. Left uncorrected, the word quietly imports a defect
reading ("couldn't be reproduced" ≈ "couldn't be trusted" ≈ "worth
less"), and that reading is not merely wrong — it is backwards.

**The value inversion.** An `exact` edge cannot create value. It
transports and transforms deterministically; its output is already
implied by its inputs — which is precisely WHY it is recomputable. A
workflow built only of exact edges is a closed pipeline: correct,
verifiable, and empty, because nothing ever entered it. Value enters
the graph ONLY through edges that cannot be recomputed. The media
conceptualization of C4 has always understood this: an artist creates
an original (a creation point — an edge with no inputs at all), or
performs transformative creation (inputs: the director's notes and the
existing shot; irreproducible work done by the artist; output: the
updated shot). These must not be dismissed as lesser nodes in any way —
in most contexts they are the core value being expressed by inclusion
in the workflow. Strip them and the workflow is soulless, valueless: an
audit trail of transformations of nothing. Irreproducibility is not a
missing guarantee on a creative edge; it is the SIGNATURE of creation.
If a deterministic re-run could have produced it, it was transport, not
authorship.

The same holds in the AI-coding receipt: the volatile llm edge is not
the receipt's weak point — it is the step the session existed to
obtain. Everything `exact` downstream (the build, the tests, the
packaging) is in service of the irreproducible step, never the reverse.

**This completes the field position; it does not just adjust tone.**
The landscape scan above located the moat at (c): the entire field
treats non-determinism as a defect to structure away (ThruWire names
"identical inputs may produce different outputs" a problem and
engineers around it; Nix forbids it; the durable-execution vendors wrap
it in apology). C4 already differed by SERVING it as a typed class. The
creation reading completes the position: the irreproducible edge is not
merely served, it is honored. The graph's purpose is to carry
verifiable transport BETWEEN acts of creation — and to certify the
creations themselves in the only way creation can be certified (below).

**Flake and creation are the same math and different testimony.** A
flaky network call and an artist's original are both `volatile` —
non-recomputable — and only one is a creation point. The distinction is
not mathematical (no re-run test separates them); it is ATTRIBUTIONAL:
someone stands behind the work. That is testimony-shaped, so it lives
in the testimony layer, never in the class axis or the edge grammar:

- The class axis stays pure verification — `exact` / `validated` /
  `volatile` — untouched.
- A creation edge is a `volatile` edge with an **`authored-by`**
  testimony line over it (or over its output node):
  `<id> authored-by <author> [at=<time>]`. Volatile + authored = a
  creation point. Unattributed volatility stays what it is — noise,
  flake, entropy.
- Sidecar doctrine, zero new concepts: attribution is exactly what
  shape-A testimony was built for, and `authored-by` is one predicate
  inside the existing budget (testimony-record.md).

**Certification inverts across the classes — the anchoring corollary.**
A derivation record can carry two mutually reinforcing certifications:
(A) "this sequence produces this output," and (B) "this record was
first known to exist at time T" (an anchor). Their weights invert
across the class axis:

- On an `exact` edge, (A) is a theorem — anyone re-runs and checks —
  so (B) adds the least: a timestamp on something already self-proving.
- On an authored edge, (A) is meaningless BY NATURE — creation has no
  re-run — so (B) is the ONLY external certification a creation can
  have, and it is precisely the one creators have always wanted:
  **priority of creation** — this work, attributed to this author,
  first known to exist at T. (The poor-man's-copyright ritual, done
  with a recomputable ID instead of a sealed envelope.)

The anchor matters most exactly where verification-by-rerun matters
least. This is also why the media wedge is the natural first home for
anchoring (per the anchoring litigation's gating): a media workflow's
graph is DENSE with authored edges, and "anchored priority of creation,
bound to exact content IDs" is the product sentence there — while in
the dev wedge the same mechanism is a compliance afterthought and stays
out of launch copy entirely.

**Rendering doctrine (binding).** No UI, doc, or badge may present
class as a quality tier. A receipt viewer that colors `exact` green and
`volatile` yellow has inverted the value model of the very workflows it
records. Prominence follows value: the authored edge is the headline of
its graph; the exact edges are its provenance apparatus. Class renders
as what it is — a verification mode ("recomputable" / "checkable" /
"recorded testimony") — never as a grade.

## Scheduling and the wedge

**Platform tail, not launch.** Call-granularity memoization needs Layer
1 (records) + Layer 2 (read-side resolution) + a per-tool input-closure
declaration convention that does not exist yet; `recorded`-intent
serving needs only Layer 1 + a provenance-disclosing read path. Both are
tier-3/4, drawer-resident until pulled — but they are the strongest
evidence that the receipt wedge installs the right substrate: every user
minting call-level edges is building a portable, cross-tool, falsifiable
action cache — and a personal, timestamped record of "what every call
actually returned" — without being told the word. The launch says none
of this; the data it accretes is what makes this shippable later.

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
