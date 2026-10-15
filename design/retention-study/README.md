# Retention study — how a c4 store decides what bytes may be removed

Status: **study record, 2026-10-03. No decision is made here.** Seventeen
agents (3 grounding, 5 proposers, 5 adversaries, 3 judges, 1 critic), all
completed; every claim marked "run" was executed on c4 1.0.18 or a build of
master `d1d1b0a`. Full structured output: `result.json` in this directory.
Experiment artifacts were left under `/tmp/retention-study/` (volatile).

## Why it was run

Joshua, 2026-10-03, on why v1.1 "smells of wrong fundamentals":

> I think there is a core problem to be solved at [layer] 0 or 1 having to do
> with data destruction. How do any files ever get removed from storage? How
> would we choose?

> We have powerful new features that only C4 enables, but we have people who
> have expectations about storage reliability that pushes us to either mimic
> a typical block storage filesystem, or retain every version of everything
> forever.

The lead's hypothesis under test: *retention is reachability from the
descriptions a store has promised to keep; holding one description is
ordinary filesystem behavior, holding all is full history, it is one dial;
the promise list is an ordinary named c4m in the store; dropping an entry is
the only way space is reclaimed; destruction by ID is a separate act.*

## Result in one paragraph

All five positions were damaged or refuted by their adversaries (one
"damaged", four "refuted"); no judge scored any position above 3 of 10. All
three judges ruled the hypothesis **partly right**: the reachability rule
itself was never refuted, "one dial" is not fundamental and should not be
the headline, and three of its clauses are false as stated. The judges agree
on the standard-level contract and on a list of fixes that are right under
any design; they disagree on the mechanism for reclaiming space, and each
proposed a different hybrid.

## What every judge and adversary agreed on

1. **The store contract (L0) carries no retention concept.** put, get, has;
   optionally list and remove(ID); and three sentences: a store never
   removes an object on its own initiative; bytes served under an ID hash to
   it; absence is an ordinary answer and never edits a description. This is
   what a vendor implements, and it matches every content-addressed standard
   that was adopted (Venti, REAPI CAS, OCI distribution, S3): none specifies
   a collector.
2. **reach(D) is defined by the c4m grammar, not by the store**: every ID on
   an entry; descend only where the parent entry says directory, folded
   sequence, or chain base; a missing listing is an error; a missing leaf is
   only absent.
3. **Retention is reachability from descriptions the store itself can
   enumerate.** Every loss the adversaries produced came from roots the
   store could not see, or from how a promise is withdrawn; none came from
   the reachability rule.
4. **Promises must be made automatically**, as a side effect of the act
   that prints the ID. A hand-curated list is the gc withdrawn on 2026-07-09.
5. **Removal must be a positive warrant** (bytes reached by something
   explicitly dropped, minus bytes reached by anything kept), never a
   negative sweep, and it must fail closed on any unreadable root or
   listing. An empty or lost root list must release nothing (restic #4612
   deleted a 128 GiB repository on exactly this).
6. **Destruction by ID is a separate act** from reclaiming space, and
   promises only "this store no longer serves these bytes".
7. **"Unaccounted bytes are never removed" does not protect other tools.**
   Three adversaries ran it independently: in a shared store, dropping a
   root releases bytes that c4git, imgbin or a 1.0.x snapshot alone was
   relying on, because the IDs coincide. The honest rule is "only a held
   entry keeps bytes", or one store per purpose.
8. **The accepted gc design (`gc-requirements.md`, retain file, verdicts)
   cannot be built against the v9 journal**, which carries neither root
   kind nor names. No position needed its vocabulary back.

## Fixes that are right under any design (found by runs)

- `c4 cat -r` exits 0 with no warning when a directory listing is absent,
  so anything built on it under-marks (the restic failure class).
- With several stores configured, master prints claims that are never
  journaled, and ingest skips any object present in *any* listed store, so a
  printed claim may reach bytes that are not in the first store.
- `c4 id -s` can print an ID when nothing was stored (unavailable store).
- Restore is all-or-nothing on one absent leaf, on both binaries; "a
  description with a destroyed leaf is partial knowledge" is true for `cat`
  and `diff` and false for restore.
- Master: `-S` does not store the fold's ID list; a `.c4m` file inside a
  tree yields a printed ID that cannot be restored (#45, #43, fixed on
  1.0.x, not yet on master).
- The help sentence "nothing a journal line reaches will ever be
  collectable" must not freeze at v1.1.0.
- **Released c4git 1.0.18: `c4git gc --force` treats "not referenced by this
  repo" as garbage and deleted another tool's snapshot in a shared store**
  (run by the use-case agent). c4git also prefers the globally configured
  store over the repo-local one named in `.c4git.yaml`.

## Where the judges split: three hybrids

| Judge lens | Hybrid | Mechanism for getting space back |
|---|---|---|
| Does it feel fundamental | **Holdings chain.** `<store>/holdings.c4m` is one c4m patch chain replacing the journal; a claim appends an entry, a drop appends a `c4 diff` section; reclaim is a second explicit act computed from removal sections. | In place, from positive removal lines. No trash directory: "dropped, not yet reclaimed" is a state in the chain. |
| Would real people trust it | **Journal + holdings + guarded trash, opt-in.** v9 journal unchanged as the automatic promise record; a readable holdings listing for withdrawals; a trash store c4 itself guards; policies scoped to stores one product owns. **v1.1 ships no reclaim verb.** | In place, opt-in per store, via `c4 drop` then a c4 verb that empties trash. Default is never. |
| What it costs | **Generations.** The store is a bag, the journal is the promise record, and c4 never unlinks in place: fill the kept roots into a fresh store, switch, read the residue report, retire the old directory. Verbs: `has`, `fill`, `destroy`. | Only by retiring a whole store. No lock, lease, compare-and-swap, tombstone or trash timing in any contract. |

The critic then ran checks against each: the chain form costs about 146 ms
per section at 10,000 entries (200 drops: 29 s against 0.08 s flat) and a
stale-base append leaves the file refused by strict readers; the journal
fold-by-rename has a race on source read (`Journal.Append` opens before it
locks); generations were run only with hard links in a simulated flat
layout, and a hard-linked old store is not a second copy.

## What the critic says the study missed

- **Durability was not studied.** "Storage reliability" also means bad
  sectors and media loss. Verify-and-heal across stores is a c4-only
  capability and no hybrid has a verb for it.
- **c4d already implements the opposite rule** (source read, not run): a
  timer-driven negative sweep with immediate unlink and silent skip of
  unparseable descriptions. It sits where a vendor would.
- **Whether the store holds the only copy** is not recorded by any design.
  Measured: of the 200 largest objects in the default store (55% of bytes),
  121 (8.27 GiB) have a byte-identical ordinary file elsewhere on the same
  Mac; at least 25% of the 32.9 GiB store is a second copy today.
- **Untried options**: release only what a claim itself added to the store
  (ownership by creation); freshen an object on a dedup hit (git's answer
  to the writer/collector race); one promise list per promiser (ZFS holds).
- **Shrink growth instead of choosing what to delete.** In the snapshot
  loop the growth is listings, not content (735 KB of new listing objects
  per snapshot against 2.3 KB of leaves on a 10,000-file tree), because a
  whole one-level listing is rewritten per change. Nobody tested snapshots
  stored as patch sections against the previous one. If growth falls to KB
  per snapshot, "nothing expires" may be affordable and the question
  narrows to a few large leaves (1% of objects hold 77% of bytes).
- Nothing was run on an object store, NFS/SMB, Windows, the real sharded
  store layout, or under real crashes; no port other than Go was executed.

## Measurements worth keeping

- Default store (`~/.c4/store`): 194,127 objects, 32.9 GiB, no journal.
  Largest 1% of objects hold 77.4% of bytes. 13,851 objects look like c4m
  (0.07 GiB). 16,243 objects (6.53 GiB) are named by no in-store
  description; about 950 (2.5 GiB) by none found anywhere. In-store
  descriptions name 107,945 IDs the store does not hold: descriptions
  pointing at absent bytes are already the normal case.
- Ash mirror store: 64,433 objects, 12.13 GiB; 943 objects (0.79 GiB)
  superseded after three pushes, 639 of them listing objects.
- Closure walk: about 100k entries/s; 194k objects opened in 29 s. Deciding
  is cheap; the costs are in locks, trash and verification reads.

## Discussion that ran alongside (Joshua, 2026-10-03; not rulings unless marked)

- **Ruling**: every `-s` invocation ends with its claim line (recorded in
  `v11-surface-review.md`).
- The goal of c4: "a universal ad hoc system that places the threads into
  the world for others to tie together in any way they need even after the
  fact... structure discoverability on a massive scale but without a massive
  infrastructure adoption obligation."
- "Retention, undo, file movers, can all be after market c4 enjoyers."
  (Leaning, stated before the study returned.)
- A strict core must not deny "the application layer the choice of
  performance over correctness". Users are used to cheap; identification
  has to happen "at clever moments when the data has to move through memory
  anyway"; skipping identification on matching metadata and not recording
  mod time are both legitimate.
- **"Files are the content, directories are the metadata."**
- "The c4m format supports the concept of a stream of directory updates...
  directories are never discrete things, they are always streams, that may
  occasionally have only a single recorded state." Run on 1.0.18: a chain
  of names-only → metadata → content IDs → change set resolves
  byte-identical to a full scan; a one-file change is 1.2 KB as a change
  set against 2.94 MB as a full description, while the store today adds
  about 3.6 MB of listing objects for that change. This is the same
  observation the critic reached independently.
- Discontinuities between directory streams can be read as branches, and
  directories are softly linked by shared content with varying strength.

## Incident

One study agent wrote two 300,000-byte random test objects into the default
store at 15:09 on 2026-10-03 (c4git 1.0.18 used the global store instead of
the repo-local one). Nothing else under `~/.c4` changed. They are
`~/.c4/store/c4/4E/c44eGrLa3D…` and `~/.c4/store/c4/4U/c44UaEHH6T…`.

## Direction from Joshua (2026-10-14): cleanup policies are required for v1.1; removal as reduced durability

Joshua, verbatim in substance: "We do need cleanup policies for v1.1 as we
are already experiencing an issue with that in current development
projects." And: we likely do not want attic or trash folders replicated
across volumes; instead a system that applies rules to give "logical
compliance to the concept of removal." Example: when an automatic choice
was made between two versions but a human could plausibly have decided
differently, the unchosen version should not be mirrored to all storage,
but should not be lost until an explicit choice or policy removes it. So:
migrate it to a zone where the choice can be made (if it is not already
there) and reduce its durability. Concretely, for a fork created on a
remote: both branches are retained only on the local machine and the
unchosen one is removed from the remote. If the user reverts locally (the
files are at hand to evaluate), the files are re-propagated; if the user
confirms, there is one last place for the removal to be applied, and the
removal becomes complete and certifiable.

Reading against the study:
- This is "trash as a state, not a place" (judge-fundamental's dissent),
  with the state expressed as **how many stores hold the bytes**: full
  replication → one decision zone → none. Removal is a ladder of durability
  reductions, not a single act.
- It uses what only c4 enables: many stores, descriptions that outlive the
  data (both branches stay described while one loses copies), and
  verification by recomputation (`c4 has` per store makes "complete" a
  checkable claim: absent from every store you control).
- It overrides the option of shipping v1.1 with no reclaim policy.
- It matches the Ash sync's attic/merge design, where the attic is the
  Mac-only zone.
- Open: who decides a zone (a store with lower declared durability, or the
  machine where the human is), how the pending choice is recorded (a c4m
  naming both branches is enough to re-propagate), and the certification
  step (removal is complete when no configured store returns the ID).
