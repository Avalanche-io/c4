# Safety Defaults — Self-Capturing Snapshots and Patch Pre-State

Requirements and design for two defaults that close the gaps between
"the store is the safety net" and what the CLI actually puts in it.
Companion to `design/store-ingest-performance.md`, whose batch barrier
makes both defaults affordable.

## Motivation (two real incidents)

1. **Snapshots don't capture themselves.** `c4 id -s tree/` stores
   every file's content and every *subdirectory's* one-level record,
   but not the manifest text itself and not the root's own record.
   Lose the `.c4m` file and the content survives while the description
   — names, structure, metadata — is gone. The one artifact the
   snapshot exists to produce is the one artifact it does not protect.
2. **Reconcile destroys silently.** `c4 patch target.c4m dir/` applied
   to the wrong directory quietly removed the prior state. The `-s`
   backup flag existed, but a safety net you must remember to bring is
   not a safety net. Worse, the `-s`/`-r` round trip was broken for
   any nested tree: the changeset's OldID is the pre-state root's
   one-level record ID (`ComputeC4ID`), while `-s` stored only the full
   manifest text — a different object with a different ID — so
   `c4 patch -r changeset.c4m dir/` failed with "pre-patch manifest
   not found in store" (verified on v1.0.13 and on the ingest branch).

## The two IDs of a description

Per `design/c4m-canonical-storage.md`, a c4m file's own identity is
the ID of its canonicalized text. A tree therefore has two related IDs:

- **manifest ID** — `Identify(Marshal(m))`: the full canonical text,
  every entry at every depth. `c4 cat <manifest-id>` returns the
  complete description in one read.
- **root directory ID** — `m.ComputeC4ID()`: the ID of the root's
  one-level record (its direct children, canonical form). This is the
  ID that appears as OldID/NewID in changesets and as a directory
  entry's ID in a parent manifest.

A self-contained snapshot must store both: the manifest text (grab the
whole description by one ID) and the root record (complete the
recursive record chain — subdirectory records were already stored; the
root's was the missing link for `c4 cat -r <dir-id>`, gc marking from
a bare directory ID, and changeset-based revert).

## Requirement 1 — self-capturing snapshots (#214)

Whenever the CLI ingests a tree and produces a manifest (`c4 id -s`,
`c4 diff -s` directory args, `c4 patch` dir-scan forms, bare `c4
<path>`), it additionally stores:

- (a) the manifest's canonical text, retrievable by the manifest ID;
- (b) the root's one-level directory record, retrievable by the root
  directory ID.

and reports, on stderr (stdout purity: stdout *is* the c4m):

```
stored: <manifest-id>
```

printed after the ingest durability barrier, so "stored" means
durable. No new flags. `-q` silences stdout only; the stored line
remains (with `-q` it is the only way to learn the ID).

Recovery contract: after losing the tree *and* the `.c4m` file,
`c4 cat <manifest-id>` reproduces the manifest byte-identically, and
`c4 patch <recovered.c4m> <dir>` materializes the tree from the store
alone.

`c4 id -s` on plain files stores the combined manifest text the same
way (there is no root record for a file list). `c4 id -s file.c4m`
(normalize-and-store) stores text + root record and reports the same
line.

## Requirement 2 — patch pre-state by default (#215)

For every reconcile-onto-directory form — `c4 patch <file.c4m> <dir>`,
`c4 patch <dir> <dir>`, and `c4 patch -r <target> <dir>` — the
destination's prior state is captured **before** anything is applied:

1. Scan dest (already required to plan the reconcile).
2. Store every file content that would vanish from dest — removed or
   overwritten: any current entry whose ID does not appear among the
   target's file entries and is not already in the store.
3. Store the pre-state manifest per Requirement 1: text, root record,
   and each directory's one-level record (the records make
   changeset-based `-r` work on nested trees).
4. Issue the durability barrier. Only then apply.
5. After a successful apply, report on stderr:

```
prior state stored: <manifest-id> (revert: c4 patch -r <manifest-id> <dir>)
```

The printed revert command works verbatim: `c4 patch -r` now accepts a
C4 ID as its first argument and loads the pre-state manifest straight
from the store. The changeset-file form still works, fixed for nested
trees: manifests loaded from the store by a changeset's OldID are
one-level root records and are expanded recursively through the stored
directory records before reconciling.

Opt-outs and edges:

- `--no-store` skips capture and the report entirely (flag existed;
  it also keeps its prior meaning of skipping source-scan ingest).
- `--dry-run` is unchanged: nothing applied, nothing captured.
- `-s` is now redundant-but-harmless: it still adds removal-time
  content capture inside Apply (`reconcile.WithStoreRemovals`), which
  after this change only catches content that appeared between the
  dest scan and its removal. Kept, documented.
- An empty or missing dest has no prior state: no capture, no report.
- No store configured: `c4 patch` reconcile forms now offer to create
  the default store (same prompt as `c4 id -s`); non-interactive runs
  proceed with a warning that the prior state was not stored.

## Durability of the capture

The invariant is **pre-state durable before destruction begins**, not
per-object durability during capture (dest is untouched while capture
runs; a crash mid-capture loses nothing). So capture uses the batch
barrier: cheap per-object `fsync(2)`, one `F_FULLFSYNC` before Apply.
This is what makes default-on affordable — the worst case (everything
vanishes) is a full ingest of dest at batch speed, ~44x cheaper than
per-object flushes.

- `--durable` upgrades capture to per-object `F_FULLFSYNC`.
- `--no-fsync` does **not** apply to capture: content about to be
  destroyed is never written unsynced. The ingest store handle is
  clamped from `SyncNone` to `SyncBatch` for the capture and stays
  there for the rest of the run (later safety writes inherit the
  floor). Scan-side ingest of the *source* tree, whose source
  survives, obeys the flag as before.
- Removal-time `-s` writes ride the same handle and are covered by a
  second barrier after Apply. This refines the ingest-branch decision
  that kept `-s` writes per-object durable: with the pre-apply capture
  in place, removal-time capture is an anti-race belt, not the primary
  net — and it is still never unsynced.

## Options weighed

- **Capture at removal time only (status quo `-s`, made default).**
  Rejected: misses overwritten files entirely, interleaves durability
  with destruction (must stay per-object durable → slow), and leaves
  the nested-tree `-r` lookup broken.
- **Full dest ingest as pre-state.** Rejected: correct but pays full
  ingest cost on every patch; the vanishing-content set is exactly
  what revert cannot re-derive from the post-patch tree, and the
  post-patch tree itself remains a content source at revert time.
- **Store the pre-state manifest under its ComputeC4ID.** Rejected:
  would forge an object whose name is not the hash of its bytes. The
  root record legitimately has that ID; store both objects and let
  each ID name its own bytes.
- **Print the revert as `c4 patch -r changeset.c4m dir/` (stdout
  changeset).** Rejected as primary: requires the user to have saved
  stdout to a file; the ID form works even when stdout was discarded —
  which is precisely the accident scenario.

## UX summary

```
$ c4 id -s project/ > project.c4m
stored: c45nGk...                                  # stderr

$ c4 patch release.c4m project/ > changes.c4m
prior state stored: c45nGk... (revert: c4 patch -r c45nGk... project/)
Reconciled project/: 12 created 340 removed

$ c4 patch -r c45nGk... project/                   # verbatim from above
Reconciled project/: 340 created 12 removed
```

Zen accounting: zero new flags, two stderr lines, one flag
(`--no-store`) grows one sentence of meaning.

## Known limits

- The guided dest scan trusts size+mtime (second precision): a dest
  file whose content differs from the target's at the same path with
  identical size and mtime is assumed unchanged, so its old content is
  not captured. This is the existing guided-scan/trusted-metadata
  contract, not new exposure.
- Folded sequence entries are not captured (warned).
- A revert target loaded from the store is refused if any directory
  record it needs is missing (`prior state incomplete`) — never
  silently reconciled toward a truncated tree.
- The changeset-form drift warning can fire spuriously for standalone
  changeset files (their BaseID is an external reference the resolver
  cannot follow); pre-existing, and the ID form has no such warning.

## Measured results (5,050-file / ~26 MB dest tree)

`c4 patch t.c4m dest/`, fresh dest copy and fresh store per run
(target snapshot pre-stored), APFS, M-series Mac. Two runs each.

| Scenario | default (capture) | `--no-store` | `--durable` |
|---|---:|---:|---:|
| catastrophic: all 5,050 files vanish | 2.7 s | 1.0 s | 43–45 s |
| light: 50 files change | 0.4 s | 0.4 s | — |

The accident case costs ~1.7 s to make fully revertible — the batch
barrier is what makes this shippable as a default (per-object flushes
would cost ~40 s, the `--durable` column). The common case is
within noise. End-to-end at scale: the printed revert command restored
all 5,050 files byte-identically (`c4 diff` empty; revert wall 26.5 s,
dominated by reconcile's default durable file writes).

## Status

Design complete. Implemented on `feature/safety-defaults` (stacked on
`feature/store-ingest-perf`).
