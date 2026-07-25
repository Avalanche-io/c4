# c4m Canonical Identity — Recorded, Never Substituted

**Status: the 2026-03-25 rule is SUPERSEDED by the decision below
(2026-07-25, Joshua). The goal it served is preserved; the mechanism
is inverted.** The original text is retained in full at the end,
because a design record should show what was decided and why it
changed.

---

## The rule (current)

**C4 never transforms a file's bytes. Ever. There is no exception.**

A file inside a scanned tree is bytes, whatever its name and whatever
its content parses as. `c4 id -s` stores exactly what is on disk, so a
tree containing a `.c4m` file round-trips byte-exact through
`c4 restore`.

Canonical identity is not lost — it is expressed as a **relationship
about** the bytes rather than a **substitution of** the bytes:

```
<canonical-id> produced-from canonicalize inputs=<raw-id> class=exact
```

An ordinary `exact` derivation edge (`derivation-records.md`, Layer 1).
Canonicalization is a deterministic, pure function of bytes, so this
edge is falsifiable in the ordinary way: run the process and the output
either hashes to `<canonical-id>` or the record is proven false.

## The decision: record the map — existence checking is a core requirement

An earlier draft of this section recommended computing the edge lazily
and recording nothing. **That was wrong**, and the requirement that
corrects it is Joshua's (2026-07-25):

> "What I care about is that two identical c4m files match IDs
> regardless of the on-disk format. I should be able to take the c4m of
> a tree, and know **authoritatively** if I already have a stored
> version of that tree… we can't have entire directory definitions be
> marked as differing when they are not different canonically."

Existence checking cannot be answered on demand, because there is
nothing to answer *against* — a lazy scheme can only tell you what an
object you already hold canonicalizes to, never whether the store
already contains that tree. **Authoritative existence requires a
recorded map.** So the map is recorded; what stays lazy is only the
materialization of forms nobody has asked for.

### The map is projection-keyed, not single-valued

A second requirement follows from the same conversation: it must be
possible to match trees **by structure alone**, with metadata nulled —
*"the case being checked is: first make metadata nil, then identify,
then check against an existing table entry."*

So the map is keyed by **projection**, using the ladder that already
exists on `c4 id -m`:

| Projection | Answers | Use |
|---|---|---|
| `f` full / snapshot | "have I stored this exact observation?" | reflog, restore targets |
| `c` content | "have I stored these exact bytes, arranged this way?" | dedup, delivery equality |
| `s` structure (names only, metadata nil) | "have I stored a tree shaped like this?" | structure matching |

The critical property: **a lookup at projection P must never report
"different" for two trees that are identical at P.** That is the
false-negative failure this map exists to eliminate.

### Every entry is an `exact` derivation edge

Projections are deterministic pure functions of a description, so the
map needs no new record kind — each entry is the same
`produced-from` edge, with a different process:

```
<canonical-id>  produced-from canonicalize       inputs=<raw-id>        class=exact
<content-id>    produced-from project-content    inputs=<full-id>       class=exact
<structure-id>  produced-from project-structure  inputs=<full-id>       class=exact
```

One predicate, three processes, all stdlib-only and all falsifiable by
re-running them. Queried **forward** (given a projected ID, which
stored objects produce it?) this is exactly the "map" the requirement
asks for; `derivation-records.md` already describes that forward query.

### When entries are written

- **At snapshot time (`c4 id -s`) — always, and nearly free.** The scan
  already holds the full description in memory; the content and
  structure projections are pure functions of it, requiring no
  re-reading of any file. Recording all three costs a hash per
  projection and no I/O. This is the main path, and it makes existence
  authoritative for everything ever snapshotted.
- **When a c4m file is ingested as a file — record the canonicalize
  edge.** This costs one parse, bounded by phase-1 detection. The
  file's bytes are still stored raw and unaltered; the edge is a
  statement *about* them.
- **Never by transforming anything.** Recording the map is the whole
  mechanism; substitution remains forbidden.

This satisfies the ergonomic case directly: a pretty-printed c4m may
sit in a project directory as an ordinary file, be stored byte-exact,
and still be known to the store as a description of a tree it already
has.

### Folding — RULED (Joshua, 2026-07-25)

> "Folded frame ranges are the same as expanded ranges but with a
> single metadata value for the set… Folded ranges deliberately discard
> individual frame metadata; expanding a compacted range expression
> does not restore anything — in fact it likely communicates
> **incorrect** metadata for individual frames. So I say we use what is
> input (folded or not) as the authority once canonicalized. It is
> valid to have two different c4m files for the same frame sequence due
> to retention or discard of the metadata."

**The ruling: the input form is authoritative. Never expand a folded
entry in order to compare.** Folding is a deliberate, lossy editorial
choice about how much per-member detail a description retains, and a
folded and an unfolded description of one sequence are **two valid,
different descriptions** — not two spellings of one.

The asymmetry that makes this correct: **folding discards; expanding
invents.** Expansion would hand every frame the set's single metadata
value, asserting per-frame facts that were never observed. A system
whose rule is "verification over assertion" must not manufacture
metadata to make a comparison succeed.

#### One mechanical correction to the premise

The ruling opens by treating folding as identity-neutral under a nil
projection ("they will expand to identical values… just a nicely more
compact format"). That holds for **names** and **member content IDs**,
but not for **sizes** — verified 2026-07-25:

```
content projection, unfolded (20 lines):
  - - 3  frame.0001.exr        c42BQmxv…
  - - 3  frame.0002.exr        c435iLjR…      … per-member sizes

content projection, folded (1 line):
  - - 60 frame.[0001-0020].exr c44kEfNj…      … the SUM, and the ID
                                                of the member ID-list
```

`help.go` states it: a folded entry's *"size is the sum of member sizes
read and hashed."* Member names are recoverable from the range
expression and member content IDs are recoverable in order from the
ID-list — but **per-member sizes are not recoverable from a folded
entry.** So folding remains lossy even with metadata nulled, and
`c4 id -q -m c` legitimately differs between the two forms today
(measured: `c44saa79UCrELV…` vs `c433C2Y6rJKHHR…`).

This does not weaken the ruling — it **strengthens** it. Folded and
unfolded really are different descriptions at every projection, so
treating the input as authoritative is the only rule that does not
require inventing data.

#### Consequence for the map (binding)

**Folding is part of the map key**, exactly as projection is. An
existence check compares like with like, scanned alike — same
projection *and* same folding choice. A folded lookup against an
unfolded entry is a legitimate miss, not a false negative, because the
two descriptions genuinely differ.

This overrides the earlier recommendation in
`v11-surface-review.md` Finding 1 to "normalize folding away." That
recommendation assumed expansion was lossless; it is not.

#### Still open

Whether the map should additionally record a *relationship* between the
two forms — a `fold` / `unfold` edge — so a caller who holds one can
discover the other. It cannot be an `exact` derivation in the
unfold direction (the process cannot reproduce per-member sizes), so
the honest class is `validated` at best, and it may not be worth a
record at all. **Not release-blocking; decide when the map is built.**

### Exclusions (open)

Exclusions produce a genuinely different tree, so they deserve a
different entry — but a lookup must not let "I have this tree" silently
mean "I have this tree minus the logs." The scan scope needs to be
recoverable from the entry, which today's rule ("the snapshot records
what was scanned, never what was omitted") does not provide.

### What this settles elsewhere

This map is also the answer to `v11-surface-review.md` Finding 1. A
bare directory ID cannot say which projection or scan choices produced
it — but the **store can**, because the map records exactly that. The
ID stays a pure hash; the disambiguation lives beside it, as a record,
which is the sidecar doctrine applied to identity itself.

## Behavior

**Identification of a c4m ARGUMENT — canonicalize (unchanged).**

```
$ c4 id -q pretty.c4m      # a description, being asked about
# parses as a description, resolves, ID is of the canonical form
```

This is the path that carries the original goal, and it already works.
A `.c4m` argument is dispatched as a description (`id.go:208`) before
any file-scanning heuristic is reached.

**A file INSIDE a scanned tree — raw bytes, always.**

```
$ c4 id -s tree/           # tree/guide.c4m is data, not a description
# stores tree/guide.c4m's exact bytes; restore returns them exactly
```

This is what `help.go` already promises — *"files inside scanned trees
are ALWAYS bytes, whatever their name"* — and what c4py, c4ts, libc4
and c4-mcp already do. The Go CLI is the outlier; see
[#43](https://github.com/Avalanche-io/c4/issues/43).

**Retrieval by canonical ID — Layer 2, no special case.**

If a canonical ID is requested and no object is stored under it, the
resolution path may consult a derivation record, run the `exact` route,
**hash the result against the requested ID**, and serve it. A lying
record is caught by identity. Core executes only built-in, stdlib-only
processes; c4m canonicalization qualifies.

**`sha512sum` no longer diverges.** The stored object is the file. The
two agree, and the documented divergence disappears with the exception
that caused it.

## Constraints

- **A canonical ID never appears in a c4m entry field.** The grammar is
  frozen and an entry line describes bytes; a canonical identity is a
  fact *about* an object, so it lives on a record line. Same rule
  already adopted for assigned (`nom:`) IDs.
- **No index on any destructive path.** If an index is ever built to
  answer the reverse query, it stays off gc/retention paths
  (`store-index-and-links.md`, `gc-requirements.md`).
- **Scope, precisely:** canonicalize a c4m *argument* (a description
  the caller is asking about); never a *file* (data the caller is
  storing). One rule was applied to both questions, and that is what
  produced the defect.

## Why the original rule changed

The 2026-03-25 rule was correct about descriptions and wrong about
files, and it could not have known: **it predates whole-tree restore.**
It anticipated that a pretty-printed c4m would "change" after
store/restore and called that expected. What it could not anticipate is
that restore would later verify *by recomputation* — so on any tree
containing a `.c4m` file, restore now fails (exit 3) and the original
bytes are unrecoverable, because they were never stored.

The three statements that appeared to conflict are each correct within
their own scope, and this decision scopes rather than overrules them:

| Source | Says | Scope where it is right |
|---|---|---|
| `c4m-canonical-storage.md` (2026-03-25) | canonicalize c4m content | **descriptions** — preserved via the argument path |
| `snapshot-loop/round-1/draft-v7.md:66` | *"no sniffing anywhere"* | **files inside trees** |
| `help.go:141-142` (shipped) | *"files inside scanned trees are ALWAYS bytes"* | **files inside trees** |

## Implementation

- **Now (release-blocking):** delete the content heuristic from the
  file-storing path — `looksLikeC4m` and the `.c4m`-suffix branch in
  `storeFileEntry`. This is a deletion in Go only; it makes the Go CLI
  agree with the four implementations that are already raw-bytes-only.
  See [#43](https://github.com/Avalanche-io/c4/issues/43) for the
  worker-pool and exit-code amendments.
- **Also amend:** `c4m-detection.md`'s "When to Apply" list, which
  currently names `c4 id -s <dir>` (for each file in the directory) —
  detection belongs to the argument/description path only.
- **Next (the map):** record the three projection edges at snapshot
  time and the canonicalize edge at c4m ingest. Needs, in order: a
  decision on folding normalization (open question 1); the record line
  shape, which is `produced-from` and already reserved; and a
  read-side lookup that stays **off every destructive path**
  (`store-index-and-links.md`, `gc-requirements.md`).

**Sequencing note.** The deletion and the map are separable and should
not be bundled. The deletion stops active data loss and is a pure
subtraction; the map adds a capability and needs the folding question
answered first. Ship the deletion immediately; land the map when its
open questions close. In the interval nothing regresses — today's
transformation does not provide authoritative existence checking
either, it merely destroys bytes while failing to.

---

# Original document (2026-03-25) — superseded, retained as history

## Rule

When C4 encounters content that parses as a valid c4m file — in any
context (identification, storage, or retrieval) — it MUST canonicalize
the content before computing the ID and before storing.

This is the one exception to the general rule that C4 identifies raw
bytes.

## Why

Two c4m files that describe the same filesystem should have the same C4
ID, regardless of formatting. A pretty-printed c4m and its canonical
equivalent describe the same thing. They should have the same identity.

Without this rule, `c4 id pretty.c4m` and `c4 id canonical.c4m` would
produce different IDs for files with identical logical content. This
defeats the purpose of content identity.

*(This reasoning is still endorsed. It is the goal the current rule
preserves — by recording the relationship instead of replacing the
bytes.)*

## Behavior (as originally specified)

- **Identification:** `c4 pretty-project.c4m` → parse, canonicalize,
  hash the canonical form.
- **Storage:** the canonical form is stored; pretty formatting is lost.
- **Retrieval:** `c4 cat <c4m-id>` returns canonical c4m.
- **The sha512sum divergence:** `sha512sum project.c4m` and
  `c4 project.c4m` differ if the file is not already canonical —
  "documented and expected."
- **Non-canonical c4m as ephemeral view:** a c4m file "changes" after
  store/restore; formatting normalized; all expected.
- **Detection:** parse any file as c4m before falling back to raw
  bytes.
- **Impact:** to be implemented in all identification and storage paths
  in every language.
- **Status:** design complete, implementation pending.
