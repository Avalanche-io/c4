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
- **Research converging on (a)+(b) — reported, preprint-level (May–Jun
  2026, not deep-verified).** "Portable Agent Memory" (BLAKE3
  Merkle-DAG content-addressed + Ed25519 signed provenance edges) is the
  closest published shape to C4's store + signed testimony; "From Agent
  Loops to Deterministic Graphs" proposes identity-based replay over an
  artifact DAG; PROV-AGENT/AgentTrace give typed derivation relations
  with artifact-hash edges. All are descriptive provenance, not an
  executable recompute route, and **all drop (c)(d)(e)**.

**Reading.** (a) and (b) are validated-but-converging — the academic
field is actively moving toward content-addressed agent provenance, so
the substrate direction is right *and* not ours alone for long. The
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
