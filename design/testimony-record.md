# Media element declarations — the testimony record

Status: requirements draft (2026-07-10). Design before code; nothing
here is frozen. This is PATH-FORWARD Phase 8's "conform-record /
attestation format," renamed to what it actually is: a record of
**testimony** — declared facts bound to exact content IDs — kept
strictly apart from **evidence**, which is never recorded because it is
recomputed from bytes.

## One sentence

A plain-text sidecar in a delivery package that binds declarations
(origin, license, AI-use, transform lineage) to exact C4 IDs, so that
everything checkable is checked from bytes and everything merely
*stated* is visibly a statement with an author.

## Founding principle (Joshua, 2026-07-25)

> "Everything in c4 is content, but testimonial content is a known
> parseable type."

This is the unifying statement, and it retires a category error that
had crept into these docs — the habit of speaking about "the testimony
layer" as though it were a second storage mechanism beside content.
**There is no second mechanism.** A testimony record is an ordinary
object: raw bytes, a content ID, stored and replicated like any other.
What distinguishes it is only that its bytes are *recognizable* — it
declares its own type in its first token.

### The rule this implies

**Type is something you recognize, never something you apply.**
Recognition may add capability (a verb can explain it, resolve it,
expand it); recognition may never change what is stored or what an ID
names. That single sentence is the correct form of the rule that
2026-03-25 got wrong: the mistake was never *recognizing* c4m, it was
*transforming* on recognition (`c4m-canonical-storage.md`,
[#43](https://github.com/Avalanche-io/c4/issues/43)).

It also reconciles with the round-1 pin — *"the store has no object
types; what an ID names is knowledge the caller brings."* Both are
true, at different altitudes: **the store is typeless; the caller and
the verbs recognize types.** Recognition lives at the verb, never in
the store.

### What falls out for free

Because testimony is content rather than a parallel layer, every
ordinary verb already works on it, with no new machinery:

- a record can be snapshotted, restored, diffed, patched, replicated,
  and journaled like anything else;
- a record has a content ID, so **testimony about testimony is free** —
  a signature over a record's ID, an anchor over a statement's ID, and
  the `resolves-to` and `anchored` predicates all compose without a
  special case;
- a record can sit inside a scanned tree as an ordinary file and still
  be recognized when someone asks about it — which is exactly the
  ergonomic case that motivated the c4m ruling.

### Edge cases this principle has to survive

1. **The journal is the one mutable artifact.** The store is
   hash-named immutable objects **plus one append-only journal file**.
   The journal is therefore not content in the same sense — it is the
   log of claims *about* content, and it is the only thing in the
   system that changes in place. The precise form of the principle is:
   *everything in c4 is content, except the append-only log of claims;
   and every prefix of that log is content* — which is exactly why A3's
   tamper-evidence works by signing a journal **prefix**, and why an
   `anchored` line takes that prefix statement's ID as its subject.
2. **Recognition must never be load-bearing for correctness.** If a
   known type is treated as plain bytes, nothing may break. This is
   what makes false-positive detection harmless and keeps the store
   typeless.
3. **A known type must be evident from the content itself.** The
   `@c4 <kind> <version>` first token is what makes a record
   self-identifying and c4m-parser-rejecting. A type that can only be
   known from a filename or a sidecar is not a known type.
4. **"Known parseable type" must not become a schema registry.** The
   philosophy forbids registries, and the count is already drifting
   (c4m, journal, and the designed receipt/process/testimony kinds).
   The bound this principle suggests is the one the surface review
   arrived at independently: **two kinds — the positional grammar that
   must be versioned (the journal) and the line-local one that must
   not (records)** — see `v11-surface-review.md`.

### What it does NOT change

Testimony remains **non-authoritative by construction**: the journal is
the sole authority on claims, retention on verdicts, and testimony only
ever adds statements about IDs. Being content makes a record
*addressable and verifiable*; it does not make it authoritative. Those
are independent properties, and conflating them would undo R2.

## Positioning — beside, not instead

| Plane                           | Owner                                                                           | This design's relationship                                                                       |
| ------------------------------- | ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| Byte identity & containment     | C4 IDs + c4m files                                                              | uses as-is; adds nothing to the format                                                           |
| Checksums in delivery specs     | ASC MHL (already supports C4 digests; Netflix mandates MHL ingest verification) | sits beside; never replaces                                                                      |
| Content credentials, signatures | C2PA 2.2, in-toto/SLSA                                                          | the record is *what gets signed*; signing is never implemented here                              |
| Editorial structure             | OpenTimelineIO                                                                  | out of scope entirely                                                                            |
| Execution / job description     | OpenJD, REAPI semantics                                                         | out of scope entirely (the resolver lab's action records must not become a standard by accident) |

The only genuinely new artifact is the testimony record itself, and it
is deliberately small.

## What already works (no design needed)

On shipped v1.0.15, from bytes alone, recomputable by anyone:

- **Containment**: an element's file C4 ID recurs verbatim inside any
  package's c4m. `c4 intersect id <a> <b>` answers "same content,
  regardless of path" today; `c4 intersect path` answers "same
  location." (File-level claims are honest today; tree-level claims
  wait on the `-m c` conformance work.)
- **Same-name/different-bytes detection**: path-intersect minus
  id-intersect.
- **Duplicates**: repeated IDs within one c4m.
- **Unaccounted remainder** (candidate recipe, to verify): C4 IDs are
  plain text, so set difference composes with coreutils —
  `comm -23 <(package IDs, sorted) <(declared-source IDs, sorted)` —
  a recipe, not a verb, unless real use shows it deserves porcelain.

## Requirements

**R1 — Plain text, line-local, appendable.** Human-readable, diffable,
mergeable line-by-line. A vendor appends declarations without parsing
anything. No binary, no schema registry, no versioned envelope.

**R2 — Testimony only; evidence is never recorded.** The record never
stores a claim that bytes could prove (sizes, containment, equality) —
those are recomputed on demand. Every line is a *statement by an
author* about an ID. This is the structural encoding of the honest
boundary: evidence between transforms, recorded testimony at them.

**R3 — Transform boundaries are first-class testimony.** Where byte
lineage stops (transcode, grade, render), the record carries a declared
event: named inputs (IDs) → named outputs (IDs), transform description,
declaring actor. Lineage across a transform is never inferred, only
declared — and the declaration is falsifiable at both ends because its
endpoints are recomputable IDs.

**R4 — Three-valued recall.** "Does any recorded package contain
element ID X?" must distinguish *present* / *not present* / *no record
covers this package*. Partial knowledge is a valid state; silence is
never treated as absence.

**R5 — Content-projection IDs by default for tree references.**
Evidence from the resolver lab (2026-07-10): a provenance chain keyed
to full-fidelity manifest IDs is mtime-fragile — re-extracting an
identical archive changes every downstream ID. Tree-level references
use the full-null content projection once conformance ships; file IDs
are projection-free; any full-fidelity reference is labeled (like-mode
comparison discipline).

**R6 — The signature slot is reserved, never implemented.** A record
is signed by signing *its* C4 ID with existing tools (C2PA manifest,
in-toto attestation, plain detached signature). The design defines what
the signable artifact is; it does not do cryptography.

**R7 — Mandate-shaped.** The whole design must keep the delivery-spec
ask at one line: *"deliverables accompanied by a c4m file; AI-origin
elements enumerated by C4 ID."* Complying must cost a vendor one
command of a free tool plus appending declaration lines.

**R8 — ID syntax defers to the conformance spec.** The record's grammar
references the machine-readable C4 ID grammar (to be pinned in the
`-m c` conformance work), not its own regex. (The resolver lab carried
two disagreeing ID patterns in one small project; this class of bug
dies in the conformance spec, not here.)

## Candidate shapes (open — pick in design iteration, not here)

**A. Predicate lines (leading candidate).** One declaration per line:

    <c4id> <predicate> <author> <value...>

    c4xx... origin        vendor:fx-house-a  ai-generated model=...
    c4xx... license       vendor:fx-house-a  CC-BY-3.0
    c4yy... produced-from vendor:lab-b       inputs=c4aa...,c4bb... transform="prores422hq conform"

  Pros: line-local, appendable, greppable, trivially diffable; the
  file's own C4 ID is the signable subject. Cons: a new (tiny) grammar
  to freeze; predicate vocabulary needs a budget and a home.

**B. Ordinary c4m + conventions.** Reuse c4m entries whose *names* are
  structured (e.g. `declarations/<c4id>/origin`). Pros: zero new
  formats. Cons: abuses the name field as a schema; grammar is frozen
  so predicates can't carry values cleanly; violates "fewest concepts,
  fully load-bearing" in spirit by overloading one.

**C. C2PA-native.** Express declarations only as C2PA assertions
  referencing C4 IDs. Pros: no new artifact at all. Cons: not plain
  text, requires C2PA tooling on both ends, and the mandate ask stops
  being one line + one free tool. Fails R1/R7 as the *primary* form —
  but the design should define the lossless mapping so a C2PA
  ingredient assertion can be generated *from* the record.

## Demo B traceability (DEMO_PORTFOLIO_V2.md §B)

| Storyboard beat                              | Covered by                              |
| -------------------------------------------- | --------------------------------------- |
| Inventory a package without copying it       | c4m file (exists)                       |
| Intersect v1/v2 by content, not pathname     | `c4 intersect id` (exists)              |
| Same-name/different-bytes replacement        | path-intersect vs id-intersect (recipe) |
| Duplicates, missing, unaccounted remainder   | recipes over ID sets (R-remainder)      |
| AI-origin declaration bound to exact IDs     | testimony record (this design)          |
| "Byte evidence stops here; testimony begins" | R3 boundary events                      |
| C2PA credential beside C4                    | R6 + shape-C mapping                    |
| One-line delivery-spec pilot ask             | R7                                      |

## Non-goals

Frame-level AI detection (banned permanently); inferring lineage across
transforms; implementing signatures or trust; a registry service (the
Obelus paid tier — exists only on a signed mandate); network transport;
c4d; any implementation before this document stabilizes.

## Honest gates

- File-level containment/declaration claims: honest on shipped v1.0.15
  today.
- Tree-level references: gated on the `-m c` convention freeze + four
  implementations conformance-green (PATH-FORWARD Phase 8).
- Any cross-organization claim: gated on a mandating customer.

## Open questions (for Joshua / next iteration)

1. Shape A vs B vs C-as-mapping — recommend A with a hard predicate
   budget (≤ 6 predicates at freeze) and shape-C export defined.
2. Does "unaccounted remainder" stay a documented recipe (`comm` over
   sorted ID lists) or earn porcelain after real Demo B use?
3. Predicate vocabulary home: this repo's design dir, or the (future)
   conformance spec beside the ID grammar?
4. Record filename convention inside a delivery package (beside the
   MHL): e.g. `<package>.declarations` — bikeshed deliberately deferred.

## Update (2026-07-10, accepted)

Shape A's line grammar is adopted for the store's retention record
(`<id> <verdict> <actor> <time>` — see store-records-architecture.md),
so the two records converge by construction. The interchange record's
own freeze remains open on this doc's track, with shape A leading and
now field-tested by the retention record.

## Assumptions deposited by the snapshot-loop rounds (2026-07-13, draft-v9)

Round 3 (`snapshot-loop/round-3/draft-v9.md`) deferred four concerns
to "the testimony layer" while stripping the journal to two fields
(scan-start, claim ID). Those deferrals are ASSUMPTIONS about this
design's future scope, recorded here so the testimony round inherits
them explicitly instead of as conversation residue. None are
commitments; each is a candidate requirement to accept or reject.

**A1 — Claim provenance.** The journal dropped origin (`host:abs-path`)
and the name label because roots are purely virtual and the journal
travels with the location-independent store. Assumption: testimony is
where "claim `<id>` was captured from `<host>:<path>` as `<name>`"
lives, as a statement with an author — satisfying the reflog-usability
and future workflows (re-snapshot same origin; restore to origin; c4d
suggesting standing flows from observed capture patterns, which
DEPENDS on provenance existing somewhere). Implied requirements:
per-claim binding to journal claims (ID + scan-start suffice as the
key); separable from the store or explicitly optional when stores
travel (origins leak usernames and directory layouts — the privacy
argument that evicted them from the journal applies to any record that
travels by default).

**A2 — Diagnostics.** Scan end / duration / counters were ruled
narration (stderr), with testimony as the durable home IF ever needed.
Assumption: per-claim metrics are testimony-shaped (statements about a
claim), never journal-shaped.

**A3 — Journal tamper-evidence.** CT-style hash chaining was noted and
NOT adopted (trust boundary = the store directory; foreign edits are
corruption). Assumption: if tamper-evidence is ever wanted, it arrives
as testimony/integrity work — e.g. a signed statement over a journal
prefix — never as journal grammar.

**A4 — Human labels generally.** Wherever v9 removed a human-facing
field from a root record, the standing answer was "labels are
testimony." Assumption: a lightweight local label mechanism (possibly
never shipped) is testimony's concern; root records stay minimal.

**Boundary conditions the deferrals assumed (binding on this design):**

- Testimony is NEVER release-gating: v1.1.0 and the whole snapshot
  loop are complete without it; losing every testimony record loses
  nothing recoverable (testimony never pins bytes, mirroring the
  links-never-pin rule).
- Testimony is non-authoritative by construction (R2 already says
  this): the journal remains the sole authority on claims; retention
  remains the sole authority on verdicts; testimony only ever adds
  statements about IDs.
- Shape A's line grammar remains the leading candidate (per the
  2026-07-10 update); A1-A4 should be expressible as predicates within
  the existing predicate budget or explicitly rejected, not grow a
  second grammar.

## Amendments: the anchoring litigation + the creation reframe (2026-07-16)

The anchoring litigation (pro/con xhigh briefs → judge; full ruling in
the GTM record) and the same-day creation reframe (derivation-records.md
"Creation is a first-class edge") amend this design as follows. None of
it is journal grammar; all of it is testimony — consistent with A3's
boundary. None of it is release-gating, exactly as the boundary
conditions above require.

**A3 extended — local vs external tamper-evidence.** LOCAL
tamper-evidence arrives, as A3 already says, as a signed statement over
a journal prefix — a statement whose text covers the journal head hash
and the session's receipt ID. A same-machine key is forgeable by an
adversary who holds the shell (receipt-record.md threat model, tier 3c),
so these are always called "signed statements" — testimony — and are
never marketed as anchoring. EXTERNAL tamper-evidence arrives as an
`anchored` testimony line over that statement's C4 ID — never as
journal grammar, never default-on.

**Reserved predicate — `anchored`.** One predicate of the budget:

    <subject-id> anchored <witness> proof=<c4id-of-proof-blob> at=<rfc3339>

Subject is normally the C4 ID of a signed journal-prefix statement (or
a receipt ID directly); witness is a short registered token (`ots`,
`rfc3161:<host>`, `rekor:<log>`, `obelus`); the proof blob (a `.ots`
file, a DER token, an inclusion proof) is an ordinary content-addressed
store object riding normal replication. Losing the proof loses the
seal, never bytes — testimony never pins, and the original time T is
not re-obtainable (re-anchoring seals only from now). Verification
recomputes the subject ID from bytes, then verifies the proof against
the witness — offline-capable, reporting pending vs final. The anchor
is itself verified, never asserted.

**Reserved predicate — `authored-by`.** One predicate of the budget:

    <subject-id> authored-by <author> [at=<rfc3339>]

Marks a creation edge: a `volatile` derivation whose irreproducibility
is the signature of authorship, not a flake (full case in
derivation-records.md). Attribution is testimony by construction — a
statement with an author about an ID — and the class axis stays pure
verification. `authored-by` + `anchored` on one subject is the
creator's certificate: this work, this author, first known to exist at
time T.

**Naming discipline.** The word is "anchor", never "pin" —
links-never-pin-bytes is about byte retention, a different concept, and
the collision would be corrosive.

**Absence semantics (R4 applied).** Anchor recall is three-valued:
anchored / not anchored / no record covers this. An unanchored
statement remains valid testimony — partial knowledge is a valid state.
An anchoring GAP is a signal only where a policy declares the cadence.

**Privacy boundary.** Every external witness learns the existence and
timing of activity; per-session anchoring publishes development cadence.
Anchoring is opt-in in any dev profile; a paid registry, if one ever
exists (Obelus — gated on a signed media-wedge mandate), is
private-log-with-published-roots, so it is verified, never trusted.

**Predicate budget accounting (freeze planning).** Candidates now
enumerated against the ≤6 budget: `origin`, `license`, `produced-from`,
`resolves-to`, `anchored`, `authored-by` — the budget is exactly full.
Any seventh predicate must evict one or raise the budget explicitly at
the freeze; this is now a freeze decision, not a drawer item.
