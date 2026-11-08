# Incomplete ingests are collected when the store opens

Status: ruled by Joshua 2026-11-08 ("On boot a system should garbage
collect incomplete transactions that cannot, or should not, be replayed
to completion"); implemented on branch `ingest-sweep`, not landed.

## The finding

c4d's commit measurements (c4d-to-c4-fleet.md, 2026-11-08): a `SIGKILL`
partway through a `SyncBatch` batch leaves its `.ingest.*` temp files in
the store root (6 and 14 files in two cases). They are harmless, nothing
reads them, and nothing ever removes them.

## Why they exist and why they cannot be replayed

`design/store-ingest-performance.md` made batch durability the default:
each object is written to a temp file in the store root, flushed cheaply,
and renamed to its hash name only after the one device barrier at the end
of the batch, so a name in the store always means durable bytes. A writer
that dies before the barrier leaves every pending temp behind. The temp's
name carries no ID, so a complete temp and a torn one look the same; the
only way to tell would be to hash the bytes and trust the result, which
would publish an object nobody claimed and may be half of one. These are
transactions that cannot be replayed to completion, and should not be.

## The rule, applied

Opening a store is its boot. `NewTreeStore` sweeps, and `SweepIngest` is
exported for long-running processes that want to sweep again.

- **The writer is recorded in the temp name.** Temps are now
  `.ingest.<host>.<pid>.<random>` (`ingestPattern`). Hostnames may contain
  dots, so the name is parsed from the right.
- **Removed:** a temp from this host whose pid is not running.
  Liveness is `kill(pid, 0)` on Unix (EPERM counts as alive) and
  `OpenProcess` + `GetExitCodeProcess == STILL_ACTIVE` on Windows (access
  denied counts as alive). When in doubt, alive: keeping a stray file
  costs nothing; removing a live writer's file fails its ingest.
- **Kept:** a live writer's temps; another host's temps (a store on shared
  storage sees pids from several machines, and ours mean nothing there);
  and the pre-1.1 form `.ingest.<random>`, which records no writer, until
  it is older than a day (`legacyIngestAge`), the transitional rule for
  files that 1.0.x binaries wrote or may still be writing.
- **Never touched:** a store object. The sweep looks only at names with
  the `.ingest.` prefix in the store root; hash-named files live in the
  trie below.
- **Best effort.** A sweep that cannot read the root does not stop the
  store from opening.

Pinned by `TestSweepIngestOnOpen` (dead and live pids, another host, old
and fresh legacy names, an object), `TestSweepIngestKeepsOwnPending` (a
batch in progress survives a concurrent open and still publishes at its
barrier) and `TestParseIngestName`.

## For other implementations

Any local store that publishes by rename after a barrier has the same
leak. Apply the same three rules: record the writer (host and pid) in the
temp name; on open, remove the temps of this host's dead writers and
nothing else; never remove or adopt an object. Do not hash a stray temp
to recover it: an object enters the store only through a writer that
claims it.

## Not done here

- The S3 store has no temp files (multipart uploads are aborted or
  completed by the SDK; an abandoned multipart upload is the bucket's
  lifecycle rule's job).
- `Folder` and `ShardedFolder` write through `DurableWriter`, whose temp
  is beside the final path, not in the root, and is removed on every
  error path; a crash between create and rename leaves one `.tmp` beside
  one object. Same rule applies in principle; not swept here because no
  measurement has shown them accumulating.
