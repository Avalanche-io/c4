# C4 project brief for Ash — spinning up and managing the next phase

Written 2026-10-14 for an operator (Ash, on ash-cloud or anywhere) that has
to pick this project up cold, start agents, and keep it moving while Joshua
is traveling. Everything here is either a Joshua ruling, a measured fact, or
marked as open. Read `c4-vision-threads` in the C4 memory directory first
if it is available; it is the yardstick.

## 1. What c4 is, in one paragraph

c4 (SMPTE ST 2114) gives any bytes a 90-character content ID and describes
a filesystem as plain text (a c4m file: one line per entry; a directory's
ID is the ID of its listing). Files are the content; directories are the
metadata; directories are streams of updates, sometimes with one recorded
state. The goal is a universal ad hoc system where descriptions made by
anyone, any time, with no infrastructure, can be tied together decades
later by matching IDs. The suite ships eight components in lockstep (Go
`c4`, `c4sh`, `c4git`, Python `c4py`, TypeScript `c4ts`, `c4-swift`,
`libc4`, container images) through `oss/c4toolkit`.

## 2. Where things stand (2026-10-14)

- **Released:** suite v1.0.18 (2026-10-03). It is on Homebrew, the Go
  proxy, PyPI, npm, GitHub and ghcr.io. The website cccc.io documents it.
- **v1.1 (branch `master`) is built, pushed, CI-green, and ON HOLD.**
  Joshua judged its surface to "smell of wrong fundamentals". It is being
  re-scoped (section 5), not released.
- **Joshua has approved two pieces of work:** a **1.0.19** patch release
  and the **v1.1 re-scope design document**. Nothing else is approved.
- All design records are in `oss/c4/design/` on `master`, pushed to
  GitHub: `retention-study/`, `store-layout.md` (+ two results JSON),
  `snapshot-growth.md`, `v11-surface-review.md`, `chain-resolution.md`.
- The dogfood consumer is the Ash cloud failover sync (ashnarrate docs
  17, 22, 23); it runs on 1.0.18 and needs none of v1.1.

## 3. Rulings from Joshua (binding)

1. Every `c4 id -s` invocation ends with its claim line, no exceptions.
2. v1.1 adds `golang.org/x/sys` (the only approved dependency), for
   symlink timestamps and clonefile.
3. **Store layout:** shard directories are named by the ID two characters
   at a time, ASCII-lowercased, at every level; the file keeps its exact-case
   ID; an object is valid at any level of its own prefix path. Readers
   assert nothing about directory capitalization (probe every spelling that
   exists); writers create lowercase; migration is opportunistic and
   rename-only; duplicate pruning is explicit. No marker file, no fixed
   depth, no threshold in the format.
4. FAT32/exFAT stay targets via special strategies; the common case gets
   the least special handling. Directory counts are an evolving range set
   by local constraints and usage, not a fixed band (any-depth allows
   partial, incremental splits).
5. Low-level delete-by-ID is never in question. **Cleanup policies are
   required in v1.1.** Model: removal as reduced durability. An unchosen
   alternative is not trashed and not replicated; it moves to the zone
   where a human can decide (usually local), loses its remote copies,
   re-propagates if the choice is reverted, and when confirmed there is one
   last place to apply the removal, which makes it certifiable (no store
   you control returns the ID). No replicated attic or trash folders.
6. Retention, undo, sync and file movers are "after-market c4 enjoyers":
   built on the core, possibly in the same box as suite members, not inside
   the primitives. The core is strict about meaning (an ID means these
   bytes; a null means unknown) and free about effort (applications may
   trust metadata to skip work, but a cheaper answer must never claim to be
   a stronger one).
7. Versioning (permanent): never a v2; minor = breaking tier; patch =
   blind-upgrade safe; zero new exported Go identifiers in a patch release.
8. Releases are suite releases through `oss/c4toolkit`, from the laptop
   only (PyPI, npm, Homebrew credentials live there). The toolkit tag goes
   on the bump commit (`release.sh` enforces it).

## 4. Work item A: 1.0.19 (approved)

Branch from `release/1.0.x` in each repo (worktrees under `.worktrees/`).
Content, all verified defects:

1. **c4git `gc`** (repo `oss/c4git`): `runGC` walks `.c4git.yaml`'s store
   path while the clean/smudge filters use `OpenConfigured()` (C4_STORE,
   then `~/.c4/config`) first, so gc and the filters can target different
   stores; `gc --force` deleted another tool's objects from a shared store
   (verified in isolation 2026-10-14). Fix: same resolution as the filters;
   never remove objects the repo did not write, or refuse outside a
   repo-private store; `--force` gets a dry run; `gc --help` must not run
   gc.
2. **Split moves reserved root files** (`oss/c4`, both lines): when the
   store root crosses 4,096 objects, `split` moves `journal` to
   `jo/journal` and `log.c4m` to `lo/log.c4m`, and counts them toward the
   threshold. Split must move only names that parse as C4 IDs.
3. **Layout rule in the store package** (ruling 3): lowercase shard names
   on create; deepest-first any-depth reader; case-agnostic probing (1.0.18
   already has the fold lookup; keep it as the shared rule, not a legacy
   mode); `Remove` deletes at every level; `Walk` enumerates every
   directory spelling and reports each ID once; a flat root may split
   straight into `c4/xx`; opportunistic rename-only migration (`c4 store
   normalize`, dry-run default, SameFile guard before any prune).
   Conformance vectors are drafted in `store-layout.md`.
4. Port the same to `c4py` (`store.py`) and tell imgbin's owner
   (`~/ws/active/software/imgbin/Sources/ImgbinCore/Store.swift`); `c4ts`'s
   store is wrong today and is rewritten to the rule.

Then: CI green on every repo (Windows and Go 1.16 legs included), then the
suite release exactly as 1.0.18 was done (version-only bumps for siblings
from their v1.0.18 tags, PyPI, npm, toolkit release on the bump commit,
Homebrew tap, container tag). **Joshua says yes to publish; nobody else.**

## 5. Work item B: the v1.1 re-scope design document (approved)

Write `oss/c4/design/v11-rescope.md` before any v1.1 code. Joshua signs
off on the document. It must cover:

- **Three layers:** L0 the standard (ID, c4m grammar, a store contract of
  put/get/has, optional list/remove-by-ID, and three sentences: a store
  never removes bytes on its own initiative; served bytes hash to their ID;
  absence is an ordinary answer and never edits a description); L1 the
  toolbox (id, diff, patch as text algebra, merge, paths, restore as
  verified reconcile); L2 the product (snapshot, undo, history, sync,
  retention), possibly a separate suite tool.
- **What moves out of the core:** the print barrier as an output protocol,
  claim-vs-description line semantics, exit codes 2/3, the `@c4 journal 1`
  format, gc verdicts/retain file (withdrawn), receipts/testimony records.
  Keep what follows from ID + c4m: streaming chain output, content ID
  (`-m c`), restore, nothing writes without `-s`.
- **Cleanup as the durability ladder** (ruling 5): zones, how a pending
  choice is recorded (a c4m naming both branches), certification (`c4 has`
  per store), and the rule "only a held entry keeps bytes" or one store per
  purpose (three adversaries showed shared stores are unsafe to prune under
  dedup).
- **Directory streams in the store:** measured 2026-10-14, 200 snapshots
  cost 289 MB as full listings and 1.55 MB as a chain; design keyframes +
  chain sections, prerequisite = the July chain-resolution fixes (now
  pushed) and their ports to the other four languages.
- **Symlink times via x/sys**; the symlink-ID rule (null unless the target
  is an in-tree regular file).
- Answers to the July 25 questions still open: store bootstrap (lead
  recommends: create `~/.c4/store` silently, never prompt), record-kind
  freezing (freeze only the `@c4 <kind> <version>` header convention and
  the unknown-version rule), the 797-word front door (cut duplicates, keep
  the SCRIPTING contract), gc root-set rule (keep frozen).
- Port of every 1.0.17/1.0.18 fix to master (`DurableWriter.Abort`,
  verified reads, diff hashing, racy scan, cat verify, case-tolerant store,
  dir mtimes, panic fix), plus the two unmerged July branches
  (`fix/store-integrity`, `fix/restore-symlink-traversal`), plus issues
  #41, #42, #44, plus `restore` materializing what is held and declaring
  the rest (today it is all-or-nothing on one missing file).
- Then docs, the cccc.io rewrite (it documents 1.0.x verbs), CI, release.

## 6. Spinning up on ash-cloud

- **Access:** `ssh ash-cloud` (root) or `ssh ash-cloud-tn` (user ash) from
  Abyss: port 2222, key `~/.ssh/ash-cloud`. Never port 22; that is Tailscale
  SSH and hangs on re-auth after any Tailscale upgrade.
- **Present (verified 2026-10-15):** Ubuntu 24.04, 2 vCPU, 3 GB usable
  RAM, 40 GB free; node 22; Go 1.26.9 at `/usr/local/go/bin/go` (not on
  root's PATH); `gh` 2.45 installed but not logged in; c4 1.0.18 at
  `/usr/local/bin/c4`; git identity set for user ash; Claude Code 2.1.288 for user `ash`
  via
  `/home/ash/bin/claude` (pinned to the Ash account); the Mac mirror at
  `/Users/joshua/ws/active/software/c4` is a live git clone.
- **Missing, needs Joshua's okay:** a GitHub login for `gh` or a
  fine-grained token scoped to the Avalanche-io repos (no workflow
  permission). Without it the VM can build and test but not push.
- **Correction (2026-10-15):** an earlier edit of this section said no c4
  work had run on the VM. That was wrong: it looked only at the Mac mirror.
  An unattended agent under user `ash` carried out all of work item A on
  2026-10-15 from `/home/ash/ws/c4/prompt-1.0.19.md`, in clones at
  `/home/ash/ws/c4/<repo>` (not the Mac mirror). Its report is
  `/home/ash/ws/c4/STATUS-1.0.19.md`; branches are listed there. The
  branches were fetched to the laptop as `vm/fix/*` refs and run on macOS:
  c4git, c4py and c4ts pass; c4 fails split vector S2 because the split
  counter is keyed by directory spelling, so on a case-insensitive volume
  one directory gets two counters and splits late. Nothing is pushed.
- **Capacity:** one or two agents at a time. Run them in-process (no
  named pane agents: those trigger a folder-trust dialog). Each fix on its
  own branch and worktree under `.worktrees/`; push often; coordinate with
  the laptop through branches and these design docs, not through memory
  (memory only follows the mirror).
- The VM's case-sensitive filesystem is where layout work must be
  verified; the Mac cannot exercise it.

## 7. Rules for every agent

- Read-only on `~/.c4`, `~/.ash` and every real store. Experiments only
  under `/tmp/<task>/` with `C4_STORE` set there; `c4 <path>` and
  `c4 id -s` WRITE to the configured store, and `c4git` prefers the global
  store over the repo-local one. A study agent once wrote test objects into
  Joshua's real store this way.
- Never `rm`/`rmdir` a shell variable; use literal paths.
- Commit messages: one short line (a hook rejects anything else).
- Claims about c4 behavior must be run on the binary, not read from docs.
- Patch releases: no ID changes, no new flags, no new exported Go
  identifiers, Go 1.16-compatible, stdlib only.
- Subagent results: read every one, reconcile started versus returned,
  name what was lost.
- Markdown tables aligned; design before code; keep
  `ARCHITECTURAL_CATALOG.md` current.

## 8. Known hazards still live in released code

- `c4git gc --force` (item A1).
- Shared stores cannot be pruned safely under content dedup.
- rsync `--delete-after`/`--delete-delay` between stores whose shard names
  differ only in case wipes the shard with exit 0; normalize with c4 on
  each replica, never rely on rsync.
- `c4 patch` trusts a destination file whose size and mtime match; verify a
  materialized tree with `c4 id`.
- Symlink timestamps are not restored (x/sys, v1.1).

## 9. Open decisions (Joshua only)

- Publish 1.0.19 when tested (yes in principle; confirm at the time).
- Sign-off on `v11-rescope.md`.
- Install Go/gh and a GitHub token on the VM.
- File the public c4git issue.
- Default split threshold for the common case (65,536 is better above
  10^6 objects; 4,096 is FAT32-safe); whether 1-character levels deserve a
  study.

## 10. People and channels

- Joshua Kolden decides; GitHub `mrjoshuak`; org `Avalanche-io`.
- The ashnarrate session owns the Ash failover sync design (docs 17, 22,
  23, `handoffs/c4-sync.md`) and the VM runbook (`ashin` branch
  `cloud-vm`, `docs/CLOUD-VM.md`).
- Release mechanics: workspace `RELEASE.md`, `oss/c4toolkit/README.md`,
  `oss/c4toolkit/scripts/*.sh`.
