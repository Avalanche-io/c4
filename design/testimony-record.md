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
