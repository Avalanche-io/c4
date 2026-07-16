# Receipt records — the AI-coding derivation edge

Status: design mockup (2026-07-16, Joshua + agent). NOT frozen. Purpose:
walk a concrete 2-prompt Claude Code session end-to-end — every object,
every edge, every reconstruction — to (a) discover missing functionality
and (b) fix the UI surface (hooks / MCP / CLI). Grounded in the shipped
v1.1.0 surface and the existing designs: `derivation-records.md` (the
receipt IS a `produced-from` edge, class `volatile`), `testimony-record.md`
(shape-A predicate lines; evidence recomputed, testimony recorded), the
minimal journal (`@c4 journal 1`), `store-records-architecture.md` (one
authority per question).

## The one-sentence design

A receipt is **one derivation edge**: `post-state produced-from
<harness> inputs=<pre-state> process=<process-object> class=volatile`.
The nodes (pre/post) are ordinary journaled snapshot claims; the edge is
a testimony line; the process object is a content-addressed blob. **No
new primitive** — it is the derivation record's first shipped instance,
and an AI coding session is just determinism-class `volatile`.

## Actors and stores

- `proj/` — the working tree.
- The store lives OUTSIDE the tree (`~/.c4/store`) — mandatory: if the
  store were inside `proj/`, `rm -rf proj` would kill the undo. This is
  why a pasted receipt is "recomputable where the bytes live," never
  remote proof (the honest limit, structural, not incidental).
- `<store>/journal` — the minimal claims log (scan-start + ID per claim).
  Holds NODES.
- Receipt file — a new `@c4 receipt 1` records file. Holds EDGES. The
  plugin owns one per session; it is itself ingested → journaled like any
  object once complete.

## Timeline of a 2-prompt session (every object it mints)

```
t0  SessionStart hook
      c4 id -s proj/            → S0   (journaled claim, scan-start t0)   ← BASELINE NODE
      plugin opens session state: {session=SES, baseline=S0, edges=[]}

t1  UserPromptSubmit hook  (prompt 1: "remove parseLegacy from parser.go, fix callers")
      plugin stashes prompt-1 text → ingest → PR1  (prompt object, C4 ID)

    ── agent works: edits parser.go, 2 callers; runs pytest (writes
       .pytest_cache/); a shell step accidentally leaves .env.bak;
       transcript says "Done. Modified parser.go + 2 callers. Tests pass."

t3  Stop hook  (turn 1 complete)
      c4 id -s proj/            → S1   (journaled claim, scan-start t3)   ← NODE
      plugin ingests transcript slice for turn 1 → TR1 (transcript object)
      plugin mints PROCESS OBJECT P1:
          @c4 process 1
          harness    claude-code 2.1.206
          model      claude-opus-4-8
          prompt     PR1
          transcript TR1
          started    t1
          ended      t3
        → ingest → P1 (C4 ID)
      plugin appends EDGE-1 to the session receipt:
          S1 produced-from claude-code inputs=S0 process=P1 class=volatile

t4  UserPromptSubmit hook  (prompt 2: "rename it parseModern everywhere, add a test")
      prompt-2 text → ingest → PR2

    ── agent works: renames symbol across 3 files, adds tests/test_parse.py

t6  Stop hook  (turn 2 complete)
      c4 id -s proj/            → S2   (journaled claim)                  ← NODE
      transcript slice → TR2 ; process object P2 (prompt=PR2, transcript=TR2)
      EDGE-2 appended:
          S2 produced-from claude-code inputs=S1 process=P2 class=volatile

t7  SessionEnd hook
      finalize the session receipt file → ingest → REC (its own C4 ID,
      journaled). REC is the portable, verifiable session artifact.
```

## The graph this draws

```
        [P1: prompt PR1 / transcript TR1]        [P2: prompt PR2 / transcript TR2]
              │ produced-from                          │ produced-from
   (S0) ──────┴──────▶ (S1) ─────────────────────────┴──────▶ (S2)
  baseline            after prompt 1                        after prompt 2
   node                node                                  node

  Nodes  = journaled snapshot claims (whole tree, .git/.env/ignored/untracked included)
  Edges  = receipt lines (testimony: "claude-code produced Sn from Sn-1 via Pn")
  Inputs of each edge recurse: PR1, TR1 are themselves content-addressed objects.
```

Objects in the store after the session: **S0 S1 S2** (nodes) + **PR1 PR2
TR1 TR2** (process inputs) + **P1 P2** (process objects) + **REC** (the
receipt file). Journaled claims: S0, S1, S2, and REC (the prompt/
transcript/process blobs are ingested but need not each be journaled —
they are reachable from REC; open question O4).

## The receipt file (the portable artifact)

```
@c4 receipt 1
session claude-code SES started=t0 baseline=S0
S1 produced-from claude-code inputs=S0 process=P1 class=volatile
S2 produced-from claude-code inputs=S1 process=P2 class=volatile
```

Shape-A predicate lines (testimony-record.md), one edge per line, plus a
session header. `@c4 receipt 1` magic = self-identifying, c4m-parser-
rejecting (the pattern we just shipped for the journal). Appendable,
greppable, line-local, diffable. Its own C4 ID (REC) is the signable
subject — the hook for future signed receipts (testimony/signatures,
post-launch).

## Reconstructions — what each question costs

Everything in the "shipped" column runs TODAY on v1.1.0 with zero new code.

| Question | Command | Status |
|---|---|---|
| What did prompt 1 *actually* change? | `c4 diff S0 S1` | ✅ shipped — the receipt headline |
| …vs what the transcript claimed | eyeball TR1 next to the diff | ✅ shipped |
| What did prompt 2 change? | `c4 diff S1 S2` | ✅ shipped |
| Whole-session net change | `c4 diff S0 S2` | ✅ shipped |
| Undo prompt 2 | `c4 restore --force S1 proj/` (prints undo handle) | ✅ shipped |
| Undo the whole session | `c4 restore --force S0 proj/` | ✅ shipped |
| Is the prompt text authentic? | `c4 cat PR1 \| sha…` = the ID in P1 | ✅ shipped (content-addressed) |
| Are the nodes real, not invented? | S0/S1/S2 appear in `c4 log` | ✅ shipped |
| Which prompt introduced file X? | walk edges, first diff that adds X | ⚠ manual today; index later |
| Re-derive S1 from S0 + process | — | ❌ impossible (volatile: can't re-run inference) — BY DESIGN |

The last two rows are the honest core: **the process is testimony, the
states are evidence.** You cannot reproduce S1 by re-running the agent,
so the receipt never claims that. It claims: these two states are real
and complete (journaled), and here is the exact, recomputable delta
between them — including everything the transcript and `git diff` never
saw. That is the whole pitch, and it is exactly determinism-class
`volatile` behaving as `derivation-records.md` specified.

## Two properties that fell out (both good, both quotable)

1. **The receipt leaks no secret content.** `c4 diff` emits a c4m patch —
   entry lines (mode, time, size, name, ID), never file bytes. A changed
   `.env` shows as "path `.env`, ID a→b," not the secret. The receipt is
   an ID-level delta; it is inherently safe to paste. (Contrast: a git
   diff of a committed `.env` leaks it.)
2. **Undo is itself an edge.** A `/undo` to S0 is a derivation edge
   `S0' produced-from user process=<undo to S0>` — the reflog literally
   becoming the graph. Manual recovery actions record in the same
   grammar as agent actions. (Whether the plugin writes these at launch:
   open question O5.)

## The two axes every edge carries (the replay model)

The session edge above is one granularity of the same record that
`derivation-records.md` defines at tool-call granularity ("Call-
granularity derivation" and "Resolution intent — the second axis").
Two orthogonal axes govern whether a call is replayed live or served
from a prior recording — and they are the whole reason a partial re-run
can reuse the unchanged work. They belong in the receipt design because
a receipt is a stack of these edges.

**Axis 1 — reproducibility class (property of the edge):** would a fresh
run reproduce the output?
- `exact` — pure function of a fully content-addressed input closure
  (a deterministic transform, a `c4m` patch, `sort` over known bytes).
- `validated` — reproducible up to accepted tolerance (a render — the
  x264 lesson); reconstruct-with-acceptance, never silent elision.
- `volatile` — not reproducible (an `llm_call`, `web_search`,
  `bash date`). **A session/receipt edge is `volatile` because it
  contains an llm call** — volatile poisons upward, exactly like a null
  descendant poisons a directory's rollup. But `exact` sub-edges *under*
  a volatile parent stay individually reusable: you cannot re-derive the
  session, yet you can reuse its deterministic sub-steps when you replay
  it. The graph memoizes the pure steps; the model, the network, and the
  clock always re-run.

**Axis 2 — freshness intent (property of the request):** do you even
want a fresh run?
- `current` (default) — the value a fresh run would produce now. A
  `volatile` call must re-run; an `exact` hit with unchanged inputs is
  served; changed inputs are a cheap re-derive.
- `recorded` (a.k.a. `as-of`) — the value the call *produced*, as a
  historical byte-fact, served straight from the edge **regardless of
  class**. A content-addressed output is an exact answer to "what did it
  produce?" even when it is a useless answer to "what would it produce
  now?" Served WITH its provenance edge (inputs, process, timestamp,
  class) so it self-labels as the recording — disclosed recording is
  honest; undisclosed staleness is the web's lie ("Confirm Form
  Resubmission" is the browser conflating the two axes: it demands a
  re-POST because it assumes intent is always `current`). Takes a
  selector — `latest`, `as-of <time>`, or a session ID — answered from
  the journal's per-edge timestamps.

**Axis 3 (adjacent, for effectful calls) — effect-freedom** (`pure` vs
`effectful`): `send_email`/`charge_card`/`POST /order` are `volatile`
AND `effectful`. `recorded` intent suppresses re-execution and therefore
the side effect — **replay-without-re-sending**, the correct semantics
for replaying any effectful step, which no build cache or browser
offers.

**The node's identity basis** (deeper than these axes — a property of
each node, not the edge or request): a node is `content`-addressed
(verified hash, the default), `assigned`/nominal (a namespace name, no
verified bytes — cheap structural capture, external/un-hashable nodes,
forward references), or `null` (no handle yet — the streaming rung).
Three rungs: unknown → named → verified, promoted by a `resolves-to`
binding. Load-bearing rule: no verified operation ever treats an
assigned ID as content. A launch receipt is always `content`-addressed
(the snapshots are hashed); the assigned regime is a platform-tail
capability for when hashing is unavailable or undesired. Full model in
`derivation-records.md` "The identity-basis axis."

### Why this is the receipt design's payoff, not a tangent

The user's replay scenario — *do something, undo it, change some things
but not others, re-run* — is exactly this model at work: on the re-run,
each tool-call edge whose `(process, input-closure)` matches a prior
recording and whose class is `exact` is served from the store (skip the
call); the calls whose inputs changed re-execute; the effectful calls
under `recorded` intent serve their prior outcome without re-firing.
The receipt already records these edges at session granularity; pushing
the SAME edge to call granularity turns the receipt into an incremental-
replay cache. Nothing here is launch scope — see the scheduling note in
`derivation-records.md` — but it is why the launch's per-session edges
are the right substrate: they are the coarse tier of a graph that
refines downward without a format change.

## Missing functionality — the discovery

Sorted by whether the launch needs it.

**Free today (v1.1.0 shipped) — the launch rides these unchanged:**
- Snapshot nodes: `c4 id -s` (S0/S1/S2). ✅
- The receipt body: `c4 diff <id> <id>` between two snapshot IDs. ✅
  (v9 confirmed diff sides accept store IDs.)
- All reconstructions/undo: `c4 restore --force`. ✅
- Content-addressing prompts/transcripts/process objects: `c4 id -s` on a
  file. ✅
- Journaling nodes: automatic under `-s`. ✅

**Must build for launch (all plugin-layer porcelain, no c4 core change):**
1. **The receipt record grammar** — `@c4 receipt 1` + the shape-A edge
   line + session header. Frozen text spec (rides the testimony freeze;
   ≤6-predicate budget — `produced-from` is one).
2. **The process-object schema** — `@c4 process 1` fields (harness,
   model, prompt, transcript, started, ended). Small; pin the field set.
3. **The plugin session-state file** — hooks are separate process
   invocations; something must carry {SES, S0, prior Sn, edges} across
   them. Plugin-local (`<store>/sessions/<SES>` or XDG state). 
4. **Hook wiring** — SessionStart→S0, UserPromptSubmit→stash prompt,
   Stop→Sn+edge, SessionEnd→finalize. Depends on Claude Code hook
   payloads carrying prompt text + transcript path (verify availability).
5. **`/receipt` command** — render the current session's receipt as one
   pasteable block (the acquisition artifact). Pure composition over
   `c4 log` + `c4 diff`.

**Build later (NOT launch — the platform tail):**
6. **`c4 receipt` core verb** (render / verify) — promotes the plugin
   composition into porcelain; `verify` checks nodes-in-journal +
   prompt/transcript hashes + recomputes each diff. Launch uses plain
   text; the verb is the hardening.
7. **A read-side edge index** — "which session introduced byte X" needs
   finding edges by node. Stays off every destructive path
   (store-index-and-links.md). Enables the DAG queries.
8. **Signed receipts** — testimony signatures over REC's ID → the only
   path to honest *remote* (maintainer-side) verification, which we
   ruled out of the launch as dishonest today.
9. **`elide`** — once edges exist, exact edges can license storage
   elision (derivation-records.md Layer 4). Volatile AI edges never
   elide; this rides `exact` media/build edges.
10. **Call-granularity edges + the two-axis resolver** — push the
    session edge down to per-tool-call edges, and teach the resolver
    the `current` vs `recorded` freshness intent so a partial re-run
    reuses unchanged deterministic calls and replays effectful ones
    from their recording. Full model in `derivation-records.md`
    ("Call-granularity derivation", "Resolution intent"). The
    `recorded`-intent half is the cheaper blade (needs only Layer 1 +
    a provenance-disclosing read path — no input-closure convention)
    and could ship before `current`-intent memoization.

## The UI surface — and the one principle that governs it

**Receipts are minted by HOOKS, never by agent-callable tools.** The
entire pitch — "the agent writes the transcript, it cannot write the
receipt" — dies the instant the agent can author its own receipt. This
is the Berkeley RDI lesson (evidence produced inside the agent's control
is forgeable) applied to our own design. So the surface splits by trust:

### Hooks — the trust boundary (deterministic; the agent cannot invoke or suppress)
| Hook | Action |
|---|---|
| SessionStart | `c4 id -s` → S0; open session state |
| UserPromptSubmit | ingest prompt text → PRn; stash in session state |
| Stop (turn end) | `c4 id -s` → Sn; ingest transcript → TRn; mint Pn; append edge |
| SessionEnd | finalize + ingest the receipt file → REC |

This is the only surface that writes receipts. It runs as plugin code the
model does not control.

### MCP tools (c4-mcp) — agent-facing CAPABILITY, not receipt authority
The agent may USE these to work safely; none of them mint receipts.
| Tool | Purpose |
|---|---|
| `snapshot(dir) → id` | agent takes a checkpoint before a risky step |
| `restore(id, dir) → preimage-id` | agent rolls back its own experiment |
| `diff(a, b) → manifest` | agent inspects what it changed |
| `contains(id) → bool` | store-membership probe |
Byte-pure single-line outputs (the machine-output contract). A receipt is
NOT an MCP tool — deliberately.

### CLI / API — human + CI
`c4 id -s`, `c4 diff`, `c4 restore`, `c4 log` today; `c4 receipt
render|verify` later. CI use: a post-job hook mints a receipt for an
autonomous agent run; the reviewer reads `c4 diff` between the job's
pre/post nodes — server-side, where the bytes live, so recomputation is
honest there even though a PR paste is not.

## Open questions (for Joshua)

- **O1 — node granularity.** Per-prompt (one turn = one edge) is the
  natural unit of intent and the cleanest graph. Confirm vs per-tool-batch.
- **O2 — store location for receipts.** Nodes must live outside the tree
  (undo requires it). Do prompt/transcript/process/REC objects share the
  same default `~/.c4/store`, or a per-repo store for portability? (Trade:
  portability vs blast-radius vs repo bloat.)
- **O3 — magic header reuse.** `@c4 receipt 1` and `@c4 process 1` mirror
  `@c4 journal 1`. Confirm the pattern generalizes (one records-family
  convention) vs distinct treatment.
- **O4 — journaling depth.** Journal only S0/S1/S2 + REC (blobs reachable
  from REC), or journal every object? Fewer claims = leaner journal;
  reachability-from-REC must then be a guaranteed property.
- **O5 — record manual undos as edges?** `/undo` as a `produced-from
  user process=undo` edge (reflog-as-graph, elegant) vs keep receipts
  agent-only at launch (simpler pitch).
- **O6 — class vocabulary at freeze.** `volatile` (AI), `exact` (media/
  build), `validated` (renders). Freeze all three now, or only `volatile`
  for the launch and widen with the derivation round?
- **O7 — do launch receipt lines carry the axis labels?** Every launch
  edge is `class=volatile` (contains an llm call) and the launch never
  serves `recorded`, so the labels are inert at launch. Still, writing
  `class=volatile` on every line now (a) costs nothing, (b) makes the
  coarse tier forward-compatible with the call-granularity refinement,
  and (c) states the honest boundary in the artifact itself ("this edge
  is not reproducible"). Recommend: yes, carry `class=` from day one;
  defer `pure`/`effectful` and the freshness-intent selector to the
  derivation round (they are read-path concepts, not receipt-write
  concepts).
```
