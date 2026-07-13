# c4 gc — requirements (gc design round exit; judged merge)

Status: round-exit requirements, 2026-07-13. Semantics are the ACCEPTED
store-records architecture (`design/store-records-architecture.md`,
2026-07-10) and are not re-opened here. This document is the judged merge of
the round's two scoped designs (surface; grammar) with both prosecutions'
findings repaired; every structural repair is recorded with its forcing
defect. It pins exactly what the round was chartered to pin: (a) verb/flag
spellings for the retention porcelain and gc; (b) the closure-walk TOTAL
grammar; (c) the gc stdout/stderr/exit contract; (d) the aggregate
condemnation primitive. A newcomer implements from this document plus its
normative citations. Build prerequisite: the v8 build
(`design/snapshot-loop/round-2/draft-v8.md`); see §12.

Normative citations (this document deliberately restates none of them):
- C4 ID grammar: the `-m c` conformance ID grammar (operationally: exactly 90
  bytes, prefix `c4`, base58). No regex of our own (testimony-record R8).
- c4m text: `c4m/SPECIFICATION.md` — §Filename encoding (SafeName),
  §c4m field-boundary escaping, §Media File Sequences, §Patch Format,
  §Empty Patch Rejection.
- Journal grammar and durability: draft-v7 §3, §5, §6; v8 Amendments 1–3.
- Retention line shape: testimony-record shape A, adopted verbatim
  (Decided item 1).

Surface spend, total: 2 new verbs (`retain`, `gc`); 1 new long-form flag
(`--empty-trash`); 1 new store-root file (`<store>/retain`); 1 transient
store-root staging directory (`<store>/scrub/`); 1 ledgered journal
name-rule amendment (gc-A1, §10 — two new originless name values, no change
to the entry line grammar); 1 new `explain` topic. 0 dependencies, 0 changes
to the journal entry grammar or any shipped identity concept, 0 indexes on
any destructive path, stdout byte-pure everywhere.

---

## 0. Terms and layers

- **object** — bytes at a content address inside the store.
- **record files** — `<store>/log.c4m` (journal) and `<store>/retain`
  (retention record). Store-root FILES, not objects: structurally invisible
  to the closure walk, parsed only by their own pinned grammars, never
  token-scanned. This is what makes it impossible for the record that
  licenses deletion to mark its own condemned IDs.
- **record layer** — structured parsing of the record files plus the specific
  store objects they designate (aggregate list objects §4, retained archive
  objects §3.2). Record-layer reads are always full reads, rehash-verified.
- **walk layer** — the closure walk over store objects (§5). The two layers
  share one byte-classification grammar but different entry points; nothing
  is classified by heuristic at either layer.
- **walk-class / leaf-class** — an ID whose object the walk reads
  (rehash-verified) and parses for edges, versus an ID that is marked into a
  closure set and whose object is NEVER opened by the walk.
- **root catalog** — every claim in the installed journal plus every claim in
  every retained split archive (§3.2). **fold** — effective verdict per
  target: latest line wins per target, aggregate lines distributing per §4.
  **E (the condemned set)** — every catalog root whose effective verdict is
  trash, scrub, or drop (§6.3 — drop roots re-enter by design).
- **keep-closure** — union of walk closures of every catalog root whose
  effective verdict is absent (live) or keep, plus the substrate set (§4.5).
  **drop-closure** — union of walk closures of every root in E.
  **warrant** — drop-closure minus keep-closure, intersected with present
  objects. **stray set** — present objects in neither closure: reported,
  never touched.

The sweep is a deterministic function: (journal bytes, retention-record
bytes, store object bytes) → (keep-closure, drop-closure, warrant, stray
report). Two conforming implementations given identical inputs MUST produce
identical sets and byte-identical machine output (ordering pinned in §5.7,
§6.2, §6.3). No index, cache, mtime, locale, or platform behavior may
influence any set.

---

## 1. The retention record — file and line grammar

- Filename: `<store>/retain`, beside `log.c4m`. Extensionless: it is not c4m
  text; an extension would lie. The verb and the file share one word.
- One verdict per line, LF-terminated, single ASCII spaces, no trailing
  whitespace, exactly four fields (testimony shape A, verbatim):

      <id> <verdict> <actor> <time>

  - `<id>` — a full 90-character C4 store address per the conformance ID
    grammar. Never abbreviated: this file licenses deletion; truncated
    prefixes belong to hint indexes only.
  - `<verdict>` — exactly one of `keep` `trash` `scrub` `drop`, lowercase.
    `drop` is tool-authored only (§2.1, §6.3).
  - `<actor>` — one token, no whitespace. User lines:
    `<username>@<hostname>` as reported by the OS, any whitespace byte
    replaced by `_`. The `c4:` prefix is reserved for tool-authored lines:
    `c4:adopt` (adoption keep lines), `c4:gc` (drop lines), `c4:split`
    (archive keep lines). No `--actor` override ships this round.
  - `<time>` — RFC 3339 UTC seconds (`YYYY-MM-DDTHH:MM:SSZ`), the append
    instant. Provenance, never identity.
- Append-only. NEVER split (`c4 split` applies to `log.c4m` only): drop
  lines are permanent tombstones, falsifiable for the store's lifetime.
  Created on first append, O_CREAT + O_APPEND.
- Journal mechanics inherited verbatim (v7 §6): appends serialize under an OS
  append lock that dies with its holder; writers truncate a torn tail before
  appending; readers warn once and ignore a torn tail; a torn line belongs
  only to an append that never reported success. Any malformed NON-tail line
  is corruption: every reader (retain, gc, explain) refuses loudly, exit 1 —
  no verb ever guesses around the file that licenses deletion.
- Durability: every retention append is flushed durable BEFORE its line
  prints — fsync plus the platform device barrier on macOS/Linux;
  write-through on Windows (v7 §3 voice, ordering-is-the-contract). Printed
  means durable; unprinted means unreported.
- Lock sides (waits, never fails — §9.4): verdict appends and `retain adopt`
  hold the sweep lock (SHARED for appends, EXCLUSIVE for adopt) per v8
  Amendment 3(c); the fold view is a lockless read like `c4 log`.

Zero-tool access is a feature and the man page says so: `grep <ID>
<store>/retain` is the raw history query.

---

## 2. `c4 retain` — the one retention verb

    c4 retain                        # fold view: effective verdict per target
    c4 retain keep  <ID>...          # direct keep lines (rescue / pin), one per ID
    c4 retain trash <ID>...          # direct trash lines (revocable condemnation)
    c4 retain scrub <ID>...          # direct scrub lines (trash + scrub obligation)
    c4 retain keep  -                # AGGREGATE keep: stdin IDs -> one list claim + ONE line
    c4 retain trash -                # AGGREGATE trash: same shape
    c4 retain scrub -                # AGGREGATE scrub: same shape
    c4 retain adopt                  # initialization / stray-adoption pass

The mode keywords ARE the record vocabulary — the surface teaches the file
grammar. `drop` as a mode is REFUSED, exit 1, with the explanation: drop
lines are tool-authored by `c4 gc --empty-trash` at empty time; users condemn
with trash or scrub. A verdict keyword with zero subjects is a usage error,
exit 1 (empty-variable expansion must never silently become a listing).

### 2.1 Direct verdicts (argument form)

- Subjects are bare 90-char store addresses only — never paths, never `.c4m`
  files, never journal indexes, never prefixes.
- Validation per subject, in argument order, all attempted (v7 multi-path
  pin). A subject is accepted iff it is a catalog root (§3.1) that is not an
  aggregate claim (§4.1), not an archive subject (§3.2), whose effective
  verdict is not drop. Refusal classes, each with its own stderr explanation:
  - **interior object** (reachable but not a root): "verdicts attach to
    roots; condemn the roots that reach it (`c4 log` lists them)" — for
    `keep` mode the explanation flips: "keep attaches to roots; keep the
    root(s) that reach it."
  - **unaccounted object** (present in the store, in no catalog): "not a
    root; cure: `c4 retain adopt`, then act through an aggregate over
    adopted-lineage IDs" (§4.2).
  - **unknown** (absent from catalog and store).
  - **aggregate or archive subject in a condemning mode**: aggregates are
    condemned by condemning their MEMBERS (a verdict on an aggregate claim
    distributes, §4.3, and is legal only when it distributes to at least one
    live target); archive subjects are record substrate and take no user
    condemnation — forget an era by condemning its roots (§3.2).
  - **effectively dropped subject** (any mode, including keep): refused;
    narration cites the winning drop line (time, actor) and the cure —
    re-ingest the content (`c4 id -s`) to mint a fresh claim; a keep line
    cannot resurrect bytes, and accepting one would wedge every future gc on
    keep-closure damage. `c4 explain retain <ID>` shows the history.
- Each accepted subject: one line appended, flushed durable, THEN printed.
  Duplicate/no-op verdicts (trash of trashed, keep of live) append normally —
  append-only simplicity, latest-per-target wins, no dedup logic.
- stdout (data, byte-pure): the appended record line(s), byte-identical to
  what landed in `<store>/retain`, one per succeeded subject, in attempt
  order. A refused subject prints nothing (positional attribution valid only
  on exit 0 — the v7 `id` pin).
- stderr (narration, non-contractual, one line per event class): on trash —
  the three-part truth: nothing physical changed; `c4 retain keep <ID>`
  rescues; nothing is deleted until `c4 gc --empty-trash`. On scrub — the
  per-platform honesty sentence at the point of promise: scrub overwrites
  before unlinking wherever the platform can honestly back it; on CoW
  filesystems, wear-leveled SSDs, and under snapshots, physical erasure is
  not c4's to promise; the empty-time summary says exactly what was done. On
  keep of a condemned root — one rescued line. Plus ONE summary line when
  more than one subject was given.

### 2.2 Aggregate verdicts (stdin form) — the aggregate producer

`-` as the sole subject reads one bare 90-char ID per LF line from stdin and
performs ONE aggregate act (this is the spelled aggregate-condemnation
producer; semantics in §4):

1. Validate every member (same acceptance rule as §2.1, plus: adopted-lineage
   IDs are accepted, §4.2; aggregate claims and archive subjects are refused
   as members — distribution is single-level by construction). A blank line
   or any malformed line refuses the whole act (one act, all-or-nothing;
   strip producer blanks with `grep .`). Zero members refuses the act
   (a condemnation of nothing is a no-op wanting to be an act).
2. Deduplicate is NOT performed; the list object stores the lines as given
   (first-occurrence position governs fold order, §4.4).
3. Ingest the ID list as one object and journal it under the full v7 claim
   protocol as an ORIGINLESS claim named `condemned` (name-rule amendment
   gc-A1, §10; section time = scan start per v8 Amendment 2). Content
   addressing dedups re-submissions of the same set.
4. Append ONE verdict line whose subject is the list claim's ID; flush
   durable; print it.

stdout: the one appended line. stderr: one summary line (member count,
verdict class) plus the §2.1 class narration. The act holds the SHARED lock
from first store interaction through its print (it is a claim path).

Rationale, stated in retain(1): arguments spell N deliberate acts; stdin
spells ONE deliberate act over N targets. The record stays proportional to
deliberate acts at any scale (550k per-root lines collapse to one store
object, one journal section, one line), and per-member rescue works
identically in both forms (§4.4).

### 2.3 Fold view (`c4 retain`, no arguments)

- stdout (data): the deterministic fold — one four-field line per TARGET that
  currently has an effective verdict, in the §1 grammar:
  - direct target: its winning record line, verbatim;
  - target whose winning verdict arrives through an aggregate line: a
    synthesized line — member ID + the aggregate line's verdict, actor, time.
  - Order: record order of winning lines; aggregate expansions appear at the
    aggregate line's record position, in list member order (line order, then
    chunk order); a duplicate member within one list takes its first
    occurrence's position. Aggregate subjects themselves NEVER print (they
    are substrate, not targets, §4.3); archive subjects never print as roots.
  - Byte-identical across runs and across conforming implementations OVER AN
    UNCHANGED record and store (the lockless read may see a torn tail
    mid-append; the quiescent fold is the contractual one).
- Live (unverdicted) targets print nothing — no verdict means LIVE, and the
  fold lists verdicts, not silence. Unverdicted roots are composition (§8).
  Dropped targets DO appear (drop is a verdict) — the fold is also where a
  lost drop-line print is recovered, per member.
- Absent record: prints nothing, exit 0 (an absent record is an empty fold —
  the exact analog of v7's absent journal).
- Expansion reads every aggregate list object designated by a retention line,
  in full, rehash-verified (record layer). An absent or rehash-failing
  aggregate list object refuses the whole fold, exit 1 — a fold that silently
  omitted or mis-expanded members would misreport verdicts, and the object is
  substrate that gc can never have removed (§4.5), so absence is damage.
- stderr: nothing on success (optionally ONE summary line: targets by verdict
  class). Exits: 0; 1 record corruption / aggregate-object damage; 3
  transient read failure.

### 2.4 `c4 retain adopt` — initialization and the stray cure

- Under the EXCLUSIVE sweep lock: walk the store; every present object
  unreachable from the catalog's walk closures becomes a member of one
  bare-ID-list object; that object is ingested and journaled under the full
  v7 claim protocol as an ORIGINLESS claim named `adopted` (gc-A1);
  then one keep line `<list-ID> keep c4:adopt <time>` is appended, flushed,
  printed. Explicit visible roots instead of silent vulnerability.
- Zero strays on a virgin record: the FULL path still runs — the empty ID
  list IS the empty object (v7's empty fixed point); it is ensured present,
  journaled as `adopted`, and keep-pinned. The initialization marker is
  therefore always a real record line over a real catalog claim (this is the
  pinned reading; appending a bare keep line without the claim would violate
  retain's own subject validation and wedge gc on a store that never held
  the empty object).
- On an already-initialized record with zero new strays: appends nothing,
  prints nothing. Idempotent, quiet, deterministic.
- Initialization test used by gc: at least one keep line with actor
  `c4:adopt` present in the record. The JOURNAL NAME `adopted` alone licenses
  nothing (§10).
- Exclusive lock rationale: adopt computes store-wide unaccountedness; under
  SHARED it could adopt a concurrent ingest's post-barrier, pre-journal
  objects (benign double-rooting, but sloppy accounting on the file that
  gates the sweeper). Adopt is init-rare; claim paths wait with one narration
  line. Adopt's own ingest is claim work performed by the exclusive holder —
  exclusive subsumes shared; there is no lock nesting.
- Ingest-temp staging left by crashed ingests (good bytes by the
  barrier-before-rename protocol) is v7's affair and is not adopted; the
  scrub staging directory (§6.3) is never adopted.
- stdout: the appended keep line, or nothing. stderr: ONE summary line
  (objects walked, strays adopted, or already-initialized). Exits: 0 done
  (including nothing-to-do); 1 record corruption; 3 curable (walk read
  failure, disk full).
- There is no `--adopt` flag on gc and no standing flag anywhere (Decided
  item 7): gc REPORTS unaccounted objects; re-running `c4 retain adopt` is
  the manual cure, and gc's report names it.

### 2.5 `c4 explain retain <ID>` (existing prose verb, new topic)

Prose only, scripts keep out: every record line whose subject is `<ID>` or
whose aggregate expansion covers `<ID>`, oldest first, with journal context
(claim date, name) and — for aggregate lines — the list claim whose
membership proves coverage. Exit 0 even for a live target (it says: live, no
verdicts).

---

## 3. The root catalog and the fold

### 3.1 Catalog

The root candidate set is every claim in the installed journal PLUS every
claim in every retained split archive, both parsed by the v7 §6 journal
grammar at the record layer. Claim kind comes from the journal name field
exactly as v7 pins it: names ending `.c4m` are description-class
(walk-class); all others are file-bytes claims (leaf-class). Two catalog
claim classes are carved out of the root business entirely and consumed at
the record layer instead (they are never closure-walk roots, whatever their
names):

- **aggregate claims** — originless claims named `adopted` or `condemned`
  (§4.1);
- **archive subjects** — subjects of `c4:split` keep lines (§3.2).

### 3.2 Retained split archives are record-layer, never walked

Forcing defect (prosecution, blocking): edge-walking a retained archive as a
description re-roots every archived claim's full tree into keep-closure, so
"trash 11 months of archived roots, empty" frees zero bytes — the exact
archive-reclamation workload the primitive exists for. Repair, pinned:

- `c4 split` (behavior landing WITH gc, §12) ingests its archive object into
  the store and appends `<archive-ID> keep c4:split <time>`. The `c4:split`
  actor is the discriminator: the subject of any `c4:split` keep line is an
  ARCHIVE SUBJECT.
- Archive subjects are consumed at the record layer: their objects are read
  in full, rehash-verified, and parsed under the JOURNAL grammar; their
  claims join the root catalog as fold-filtered root candidates. Parse or
  rehash failure REFUSES the run (exit 1): the archive supplies catalog
  roots — protection-side knowledge.
- The archive object itself joins every keep-closure, object-level
  (leaf-class), permanently — substrate (§4.5): the archived era's claims and
  tombstones stay falsifiable forever. It appears in no warrant. It is never
  edge-walked as a description (its section entries are catalog candidates,
  never walk edges).
- Condemning verdicts on archive subjects are refused (§2.1): forgetting an
  era means condemning its ROOTS (aggregate over the archived claim IDs);
  the record that names the era is not user data.
- Pre-gc split stores (archives created before retention existed): the
  archived-era claims are not in the catalog; their objects, if still
  present, are unreachable from the catalog and get adopted at init —
  protected object-level, reported honestly. Documented in gc(1) NOTES with
  the general mitigation (§9.6).

### 3.3 The fold (effective verdicts)

Latest line wins per TARGET, by record position. A line's targets:

- subject is an aggregate claim → each list member, at the line's position
  (§4.3); the subject itself is never a target.
- any other subject → the subject itself.

No verdict = LIVE. keep and live targets root the keep-closure; trash, scrub,
and drop targets form E (drop targets re-enter every empty by design — §6.3
convergence). The fold is total over hand-edited records too: a protective
line on an unknown subject marks that ID object-level into keep-closure
(over-marking is the safe direction) and is reported; a condemning line on an
unknown subject is INERT for the warrant and reported — a forged or corrupt
line must never widen deletion. (Well-formed porcelain never writes either;
this is the sweeper's total behavior.)

---

## 4. The aggregate condemnation primitive

Problem (architecture appendix): "keep 1 month, drop 11" over 300k snapshots
needs ~550k permanent per-root lines (~80 MB) in a record designed never to
split. Pinned answer: an aggregate verdict is an ORDINARY shape-A line —
grammar untouched — whose subject is a porcelain-minted aggregate claim.
Adoption is the same primitive (an aggregate keep): one mechanism, two uses.

### 4.1 Aggregate claims — identity, not sniffing

An aggregate claim is a journal claim that is ORIGINLESS and named `adopted`
or `condemned` (gc-A1, §10). Only `c4 retain` (stdin form; adopt) and `c4 gc`
(residual lists) mint them. Forcing defects of the two rejected
identification schemes: `.c4m`-suffixed names have no possible writer (v7
dispatch decodes `.c4m` arguments as descriptions; a bare ID list always
fails description decode per §Empty Patch Rejection) and spend the very v7
name pin they stand on; bytes-shape sniffing of arbitrary verdict subjects
puts unbounded blob reads on every fold and lets bit-rot silently reclassify
a protective aggregate as a direct keep (under-marking). Under gc-A1 the
discriminator is the (originless AND name) conjunction — unforgeable within
the no-foreign-writer contract, because v7's total name rule gives every
file-argument claim an origin, and stdin claims are named `stdin`/`stdin.c4m`
only. A user file literally named `adopted` journals WITH origin and is an
ordinary blob claim; a user blob whose bytes happen to look like an ID list
is an ordinary blob claim. The journal NAME alone licenses nothing; the
record's `c4:adopt` line is the initialization marker (§2.4).

An aggregate claim's object must parse as an ID-list (§5.3 shape ii) or be
empty (the adoption marker's empty list); its record-layer read is always
full and rehash-verified; absence, rehash failure, or non-list bytes REFUSE
the reading verb (fold exit 1, gc exit 1) — the record's referent is damaged
and the record is the license.

### 4.2 Member classes and the condemnation asymmetry

Members of an aggregate act are, each: a catalog root (kind from the journal
per §3.1), or an ADOPTED-LINEAGE ID — a member of any `adopted` claim's list
object (direct membership test over those rehash-verified lists).
Adopted-lineage members distribute as object-level, leaf-class targets:
protection or condemnation of exactly those bytes.

The asymmetry, in one sentence with its rationale: a DIRECT condemning
verdict on an unjournaled ID is refused at the verb and inert at the sweep,
while the SAME ID is condemnable as a member of a porcelain-minted aggregate
— because every condemnation's subject must be journal-attested, and the
aggregate's claim line is exactly that attestation (enumerable, immutable,
rehash-verifiable membership), which a bare unjournaled ID on a record line
can never be.

### 4.3 Distribution — single-level, subjects are substrate

A verdict line whose subject is an aggregate claim applies to each list
member, in list order, at the line's record position; latest-per-target
across ALL lines (direct and synthesized alike) wins. Distribution is
SINGLE-LEVEL and members are terminal: aggregate claims are refused as
members at produce time, and a sweeper meeting one anyway (hand-forged
record) treats it as a terminal object-level target. Rationale: every real
workload (adoption, archive reclamation, residuals, mass rescue) is a flat
set; recursion would force member-object classification reads into every
fold and a recursion grammar into the deletion path for zero demanded
capability. Condemning members-of-members is spelled by making each list its
own act.

The aggregate SUBJECT itself is never a target: it carries no root verdict,
never appears in the fold or in E, and its object is permanent substrate
(§4.5). Forcing defect: giving the subject a root verdict makes drop
tombstone exactness circular (a drop covering the subject must cover its
members; a residual then needs its own residual), while leaving it a LIVE
root invites re-rooting readings; substrate-not-root dissolves both, and the
one thing the subject's bytes must do — stay readable as the tombstone's
enumeration — is exactly what substrate pins.

### 4.4 Rescue and drop exactness

- A later single-target line overrides the aggregate for that member:
  rescuing one snapshot from a 550k-member trash is one `keep` line.
- A later aggregate overrides an earlier one member-wise.
- The empty appends drop lines whose distributed target set equals EXACTLY
  the set of roots it empties (§6.3): if every distributed member of a
  condemned aggregate is emptied, one drop on the original subject covers
  it; if any member was rescued or is otherwise not emptied, gc materializes
  the RESIDUAL list (exactly the emptied members), journals it as a
  `condemned` claim, and drops THAT. Drop lines distribute by the same §4.3
  rule at their own record position, so no drop's expansion can ever cover a
  rescued member (the rescue-revocation defect is structurally closed), and
  every emptied member's effective verdict becomes drop (the fold never
  shows a freed root as merely trash).
- Warrant arithmetic is unchanged: drop-closure minus keep-closure. An
  aggregate that (through forgery or accident) names an interior object of a
  retained root cannot kill it — the subtraction protects it structurally.

### 4.5 Record substrate is permanent

The substrate set = every aggregate-claim list object designated by any
retention line, every residual list object, and every archive subject's
object (§3.2). Substrate joins every keep-closure unconditionally,
OBJECT-LEVEL and LEAF-CLASS (pinned kind — it never re-roots anything it
names), and is excluded from every warrant, forever: a dropped aggregate's
list survives its own empty as the tombstone's readable enumeration.
Scope is precise (forcing defect: the earlier "every retention-line subject"
rule made a scrubbed secret's own bytes immortal): DIRECT subjects get no
substrate protection — their objects live or die by ordinary closure
arithmetic, so condemned direct claims are freeable and scrubbable. This
does not touch the permanent links-never-pin-bytes stance, which governs
testimony and link records, not the retention record.

### 4.6 Per-root falsifiability

"Was root R deliberately dropped, and when?" is one query over two
authoritative sources: fold the record (small — one line per deliberate act
plus residuals), resolving aggregate subjects through their retained,
rehash-verified list objects. The answer names the verdict line, its actor
and time, and — for an aggregate — the list object whose membership proves R
was covered. `c4 explain retain R` is the prose form; `c4 retain | awk`
is the machine form; `grep` of the record plus `c4 cat` of list objects is
the zero-tool form.

### 4.7 Rejected alternatives (recorded; do not re-litigate without new context)

- **Journal-section-range subjects** (`drop sections 1..250000`): breaks the
  shape-A subject-is-an-ID grammar; per-file indices shift meaning at split
  (tombstone referent depends on file lineage, not content); ranges cannot be
  rescued or verified member-wise.
- **`.c4m`-named ID-list claims**: no possible writer under v7 dispatch;
  spends the name pin it stands on (prosecution, blocking).
- **Bytes-shape aggregate detection on arbitrary subjects**: unbounded
  fold-time blob reads; rot silently reclassifies protective aggregates
  (under-marking); replaced by gc-A1 identity, which also structurally
  removes the trash-a-blob-fans-out footgun rather than documenting it.
- **Recursive (nested) distribution**: forces member classification reads
  into every fold; no workload demands it; single-level with per-list acts
  covers the space.

---

## 5. The closure walk — the pinned TOTAL grammar

### 5.1 Root-kind assignment

Walks start from (ID, kind) pairs supplied by the record layer (§3):
catalog roots with journal-name kinds, keep/live roots into the keep walk,
E roots into the drop walk; adopted-lineage and unknown-subject protective
marks enter object-level (leaf-class). Kind is assigned ONLY from the
store's own records, never by probing unattributed bytes. Unjournaled IDs
are leaf-class, always (§5.6).

### 5.2 The total classification rule

An object read by the walk is classified by attempting, in pinned order,
(i) the description shape, then (ii) the ID-list shape; an object whose
bytes are not canonical c4m text under (i) and not an ID-list under (ii) is
a LEAF — zero edges, no partial credit, one nonconforming byte anywhere
reclassifies the whole object. Leaf-class arrivals are never read at all.
Edges come only from the structural positions of §5.4; no object's bytes are
ever scanned for embedded ID tokens (killing the base58 false-positive class
on media stores permanently). No heuristic — size, extension, entropy,
"looks like text" — ever participates: classification is a total function of
(arrival kind, exact bytes).

Every walk-class read is rehash-verified (SHA-512 = ID) before parsing.
Damage handling is closure-asymmetric, pinned in §5.8.

A walk-class arrival from a `.c4m`-named JOURNAL CLAIM whose rehash-clean
bytes classify leaf is legal and counted: one stderr narration class ("N
description-class claims classified leaf"). Decision, with rationale:
report-not-abort, because rehash-clean bytes are ground truth and the name
is only an expectation (a user file named `foo.c4m` must not brick sweeps);
actual rot is caught by the rehash and takes the abort path. The V-24
encoder fixture (§11) gates this leniency against real encoder bytes before
any sweeper ships.

### 5.3 Line grammar (walk-class reads only)

Bytes split at LF (0x0A). CR anywhere, invalid UTF-8, or a line beginning
`@` → the object is a leaf. A final unterminated line is accepted (leniency;
V-24 pins the shipped encoder's actual final-LF behavior as the canonical
fixture). Each line is exactly one of:

- **L0 — blank**: zero bytes. Ignored.
- **L1 — bare-ID line**: exactly 90 bytes forming one conformant ID.
- **L2 — ID-concatenation line**: length > 90, a multiple of 90, every
  consecutive 90-byte chunk a conformant ID.
- **L3 — canonical entry line**: single 0x20 between fields, no leading
  indentation:
  `<mode> <timestamp> <size> <name>[ <link-op>[ <target>]] <id>`
  with mode `-` or a 10-character mode string (first char in `-dlpsbc`,
  remainder in the pinned rwxsStT- classes); timestamp `-` or exactly
  `YYYY-MM-DDTHH:MM:SSZ`; size `-` or `0` or a decimal with no leading
  zeros; name a SafeName-encoded token per SPECIFICATION.md §Filename
  encoding and §c4m field-boundary escaping (normative — LITERAL spaces,
  quotes, and brackets appear only backslash-escaped; an UNESCAPED `[...]`
  pair is the fold-range syntax per §Media File Sequences and dispatches
  §5.4 rule 2; the tokenizer honors `\` escapes and names never contain
  unescaped 0x20); optional link operator `->`, `->N` (N a digit, no
  space), `<-`, or `<>` with its target token per the spec's mode-based
  disambiguation; final field `-` or one conformant ID. This one production
  covers store listing entries AND journal/split-archive section entries
  (origin `<- host:path` is the inbound operator; a Windows drive colon
  belongs to the path, split at the first colon, per v7).

Anything else → the whole object is a leaf.

**Shape (i) — description**: a sequence of L0/L1/L2/L3 lines satisfying
SPECIFICATION.md §Patch Format, with §Empty Patch Rejection strictness
adopted verbatim (normative): a non-empty description contains at least one
L3 entry line, and every patch section contains at least one L3 entry line.
An L1 as the first non-blank line is the external base reference; a later L1
is a block checkpoint; two consecutive L1 lines, or an L1 with no preceding
L3 in its section, fail the shape. This strictness is load-bearing: it is
what makes a one-ID-per-line list — and a single-L2-line object, which
contains no L3 — fail shape (i) and fall through to shape (ii), so the same
bytes can never classify differently at two arrival sites. The empty object
(zero bytes) IS a description — the empty-description constant — with zero
edges. Natural-sort order and duplicate names are NOT checked (leniency);
duplicates contribute the union of their edges.

**Shape (ii) — ID-list**: one or more non-blank lines, every one L1 or L2.
Members are all 90-byte chunks in line order, then chunk order — one
production covering both shapes the store has ever written (one-ID-per-line
and legacy bare concatenation; the shipped `parseIDList` in c4m/sequence.go
accepts both).

**Leniency principle (pinned, exhaustive)**: misclassifying a store-created
description as a leaf under-marks its children — the catastrophic direction
— so the walk accepts a pinned SUPERSET of everything any conforming c4
writer ever stored as a description: blank lines ignored, sort order
unchecked, duplicates unioned, final LF optional. Every leniency widens
acceptance only (over-marking, safe) and the list above is exhaustive.
Ergonomic forms (padded sizes, comma separators, non-UTC timestamps,
indentation, alignment) are NOT accepted: store-created descriptions are
canonical by construction, so ergonomic text is user file bytes and
correctly classifies leaf.

### 5.4 Edge extraction (total, per structural position)

From a **description**:

- **L1 base reference** (first non-blank line) → walk-class edge.
- **L1 checkpoint** → walk-class edge (absent-tolerant per §5.8; the mark is
  recorded regardless).
- **L2 inline ID-list line** → each chunk a leaf-class edge.
- **L3, null ID** → no edge. Null fields never affect extraction.
- **L3, non-null ID**, evaluated in pinned order:
  1. mode begins `l` (symlink) → NO edge, any era. v7 pins symlink entries
     as naming no store object; legacy non-null symlink IDs are provenance
     echoes. Bytes reachable only that way land in the stray set — reported,
     never touched, adoptable — never in any warrant.
  2. name carries an unescaped `[...]` range (a fold) → walk-class edge to
     the ID-LIST OBJECT the ID names; member kind is declared by the entry:
     name ends `/` (legacy directory sequence) → members walk-class; else →
     members leaf-class (v8 scanners fold only plain files; the `/` clause
     keeps the rule total over legacy text).
  3. name ends `/` (directory) → walk-class edge (a listing object).
  4. name ends `.c4m` → walk-class edge: read once, rehash-verified,
     classified by §5.2 — canonical c4m protects its references
     (over-marking, safe); anything else is a leaf. No name-only decision
     ever extracts an edge from unparsed bytes.
  5. otherwise (plain file; hard link `->`/`->N`; flow-linked file) →
     leaf-class edge. Link operators never change edge kind; a flow-linked
     directory entry falls under rule 3.

From an **ID-list** (reached as a fold-entry target): every member chunk is
an edge with the kind the fold entry declared (rule 2). The list object
itself is always marked. (Aggregate-claim lists are record-layer objects and
never arrive here as roots; the same BYTES referenced by some user fold
entry classify by this same rule — ordinary marking, no distribution.)

From a **leaf**: no edges, and no read ever occurred for leaf-class
arrivals.

### 5.5 Chain semantics — union over sections, never final state

A walked description that is a patch chain contributes every structural ID
in EVERY section: entry edges per §5.4, the base reference, every
checkpoint, every inline ID-list chunk. Rationale: patch semantics make
removal a restatement; final-state-only marking would let a
removal-restatement silently unroot history — the condemned under-marking
class. (Retained journal ARCHIVES are not walked at all — §3.2; this rule
governs user description chains reached as closure roots.)

### 5.6 The adopted-member pin (the ~100× sentence) and its soundness

**Pinned: members of the adopted ID-list are LEAVES — leaf-class,
object-level targets, marked by ID, never read.** Equivalently: kind
assignment trusts only the store's own records, and adopted members are
unjournaled, so no record assigns them walk-class. The first sweep of an
adopted 500 GB media store therefore reads only description and ID-list
objects — seconds to minutes over NAS — instead of all 500 GB
(content-bound, hours): the architecture appendix's ~100× swing, made an
executable assertion by V-22.

**Soundness argument.** Let P₀ be the objects present at adoption, J the
union of walk closures of all journaled roots (computed by this grammar),
and A = P₀ ∖ J the adopted set. Adoption is transitively closed at init BY
CONSTRUCTION of that subtraction: every present object is either in J
(marked whenever its roots are retained) or in A (marked directly as an
adopted member). So there exists no store-resident object that member-walking
an adopted description would mark and member-as-leaf does not — every
would-be child is independently marked through its own membership in J or A.
Under the no-foreign-writer contract (only c4 writes inside a store), no new
unaccounted structure appears after init, so the partition persists. At the
default fold (no verdicts), member-as-leaf and member-walk compute identical
survivor sets.

**What invalidates it — stated honestly, both edges:**

1. **A foreign or pre-suite writer after adoption.** Its new objects land in
   neither closure → reported, never swept (safe), but any STRUCTURE it
   creates is invisible: a foreign-written description's children get no
   protection through it. Documented out of gc's contract, gated on the
   coordinated suite release (§9.5) — one of the two premises of the proof,
   not a side note.
2. **The overlap case: adoption grants object protection, not closure
   protection.** If an adopted member A is itself description-shaped and
   references object C that also sits inside journaled root R's closure,
   then C's only closure-grade protection is R: drop and empty R, and C is
   warranted and swept while A survives with a dangling interior reference.
   This bounded gap is kept deliberately: the store never created A's
   structure, never verified it, never promised it a closure. The cure is
   one command, stated at the adoption surface and in the gc bill whenever
   adopted lists are present: a pre-journal description you want
   closure-protected must be CLAIMED — ingest it, making it a journaled root
   with a real closure. (V-19/V-20 pin both edges as behavior, not
   surprise.)

**Why probing would be wrong even if free:** reading adopted members and
honoring any that parse puts a parse of unverified-provenance bytes on the
deletion path (the banned posture) and hands unclaimed strays a silent veto
— a copied `project.c4m` lying in a pre-journal store would invisibly pin
every ID it names, so a deliberate trash-and-empty frees less than the bill
said, with the protector appearing in no record. Deletion is deliberate user
action; so is protection. Member-as-leaf keeps both attributable to a record
line.

### 5.7 Determinism, termination, ordering

- **Memoization**: each ID is processed AT MOST ONCE PER KIND; a walk-class
  arrival of an ID already marked leaf-class re-processes it (kind widening
  is monotone — upgrade, read, extract — so arrival order cannot change any
  set; V-33 exercises both orders). An object walked in both closures is
  read once; membership is tracked per closure.
- **Termination**: finite store + per-kind visited set; adversarial cyclic
  references terminate (an already-processed ID adds no work).
- **Walk order**: the keep walk completes before the drop walk (damage
  asymmetry, §5.8, depends on knowing keep-reachability first).
- **Externalized orders, exact** (byte-identical across implementations):
  - warrant stdout: byte-ascending, unique (§6.2);
  - stray report: byte-ascending, unique;
  - unlink sequence: three passes — (1) all warranted never-walked (leaf)
    objects, byte-ascending; (2) all warranted ID-list objects,
    byte-ascending; (3) warranted description objects by iterative peel:
    repeatedly unlink the byte-ascending smallest description all of whose
    warranted walk-class-referenced descriptions are already unlinked
    (children before referrers). Bulk bytes vanish first and the structural
    skeleton last, which bounds any crash leak to description-skeleton
    objects (§6.3 convergence).
- Dry run and empty are independent full recomputations of the same
  deterministic function — nothing cached ever sits on a destructive path;
  the honest cost is that trash → review → empty pays the walk twice.

### 5.8 Absence, corruption, damage asymmetry

- **Keep-closure walk** (protection-side): a MISSING object or a REHASH
  MISMATCH at any walk-class root or edge REFUSES the run, exit 1, naming
  the ID — an unverifiable keep-closure means under-marking risk, the one
  failure mode that destroys kept data, so nothing may be billed or emptied.
  (gc's own operation never removes keep-reachable objects, so keep-side
  absence always indicates external damage.)
- **Drop-closure walk** (condemnation-side): an absent walk-class object is
  marked, contributes no edges, and is counted once in narration (normal
  during crash re-runs — half-unlinked closures re-warrant idempotently); a
  rehash-mismatching object not in keep-closure is marked, NOT parsed
  (corrupt bytes contribute no edges — a bogus edge could pull a stray into
  the warrant, widening deletion), counted, and remains warrantable (it is
  condemned bytes). Descendants reachable only through damaged objects leak
  to the stray report — a leak, never a loss.
- **Record-parse failure** (journal, retention record, archive subject, or
  aggregate list object failing its pinned grammar or rehash): the run
  refuses, exit 1 — the record layer is the sweep's license; an unreadable
  license licenses nothing. (Torn-tail truncation at next append is the
  record files' own pin and precedes this rule.)
- **Verdicts on unknown subjects**: §3.3 — protective over-protect,
  condemning inert, both reported.

---

## 6. `c4 gc` — the collector

    c4 gc                # dry run (default): print the warrant, bill on stderr
    c4 gc --empty-trash  # the empty: recompute, record, sweep

No positional arguments in either mode; any argument other than
`--empty-trash` is a usage error, exit 1. No short form, no interactive
confirmation — the long-form flag IS the consent (no prompts in composable
verbs). Selective emptying is not a flag: rescue first
(`c4 retain keep`), then empty.

Both modes refuse, exit 1, on an uninitialized store (no `c4:adopt` keep
line in `<store>/retain`, including an absent record) — stderr names the
cure: `c4 retain adopt`. Both refuse, exit 1, on any §5.8 record-layer
failure or keep-closure damage.

### 6.1 Shared machinery

Parse the two record files once at start; build the catalog (§3.1, archives
record-layer per §3.2); fold (§3.3, §4.3); walk keep-closure then
drop-closure (§5); warrant = drop-closure − keep-closure ∩ present, minus
substrate (§4.5). Preview honesty, stated in gc(1): each invocation reflects
the records at ITS start instant; the empty always recomputes its own
warrant and may differ from a prior dry run if verdicts changed in between —
it never accepts a warrant file, a prior dry run's output, or any caller
memory (the withdrawn-gc inversion, kept structural).

### 6.2 Dry run (`c4 gc`)

- Holds the SHARED lock (a preview must never block claim paths for a
  potentially 25-minute walk).
- stdout (data): the would-be warrant — one bare 90-char ID per line, the
  exact object set emptying would unlink, sorted byte-ascending, unique.
  Sorted because the warrant is a set: two bills diff with `comm`, and the
  print contract decouples from walk order. Empty warrant = empty stdout,
  exit 0.
- stderr (the human bill; format non-contractual, content required): one
  line per DIRECTLY-condemned root — ID, verdict class, journal date/name,
  bytes and object count uniquely reachable through it (what emptying
  actually frees; shared objects survive and the bill says so); ONE line per
  condemned aggregate — member count and aggregate unique totals (the bill
  stays human-scale at 300k members); a count of outstanding scrub-staging
  objects if any (§6.3 step 0 will complete them); the overlap-case note
  when adopted lists are present (§5.6); then ONE summary line: total roots,
  objects, bytes to free, scrub-class counts, and the unaccounted-object
  count with the cure spelled (`N objects unaccounted — c4 retain adopt`).
- Cost honesty (gc(1) COST and the summary): the dry run pays the same
  closure walk as the empty — proportional to retained history, not
  reclaimed bytes; trash → review → empty pays the walk twice, deliberately.
  Media-store sweeps ride §5.6: only description/ID-list objects are read.
- Exits: 0 complete warrant printed; 1 refused (uninitialized, record-layer
  failure, usage, keep-closure damage — §5.8); 3 curable — transient store
  read failure, or ANY stdout write failure including EPIPE (a truncated
  warrant must never be acted on: nonzero mandatory; re-run reprints).

### 6.3 The empty (`c4 gc --empty-trash`)

Holds the EXCLUSIVE sweep lock through parse + closure + record + sweep
(v8 Amendment 3(c)); every claim path (ingest, restore, retention append)
waits, with one narration line, until release. gc's own residual-list ingest
is claim work performed by the exclusive holder — no lock nesting, no
deadlock.

Order of operations, the printed contract:

0. **Scrub-staging recovery.** Complete every outstanding obligation in
   `<store>/scrub/`: overwrite, then unlink, every staged file (staging
   contains only scrub-class objects by construction). Unconditional — runs
   even when the new warrant is empty. Forcing defect: a crash after
   vacate-rename left plaintext at staging names invisible to every
   recomputed closure; exit 0 must attest staging empty or "scrub completed"
   is a silent downgrade.
1. **Parse and fold** (§6.1). E = every root with effective verdict trash,
   scrub, or drop. Drop roots ALWAYS re-enter (their still-present closure
   objects re-warrant idempotently); this is the pinned convergence rule —
   two conforming sweepers compute the same fixed point, and an object that
   once survived through sharing is freed by the later empty that condemns
   its last protector.
2. **Mark**: keep-closure then drop-closure (§5), damage rules §5.8.
3. **Residual materialization** (§4.4): for each condemned aggregate whose
   distributed member set is not exactly the emptied set, compute the
   residual list; ingest and journal it (`condemned`, originless) — content
   addressing reuses the identical claim from a crashed prior run rather
   than duplicating.
4. **Drop-line append**: one line per NEWLY-dropped target set —
   `<subject> drop c4:gc <time>` where subject is the original aggregate
   (full empty), the residual list (partial empty), or the direct root; in
   record order of the winning condemnation lines. Never re-appended for
   targets already effectively dropped. Flushed durable, THEN printed.
5. **Print** the appended drop lines (stdout, byte-identical to the record)
   BEFORE unlinking begins — the restore-line-1 pattern: the printed line is
   the durable claim; the destructive work follows; the exit code attests
   completion. A failed print does NOT stop the sweep (the drops are already
   durable); it forces exit 3.
6. **Unlink** warrant objects in the §5.7 three-pass order. Scrub disposal
   per object: atomically rename to `<store>/scrub/<id>.scrub` (vacating the
   hash name so presence at a hash name always implies good bytes), then
   overwrite, then unlink. **Disposal class is recomputed from record
   history, never from winning lines** (the 4-field grammar carries no class
   on drop lines): an object takes scrub disposal iff any record line with
   verdict scrub — direct or distributed, at or before the covering drop —
   targets any root through which the object entered the warrant; the
   stronger promise wins, over-scrubbing is safe, and crash re-runs re-derive
   the same answer (V-36).
7. **Report** stray objects (never touched) and the summary.

- stdout (data): the appended drop lines only.
- stderr: ONE summary line at completion — roots emptied, objects unlinked,
  objects scrubbed ("scrubbed where the platform backs it" — short form of
  the per-platform honesty sentence), staging obligations completed, bytes
  freed, unaccounted objects with the cure. No live progress line.
- Convergence, honestly stated: a re-run after any crash converges to a
  survivor SUPERSET of the uncrashed run — objects whose warrant membership
  became uncomputable (an interior description already unlinked) leak to the
  stray report, bounded to description-skeleton objects by the three-pass
  order; a leak, never a loss, and never a kept object. Re-runs append no
  new drop lines and print nothing when the record already leads; they
  complete staging, unlink the remainder, and exit 0.
- Exits: 0 — staging empty, record led, every warrant object unlinked, scrub
  obligations verified complete, drop lines printed; 1 — refused BEFORE any
  modification (uninitialized, record-layer failure, usage, keep-closure
  damage; nothing dropped, nothing unlinked; staging recovery is the one
  exception: it may complete before a refusal is discovered, which only ever
  finishes a prior empty's promise); 3 — curable: residual-ingest or
  drop-append failure before any unlink (record could not lead, bytes did
  not follow), unlink/scrub failure mid-sweep (the record already leads;
  re-run converges), stdout write failure (drops durable; re-run prints
  nothing; the fold holds the drop lines per member, and gc(1) says exactly
  this).

Exit 2 is deliberately unassigned for gc (both modes) and for retain: no
partiality class exists — a sweep converges or is curable; unaccounted
objects are narrated standing state with a named cure, not this run's
partiality. Stated in the exit map so nobody backfills it.

---

## 7. Output and exit contract (v7 §5 conventions)

stdout is data, byte-pure; stderr is narration under the light
non-contractual pin (ONE summary line; parse nothing). Uniform pins: lines
already printed remain true, durable claims at every nonzero exit; EPIPE is
an error exit, never a silent death; capture stdout whole, then split.

Per-verb stdout, total enumeration:
- **retain keep/trash/scrub (args)**: appended record lines, one per
  succeeded subject, attempt order.
- **retain keep/trash/scrub (stdin)**: the ONE appended aggregate line.
- **retain (fold)**: four-field verdict lines per §2.3, or nothing.
- **retain adopt**: the appended keep line, or nothing.
- **gc**: sorted unique bare-ID warrant lines, or nothing.
- **gc --empty-trash**: the appended drop lines, or nothing.
- **explain retain**: prose — scripts have no business parsing it.

Exit map:

| verb                     | 0                                          | 1                                                                                    | 2 | 3                                                                                     |
|--------------------------|--------------------------------------------|--------------------------------------------------------------------------------------|---|---------------------------------------------------------------------------------------|
| retain keep/trash/scrub  | all lines appended+flushed+printed         | any refused subject (printed lines stay true); print failure after durable append (verdict stands; re-run re-appends harmlessly) | — | append/read failure before report — nothing reported, nothing claimed; cure, re-run    |
| retain (fold)            | fold printed whole (possibly empty)        | record corruption; absent/corrupt aggregate list object                                | — | transient read failure                                                                  |
| retain adopt             | adopted or nothing-to-do                   | record corruption                                                                      | — | walk/ingest failure; cure, re-run                                                       |
| gc (dry run)             | complete warrant printed                   | uninitialized; record-layer failure; usage; keep-closure damage                        | — | transient read failure; stdout failure incl. EPIPE (never act on a truncated warrant)   |
| gc --empty-trash         | staging empty; record led; sweep complete  | refused before modification (as dry run) — nothing dropped, nothing unlinked           | — | mid-sweep failure (re-run converges); stdout failure (fold holds the drops, per member) |

The retain print-failure exit is 1 (the `id` convention: the claim is
journaled/appended; a re-run reprints); the gc print-failure exit is 3 (the
`restore` convention: never act on truncated output; the cure is a re-run).

---

## 8. SCRIPTING (leads --help on retain(1) and gc(1); first screen carries the two-act recipe)

    # Warrants and folds are byte-ordered; pin the locale on anything that sorts or joins them.
    export LC_ALL=C

    # trash is one revocable line; nothing is deleted until the long-form empty
    c4 retain trash "$SNAP"          # prints the appended verdict line (durable when printed)
    c4 retain keep  "$SNAP"          # the rescue — same shape, opposite verdict

    # capture the bill: stdout is the warrant — sorted bare IDs, one per object emptying would free
    c4 gc > bill.txt                 # the human bill (per-root bytes) is stderr narration
    wc -l < bill.txt                 # empty file = nothing to free
    comm -13 old-bill.txt bill.txt   # what changed since the last review (warrants are sorted sets)

    # test a verdict: the fold is one 4-field line per verdicted target; empty answer = live
    c4 retain | awk -v id="$ID" '$1==id{print $2}'

    # unverdicted roots are composition: installed-journal roots minus verdicted targets
    # (scoped honestly: c4 log reads the INSTALLED journal; archived-era claims live in the
    #  kept archive text — plain c4m, awk-able — and in gc's own catalog)
    comm -23 <(c4 log | awk '{print $NF}' | sort -u) <(c4 retain | awk '{print $1}' | sort -u)

    # aggregate condemnation (archive reclamation): ONE record line for many roots
    c4 log | awk '{print $NF}' | your-filter | sort -u | c4 retain trash -
    # excluding already-dropped roots from a mass act:
    c4 retain | awk '$2=="drop"{print $1}' | sort -u > dropped.txt
    ... | sort -u | comm -23 - dropped.txt | c4 retain trash -

    # empty safely from a script: review first, then the long-form act; check the exit
    c4 gc > bill.txt || exit         # 1 = refused (uninitialized? damaged?), 3 = re-run
    c4 gc --empty-trash              # recomputes its own warrant; prints drop lines (durable when printed)
    # exit 3: the record already leads — cure, re-run; drops are never lost:
    c4 retain | awk '$2=="drop"'

Recipe pins carried: every recipe line's stdout is bare IDs or 4-field
verdict lines — never prose; field extraction over structured records only;
one path/subject per invocation wherever positional attribution matters.

---

## 9. Interaction sites on existing verbs (narration pins, no flag spend)

1. **restore** of a root whose effective verdict is trash or scrub: works
   unchanged; ONE stderr narration line — target is in trash (verdict, date,
   actor); `c4 retain keep <ID>` rescues it. No silent state change, no
   auto-rescue (rejected in the architecture).
2. **restore** of an effectively-dropped root: fails closed at the existing
   pre-flight closure gate WHEN its closure no longer resolves — the normal
   case after an empty — exit 1, nothing touched, with the retention history
   in the hint (dropped `<date>`; `c4 explain retain <ID>`). If the closure
   happens to survive whole (sharing with retained roots), restore succeeds
   and the same effective-verdict narration line fires; success or failure is
   decided solely by the existing closure gate, and the man page words it
   conditionally.
3. **c4 log**: untouched, byte-for-byte, scoped to the installed journal as
   v7 pins. The fold view is `c4 retain`; the "master c4m" folder-tree
   rendering (trash/, scrub/ folders) remains a derived app-layer view
   (Avalanche) of the same fold — no CLI verb spent.
4. **Locks**: sweep-lock acquisition WAITS — it never fails a verb; when not
   immediately available, one stderr narration line says what is being waited
   on. Single lock, no nesting, no deadlock. NAS caveat, both man pages: the
   lock and flush pins assume local filesystem semantics; a store path on
   NFS/SMB must re-prove them per medium, exactly as remote stores must.
5. **Mixed fleets** (gc(1) NOTES): pre-journal writers after adoption are
   outside gc's contract until the fleet upgrades — the coordinated suite
   release gate (RELEASE.md). Their objects are reported strays; their
   structure gets no protection (§5.6 invalidator 1).
6. **Interim mitigation** (gc(1) NOTES until the suite ships gc): re-ingest
   the roots you care about into a fresh store and swap directories.
7. **c4 split** (lands with gc): ingests its archive object and appends the
   `c4:split` keep line (§3.2); split never touches `<store>/retain`'s
   lifecycle (the retention record is never split).

---

## 10. Journal name-rule amendment gc-A1 (ledgered)

v7 §6 pins one total rule per journal field; its name rule maps originless
claims to `stdin`/`stdin.c4m` only. gc-A1 amends the NAME VALUE set (entry
line grammar unchanged, zero parser impact): two new originless name values,
**`adopted`** (minted only by `c4 retain adopt`) and **`condemned`** (minted
only by `c4 retain <verdict> -` and by gc's residual materialization).
Defect forcing the amendment: aggregate and adoption claims need an
unforgeable, zero-read, record-visible identity; every alternative either
has no possible writer (`.c4m` names decode as descriptions), reads
unbounded bytes on the fold path, or lets rot silently reclassify a
protective aggregate. Because v7 gives every file-argument claim an origin
and names stdin claims `stdin`, no user input can mint these claims; the
(originless AND name) conjunction is the discriminator, and the name WITH an
origin licenses nothing. The retention record's `c4:adopt` /`c4:gc`
/`c4:split` actors carry the corresponding record-side authority (§2.4,
§3.2).

---

## 11. Conformance tests (required green, cross-implementation — two independent implementations must emit byte-identical sets and machine output — before any sweeper ships)

Classification and edges:
- V-1 empty object → description, zero edges (the empty constant).
- V-2 single canonical file entry → one leaf edge; leaf never opened
  (read-counter assertion).
- V-3 directory entry → walk edge; nested listing walked; grandchildren
  marked.
- V-4 fold entry + one-ID-per-line list object → list walked, members leaf,
  member objects never opened.
- V-5 legacy bare-concatenation list object → identical member set to V-4.
- V-6 mixed list (multi-chunk lines) → all chunks, line order then chunk
  order.
- V-7 chain: base L1 + entries + checkpoint L1 + patch section → base and
  checkpoint walk edges; all sections' entry IDs edged.
- V-8 removal-by-restatement chain → the removed entry's ID IS in the
  closure (union-not-final-state).
- V-9 retained split archive (null modes, `<- host:path` origins) →
  consumed at the RECORD layer: archived claims appear as fold-filtered
  catalog roots; the archive object is substrate; it is never edge-walked;
  aggregate-trash of an archived era + empty actually FREES the era's bytes
  while the archive object and dropped-claim tombstones survive.
- V-10 each ergonomic/nonconforming mutation → whole object leaf: indented
  entry; padded/comma size; leading-zero size; non-UTC timestamp; CR; CRLF;
  `@` line; invalid UTF-8; one malformed line amid valid ones.
- V-11 lenient accepts: blank lines; unsorted entries; duplicate names
  (edge union); missing final LF — all still descriptions.
- V-12 symlink entries, null-ID and legacy non-null-ID → zero edges; bytes
  reachable only that way stray, never warranted.
- V-13 hard-link entries `->` and `->2` → leaf edge each.
- V-14 flow-linked directory entry → walk edge; location target ignored.
- V-15 90-byte ambiguity: one-bare-ID-line object as fold target → 1 member;
  as probed root → shape (i) fails (no L3), shape (ii) yields 1 member; a
  pure one-ID-per-line multi-line object → ID-list, never description; a
  SINGLE-L2-LINE object → ID-list (no L3 ⇒ shape (i) fails), including
  through a legacy `/`-fold arrival where its members are walk-class.
- V-16 entry named `foo.c4m` over canonical c4m → walk edge, references
  protected; same name over binary bytes → read once, leaf, counted in the
  "description-class classified leaf" narration when journal-claimed.
- V-17 directory named `foo.c4m/` → rule order: directory wins.
- V-18 legacy directory-sequence fold `shot_[001-100]/` → members
  walk-class; legacy symlink-mode fold → no edge; list object strays.
- V-24 encoder fixture: capture the shipped canonical encoder's actual
  final-LF and blank-line emission as the leniency fixture; assert the §5.3
  superset covers it exactly (gates V-10/V-11).

Adoption and the ~100× pin:
- V-19 adopted list containing a member whose bytes ARE a stored
  description → member stays leaf; its children are NOT in closure via
  adoption; ARE in closure when independently journal-rooted.
- V-20 overlap case end-to-end (§5.6 invalidator 2): adopted description A
  references C; C in journaled root R's closure; trash+empty R → C warranted
  and swept; A's bytes intact; documented behavior, not surprise.
- V-21 binary object embedding valid 90-byte c4 tokens → leaf arrival:
  never opened, tokens never edges.
- V-22 performance assertion: synthetic adopted media store (large leaves,
  small descriptions); instrument reads; assert the walk opens ZERO
  leaf-arrived objects, reads each walked object exactly once per sweep, and
  total bytes read ≤ Σ sizes of description- and ID-list-class objects.

Aggregates:
- V-23 aggregate trash over a 10k-member list of snapshot roots → fold shows
  every member trashed at that line's position; one later keep rescues one
  member; empty materializes the residual, drops it, and ACTUALLY FREES the
  un-rescued members' bytes (asserted); the rescued root's closure intact;
  original and residual list objects survive the empty (substrate); the
  rescued member's effective verdict is keep and NO drop expansion ever
  covers it; per-root falsifiability answers for a dropped and a rescued
  member.
- V-25 aggregate claims as members → refused at produce time; a hand-forged
  record with one anyway → terminal object-level target, single-level
  distribution, no recursion.
- V-26 forged condemning line on an unknown subject → inert, reported,
  warrant unchanged; forged aggregate naming an interior of a retained root
  → subtraction protects it.
- V-27 aggregate scrub → a freed shared member gets scrub disposal
  (stronger-promise-wins), verified through vacate-name ordering.
- V-32 residual crash windows: kill between residual ingest and drop append
  → re-run reuses the content-addressed residual claim, appends the drop,
  converges; a stranded verdict-less residual claim pins only its own small
  object.
- V-34 keep-after-drop: `retain keep` on an effectively-dropped root refused
  with the re-ingest cure; gc never wedges on a rescue-too-late.
- V-35 zero-stray adopt on a virgin store → empty list object present,
  `adopted` claim journaled, marker keep line appended; `c4 gc` runs.
- V-38 aggregate-identity forgery: a user FILE named `adopted`/`condemned`
  (origin present) and a user blob whose bytes are exactly N bare catalog-root
  IDs → both are ordinary direct subjects; no distribution, no fan-out.

Safety machinery:
- V-28 rehash mismatch in the KEEP walk → refuse, zero unlinks, zero record
  appends; rehash mismatch on a drop-only object → marked edgeless, counted,
  sweep completes.
- V-29 absent walk-class child in the drop walk → marked, sweep completes,
  one narration count; absent under a KEEP root → refuse.
- V-30 crashed-sweep replay: kill after drop append at each unlink pass
  boundary → re-run appends nothing, prints nothing, completes staging,
  converges to a survivor SUPERSET whose difference is description-skeleton
  strays only; no kept object ever lost; dropped closure recomputable at
  every intermediate state.
- V-31 determinism harness: one store fixture through both implementations
  and repeated runs at different concurrency → byte-identical
  keep/drop/warrant/stray outputs, byte-identical fold.
- V-33 kind-widening: same fixture with leaf-before-walk and
  walk-before-leaf arrival orders → identical sets.
- V-36 scrub-class recovery: scrub root, empty, kill (a) after vacate-rename
  before overwrite, (b) after drop append before any unlink → re-run
  completes staging first, scrubs (never plain-unlinks) every object owed
  scrub by record history, exits 0 only when staging is empty.
- V-37 drop re-entry: trash two roots sharing objects; empty one; the shared
  objects survive (bill said so); empty after condemning the second → shared
  objects freed; no duplicate drop lines anywhere.

---

## 12. Implementation order

gc lands AFTER the v8 build (`feature/snapshot-loop-v8`; v8 build order
steps 1–6 are the prerequisite — journal with scan-start timestamps, print
barrier, restore, machine-output contract). The sweep-lock SPEC is v8
Amendment 3(c); the claim paths are already written lock-ready and the lock
itself lands with gc. Within gc:

1. Retention record reader/writer (grammar §1, durability, torn-tail,
   append lock) + `c4 retain` direct verdicts + fold view + explain topic.
2. V-24 encoder fixture FIRST (it gates the §5.3 leniency set), then the
   walk library against conformance vectors V-1..V-22, V-33 (two
   implementations or one implementation + an independent oracle walker).
3. `c4 retain adopt` (gc-A1 `adopted` claims) + the stdin aggregate producer
   (gc-A1 `condemned` claims) + distribution in the fold (V-23, V-25, V-26,
   V-35, V-38).
4. `c4 gc` dry run (warrant + bill) under the shared lock.
5. `c4 gc --empty-trash`: exclusive lock, staging recovery, residuals, drop
   append/print, three-pass unlink, scrub disposal (V-27..V-37).
6. `c4 split` archive ingest + `c4:split` keep pin + record-layer catalog
   (V-9).
7. Man pages retain(1)/gc(1) + SCRIPTING blocks per §8; interim-mitigation
   and mixed-fleet NOTES.

Build-note handoffs to the v8 branch: which ID-list shape the v8 encoder
WRITES (the walk accepts both; SPECIFICATION.md's self-verification sentence
holds only for the concatenation form — reconcile there); the T1-class
benchmark of fold cost at 300k-member aggregates runs before any
bill-granularity tuning.

Man-page placement (content pinned here; prose freedom to the surface
writer): retain(1) — SYNOPSIS (modes incl. stdin-aggregate form), the §1
record grammar verbatim, VERDICTS (four words; drop refused with why), FOLD,
ADOPT, DURABILITY (printed-means-durable), AGGREGATES (one act, one line;
per-member rescue; substrate permanence), NOTES (verdicts attach to roots;
the record never splits; actor grammar; dropped-subject refusal + re-ingest
cure). gc(1) — SYNOPSIS (two invocations), THE TWO ACTS, WARRANT, BILL, THE
EMPTY (record leads, bytes follow; staging step 0; exit attests the sweep),
COST, SCRUB (full per-platform honesty sentence), UNACCOUNTED, LOCKS + NAS
caveat, MIXED FLEETS, EXITS, NOTES (interim mitigation; pre-gc split
archives).

## 13. Surface accounting (honest)

2 verbs (retain, gc); 1 long-form flag (--empty-trash); 1 record file
(<store>/retain) with 1 pinned 4-field grammar and a 4-word vocabulary; 1
transient staging directory (<store>/scrub/); 1 ledgered journal name-value
amendment (gc-A1: adopted, condemned — entry grammar untouched); 3 reserved
tool actors (c4:adopt, c4:gc, c4:split); 1 explain topic; 0 new flags on
retain (stdin `-` is the v7 argument convention); 0 dependencies; 0 indexes
on any destructive path; stdout byte-pure everywhere; exit 2 deliberately
unassigned.