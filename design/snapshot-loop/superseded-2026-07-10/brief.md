# Crucible brief: the snapshot–store–restore loop

Date: 2026-07-08. Status: crucible run in progress. This brief is the fixed
measuring stick for the whole run — it does not change between rounds.

## Design question

Design c4's snapshot–store–restore loop as ONE coherent surface:

1. **Identity** — what identity/identities does a tree and its description
   have, and how are they named to users? (Today: directory IDs hash child
   mtimes/modes, so identical content on two machines gets different IDs;
   the spec's null fields permit a stat-free identity. A c4m's own ID is
   the ID of its canonicalized text.)
2. **Self-description** — how does a snapshot describe itself so the store
   ALONE is sufficient to recover everything, including the description?
3. **Durability** — what are the write-durability semantics of store
   ingestion and tree materialization, and what does the user get to
   assume on power loss?
4. **Destructive-reconcile safety** — reconciling a directory to a target
   state destroys its prior state; what is the default protection and the
   undo story?

Unconstrained by any current or in-flight implementation. The designers
must not be told what is currently built beyond frozen facts listed below.

## Frozen facts (constraints, not design)

- The c4m format grammar is FROZEN: plain-text, entry-only stream; no
  headers, no directives (`@` lines rejected); null fields render as `-`
  and are first-class; directory C4 ID = ID of the one-level canonical
  listing of direct children (Merkle); patch chains are concatenated diff
  sections (resolve/log/split exist); sequences fold.
- C4 IDs are SMPTE ST 2114: SHA-512, base58, 90 chars. Fixed.
- Existing verbs: id, cat, diff, patch, merge, log, split, paths,
  intersect, explain, gc. Store: content-addressed sharded directory,
  put-by-ID, dedup by construction.
- Measured physics (APFS/M-series): per-file F_FULLFSYNC ≈ 6.4 ms (20k
  files ≈ 3 min); plain fsync(2) ≈ 126 µs/file; no fsync ≈ 112 µs/file;
  one directory-fd F_FULLFSYNC barrier ≈ 5.4 ms; whole-tree APFS clonefile
  of 14k files ≈ 0.26 s.
- Zero dependencies in the core module. Zero format changes available.

## The two founding complaints (verbatim intent)

1. "It feels unnatural to create a c4m that captures all the files into
   the store but doesn't then capture the c4m output. If the c4m file is
   lost, the content still exists in the store but the metadata is lost."
2. "I patched over a directory because I got the usage wrong and it
   quietly destroyed the prior state. We built flags for the backup
   solution — it needs to be the default."

## Incumbents it sits beside

git (per-repo, committed-state only), Time Machine/backup tools (opaque,
not content-addressed), harness checkpoints (session-local, tool-blind).
LLM agents are first-class users driving the CLI cold from --help.

## Success tasks (fixed; a cold user + only the user-facing surface)

- **T1 recover-from-store-alone**: Snapshot a working tree with content
  storage. Delete the tree AND every file the snapshot produced outside
  the store. Recover the tree byte-exact using only the store.
- **T2 undo-the-accident**: Reconcile the WRONG directory to a snapshot.
  Realize the mistake. Restore that directory byte-exact.
- **T3 same-content?**: Two checkouts of the same project on different
  machines (different mtimes, umask). Produce one identifier on each that
  answers "byte-identical content?" by string equality.
- **T4 fast-and-safe**: Snapshot ~20,000 files. Say how long it takes and
  state precisely what is guaranteed if power fails mid-command or just
  after it returns.
- **T5 read-the-store**: Given one ID and the store only, list what the
  snapshot contains and extract a single named file.

## Surface budget

- ≤ 1 new verb; ≤ 2 new flags total across all verbs
- ≤ 2 user-visible identity concepts, each with a name and one sentence
- 0 grammar changes; 0 new dependencies; stdout stays byte-pure data

## Non-goals

Networking/daemon, multi-repo binding, GC policy (exists), UI, and any
implementation — the deliverable is a design document.
