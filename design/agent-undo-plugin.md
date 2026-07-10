# Agent whole-tree undo/fork — c4-mcp + Claude Code plugin

Status: requirements draft (2026-07-10). Design before code. Claims in
this document are constrained by the hands-on incumbent bake-off of
2026-07-10 (`plans/bakeoff-2026-07/RESULTS.md`) — every competitive
sentence below traces to a measured cell in that matrix, not to
documentation or assumption.

## The claim (bake-off-narrowed, verified wording)

> C4 is the only tool that restores your *whole* tree — the gitignored
> `.env`, the shell-command mess, and `.git` itself — from one durable,
> portable snapshot you can move to another machine and reversibly
> undo; git, Copilot, and harness checkpoints each only ever hand you
> back the slice they were tracking.

What changed from the July framing: **"untracked files" is no longer a
unique differentiator** (Copilot CLI, jj, and APFS snapshots all
recover untracked state now) and must not lead. The verified unique
set is: gitignored state (S3), full shell side effects (S4), `.git`
deletion (S5), reversible undo-of-restore (S7), and durable + portable
+ no-expiry history (S8). Copilot's own snapshot metadata backs up
untracked files but **never** gitignored ones, and its rewind is
documented and observed irreversible; git/jj store history inside the
tree, so S5 is fatal; APFS matches breadth but is whole-volume,
~24h-purgeable, non-portable, and non-reversible.

## The gap the plugin exists to close

The bake-off's one adverse finding: **C4 loses on ergonomics and
automation, not coverage.** Every incumbent auto-captures (per turn,
per edit, per hour); shipped c4 requires a manual `c4 id -s` *before*
the damage, and with no prior snapshot there is nothing to restore.
The plugin's entire job is to delete that sentence: after install,
snapshots happen without the user doing anything, and recovery is one
command.

## Components

### 1. c4-mcp (published, hardened)

- Replace the hand-rolled scanner with the library `scan` package.
- Tools: `snapshot` (tree → printed snapshot ID), `restore` (ID + dir →
  byte-exact tree, printing its pre-image ID), `diff`, `contains`
  (store-membership by ID). Tool outputs are the machine-output
  contract from the snapshot-loop design — single-line IDs, byte-pure.
- Remove the `go.mod` replace; rewrite README around the undo demo;
  add to the suite manifest; flip public under the org; list in MCP
  registries.
- Gate: the restore tool must not exist before protected-path behavior
  is in the released base (satisfied — v1.0.15).

### 2. Claude Code plugin (thin, hook-driven)

- **Auto-snapshot**: SessionStart snapshot, then per-prompt (or
  per-tool-batch — open question 1) incremental snapshots via hooks.
- **`/undo`**: restore the tree to the pre-prompt snapshot; prints the
  pre-image ID so the undo is itself undoable (bake-off S7 — no
  incumbent has this).
- **`/fork`**: materialize the current snapshot into a sibling
  workspace per agent.
- **Receipts**: per-session summary — what changed across the whole
  tree, by ID.
- The plugin is porcelain over c4-mcp; it defines no identity or
  durability semantics of its own — those belong to the snapshot-loop
  design (crucible, in progress). Where this doc says "snapshot ID,"
  the crucible's frozen surface governs.

## Honest limits (stated in the README, first)

- Files in the tree only: no databases, cloud state, or running
  services.
- The store shares the working directory's blast radius: this is undo
  for accidents; it composes with a sandbox and is not containment.
- History starts at the first snapshot; nothing before install is
  recoverable.
- Secrets posture: Joshua's Decision D5 (pending) — the mechanism is
  exclusion-with-visible-report; every snapshot report lists what was
  excluded and why.

## Launch gates (each sentence dishonest until its gate ships)

| # | Claim | Gate | Status |
|---|--------------------------------------------|--------------------------------------------|-----------------|
| 1 | any undo/rollback sentence | safety-defaults released | ✅ v1.0.15 |
| 2 | any speed number | measured on released binary | ✅ v1.0.15 (4.3s/4.4s, 20k) |
| 3 | "anything printed is recoverable" | print barrier + restore verb built + released | ⏳ crucible → build |
| 4 | per-prompt viability | incremental cost measured on gitignored-heavy tree | ⏳ measure (release gate, not promise) |
| 5 | "works with your MCP agent" | c4-mcp rebuilt + public | ⏳ |
| 6 | "/undo in Claude Code" | plugin exists | ⏳ |
| 7 | competitive table in launch post | hands-on bake-off | ✅ 2026-07-10 |

## Demo (Demo A, DEMO_PORTFOLIO_V2 §A — 90 seconds)

Storyboard as specified there; the destructive beat runs uncut; the
recovery beat ends on post-state root ID == pre-state root ID; the
final beat undoes the restore. Kill-condition from that doc is
resolved by the bake-off: the high-value failures only C4 recovers are
S3 (gitignored), S4-full (shell effects), S5 (`.git`), with S7/S8 as
the durability/portability close.

## Non-goals

Sandboxing/containment, autonomous merge or reconciliation (permanent
guardrail), databases/external state, multi-repo binding
(constellations are later), any network/registry story, harness
lock-in features (the substrate must remain agent-agnostic — the
bake-off's S8 column is the moat).

## Open questions (for Joshua / crucible outcome)

1. Auto-snapshot granularity: per-prompt vs per-tool-batch vs
   both-with-coalescing. Cost data from gate 4 decides.
2. Retention default: no-expiry (bake-off differentiator) vs
   configurable cap — and whether "nothing ever expires" survives a
   store-growth complaint without gc (which was withdrawn).
3. `/fork` mechanics: sibling directory materialization vs worktree
   integration; naming of forks.
4. Whether the plugin ships in the same release as the crucible's
   restore verb (strongest single story) or after it (faster, two
   moments). PATH-FORWARD Decision 5 territory — Joshua's launch
   window to spend.
