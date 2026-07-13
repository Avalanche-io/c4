# Draft v9 — the streaming/projection amendment (FROZEN 2026-07-13)

Status: RATIFIED. The court's ruling (`ruling-v9.md`, all four counts
adopted with modifications) plus Joshua's post-ruling refinements,
ratified in conversation 2026-07-13. This is the requirements document
for the round-3 implementation branch. v8 (merged to master,
unreleased) is the base; everything below amends it and ships in
v1.1.0.

## 1. Eval-side projection; full is the default (Count 1)

- The scanner captures full fidelity whenever it hashes. `-m c` is an
  output/identity-time PROJECTION: mode and timestamp nulled in what
  is printed and hashed; content directory IDs recomputed from
  projected listing text (no byte rehash). `-m s`/`-m m` remain
  genuinely cheaper scans whose nulls are real ignorance.
- A null in any saved record means "genuinely unknown at scan time".
  The 2026-07-09 permanent full-null projection decision is untouched;
  only its default-placement is reversed.
- Default listing level and default `-q` identity are FULL. The
  content ID is `c4 id -q -m c dir/` (two-flag contract, precedented).
- `-s -m` (including `-m c`) stays refused; the error teaches
  `c4 cat -r "$SNAP" | c4 id -q -m c -` (the stdin path must support
  `-m` downward projection).
- `scan.ModeContent` becomes library-only; the CLI stops generating
  with it.

## 2. Streaming chain output (Count 2)

Flat canonical text cannot stream (a directory's line precedes its
children; its ID/size are bottom-up). The default output becomes a
patch chain, streamed:

1. Base section: entries in canonical DFS pre-order; directory lines
   carry null ID and null size.
2. ONE bare boundary line = the base section's own canonical-text ID
   (equals the accumulated state at that position; never the root).
3. ONE refinement patch restating exactly the directory lines whose
   ID or size changed from null (never restate an unchanged line —
   restated-identical means removal; unreadable directories stay null
   forever; empty directories emit complete in the base section).
4. The resolved root ID on a bare final line — the closing validator,
   consumer-verified (MUST verify on resolve, ErrPatchIDMismatch).

Rules: streaming is unconditional (no flag, no isatty); the contract
is BYTES at any concurrency (a fully buffered implementation
conforms); ordered emission = per-directory canonical sort at
readdir, pre-order dir lines, parallel hashing with emission gated on
next-in-canonical-order under a bounded lookahead that backpressures
workers; each line written in a single write(2); consumers discard an
unterminated final line. `-e` implies buffered (resolved flat aligned
entries + unpadded final root-ID line). `c4 id file.c4m` and
`c4 id -` emit resolved FLAT canonical text closed by the root-ID
validator line. A single regular-file argument emits one entry line
and NO trailing line (the final-line guarantee is scoped to
listing-emitting invocations: directory, .c4m, stdin). Empty
directory: exactly one line, the empty-description constant.

## 3. `-s` streams; the claim is the final line (Count 3)

- `-s` prints the same stream as bare id, storing underneath; the
  final bare root-ID line prints only after the store barrier and the
  journal append. `stored: <ID>` remains stderr narration.
- Claims are exactly: every `-q` line, and the final bare root-ID
  line of an `-s` stream that exited 0 or 2. Streamed entry lines are
  description, never claims; a killed stream claims nothing.
  Replacement invariant: "a printed CLAIM never dangles."
- `-s -q` keeps v8's one-line contract verbatim — the blessed capture
  `SNAP=$(c4 id -s -q dir/)`.
- Exit 2 is claim-bearing (partial scan journals and prints its true
  claim). The unreadable-directory hard-abort becomes
  record-nulls-and-continue exit 2.
- Multi-path without `-q` is refused with a teaching error; `-q`
  multi-path unchanged (one line per succeeded path, pairing valid
  only on exit 0).
- SIGPIPE/EPIPE: under `-s`, stdout death never cancels the snapshot
  — ingest, barrier, and journal complete; exit 1; `c4 log` holds the
  claim. Read-only id exits 1.
- `tail -1` is documented, exit-conditioned contract in id(1) (last
  stdout line of a listing-emitting invocation exiting 0 or 2 is the
  root ID), never a SCRIPTING recipe; `-q` remains THE scripted form.
- kill9-crucible gains streaming `-s` kill windows and a SIGPIPE mode
  and must pass before freeze.

## 4. Chain-grammar erratum (the four boundary rules)

One erratum to C4M-STANDARD.md + SPECIFICATION.md, landed in all five
implementations (Go strict Decode unified with DecodePatchChain,
c4py, c4ts, c4m-swift, libc4) with one shared cross-implementation
test vector. Release-gates v1.1.0.

1. FIRST LINE bare ID: external base reference (unchanged).
2. INTERIOR boundaries: accumulated-state checkpoints, officially
   (ratifies every artifact ever emitted — EncodePatch/diff always
   wrote resolved-state IDs; SPECIFICATION.md:323's block-link prose
   was write-side-unimplemented drift). A resolving decoder MUST
   verify (one comparison per boundary — it is already folding);
   non-resolving passes MAY skip.
3. TRAILING bare ID at EOF: legal; means the resolved manifest; MUST
   verify on resolve (promotes C4M-STANDARD §10.7's SHOULD-append to
   c4's emitted norm; deletes SPECIFICATION.md:347's
   ErrEmptyPatch-at-EOF, which contradicted §10.7). An empty section
   strictly BETWEEN boundaries remains ErrEmptyPatch, except
   consecutive boundaries = close-then-supersede per
   DecodePatchChain's shape semantics.
4. The JOURNAL is not c4m and drops out of chain grammar entirely
   (§5).

Consumer gates: libc4 full chain resolution (silent flattening is a
release blocker); CLI loader unification (id file args, -c guides,
merge, restore targets, explain, intersect, paths — one
chain-resolving validator-verifying loader; stdin/file asymmetry
closed); c4 cat's raw-echo fallback and c4 paths' first-line dispatch
fixed to resolve-or-error-loudly; c4sh's pinned c4 dependency bumped.

## 5. The journal is its own format (ratified refinements)

The v8 journal masqueraded as c4m (chain-shaped, .c4m extension) while
satisfying no chain semantics — its boundaries were claim IDs. It has
never shipped; migration cost is zero. Replacement:

- File: `<store>/journal` (renamed from `log.c4m` — it is not c4m and
  does not wear the extension).
- Line 1 magic: `@c4 journal 1` — every conforming c4m parser MUST
  reject @-lines, so misinterpretation is structurally impossible in
  both directions; the trailing integer versions the format.
- One line per claim, two fields, both load-bearing:
  `<scan-start RFC3339 UTC seconds> <claim-ID>`
- No bare-ID boundaries (the 90-byte redundancy and the chain shape
  are gone). No name, no size, no origin, no flow operator, no
  scan-end: nothing programmatic consumed them. The racy rule needs
  scan-start + ID; ordering is line position; duration is stderr
  narration; provenance (host:path) is TESTIMONY-layer work — roots
  are purely virtual, and the journal travels with the
  location-independent store (prior art: WAL-minimal records travel;
  git's rich reflog is deliberately machine-local).
- Mechanics unchanged: OS lock that dies with its holder; torn tail =
  unterminated final line, truncated before the next append; appends
  fsynced before the claim prints; `c4 log` renders
  `index scan-start ID` (last field remains the ID; awk '{print $NF}'
  contract survives); repeat claims repeat freely;
  resolveGuideScanStart matches latest claim by ID as before.
- Windows drive-colon parsing rule: deleted (no paths in the journal).
- Retention/split: journal archiving is line-based (c4 split no
  longer applies to journals; the RETENTION doc text moves to a
  line-count idiom).

## 6. Machine-contract text (SCRIPTING block, post-amendment)

- `SNAP=$(c4 id -s -q dir/)` — THE snapshot ID: one line, nothing
  else, printed only after the claim is durable.
- `CID=$(c4 id -q -m c dir/)` — THE content ID: equal iff
  byte-identical, any machine/clock/umask (scanned alike).
- `c4 id -q dir/` — the tree's full ID; equals its -s snapshot ID
  while the tree is unchanged.
- Claim sentence: "With -s, entry lines stream as description, never
  claims; the claim is the final bare root-ID line, valid on exit 0
  or 2 — scripts use -q."
- Exit map: "1 error — bare -q ID lines already printed remain true
  claims; streamed description lines were never claims."
- Durability sentence: "A claim line — every -q line, and the final
  bare root-ID line of an -s stream that exits 0 or 2 — prints only
  after everything it names is on stable media… Lines above it are
  description, never claims."
- One path per listing stream: without -q, id refuses several paths.

## 7. Deletions and fixes riding this round

- `scan.WithGuide` (dormant path filter) deleted; `-c` carries trust
  only (null-ID-implies-filter rejected as implicit magic).
- Unreadable-directory scan behavior: record-nulls-and-continue,
  exit 2 (replaces hard abort).
- Docs/help/changelog re-sweep for every change above, including
  one-line migration notes (-s stdout shape; default level; journal
  file). vscode-c4m grammar refresh is cosmetic, not gating.

## 8. Open items (severable, not gating)

- `-s -m c` as later additive surface.
- An explicit filter flag if a named task revives scan-filter-continue.
- Journal tamper-evidence (hash chaining) as testimony-layer work.
- Windows durability constant measurement (carried from v7).
- gc/retention implementation (follows this round, per accepted
  scheduling).
