# Content Retention: Declarative Disposition

> You never expect files to disappear from your hard drive.

## The Mental Model

c4 storage works like your laptop's hard drive. You put files there,
they stay there. You delete files when you choose to. Nothing
disappears on its own.

The user describes what they value through existing structural tools —
c4m files, managed directories, tags, pins, locations. Content
reachable from these structures is alive. Content that loses all
references lingers as a hidden cache (purgatory), then is eventually
reclaimed by storage pressure.

## Why Content-Addressed Storage Is Different

In a traditional filesystem, editing a file overwrites it in place.
Change one byte of a 10 GB file and you still have one 10 GB file.

In a content-addressed store, every version is a distinct object.
Change one byte and you have two 10 GB objects. Every saved draft,
every re-render, every minor tweak is a completely new blob. Storage
growth in a CAS is far more aggressive than filesystem intuition
suggests.

This means retention must be managed — but the management should be
declarative, not imperative. The user describes what they value. The
system handles the rest.

## The Reachability Spectrum

Content in c4 exists on a spectrum of reachability:

```
Active ──────────────→ Purgatory ──────→ Gone
(referenced)           (cache)           (freed)
reachable              unreachable       deleted
user-visible           hidden            —
```

### Active

Content reachable from any established path — a c4m file, a managed
directory, a tag, a pin, a location. Stays until the user changes
its disposition. Some paths may carry a TTL (see TTL-Bearing Paths
below); content stays active as long as any reference keeps it.

### Purgatory

Content that has lost all references — not reachable from any path,
not in any undo history. Hidden from the user. But still physically
present in the store as a **cache optimization**: if a new c4m arrives
from a remote referencing this blob, it's already here. No transfer
needed.

Purgatory has a system-managed TTL, typically driven by storage
pressure. Content here is not recoverable through any user-facing
mechanism — it's not undo, it's cache.

### Gone

Content deleted from the store. Storage freed. Irreversible.

## How Content Enters Purgatory

Content enters purgatory when it loses all references. This happens
via two paths:

### 1. Explicit Removal (User-Driven)

`c4 rm` operates on two different things depending on target:

**Managed directory edits** — `c4 rm :file` removes an entry from
the current c4m state. This is a structural edit, recoverable via
`c4 undo :` or snapshots. The blob stays in the store, still
referenced by snapshot history. It enters purgatory only when all
snapshots containing it are pruned.

**Path un-establishment** — `c4 rm project.c4m:` un-establishes a
c4m file or location. The c4m is no longer a reachability root.
Content referenced only by that c4m (and not by anything else)
enters purgatory.

These are different operations:

| Command | What it does | Recovery |
|---|---|---|
| `c4 rm :file` | Remove entry from managed c4m | `c4 undo :`, snapshots |
| `c4 rm project.c4m:` | Un-establish a c4m | Re-establish it |
| `c4 rm location:` | Un-establish a location | Re-establish it |
| `c4 rm --shred :file` | Nuclear delete + tombstone | Not recoverable |

### 2. Implicit Pruning (Policy-Driven)

Sparse history, snapshot retention, and TTL-bearing paths
automatically remove references:

- A managed directory with `--snapshot-retain 10` prunes its 11th
  oldest snapshot. Unique blobs lose their reference.
- A TTL-bearing path (see below) expires. Its content loses its
  reference.
- Keystroke-level churn: between the current state and sparse
  history marker #1, intermediate blobs are unreferenced.

These blobs enter purgatory directly — no user action needed.

## TTL-Bearing Paths

Any established path can carry a TTL. There is no system-defined
"trash" concept — if a user wants a trash folder, they create one:

```
c4 mk my-trash.c4m: --retain 30d    # user's trash, 30-day TTL
c4 mk builds: --retain 14d          # build artifacts, 14-day TTL
c4 mk tmp: --retain 7d              # temp workspace, 7-day TTL
```

When the TTL expires, the path is automatically un-established.
Content referenced only by that path enters purgatory. Content
referenced by other active paths stays active.

TTL-bearing paths are visible and browsable like any other path.
The user chose the name, the TTL, and what to put there. The system
just enforces the expiration.

## Reference Model

Content is alive if ANY reference keeps it. Content enters purgatory
only when ALL references have been released:

```
                  +------------------+
                  |  Content Blob    |
                  |    (C4 ID)       |
                  +--------+---------+
                           |
              referenced by?
                           |
         +-----------+-----+--------+-----------+
         |           |              |           |
  +------+------+ +--+-------+ +---+-----+ +---+------+
  |  Established| | Managed  | | Tagged  | | TTL-     |
  |  c4m/pin/   | | Dir      | | Snap-   | | bearing  |
  |  location   | | current  | | shots   | | paths    |
  +------+------+ +--+-------+ +---+-----+ +---+------+
         |           |              |           |
       keeps       keeps         keeps       keeps
      forever   while managed  while tag   until TTL
                                exists      expires
```

Every reference type uses the same reachability engine. The only
difference is lifetime: some are permanent until explicitly removed,
some are governed by retention policy, some expire on a timer.
Nothing the user values disappears without the user changing its
disposition.

## Snapshots: Diffs, Cadence, and Sparse History

### Snapshots as Diffs

Snapshots are stored as diffs from the previous snapshot, not as full
c4m copies. A diff records what changed — added entries, removed
entries, modified entries (same path, different C4 ID). The diff
itself is a c4m patch.

This makes snapshots cheap. A 200,000-entry project where 3 files
changed produces a 3-entry diff, not a 200,000-entry full copy.
Frequent snapshots become practical because most diffs are small.

To reconstruct a historical state, apply diffs backward from the
current state (or forward from a base snapshot). Periodic base
snapshots (full c4m copies) bound the reconstruction cost — e.g.,
one base per 100 diffs.

### Cadence-Based Snapshots

Snapshots are created automatically on a time cadence, not only on
explicit `c4 sync`:

```
c4 mk : --snapshot-cadence 5m        # snapshot every 5 minutes
c4 mk : --snapshot-cadence 1h        # snapshot every hour
c4 mk : --snapshot-cadence off       # manual only (c4 sync)
```

A cadence snapshot is only created if something actually changed
since the last snapshot. No change, no snapshot.

### Conditional Cadence

A diff-size threshold adds conditional triggering — snapshot early
if enough has changed, regardless of the cadence timer:

```
c4 mk : --snapshot-cadence 5m --snapshot-threshold 100
# snapshot every 5 minutes OR when 100+ entries have changed,
# whichever comes first
```

This catches large batch changes (VFX re-renders, bulk imports)
immediately rather than waiting for the next cadence tick. The
threshold is measured in entry count — how many files were added,
removed, or modified since the last snapshot.

### Sparse History

Snapshot retention controls implement sparse history — keeping
selected snapshots and pruning the rest:

```
c4 mk : --snapshot-retain 10         # keep last 10 snapshots
c4 mk : --snapshot-retain tagged     # keep only tagged snapshots
c4 mk : --snapshot-retain 30d        # keep snapshots younger than 30 days
```

Default: last 10 snapshots + all tagged snapshots.

Because snapshots are diffs, pruning means **collapsing** adjacent
diffs rather than deleting full snapshots. Pruning snapshot S3
between S2 and S4 composes the S2→S3 and S3→S4 diffs into a single
S2→S4 diff. The intermediate state is lost, but the before and after
states are preserved exactly.

Content blobs referenced only by pruned diffs — the blobs that were
the "in between" states — enter purgatory if not referenced by
anything else.

**The keystroke churn problem:** Cadence-based snapshots capture
every intermediate state cheaply (small diffs). Sparse history
collapses them naturally. Between the current state and sparse
history marker #1, the intermediate diffs are collapsed into one.
The unique blobs from those intermediates enter purgatory. The
retention policy on the managed directory IS the disposition for
this churn — no user action needed.

The critical safety property: between the current file, sparse
history marker #1, and all the intermediate blobs, we are only
unlinking the intermediates. The current state and the markers are
protected by active references. Diff collapse is an atomic
operation — the combined diff replaces the individual diffs in a
single atomic write.

**Auto-tagging on significant changes:** When a large batch of entries
is replaced (e.g., VFX re-render — detected via the snapshot
threshold), the system auto-tags the pre-change snapshot. Tagged
snapshots survive beyond the retention window, giving you the
selected prior states that matter without keeping every intermediate
version.

## Purgatory as Cache

Purgatory is not undo. Content in purgatory is unreachable from every
user-visible path and every undo history chain. The user cannot
recover it through any c4 command.

But the blob is still physically present. Its value is **transfer
avoidance**: if a new c4m arrives from a remote peer referencing a
blob in purgatory, the blob is immediately reachable again — no
network transfer needed. The blob moves from purgatory back to
active.

```
                         new c4m references it
                         ┌──────────────────────┐
                         │                      │
Active ──lose all refs──→ Purgatory ──pressure──→ Gone
  ↑                           │
  └───new c4m references──────┘
```

Purgatory TTL is managed by c4d based on a tunable pressure curve
relative to the configured storage limit (see Storage Limits below).
As consumption approaches the limit, purgatory TTL decreases
smoothly — lots of cache when there's headroom, aggressive reclaim
near the ceiling. The specific curve shape and defaults will be
discovered through testing and early feedback.

## Retention Anchors

From strongest to weakest:

| Anchor | Lifetime | Release mechanism |
|--------|----------|-------------------|
| **Auth-required path** (legal hold) | Until authorized release | Un-establish with auth |
| **Explicit pin** | Indefinite | `c4 unpin` |
| **Established c4m** (no TTL) | Until un-established | `c4 rm project.c4m:` |
| **Managed dir current** | While managed | `c4 rm :` (teardown) |
| **Tagged snapshot** | While tag exists | `c4 rm :~tagname` |
| **Managed dir snapshot** (in window) | Within retention config | Automatic pruning |
| **TTL-bearing path** | Until TTL expires | Automatic expiration |
| **Location registration** | While registered | `c4 rm location:` |

The **establishment registry** is the root set. Everything reachable
from an active root is alive. Reachability is computed continuously
by c4d, not by a user-invoked command.

## Storage Limits

c4d is configured with a storage limit — a concrete size, not a
percentage of the filesystem:

```
c4 config store.limit 200G     # this node may use up to 200 GB
c4 config store.limit          # show current limit
```

The storage limit governs the purgatory pressure curve. Referenced
content is always kept. Purgatory content is the flex space — cached
blobs that c4d reclaims as needed to stay within the limit.

### The OS Visibility Problem

On a laptop with a 1 TB SSD, if c4d stores 300 GB total but 200 GB
of that is purgatory cache, the OS reports 300 GB used. Every other
application — the OS itself, Finder, disk space warnings — sees the
disk as 300 GB fuller than the user's "real" data warrants.

The user needs to understand the difference between referenced content
(which can't be reclaimed without losing something) and purgatory
cache (which c4d can free instantly). And they need a way to act on
that understanding:

```
c4 du
  Active:      94 GB  (847 objects)
  Purgeable:  206 GB  (12031 objects)
  Total:      300 GB  (12878 objects)
  Limit:      500 GB  (60.0% used)
```

If the user wants more visible free space on their drive, they
lower the storage limit — purgatory shrinks to fit:

```
c4 config store.limit 100G
# c4d immediately reclaims purgatory to fit within 100 GB
# OS now sees ~200 GB freed
```

This is the only tuning knob most users need. Lower the limit to
free disk space; raise it to maximize cache. Referenced content is
never affected by the limit — if referenced content exceeds the limit,
c4d keeps it all and warns that purgatory is disabled.

## Observability

There is no `c4 gc` command. Instead, the user can observe:

```
c4 du                      # storage breakdown: referenced vs purgeable
c4 status                  # overall node health + storage summary
c4 status project.c4m:     # what this c4m keeps alive
c4 status :                # managed dir storage breakdown
```

`c4 du` is the primary storage tool — it shows the reachability
breakdown and makes the distinction between referenced content and
purgatory cache explicit.

If the user wants to force immediate reclaim (e.g., before shipping
a drive):

```
c4 rm --purge              # flush purgatory → gone (force reclaim)
```

This is still declarative — "flush the cache" is a disposition
statement, not a sweep command.

## Shred

`c4 rm --shred :secret.doc` is the nuclear option:

1. Remove entry from current managed state
2. Scrub the C4 ID from all local snapshots (rewrite history)
3. Delete content from local c4d store immediately (skip purgatory)
4. Write a **tombstone** for the C4 ID

### Tombstones

A tombstone is a signed record: "this C4 ID is rejected by this node."

- Prevents the local node from re-caching the content (blocks
  purgatory-to-active resurrection)
- Propagates to mesh peers during sync (lazy, batched)
- Checked at fetch time: receiving node rejects tombstoned content
- **Must be PKI-signed** — only the signing node's namespace is affected
- Other nodes' independently-referenced copies are unaffected

Tombstones are not global delete commands. They are "I don't want this
and don't send it to me." Each node's copy survives or dies based on
that node's own retention anchors.

### Tombstone Lifetime

Tombstones persist for a configurable duration (default: 1 year).
After expiration, the C4 ID is no longer actively rejected — but the
content is also long gone from this node's store. If content re-appears
via a sync after tombstone expiration, it's treated as new content.

For compliance shred (GDPR), tombstones are permanent and propagated
to the relay as a mesh-wide deny list.

## Safety Mechanisms

### Last-Copy Protection

Before content transitions from purgatory to gone, c4d checks whether
it holds the last known copy in the mesh (via bloom filter query to
connected peers).

If this node has the last copy:
- Reclamation is blocked for that content
- User is warned via `c4 status`: "N blobs are the last known copy"
- User must explicitly override: `c4 rm --force --purge`

This prevents the cascade scenario where unreplicated content is
permanently lost from a single node's policy decision.

### Purgatory-to-Active Resurrection

When a new c4m arrives (from remote sync, user establishment, etc.),
c4d checks whether any referenced blobs are in purgatory. If so,
those blobs are immediately resurrected to active status — no network
transfer needed.

This is the primary value of purgatory: making re-reference cheap.

### Legal Hold

A legal hold is not a special mechanism — it's an established path
with access-controlled un-establishment. Content under the path stays
alive through normal reachability. No override logic, no propagation
protocol, no new commands:

```
c4 mk holds/case-2026-001.c4m: --auth-required
c4 cp project-x.c4m: holds/case-2026-001.c4m:
```

Content is now reachable from `holds/case-2026-001.c4m:`. That path
can't be un-established without authorization. Content stays alive
through normal reachability rules — the same mechanism that keeps
any other established c4m alive.

For cross-node preservation: legal sends a directive (which could
itself be a c4m file listing what to preserve). Each node operator
establishes it locally. This is how legal holds work in every
enterprise system — email, file shares, databases. It's an
organizational process, not a technical protocol.

This aligns with c4's AP model — nodes are sovereign. Node A can't
force node B to preserve content. Each node's operator responds to
the legal directive by establishing the relevant content under an
access-controlled path on their node.

### Startup Safety

On c4d startup, a full reachability scan completes before any
purgatory reclamation is enabled. This prevents data loss when c4d
has been offline — content that was in purgatory before shutdown is
re-evaluated against current references (a new c4m may have arrived
while offline).

## Theoretical Foundation

### CAP Theorem

c4d must be partition-tolerant (mesh nodes go offline). For any
distributed deletion decision, the CAP theorem forces a choice:

- **CP:** All nodes agree on what's alive. One unreachable node blocks
  all reclamation. Impractical.
- **AP:** Each node decides locally. Nodes may temporarily disagree.

The retention model is AP — each node manages its own storage
independently. Disposition is a local decision affecting local
storage only.

### Game Theory

Each node's locally optimal strategy (keep what it values, dispose of
what it doesn't need) is also globally optimal. There's no
externality — one node's reclamation doesn't affect another node's
content. This is incentive-compatible without coordination. A Nash
equilibrium where every node acting in self-interest produces the best
collective outcome.

### Precedent

- **macOS/Windows Trash** — explicit disposition, visible grace period,
  automatic emptying
- **S3 Lifecycle Rules** — declarative per-bucket retention policies
- **IPFS** — content persists unless unpinned; GC only collects
  unpinned content
- **OS filesystem cache** — invisible, automatic, storage-pressure-driven
- **Your hard drive** — files stay until you delete them

## Implementation

### Reachability Engine

c4d maintains continuous awareness of what's reachable:

1. The establishment registry is the root set
2. Each established c4m's referenced C4 IDs are tracked
3. Managed directory snapshots within retention windows are tracked
4. TTL-bearing paths are tracked with expiration times
5. Purgatory is the complement: stored blobs not in any of the above

This is a local computation — no distributed coordination needed.
Incremental updates on each Put/Rm/establish/un-establish operation,
with periodic full recomputation for consistency.

### Store Interface

The `Store` interface stays minimal — Put/Get/Has/Delete only.
Iteration and statistics are implementation-specific:

```go
// Store interface is unchanged — no List, no Stats.
type Store interface {
    Put(r io.Reader) (c4.ID, error)
    Get(id c4.ID) (io.ReadCloser, error)
    Has(id c4.ID) bool
    Delete(id c4.ID) error
}

// FileStore adds Walk for iteration (not on the interface —
// iteration is storage-backend-specific).
func (s *FileStore) Walk(fn func(c4.ID) error) error

// Reachability engine computes stats against a referenced set.
type Stats struct {
    ReferencedCount   int64
    ReferencedBytes   int64
    UnreferencedCount int64
    UnreferencedBytes int64
}
```

### TTL-Bearing Path Registration

Paths with TTLs are registered in the establishment registry with
an expiration time:

```go
type Registration struct {
    Path      string
    C4ID      c4.ID
    CreatedAt time.Time
    ExpiresAt *time.Time  // nil = permanent
}
```

When `ExpiresAt` is reached, the path is automatically un-established.

### Purgatory Management

Purgatory is not a separate store. It's a metadata state on blobs in
the existing store:

```go
type BlobState struct {
    StoredAt      time.Time
    State         string    // "active", "purgatory"
    PurgatoryAt   *time.Time
    TombstonedAt  *time.Time
}
```

c4d manages purgatory reclamation via a tunable pressure curve.
The input is the ratio of total storage (active + purgatory)
to the configured storage limit. The output is how aggressively
purgatory blobs are reclaimed — from "keep everything" when well
under the limit, to "reclaim immediately" when at the limit.

The curve is configurable but ships with reasonable defaults. The
exact shape will be refined through testing and early user feedback.
If referenced content alone exceeds the limit, purgatory is
fully disabled and c4d warns the user.

These thresholds are configurable.

### HTTP API Changes

- `GET /du` — reachability breakdown (referenced/unreferenced counts and bytes)
- `410 Gone` for tombstoned content (future)
- No `POST /admin/gc` — reclamation is automatic

## Failure Mode Analysis

### Addressed by design

| Scenario | Mitigation |
|----------|-----------|
| Malicious tombstone injection | PKI-signed, namespace-scoped |
| Simultaneous reclaim across nodes | Last-copy protection |
| Put-before-reference race | Content starts active; purgatory only after unreference |
| Crash during snapshot prune | Atomic writes; prune either completes or doesn't |
| Accidental rm in managed dir | Undo + snapshot history; content recoverable |
| Crash during purgatory reclaim | Idempotent delete; incomplete reclaim is safe |
| Disk full prevents reclaim | Reclaim doesn't write; it only deletes |
| Clock skew | TTLs are local; no cross-node comparison |
| New c4m during purgatory | Resurrection: purgatory → active (no transfer) |

### Accepted tradeoffs

| Tradeoff | Rationale |
|----------|-----------|
| Storage grows until disposition | Safe default; user controls when to dispose |
| Purgatory uses space for cache | Saves network transfers; auto-sized by pressure |
| Reachability requires parsing c4m files | Local operation; incremental optimization possible |
| Last-copy check requires peer connectivity | Degrades to warning if peers unreachable |
| Auth-required paths add access control | Required for enterprise; uses existing path model |

## Tuning Parameters (Defaults TBD Through Testing)

- **Purgatory pressure curve:** Tunable. Ship a simple curve, refine
  shape (linear, exponential, sigmoid) based on real usage patterns.
- **Snapshot auto-tagging threshold:** Tied to `--snapshot-threshold`.
  Threshold-triggered snapshots are auto-tagged. Default count TBD.
