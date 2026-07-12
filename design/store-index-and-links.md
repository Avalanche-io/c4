# Store index, deliberate GC, and link records — idea notes

Status: idea notes (2026-07-10, Joshua). DECIDED — see
design/store-records-architecture.md for the evaluated recommendation.
Originally: needs testing and discussion
before any of it becomes a requirements doc. Recorded here so the ideas
survive; nothing below is decided.

## The ideas, as stated

1. **An index of every c4m file in a store.** The vault should know
   which of its objects are listings (descriptions), enumerable without
   walking and sniffing every object.

2. **Scan incoming files for C4 IDs at ingest.** As bytes are stored,
   scan them for embedded C4 ID strings, so the store knows which
   objects *reference* which IDs — making ID-based search ("what refers
   to X?") answerable from an index instead of a full scan. The index
   need not store full 90-char IDs: a prefix long enough to make
   collisions substantially unlikely suffices, because any hit can be
   validated by opening the referenced file and reading the full ID.
   (Index as hint, file as truth — verification over assertion.)

3. **A master c4m as the authoritative root record.** A master c4m
   points at the root of a scan tree of c4m files, so mark-and-sweep
   works from an *authoritative record* rather than caller-supplied
   roots. Deletion becomes deliberate user action, not inference:
   move items to a **trash** or **shred** top-level folder to take
   those actions by choice — as opposed to simply collecting whatever
   is unreachable.

4. **GC is required in the next major release.** Current state is
   unbounded, explosive growth with very poor mitigation tools even
   for a motivated user.

5. **Testable question:** can we scan for C4 IDs in files without
   significantly slowing a copy/ingest operation?

6. **Link records.** Very simple, more or less invisible from outside:
   an enumerated list of C4 IDs in a database, plus predicate
   structures of three IDs (possibly stored as the integer indexes of
   those IDs). Take any C4 ID, look it up, reconstruct networks
   differentiated by predicate. Payloads can include signed
   attestation documents in a standard format, metadata, or any
   application-specific references. **This becomes the web of AI.**
   Later, remote systems link into a network of these databases with
   shared Bloom filters (or better), allowing an arbitrarily large
   federated network of C4 link networks.

7. **The Avalanche GUI** is the user-level app that creates and
   navigates these as workflows users build to automate processes.

## Connections to work already on the table (analysis, not decision)

- **Master c4m ≈ a binding.** The substrate map's binding primitive
  (mutable name → immutable ID, CAS update) is exactly the mechanism a
  master record needs; "the authoritative root" is the binding
  `store/master` pointing at a c4m of c4m's. Trash and shred are then
  *branches of the master record* — deletion is an edit to an
  authoritative, journaled record, which fixes the exact asymmetry
  that got `c4 gc` withdrawn (explicit caller-supplied roots deleting
  anything the caller forgot to name).
- **The c4m index vs the v7 journal.** Draft v7's journal records what
  was stored, in order, as a patch chain. Idea 1 may be the journal
  seen as an index (or a derived index over it) rather than a second
  structure — needs discussion so we don't build two overlapping
  concepts.
- **Idea 2 generalizes what gc's mark phase did ad hoc** (scanning
  stored descriptions for ID tokens at collection time) into an
  ingest-time index. The withdrawn gc code is prior art for the
  scanner.
- **Truncated-ID indexing** matches the store's existing character
  sharding and git's short-hash precedent; the validate-on-hit rule
  keeps the full-ID guarantee intact.
- **Link records are the testimony record, indexed.** A predicate line
  `<id> <predicate> <author> <value>` is a triple; idea 6's database
  is the queryable index form, while the plain-text record remains the
  authoritative, signable, interchange form (plain text is the
  universal interface; the DB is derived, rebuildable, disposable).
  Notably: if the *predicate itself is a C4 ID* (of a document
  defining it), the vocabulary is content-addressed too — no registry,
  fully in the sidecar doctrine.
- **Bloom-filter federation** is the same shape as the media "recall"
  query (does any recorded package contain ID X?) scaled across
  stores — package accounting and the "web of AI" would ride one
  mechanism.

## What needs testing or discussion before design

- **T1 (benchmark):** streaming C4-ID scan during ingest — the ingest
  path already reads every byte to hash it; measure the marginal cost
  of a `c4`-prefix base58 state machine on the same stream (target:
  unmeasurable against SHA-512 cost). Also: false-positive rate on
  binary data, and prefix length vs collision probability.
- **D1 (discussion):** journal vs c4m-index vs link-DB — one structure
  or three? What is authoritative (plain text) vs derived (rebuildable
  index)?
- **D2 (discussion):** trash/shred semantics — what exactly does
  moving to trash mean for reachability, what does shred promise
  (and on what filesystems can it promise it), and how does the
  journal record both.
- **D3 (discussion):** does the link DB live in the store directory,
  beside it, or per-application? Federation and Bloom exchange are
  explicitly later.
- **Scope note:** GC-in-next-major is a roadmap commitment to carry
  into release planning; the withdrawn design (`design/store-gc.md`)
  plus the master-record idea are its inputs.

## Link DB scale and encoding notes (2026-07-10, follow-up discussion)

Joshua's observation: naive triples (3 full C4 IDs per edge, ~280 B as
text / 192 B raw) explode at web-of-AI scale; optimizations like
zone-local partial IDs indexing to full IDs, and one-to-many grouping
(1 subject + M predicates + N objects instead of 3 IDs per edge) are
available.

Both instincts are the standard art, under established names:

- **Interning / dictionary encoding** (the original "enumerated list of
  IDs, triples as integer indexes"): one table of full 64-byte digests,
  edges as ~3 varint integers → ~12–16 B/edge plus the node dictionary.
  This is RDF dictionary encoding (HDT); every serious triple store
  does it.
- **One-to-many grouping** = adjacency-list / CSR (compressed sparse
  row) with delta-encoded sorted object lists: ~2–6 B/edge per index
  direction. Predicates are nearly free — even with predicate-as-C4-ID,
  the *distinct* predicate count is tiny, so P interns to 1–2 bytes.
- **Permutations**: answering "what references X" needs the inverse
  (OPS) index too — plan on 2–3 directions, so ~6–18 B/edge total.
  Priors: HDT compresses web-scale RDF to ~5–8 % of N-Triples text;
  WebGraph gets < 1 B/edge on web graphs.
- **Worked example**: 1 B edges over 100 M nodes ≈ 6.4 GB dictionary +
  10–20 GB indexes ≈ **20–30 GB total vs ~280 GB naive** (~10–15×).
  A media facility at 10⁷–10⁸ edges fits in single-digit GB — SQLite
  on a laptop territory.
- **Provenance survives grouping**: per the adjudication every edge
  carries per-edge provenance (which record asserted it) — one more
  small integer into a record dictionary, ~2–4 B/edge.

Two rules that keep this coherent with the adjudicated architecture:

1. **Intern integers are zone-private and never travel.** The wire and
   the authoritative records always speak full IDs (plain text);
   integers are a per-database compression detail. Federation
   exchanges Bloom hints + full-ID records, never indexes.
2. **All of this freedom exists *because* the link DB is derived and
   disposable.** It carries no authority and rebuilds from plain-text
   records + store scan, so its on-disk format can be as aggressive as
   measurements demand (CSR, mmap, SQLite, whatever) at zero
   philosophy cost. The interchange layer never carries the
   optimization burden — that is the payoff of the
   authoritative/derived split.

Scale reality check: any ONE zone's DB is bounded by that zone's
content; global scale is reached by federation, not by one table. The
enormity concern mostly dissolves into per-zone bounded databases.
