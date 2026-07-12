# Substrate map — what the agentic workspace actually requires

Status: synthesis (2026-07-10). Inputs: the application-layer analysis
(eight primitives: artifact, workspace, binding, action, realization,
zone, continuation, resolver), the media demo north star ("One Prompt.
Four Machines. One Project."), snapshot-loop draft v7 + brief v2, the
testimony-record and agent-undo-plugin drafts, the resolver lab red
team, the incumbent bake-off, and PATH-FORWARD-2026-07.

## The finding

The application layer's central architectural insight — *"replace
synchronization of mutable state with exchange of immutable values;
only small bindings need coordination"* — is the ecosystem's existing
push-intent, pull-content doctrine, independently re-derived from the
application side. That convergence is why the genuinely new substrate
is small: **three items**, everything else either ships today, is the
committed roadmap under another name, or is application layer that
must stay out of the core.

## The eight primitives, mapped

| Primitive | Verdict | Where it lives |
|--------------|--------------------------|----------------------------------------------------|
| Artifact | **exists** | C4 ID + store (v1.0.15); reads re-verify |
| Workspace | **exists** | a c4m file — partial knowledge first-class: a root ID names a tree no single machine holds; browse the namespace by reading the c4m, no bytes moved; `ExtractSubtree` + `patch` = partial materialization |
| Binding | **NEW (substrate #1)** | the only mutable object in the system |
| Action | **thin convention** | action key = C4 ID of a canonical action record; record semantics borrowed from OpenJD/REAPI, never a bespoke standard (red-team directive); core needs nothing — identifying and storing a record already works |
| Realization | **NEW (substrate #3)** | = the testimony record's `produced-from` predicate; one format serves media compliance AND agent lineage |
| Zone | **not a daemon** | a host with the c4 CLI + a store, reachable over SSH; a zone descriptor is just a small record (an artifact); c4d stays dead |
| Continuation | **application layer** | any serialized agent state is an artifact and gets an ID for free; agent frameworks own its schema; a task-protocol spec, if ever, is a separate repo |
| Resolver | **exists + substrate #2**| local: store + cat + patch; multi-source: `MultiStore` already reads from ordered sources (`C4_STORE` takes a list, `s3://` included); missing transport: a peer read-source |

## The genuinely new substrate

### 1. Bindings — mutable name → immutable ID, compare-and-swap

`project/main -> c4xx…` The variables of the system; C4 IDs are the
values. Local-first: a file per binding under the store root, updated
by atomic rename with CAS semantics ("move `main` from A to B only if
it still points to A"). This is deliberately the *only* mutable,
coordinated object anywhere — and locally, POSIX rename IS the
coordinator. Cross-zone binding replication is explicitly deferred:
it is the single place consensus could ever enter the system, and
nothing near-term needs it. Surface cost: one small verb (post
snapshot-loop freeze; separate budget conversation).

### 2. Peer read-sources — fetch-by-ID with no daemon

The cut-to-the-chase transport: an SSH source for the store, so
`C4_STORE=/local/store,ssh://nas,s3://bucket` resolves reads through
the existing MultiStore order. The wire protocol *is composition*:
`ssh zone c4 cat <id>` — auth is SSH's, transfer is a pipe, and the
store invariant does the verification (every fetched byte re-hashes on
arrival; a corrupt or lying peer is caught by identity, not trust).
No daemon, no new protocol, no c4d. Lazy subtree materialization
falls out: extract the subtree from the root c4m, patch it, and the
resolver pulls only those IDs.

### 3. Realization records — one predicate, two markets

Extend `design/testimony-record.md` with the realization form of
`produced-from`: authored testimony that action-key K, executed by
executor E (zone, timing, evidence label), produced output IDs O…,
with a **determinism class** — `exact` (same ID expected; falsifiable
by recomputation), `validated` (semantic acceptance test named),
`volatile` (must run; record only). The media declarations record and
the agent lineage record are the same primitive discovered twice —
which is the ecosystem's pattern working as intended. This is a
design edit, not a new format.

## Already the committed roadmap (the application needs it; no new work)

- **Snapshot-loop v7 → build** (restore verb, print barrier, journal,
  machine-output contract, re-scan trust posture): this IS the
  workspace-root lifecycle — consume immutable root, work in a private
  view, produce new root; scripted single-ID output is what agents
  parse.
- **`-m c` content projection + four-implementation conformance**:
  zone-independent workspace identity (the mtime-fragility evidence
  made this concrete).
- **c4-mcp (F2)**: the agent-facing control surface — inspect
  workspace / resolve artifact / snapshot / restore / diff tools live
  here, not in new core verbs.

## Explicitly not substrate — do not build

A daemon (c4d), a scheduler or economic optimizer (fetch-vs-recompute
policy is porcelain over measured costs; the resolver lab's decision
record is the template), a workflow language, consensus for
distributed bindings, a virtual filesystem mount (compatibility
adapter, later, never the center), a continuation schema, a container
format, a P2P network. Both application essays and PATH-FORWARD
independently list these as traps; the ledger here records them so
they stay rejected.

## Sequence — rides the existing plan, nothing displaced

1. Build the frozen snapshot loop (restore + print barrier + posture)
   — PATH-FORWARD Phase 4, the undo plugin's gate.
2. c4-mcp hardened + public (Phase 6).
3. **Bindings + SSH read-source** — small additions after the freeze;
   pulled by the two-node resolver experiment AND useful to the plugin
   (fork/undo across machines = bake-off S8's portability moat).
4. Realization predicate lands as an edit to the testimony-record
   design now; conformance work (Phase 8) pins the ID grammar both
   records cite.
5. Two-node resolver experiment (Demo C, its kill-gates unchanged):
   proves fetch-vs-recompute honestly on the 301 MB artifact with
   measured links.
6. The four-machine media demo — only when Demo C passes and a design
   partner or explicit strategy change pulls it (Portfolio D
   condition). The demo essays are the north star, not the plan.

## The one-line test for any future addition

If a proposed primitive is not (a) an immutable value with an ID,
(b) the one mutable binding kind, or (c) testimony about IDs — it is
application layer, and it belongs above the substrate, in a separate
repo, consuming these records.

## Amendment (2026-07-10, accepted)

The retention record (`design/store-records-architecture.md`) is the
sanctioned store-local instance of class (c) — testimony about IDs —
with an operational fold. It is not a new primitive class; the
one-line test stands.
