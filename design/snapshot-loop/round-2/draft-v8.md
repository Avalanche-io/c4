# Draft v8 — single-author integration (build reference)

Status: v8 is draft v7 (`../round-1/draft-v7.md` + `surface-v7.md`)
carried **verbatim**, amended only by the sections below. This is a
single-author integration pass (2026-07-13) — not a crucible round —
folding in the two brief-mandated changes the killed round 2 was
carrying: the re-scan trust posture (brief v2, decided 2026-07-10) and
the three retention amendments from the accepted store-records
architecture (`../../store-records-architecture.md`). It is the
reference design for the `feature/snapshot-loop-v8` implementation
branch. The crucible's freeze bar (two consecutive clean rounds) has
NOT been met; v8 is honest, unverified text, and implementation
findings feed back into it.

## Amendment 1 — the re-scan trust mechanism (replaces v7's T4 answer)

Two invariants of deliberately different strength (brief v2, verbatim
intent):

- **Store integrity is absolute.** No byte enters the store unhashed;
  store reads re-verify. Reuse never writes anything.
- **Re-scan of a previously-snapshotted tree is metadata-trusted by
  default.** Full re-hash at terabyte scale is not a viable default.

**Mechanism.** `c4 id -c <guide.c4m> <dir>` re-scans with the guide as
the reuse source (`-c` keeps its letter; its meaning is now the reuse
guide, not a path filter). For each regular file encountered by the
walk, the prior ID is **reused** iff ALL of:

1. the same path exists in the guide as a regular-file entry;
2. recorded size equals observed size;
3. recorded mtime equals observed mtime (at the format's stored
   precision); and
4. the observed mtime is **strictly older than the guide's own scan
   start** (the racy rule).

Otherwise — new path, size or mtime mismatch, or racy — the file is
**re-hashed**. Directories always recompute (bottom-up from children,
as shipped). Symlinks always re-read their target text (readlink is
cheap; no trust needed).

**The racy rule, total, no invented constants.** "The guide's scan
start" is a timestamp the snapshot records at the moment its walk
begins, carried in the guide's own journal section (Amendment 2 wires
the journal; until a guide is journal-backed, the guide file's own
mtime is the conservative stand-in, being never earlier than the scan
it recorded). A file whose mtime is not strictly older than that
instant could have been written during or after the scan within the
filesystem's timestamp granularity — so it is re-hashed regardless of
apparent equality. This is git's racy-index resolution restated for
snapshots: correctness never depends on timestamp granularity, on any
platform, without any tunable window.

**The accepted, documented risk (posture, not bug).** A same-size
byte change with a deliberately restored mtime, older than the prior
scan, is invisible to the default re-scan. The surface says this
plainly in id(1) NOTES. The defenses are the verified store (bytes
already ingested are immune — reads re-verify) and forced
verification:

**`--verify` (spends the second and final budget flag, sanctioned by
the brief).** `c4 id --verify …` ignores every reuse rule and
re-hashes everything named; with a guide present it additionally
REPORTS any file whose bytes changed under an unchanged size+mtime
(the obfuscation signature) on stderr. `--verify` composes with paths:
`c4 id --verify -c guide.c4m dir/sub/file` re-verifies one file.

**Reporting (respects the one-summary-line narration pin).** The
stderr summary line gains reuse accounting:
`… N files (R reused, H rehashed), …`. `c4 explain id` narrates the
reuse decisions per file class.

**Store interaction.** Reuse marks the entry with the guide's ID and
never touches the store: with `-s`, a reused entry's object is already
present from the prior ingest by definition of reuse (its ID came from
a journaled snapshot whose barrier passed); if the store was
independently damaged, that is the store's problem, caught by read
verification, never masked by ingest. The write-skip for re-hashed
files keeps its presence-gate (sound since the 1.0.16
barrier-before-rename fix).

**Budget note.** Flags spent: `restore --force` (1), `id --verify`
(2 of 2). `-c` re-purposes an existing shipped flag. No other new
surface.

## Amendment 2 — journal timestamps carry the scan start

v7's journal entry grammar is unchanged. One clarification with a
named defect: the section's recorded UTC time is the **scan start**
(the instant the walk began), not the append time — defect: the racy
rule needs "could a write have raced this scan?", and append-time is
too late by the whole scan duration, which would make every file
scanned during a long ingest permanently racy against its own guide.
Append still happens at print time; only the recorded instant moves.

## Amendment 3 — the three retention amendments (from the accepted architecture, landed as design text)

Verbatim adoption from `store-records-architecture.md` (ACCEPTED):

(a) The collector consumes the root catalog **filtered by the
retention fold**, and the deliberate-forget boundary is the **drop
verdict**, not split. Defect: split-as-forget is prefix-only and welds
byte-deletion to history-forgetting.

(b) v7's "nothing a journal line reaches will ever be collectable"
gains its falsifiable scope: *…except inside the recomputed closure of
a root you explicitly trashed and then emptied, which the record
permanently shows.*

(c) The sweep lock is specified: every claim path (ingest, restore,
retention append) holds the SHARED side from its first store
interaction through its print; a future collector holds EXCLUSIVE
through parse + closure + record + sweep. (The lock lands with gc, not
with this build; the claim paths are written lock-ready.)

Retention vocabulary per the accepted decisions: keep, trash,
**scrub** (renamed from shred), drop. The retention record itself, its
porcelain, and gc are NOT in this build — they follow the parallel gc
design round.

## Build order (implementation branch `feature/snapshot-loop-v8`)

1. Content projection (`-m c`, full-null per D1) + `id -q` single-ID
   output + machine-output contract (SCRIPTING block, one ID line per
   succeeded path, pinned print-failure exits).
2. Journal (`<store>/log.c4m`): append-only writer with OS append
   lock, torn-tail truncation on next append, scan-start timestamps;
   `c4 log` (no args) reads it.
3. Print barrier wiring: no ID reaches stdout before its journal
   append and store barrier complete (store side shipped in 1.0.16).
4. `restore` verb per v7 (dry-run default; `--force`; pre-image as
   line 1; interleaved verification; exit machine), with `patch`
   demoting to text algebra.
5. Re-scan trust mechanism per Amendment 1.
6. Surface deletions + help/man rewrite per surface-v7, updated for
   Amendments 1–2.

Everything else in v7 — identity concepts, durability protocol,
folded sequences, exclusion mechanism, argument-root resolution,
stance section — carries verbatim.

## Amendment 4 — stale ingest-temp cleanup (added 2026-07-13, interrogation finding)

Crashed ingests leave `.ingest.*` staging files: non-object names,
invisible to Walk and to every closure. They hold good bytes (the
barrier-before-rename protocol writes them completely before any
publication) but nothing references them. Rule: **any holder of the
EXCLUSIVE sweep lock may delete every `.ingest.*` staging file** — the
lock proves no live writer exists, so every staging file present is
stale by definition. `c4 retain adopt` and `c4 gc --empty-trash`
perform this cleanup as a side effect, reporting the count on their
summary line. No standalone verb; until gc ships, crashed-ingest temps
leak (small, rare) and the interim fresh-store recipe clears them.
