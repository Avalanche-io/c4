# Crucible brief v2: the snapshot–store–restore loop

Date: 2026-07-10. Status: fresh run. This brief supersedes the 2026-07-08
brief (archived with its five rounds in `superseded-2026-07-10/`) because
the world changed under it: suite v1.0.15 shipped, the content-projection
decision was made, and the first external consumer of the CLI produced
hard evidence about the surface. This brief is the fixed measuring stick
for the whole run — it does not change between rounds.

## Carry-forward (do not burn prior work)

Draft v6 (`superseded-2026-07-10/round-5/draft-v6.md` and
`surface-v6.md`) plus the round ledgers are **canonical input**: eleven
verified repairs stand, and every `REJECTED — <reason>` entry stays
rejected unless a *named item of new context below* reopens it. Designers
revise v6 against this brief; they do not restart from zero. v6's own
round-5 additions (unspellable-name class, fold readability precondition,
argument-root OS path resolution, range-order pin) have never been
exercised — they are open text, not settled pins.

## Design question

Design c4's snapshot–store–restore loop as ONE coherent surface:

1. **Identity** — what identity/identities does a tree and its
   description have, and how are they named to users?
2. **Self-description** — how does a snapshot describe itself so the
   store ALONE is sufficient to recover everything, including the
   description?
3. **Durability** — what write-durability semantics do ingestion and
   materialization have, and what may the user assume on power loss —
   stated honestly per platform (see Windows physics below)?
4. **Destructive-reconcile safety** — reconciling a directory destroys
   its prior state; what is the default protection and the undo story —
   including undoing the restore itself?
5. **The machine-output contract** — what exactly does each verb print,
   such that a script or LLM agent can extract THE identity it needs in
   one line, byte-pure, without regex-scraping? This is frozen product
   surface, not ergonomics (see founding complaint 3).

Unconstrained by any in-flight implementation. Designers must not be
told what is currently built beyond the frozen facts below.

## Frozen facts (constraints, not design)

- The c4m format grammar is FROZEN: plain-text, entry-only stream; no
  headers, no directives; null fields render as `-` and are first-class;
  directory C4 ID = ID of the one-level canonical listing of direct
  children (Merkle); patch chains are concatenated diff sections
  (resolve/log/split exist); sequences fold.
- C4 IDs are SMPTE ST 2114: SHA-512, base58, 90 chars. Fixed.
- **DECIDED 2026-07-09 (Joshua, permanent): the content-only projection
  is full-null** — mode and timestamp nulled entirely (no exec-bit
  preservation); names, sizes, symlink targets, child content IDs real.
  Content IDs are equal iff contents are byte-identical, on any machine,
  clock, or umask. An ID does not self-describe its projection;
  comparisons are like-mode; the projection travels out of band.
- Shipped surface (suite v1.0.15, 2026-07-09): verbs are id, cat, diff,
  patch, merge, log, split, paths, intersect, explain, version. **There
  is no gc** — it was built and withdrawn before release (explicit-roots
  deletion is unsafe without a complete root catalog); no deletion verb
  exists until something provides that catalog.
- Durability as shipped: **one default, zero flags** — batch barrier
  (atomic per-file writes with cheap flushes; one device-cache flush at
  command completion). `--no-fsync`/`--durable` were removed before
  release. Reconcile forms capture the destination's pre-state durably
  before the first destructive operation and print a verbatim revert
  command. Snapshots self-capture (manifest text + root record stored;
  `stored: <id>` on stderr after the barrier).
- Measured physics (APFS/M-series, 20k files / 105 MB): ingest with
  store ~4.3 s; materialize ~4.4 s; no-op re-apply ~0.3 s; per-file
  F_FULLFSYNC ≈ 6.4 ms; plain fsync(2) ≈ 126 µs/file; one directory-fd
  F_FULLFSYNC barrier ≈ 5.4 ms.
- **Windows physics**: a directory handle cannot be flushed
  (FlushFileBuffers on a directory is refused), so there is no
  device-barrier equivalent; per-file `Sync` IS the durability, and NTFS
  journals rename metadata. Any durability sentence the design prints
  must be true on Windows too.
- Zero dependencies in the core module. Zero format changes available.
- **DECIDED 2026-07-10 (Joshua, security/integrity posture): re-scan
  trust.** Two invariants, deliberately different in strength:
  (1) **Store integrity is absolute** — bytes never enter the store
  without being hashed at ingest, and store reads re-verify. Nothing
  may weaken this. (2) **Re-scan of a previously-snapshotted tree is
  metadata-trusted by default** — full re-hash at terabyte media scale
  is not a viable default. A file is re-hashed iff it is new, its size
  changed, its mtime changed, or it is *racy* (potentially modified
  concurrently with the prior scan — the racy rule must be total, with
  no invented constants); otherwise the prior snapshot's recorded ID is
  reused. Reuse references bytes already verified in the store; it
  extends trust only to the *description* of the live tree. The
  accepted, documented risk: a same-size byte change with a restored
  mtime (deliberate obfuscation) is invisible to the default re-scan —
  the design states this plainly as a posture, not a bug. Bitrot or
  malice against live copies is defended by the verified store and by
  **forced verification on demand** (full re-hash / re-ingest of the
  whole tree or named paths — this MAY spend the second budget flag).
  First-ever snapshot of a tree is definitionally a full scan.
  This decision REOPENS, by name, the round-1 ledger rejection of
  metadata-trusted re-scans: the round-1 objections (racy window,
  invented time constant, id-reads-store, assertion-vs-verification)
  are constraints the mechanism must now answer — not grounds for
  rejecting the posture. Note the guide for reuse can be the prior
  snapshot's *description* (a file), which need not violate any
  id-never-reads-store pin.

## The three founding complaints (verbatim intent)

1. "It feels unnatural to create a c4m that captures all the files into
   the store but doesn't then capture the c4m output. If the c4m file is
   lost, the content still exists in the store but the metadata is lost."
2. "I patched over a directory because I got the usage wrong and it
   quietly destroyed the prior state. We built flags for the backup
   solution — it needs to be the default."
3. New, from the first external consumer (2026-07-10 resolver lab): a
   friendly program needed THE identity of a c4m file; the CLI printed
   the file's normalized entries; the program regex-scraped stdout and
   silently shipped the *last leaf's* ID as the project identity on a
   public dashboard. The same consumer probed store membership by
   running `cat` and checking the exit code, and hand-rolled a session
   journal of what it had stored. Cold consumers will script this
   surface; what the verbs print is the API.

## Incumbents it sits beside (updated 2026-07-10)

git (per-repo, committed-state only); jj (replaces the VCS; IS the repo
rather than protecting `.git` as data); APFS/ZFS snapshots
(volume-scoped, OS-locked, expiring, ID-less, not per-prompt drivable);
harness checkpoints — **note: GitHub Copilot CLI now documents a
git-based whole-workspace rewind covering manual edits, shell effects,
and untracked files** (documentation claim, hands-on bake-off pending).
The surviving structural gaps an incumbent cannot close: durability of
anything printed (`kill -9` survivable), `.git` itself as recoverable
data, undo of the restore, portability of history to another machine,
independence from any one harness or session. The design should own
exactly those gaps. LLM agents are first-class users driving the CLI
cold from --help.

## Success tasks (fixed; a cold user + only the user-facing surface)

- **T1 recover-from-store-alone**: Snapshot a working tree with content
  storage; `kill -9` a later snapshot mid-write. Delete the tree AND
  every file outside the store — including `.git` — and every note of
  what the IDs were except the last ID the tool *printed*. Recover the
  tree byte-exact from the store alone. (If it printed, you can get it
  back.)
- **T2 undo-the-accident, then undo-the-undo**: Reconcile the WRONG
  directory to a snapshot. Restore that directory byte-exact. Then
  decide the restore itself was wrong and undo it too. State what the
  retention story is — what, if anything, ever expires.
- **T3 same-content?**: Two checkouts of the same project on different
  machines (different mtimes, umask). Produce one identifier on each
  that answers "byte-identical content?" by string equality.
- **T4 fast-and-safe, per-prompt**: Snapshot ~20,000 files, then snapshot
  again after touching 3 files (a gitignored-heavy tree with
  node_modules present). Say how long each takes, what is skipped and
  why it is safe to skip it, what exclusions applied and where the user
  sees them, and precisely what is guaranteed if power fails mid-command
  or just after it returns — on macOS and on Windows.
- **T5 read-the-store, scripted**: Given one printed ID and the store
  only: list what the snapshot contains, extract one named file, and — 
  in a shell script with no regex over prose — capture THE snapshot ID
  of a fresh snapshot into a variable and test whether an arbitrary ID
  is present in the store.

## Surface budget

- ≤ 1 new verb; ≤ 2 new flags total across all verbs
- ≤ 2 user-visible identity concepts, each with a name and one sentence
- 0 grammar changes; 0 new dependencies; stdout stays byte-pure data

## Non-goals

Networking/daemon, multi-repo binding, deletion/GC policy (the journal
must make a future root catalog *possible*, but designing collection is
out of scope), plugin/hook UX (the plugin consumes this surface, it does
not define it), secrets-exclusion *policy* (Joshua's open decision D5 —
the design specifies the mechanism and where exclusions are reported,
not the default pattern list), and any implementation — the deliverable
is a design document.
