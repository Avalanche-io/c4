# Round 3 case file: the streaming/projection amendment ("v9 proposal")

Status: BEFORE THE COURT. Nothing here is decided. This file is the
complete case record for a crucible ruling on amendments to the v8
surface (implemented on master 2026-07-13, unreleased).

## Procedural posture

v8 (draft-v8.md, round-2) is fully implemented, tested, kill -9
verified, merged to master, and UNRELEASED. The versioning policy
(workspace RELEASE.md) froze the major at 1 forever; minors are the
breaking tier; the CLI machine contract freezes at v1.1.0. This is
therefore the last cheap moment to change the surface. Joshua
challenged two v8 surface decisions on ergonomic grounds and proposed
a deeper reframe; the assistant synthesized it into the proposal
below. The court decides what ships as v1.1.0.

## What v8-as-built does (the status quo on master)

1. `c4 id dir` prints the manifest at CONTENT level by default (mode
   and timestamp null at every level — the D1 "full-null" projection,
   generated as nulls by the scanner, `scan.ModeContent`). `-m f`
   opts into full fidelity.
2. `-s` implies `-q`: `c4 id -s dir` prints ONE bare line — THE
   snapshot ID — after content is durable and the journal appended
   (the print barrier). The listing is not printed; get it back with
   `c4 cat -r "$SNAP"` (verified read-back).
3. `-s` conflicts with `-m` (snapshots are always full detail).
4. Output is buffered: nothing prints until the scan completes.
5. Nulls in c4m records arise BOTH from genuine ignorance (stdin,
   unreadable entries, structure/metadata scans) AND from deliberate
   projection (content mode writes null mode/timestamp into the
   emitted records).

The original v7/v8 rationale (round-1/draft-v7.md, surface-v7.md):
- Founding complaint 3: a consumer scraped an ID out of listing text
  and shipped a WRONG project ID. The root ID of a scan appears
  NOWHERE in its own listing (a directory's ID is the hash OF its
  listing; the listing holds only children's IDs), so any scrape of
  listing output grabs a child's ID. v8's cure: `-s`/`-q` print
  exactly one bare ID line; stdout of `-s` is the ID, nothing else.
- D1 decision: content projection = full-null, and content became
  the default level ("the default level for c4 id; bare ID with -q").
  Rationale: THE identity should be machine/clock/umask-independent
  by default.

## Joshua's challenge (verbatim arguments, 2026-07-13)

> "There was a reason that c4 id -s dir outputs c4m — it's pipeable,
> and using it is much like ls -l. I agree we need a consistency
> guarantee but maybe not at the expense of existing ergonomics."

> "Perhaps we should have the -s story return the id on stderr. The
> nil metadata fields can simply be on the eval side not on the
> generation side. It doesn't have to be built in to the record as
> nils. Also we don't want to wait for all scanning to be done before
> we see any output — that blocks a large class of command-line
> utilities (finding an id on a very large tree, incremental saving,
> grep interaction and other patterns). We do want a full c4m id
> gated to reliable capture under -s without the need of a pipe and
> tee to a second c4 — but again we don't want to lose the
> ergonomics."

> "A very important part of the system is the progressive refinement
> that allows incrementally complete capture. Partial data is data
> per the core design philosophy. Forcing full computation before
> progress violates that philosophy when that computation is long."

## The proposal on trial (four counts)

### Count 1 — Eval-side projection; nulls mean ignorance only

The scanner always captures full knowledge when it hashes. A null in
a RECORD means "genuinely unknown at scan time" (stdin, unreadable
entry, structure/metadata scan) — never deliberate projection.
`-m c` becomes a VIEW: at output/identity time, project the full data
(null mode+timestamp in what is printed/hashed), compute content-form
directory IDs from the projected listings (cheap — no byte rehash).
Consequences:
- Default listing level = FULL (ls-l ergonomics; saved descriptions
  carry maximum knowledge; `-m` projects downward on demand).
- `scan.ModeContent` as a generation mode becomes unnecessary for the
  CLI (the library may keep it); the projection lives at evaluation.
- The `-s`/`-m` conflict dissolves for `c` (storage always full; the
  printed VIEW may project) — though whether `-s -m c` should print a
  content view while claiming a full snapshot ID is a sub-question.
- Coherence win: `c4 id -q dir` (full root ID) equals the `-s`
  snapshot ID on an unchanged tree — one ID at the front door.
- Cost: cross-machine content identity requires `-m c` explicitly;
  the machine-contract line becomes `CID=$(c4 id -q -m c dir/)`.

### Count 2 — Streaming chain output as the default shape

Flat canonical c4m text cannot stream: a directory's line precedes
its children, but its ID (and size) are bottom-up aggregates known
only after the subtree completes. The frozen format already contains
the answer: a PATCH CHAIN.
- Stream every entry in canonical DFS order as it is scanned.
  Directory lines emit with null ID and null size — genuine ignorance
  at that moment (consistent with Count 1's null semantics).
- On walk completion, emit one refinement patch section filling in
  directory IDs/sizes, then the bare root-ID boundary line.
- The output is a valid c4m chain resolving to the complete manifest.
  `grep` sees file lines as they hash; a killed scan leaves a valid
  partial description (partial data is data); the LAST LINE of the
  stream is always the root ID.
Sub-questions for the court:
- Chain shape: one trailing patch vs per-directory patches; what -e
  (ergonomic/aligned) does under streaming (alignment needs global
  column widths — buffer? fixed widths? -e implies buffered?).
- Whether `c4 id file.c4m` normalization emits flat text (presumably
  yes — chains resolve).
- Ordered emission under the parallel walk: canonical order out,
  parallel hashing underneath, emission blocking on next-in-order;
  bounded lookahead so a slow subtree cannot balloon memory.
  (scan.WithEntryStream exists but emits discovery order under
  parallelism; sequential mode is canonical-ordered.)
- Whether streaming is unconditional or gated (a flag? isatty? — NB
  isatty-dependent DATA violates the byte-purity pin; surface-v7
  allows isatty only for stderr narration).
- Consumer compatibility: every existing tool that reads c4m files
  (c4 verbs, c4sh, c4py, c4ts, c4-swift, libc4, vscode-c4m) must
  handle chain-shaped files `c4 id dir > f.c4m` now produces.

### Count 3 — `-s` streams the listing; the claim is the final line

`-s` prints the same stream as bare `c4 id` (ergonomics preserved:
one command scans once, stores, and pipes the listing), with the
print barrier attached to the FINAL bare root-ID line: it prints only
after content is durable and the journal appended. `stored: <ID>`
remains on stderr as narration. `-q` remains the byte-pure scripted
form (one line, nothing else). This REVERSES v8's "-s implies -q".
- The founding wrong-ID trap dies the other way: the root ID is now
  the guaranteed final stdout line, so the obvious scrape (`tail -1`)
  is CORRECT — the trap existed because the root ID appeared nowhere.
- Claim semantics must be pinned: streamed lines above the final ID
  are description, not claims; only the final bare ID line (and -q
  lines) carry "printed ⇒ durable ⇒ recoverable". A kill mid-stream
  leaves description text and no claim.
- Sub-question: does a partial stream mislead? (Entries printed, then
  crash before barrier: child IDs visible on stdout that may not be
  durable in the store. Under v8, nothing printed until claim. The
  defense: without -s, printed IDs were never claims anyway; with -s,
  the doc pins the final-line rule.)
- Sub-question: multi-path `-s a b c` — per-path claims interleave
  with streamed listings; the one-line-per-path contract only holds
  under -q. Is the exit-pairing rule still coherent?

### Count 4 — Subsidiary surface questions

- `-c` carries TRUST (reuse guide, racy rule) — should it also carry
  SCOPE (the retired v1.0.x filter workflow: structure scan → edit →
  hash only survivors)? Overloading silently breaks stale-guide
  re-scans (new files would vanish instead of hashing). Options:
  trust-only (v8 status quo); explicit second flag later; guide
  entries with null IDs imply filter (implicit magic — suspect).
- Contract migration: what the machine-contract SCRIPTING block
  guarantees post-ruling; `tail -1` sanctioned or merely tolerated;
  `SNAP=$(c4 id -s -q dir/)` as the blessed capture form.
- The v1.1.0 changelog and help/man text must be re-swept if any
  count is adopted.

## Constraints binding the court

- The c4m grammar is FROZEN. Chains, null fields, bare-ID boundary
  lines are existing grammar; no new syntax may be invented.
- stdout is data, byte-pure; stderr is narration; no isatty-dependent
  data. Determinism: byte-identical output at any concurrency.
- Versioning policy: major frozen at 1; this ships in v1.1.0; the
  machine contract freezes at release. Decisions here are PERMANENT
  in a way later rounds are not.
- Zero dependencies; design-before-code; fewest concepts fully
  load-bearing. New power should be a consequence of existing design.
- Safe by default. Verification over assertion. Partial knowledge is
  a valid state, not an error.

## What a ruling must produce

Per count: ADOPT / ADOPT WITH MODIFICATIONS / REJECT, with reasoning
that engages the strongest opposing argument, plus pinned decisions
for every sub-question the adopted counts raise. The ruling should
also state the exact machine-contract text consequences (the
SCRIPTING block lines that change) and name any invariant from
round-1/round-2 that the adopted counts weaken, with the replacement
invariant stated in one sentence.

## Reading list (all paths relative to oss/c4/)

- design/snapshot-loop/brief.md — founding complaints, budgets, pins
- design/snapshot-loop/round-1/draft-v7.md + surface-v7.md — the
  surface of record and its rationale
- design/snapshot-loop/round-2/draft-v8.md — amendments 1–4 + status
- cmd/c4/id.go, cmd/c4/help.go — v8-as-built
- scan/generator.go — WithEntryStream, parallel walk, ModeContent
- c4m/chain.go, c4m/encoder.go, c4m/decoder.go — chain mechanics
- ../c4sh, ../c4py, ../c4ts, ../c4m-swift, ../libc4, ../vscode-c4m —
  sibling consumers of c4m files (workspace-relative)
