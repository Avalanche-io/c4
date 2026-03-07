# Content Retention: It's Just a Hard Drive, Until It Isn't

> You never expect files to disappear from your hard drive.

## The Mental Model

c4 storage works like your laptop's hard drive. You put files there,
they stay there. You delete files when you choose to. Nothing
disappears on its own. If you run out of space, you decide what to
remove — the system doesn't decide for you.

This is the only sane default. Surprising deletion is worse than
running out of space. You can always add storage. You can't un-delete.

## Why Content-Addressed Storage Is Different

In a traditional filesystem, editing a file overwrites it in place.
Change one byte of a 10 GB file and you still have one 10 GB file.

In a content-addressed store, every version is a distinct object.
Change one byte and you have two 10 GB objects. Every saved draft,
every re-render, every minor tweak is a completely new blob. Storage
growth in a CAS is far more aggressive than filesystem intuition
suggests.

This means "you can always add storage" is true but under real
pressure. Even with cheap storage and compression strategies, you
can't keep everything forever without thinking about it. The question
isn't whether to manage retention — it's how to do it without
surprising anyone.

## The Design Goal: Sparse History, Durable Retention

What people actually want from storage:

- **The current state.** Obviously — the files you see today.
- **Selected prior states.** A few recent versions. Tagged milestones.
  The state before a big re-render. Not every intermediate keystroke.
- **Redundancy across locations.** If one copy is lost, another exists
  somewhere in the mesh.
- **Explicit control.** You decide what stays and what goes. The system
  helps you understand what's using space, but never acts without you.

This is your local hard drive with backups. The familiar mental model.

## Two Zones

### Zone 1: Primary Storage — Everything That Fits, Sits

Your c4d node's primary storage behaves like a hard drive. Content
referenced by any active anchor is kept. Unreferenced content
accumulates until you clean it up. No TTL. No automatic expiration.
No surprises.

When you need space, you run GC:

```
c4 gc                         # interactive: review what's reclaimable
c4 gc --dry-run               # show what would be reclaimed
c4 gc --confirm               # actually reclaim
```

"Reclaimable" means content not referenced by any established c4m,
managed directory, registered location, pin, or legal hold.

GC is the broom. You pick it up when you want to sweep. It doesn't
sweep on its own.

### Zone 2: Caches and Transient Storage — TTL and Eviction

Some storage is explicitly transient. Relay caches, CI/CD artifacts,
temp namespaces — these are places where bounded growth is the whole
point. Here, TTL and eviction policies make sense:

```
c4 mk builds: --retain 30d           # build artifacts expire after 30 days
c4 mk tmp: --retain 7d               # temp namespace expires after 7 days
c4 mk ci-output.c4m: --retain 14d    # this c4m auto-unregisters in 14 days
```

When a retention policy is set:
1. The registration carries an expiration
2. When the registration expires, it auto-unregisters
3. Content referenced **only** by expired registrations becomes
   reclaimable
4. GC reclaims it (either background or on-demand)

Data in Zone 2 can still survive indefinitely if there's room.
TTL marks content as *eligible* for reclaim, not condemned. If nobody
needs the space, nothing has to die.

### How the Zones Interact

```
                  +------------------+
                  |  Content Blob    |
                  |    (C4 ID)       |
                  +--------+---------+
                           |
              referenced by?
                           |
            +--------------+--------------+
            |              |              |
     +------+------+ +----+-----+ +------+------+
     |  Zone 1:    | |  Zone 1: | |  Zone 2:    |
     | Established | | Managed  | |  Transient  |
     |   c4m file  | |   Dir    | |  Namespace  |
     |             | |          | | (--retain)  |
     +------+------+ +----+----+ +------+------+
            |              |            |
          keeps          keeps      keeps until
         forever       per config   policy expires
```

Content is alive if ANY anchor keeps it. Content is reclaimable only
when ALL anchors have released it.

## Snapshot Retention: Sparse History in Practice

Managed directories accumulate snapshots — each `c4 sync` creates
one. Without limits, snapshot history would grow as aggressively as
the content it references.

Snapshot retention controls implement sparse history:

```
c4 mk : --snapshot-retain 10         # keep last 10 snapshots
c4 mk : --snapshot-retain tagged     # keep only tagged snapshots
c4 mk : --snapshot-retain 30d        # keep snapshots younger than 30 days
```

Default: last 10 snapshots + all tagged snapshots.

Snapshots within the retention window keep their referenced content
alive. Snapshots outside the window are pruned — their unique content
becomes reclaimable if not referenced by anything else.

**Auto-tagging on significant changes:** When a large batch of entries
is replaced (e.g., VFX re-render), the system auto-tags the
pre-change state. Tagged snapshots survive beyond the retention window,
giving you the selected prior states that matter without keeping
every intermediate version.

## Retention Anchors

From strongest to weakest:

| Anchor | Lifetime | Release mechanism |
|--------|----------|-------------------|
| **Legal hold** | Indefinite, overrides all | `c4 unhold` (requires auth) |
| **Explicit pin** | Indefinite | `c4 unpin` |
| **Established c4m** (no policy) | Until un-established | `c4 rm project.c4m:` |
| **Managed dir current** | While managed | `c4 rm :` (teardown) |
| **Tagged snapshot** | While tag exists | `c4 rm :~tagname` |
| **Managed dir snapshot** (in window) | Within retention config | Automatic pruning |
| **Transient namespace** | Until policy expires | Automatic expiration |
| **Location registration** | While registered | `c4 rm location:` |

The **establishment registry** is the root set. Everything reachable
from an active root is alive. `c4 gc` computes reachability from roots
and reclaims the rest.

## Shred

`c4 rm --shred :secret.doc` is the nuclear option:

1. Remove entry from current managed state
2. Scrub the C4 ID from all local snapshots (rewrite history)
3. Delete content from local c4d store immediately
4. Write a **tombstone** for the C4 ID

### Tombstones

A tombstone is a signed record: "this C4 ID is rejected by this node."

- Prevents the local node from re-caching the content
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

Before GC reclaims content, c4d checks whether it holds the last
known copy in the mesh (via bloom filter query to connected peers).

If this node has the last copy:
- GC is blocked for that content
- User is warned: "c4abc... is the last known copy in the mesh"
- User must explicitly force deletion: `c4 gc --force`

This prevents the cascade scenario where unreplicated content is
permanently lost from a single node's policy decision.

### Grace Period on Unregistration

When a c4m is un-established (`c4 rm project.c4m:`), its content
does NOT immediately become reclaimable. A grace period (default:
30 days) applies:

```
c4 rm project.c4m:
> "project.c4m unregistered. Content protected for 30 days.
   Re-establish with: c4 mk project.c4m:"
```

During the grace period, `c4 mk project.c4m:` fully restores the
registration. After the grace period, content becomes reclaimable
by `c4 gc`.

### Legal Hold

For compliance and litigation:

```
c4 hold project-x.c4m          # place under legal hold
c4 hold --status                # show all holds
c4 unhold project-x.c4m        # release (requires auth)
```

Legal hold:
- Overrides ALL retention policies (decay, GC, shred)
- Propagates to mesh peers: "do not delete this content"
- Is logged with timestamp, identity, and scope
- Cannot be released without authorization
- Referenced content is immune to GC until hold is released

### Startup Safety

On c4d startup, a full reachability scan completes before GC is
enabled. This prevents data loss when c4d has been offline — content
that was reclaimable before shutdown is re-evaluated against current
registrations.

## Theoretical Foundation

### CAP Theorem

c4d must be partition-tolerant (mesh nodes go offline). For any
distributed deletion decision, the CAP theorem forces a choice:

- **CP:** All nodes agree on what's alive. One unreachable node blocks
  all GC. Impractical.
- **AP:** Each node decides locally. Nodes may temporarily disagree.

The retention model is AP — each node manages its own storage
independently. Zone 1 and Zone 2 are local decisions affecting local
storage only.

### Game Theory

Each node's locally optimal strategy (keep what it values, reclaim
what it doesn't need) is also globally optimal. There's no
externality — one node's GC doesn't affect another node's content.
This is incentive-compatible without coordination. A Nash equilibrium
where every node acting in self-interest produces the best collective
outcome.

### Precedent

- **S3 Lifecycle Rules** — opt-in per-bucket expiration policies
- **IPFS** — content persists unless unpinned; GC only collects
  unpinned content
- **Git** — objects persist until `git gc` (explicit, not automatic)
- **Your hard drive** — files stay until you delete them

## GC Implementation

### Reachability Computation

GC computes reachability from roots (the establishment registry):

1. Read all active registrations (c4m files, managed dirs, locations)
2. Parse each c4m to extract referenced C4 IDs
3. Expand sequence data blocks to get individual frame C4 IDs
4. Mark all reachable C4 IDs as alive
5. Everything else is reclaimable

This is a local tracing GC — no distributed coordination needed.

### Storage Metadata

Each object in the content store gets lightweight metadata:

```json
{
  "stored_at": "2026-03-06T21:00:00Z",
  "last_ref": "2026-03-06T21:00:00Z",
  "tombstone": false,
  "pinned": false,
  "hold": false
}
```

For S3-backed stores, use native object tags.

### c4d Changes Required

1. **Store interface:** Add `ListAll`, `GetMetadata`, `SetMetadata`
2. **Registry:** Already exists (establishment). Add grace periods,
   retention policies, legal holds.
3. **GC engine:** New package. Tracing reachability + reclaim.
4. **Background tasks:** Optional periodic GC for transient namespaces.
5. **HTTP API:** `POST /admin/gc`, `GET /admin/stats`,
   410 Gone for tombstoned content.

## Failure Mode Analysis

### Addressed by design

| Attack | Mitigation |
|--------|-----------|
| Malicious tombstone injection | PKI-signed, namespace-scoped |
| Simultaneous GC across nodes | Last-copy protection + jittered schedules |
| Renewal depends on GC-able content | Registry pins c4m blobs |
| Put-before-reference race | Content kept by default; no TTL on initial PUT |
| Crash during GC | GC is idempotent; incomplete sweep is safe |
| Accidental unreference | 30-day grace period on unregistration |
| Disk-full blocks GC | GC just doesn't run; content accumulates (safe direction) |
| Clock skew | Irrelevant — no cross-node TTL comparison |
| Content flooding | Local-only; quotas per source |

### Accepted tradeoffs

| Tradeoff | Rationale |
|----------|-----------|
| Storage grows without explicit cleanup | Safe default; user controls when to reclaim |
| GC requires parsing all c4m files | Local operation; incremental optimization possible |
| Last-copy check requires peer connectivity | Degraded to warning if peers unreachable |
| Legal hold adds complexity | Required for any enterprise deployment |

## Open Questions

1. **GC frequency for transient namespaces:** Should Zone 2 decay
   run as a background daemon, or only when `c4 gc` is invoked?
   Daemon is more automatic; on-demand is more predictable.

2. **Snapshot auto-tagging heuristic:** When should the system
   auto-tag before a destructive change? Threshold: >10% of entries
   modified? Any sequence replacement? User preference?

3. **Cross-node hold propagation:** How does a legal hold propagate
   through the mesh? Same mechanism as tombstones (lazy gossip)?
   Or immediate (blocking)?

4. **Quota interaction:** When a node exceeds its storage quota, which
   content is reclaimed first? Transient namespace content before
   established content? Oldest unreferenced before newest?

5. **`c4 gc` UX:** Interactive review mode? Show reclaimable content
   grouped by source c4m? Allow selective reclaim ("keep this, delete
   that")?
