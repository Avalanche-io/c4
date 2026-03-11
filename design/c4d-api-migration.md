# c4d API Migration: Namespace-Native Everything

## Goal

Migrate c4d from a mixed REST/namespace API to one where
nearly all state and configuration is expressed as namespace
content (c4m documents at paths). The only traditional HTTP
endpoints that survive are integration points that external
systems expect to find at well-known paths.

**Nothing is removed.** Channels, scanning, GC, peer
management — all of it keeps working. The change is the
interface: instead of REST endpoints with JSON bodies, every
feature is controlled by documents in the namespace. The
c4d daemon watches those paths and acts on changes.

## Why: The Asynchronous Mesh

The real payoff isn't API cleanliness — it's that namespace
documents propagate through flow channels. This means mesh
configuration is eventually consistent without requiring
real-time coordination between nodes.

**Synchronous (current REST model):**
You want to create a channel between abyss and beast2. Both
nodes must be online. You POST JSON to abyss's channel
endpoint. Then you POST to beast2's. If beast2 is offline,
you wait, retry, or script around it.

**Asynchronous (namespace model):**
You write a channel definition to `/etc/channels/ch-xyz/`
on abyss. That `/etc/` subtree syncs to beast2 through an
existing flow channel. Beast2 receives the document whenever
it next comes online — could be seconds, could be days. Its
namespace watcher fires, the flow worker starts. No
coordination needed. No retries. No "is the other node up?"

This applies to everything:

- **Channels**: Create on one node, propagate to all. Each
  node's watcher starts the appropriate flow workers when
  the channel definition arrives.
- **Scans**: An admin writes a scan definition once. It
  flows to every node in the mesh. Each node decides locally
  whether to act on it (based on the scan's target path).
- **Peers**: Add a peer on your laptop. The entry flows to
  your NAS, your desktop, your cloud VM. They all learn
  about the new peer without you logging into each one.
- **Trust**: Add a CA cert on one node. It flows to all
  nodes. The entire mesh trusts the new CA without per-node
  configuration.
- **Stats**: Each node writes its own stats to its own
  `/etc/stats/`. Those stats flow to other nodes. Any node
  can see the health of any other node by reading its
  namespace — even if that node is currently offline (you
  see its last-known state).

The mesh doesn't need a coordination protocol. The namespace
sync IS the coordination protocol. Nodes that are offline
catch up when they reconnect. Nodes that are partitioned
converge when the partition heals. Configuration changes are
idempotent (content-addressed — same document = same C4 ID =
no-op on arrival).

This is the difference between "distributed system with an
API" and "distributed filesystem where configuration is just
files." Unix got this right with `/etc/` fifty years ago.
c4d extends it across the mesh.

## Current State

c4d has three distinct API styles today:

1. **Content plane** — `GET/HEAD/PUT/DELETE /{c4id}` — already
   content-addressed, already right. No changes needed.

2. **Namespace plane** — `GET/PUT/DELETE {path}` — already
   namespace-native, already right. Minor enhancements needed.

3. **REST/JSON endpoints** — channels, scans, admin, stats,
   location proxy — these are traditional REST with JSON
   request/response bodies. These need to migrate.

## Target State

### Traditional Endpoints (Keep)

These stay as conventional HTTP endpoints because external
systems expect them at fixed paths with standard formats:

| Method | Path | Format | Purpose |
|--------|------|--------|---------|
| `GET` | `/health` | JSON | Health check (k8s probes, load balancers) |
| `GET` | `/metrics` | Prometheus | Prometheus scrape target (new) |
| `GET` | `/version` | JSON | Version info for compatibility |
| `GET` | `/versions` | JSON | Live mesh node inventory (pings peers) |
| `POST` | `/join` | PEM | Pre-auth join request (mesh-trust Phase 1) |
| `GET` | `/join/{name}` | PEM | Poll for signed cert (mesh-trust Phase 1) |

**Why these stay traditional:**
- `/health` — must work with any HTTP health checker, no c4m
  knowledge required. k8s, ELB, HAProxy all expect JSON + status codes.
- `/metrics` — Prometheus expects its exposition format at a
  well-known path. Non-negotiable for monitoring integration.
- `/version` — version negotiation between peers and CLI needs
  a lightweight, universally parseable response.
- `/versions` — live peer query, not cached state. Imperative
  by nature (it pings peers on every call).
- `/join` — pre-authentication. The joiner has no trusted cert
  yet, so it can't access the namespace. This is the only
  endpoint that relaxes mTLS, and only for CSR submission.

### Content Plane (No Change)

```
GET    /{c4id}     fetch content blob
HEAD   /{c4id}     check existence + size
PUT    /           store blob, return C4 ID
DELETE /{c4id}     delete blob
```

Already content-addressed. Already right.

### Namespace Plane (Enhanced)

```
GET    {path}      resolve path → C4 ID (text/c4m)
GET    {path}/     list directory (text/c4m)
PUT    {path}      set path → C4 ID (CAS via If-Match)
DELETE {path}      remove path
```

Already namespace-native. Enhancements:
- Remove `/mnt/` restriction on PUT/DELETE (Phase 2+ needs
  writes to `/etc/`)
- Add write authorization model (which identities can write
  which paths)
- Upgrade directory listing to full c4m format (currently
  simplified `name\tC4ID`)

### Namespace-Native Configuration (Migrate)

Everything currently behind REST/JSON endpoints moves into
the namespace under `/etc/`. c4d watches these paths and
reconfigures itself when they change.

---

## Migration Plan

### Phase 1: Channels → `/etc/channels/`

**Current API (remove):**
```
GET    /etc/channels           → JSON array of channels
POST   /etc/channels           → JSON body, create channel
GET    /etc/channels/{id}      → JSON channel object
POST   /etc/channels/{id}/approve  → approve
POST   /etc/channels/{id}/reject   → reject
DELETE /etc/channels/{id}      → delete
```

**Namespace-native (add):**
```
/etc/channels/
  {channel-id}/
    direction       → "outbound" | "inbound" | "bidirectional"
    location        → peer name (e.g. "nas")
    source          → C4 ID of source c4m (for outbound)
    local_path      → namespace path (e.g. "/mnt/studio/renders")
    status          → "pending" | "approved" | "rejected"
```

**How it works:**
- Creating a channel: write entries to `/etc/channels/{id}/`
  (the CLI does this via `c4 ln`, which tells c4d to create
  the channel — c4d writes the namespace entries)
- Approving: update `/etc/channels/{id}/status` to "approved"
- c4d watches `/etc/channels/` — when status becomes
  "approved", starts the flow engine worker
- Channel listing is just `c4 ls etc/channels/`
- Channel deletion is `c4 rm etc/channels/{id}`

**CLI surface:**
```bash
c4 ls etc/channels/              # list channels
c4 ls etc/channels/ch-abc123/    # inspect one
c4 approve ch-abc123             # sugar for status→approved
c4 reject ch-abc123              # sugar for status→rejected
c4 rm etc/channels/ch-abc123     # delete
```

**Implementation steps:**
1. Add namespace watcher for `/etc/channels/` in c4d
2. On status change to "approved": start flow worker (reuse
   existing flow engine code)
3. On status change to "rejected" or path deletion: stop
   flow worker
4. Channel auto-discovery from @intent: write to
   `/etc/channels/` instead of channel store JSON
5. Remove `internal/server/channels.go` REST handlers
6. Remove `channel.Store` JSON file persistence
7. Update `c4 ln` to write namespace entries via local c4d
8. Update channel rebuild-from-namespace to read from
   `/etc/channels/` subtree instead of JSON file

**What stays:** The flow engine, bidirectional reconciliation,
@intent auto-discovery, content push/pull — all unchanged.
Only the control interface changes (namespace documents
instead of REST/JSON). The flow engine reads channel config
from namespace instead of a JSON file.

**Async mesh benefit:** A channel definition written on one
node propagates to peers through `/etc/` sync. The remote
node's watcher picks it up and starts its side of the flow
automatically — no REST call to the remote node needed.

### Phase 2: Scans → `/etc/scans/`

**Current API (remove):**
```
POST   /etc/scans           → JSON body, start scan
GET    /etc/scans           → JSON array of jobs
GET    /etc/scans/{id}      → JSON job status
DELETE /etc/scans/{id}      → cancel scan
```

**Namespace-native (add):**
```
/etc/scans/
  {scan-id}/
    root            → filesystem path to scan
    out_path        → namespace output path
    level           → "0" | "1" | "2"
    status          → "pending" | "running" | "complete" | "failed" | "cancelled"
    progress        → "142/500 files" (updated during scan)
    error           → error message (if failed)
    started         → ISO timestamp
    completed       → ISO timestamp (when done)
```

**How it works:**
- Starting a scan: write entries to `/etc/scans/{id}/`
- c4d watches `/etc/scans/` — when a new scan appears with
  status "pending", starts the scan job
- c4d updates `status`, `progress`, `completed` as the scan
  runs (namespace mutations visible to any watcher)
- Cancelling: set status to "cancelled" (c4d stops the job)
- `c4 ls etc/scans/` lists all scan jobs with their status

**CLI surface:**
```bash
c4 scan /mnt/storage --out /projects/backup --level 2
# Creates /etc/scans/{generated-id}/ entries, returns ID

c4 ls etc/scans/                   # list jobs
c4 ls etc/scans/scan-abc123/       # inspect progress
c4 rm etc/scans/scan-abc123        # cancel
```

**What stays:** The scan engine, filesystem walking, c4m
generation, progress tracking — all unchanged. Only the
trigger and status reporting change (namespace documents
instead of REST/JSON).

**Async mesh benefit:** Write a scan definition on any node.
It propagates to all nodes. Each node that matches the scan's
root path runs the scan locally. Scan results (the output
c4m) propagate back through flow channels. A central admin
node can trigger scans across the entire mesh by writing one
document.

**Implementation steps:**
1. Add namespace watcher for `/etc/scans/` in c4d
2. On new scan with status "pending": start scan goroutine
   (reuse existing scan manager code)
3. Scan goroutine writes progress/status to namespace
4. On status set to "cancelled": cancel scan context
5. Remove `internal/server/scan.go` REST handlers
6. Update CLI to write namespace entries instead of POST JSON
7. Remove scan manager's internal job tracking (namespace IS
   the job state)

### Phase 3: Admin/GC → `/etc/admin/`

**Current API (remove):**
```
POST   /admin/purge     → trigger GC, return JSON results
```

**Namespace-native (add):**
```
/etc/admin/
  gc/
    last_run          → ISO timestamp of last GC
    last_result/
      checked         → blob count examined
      live            → reachable blob count
      collected       → deleted blob count
      freed_bytes     → bytes reclaimed
    auto              → "true" (automatic GC enabled)
```

**How it works:**
- GC continues to run automatically based on storage pressure
  (current behavior, no change)
- After each GC run, c4d writes results to `/etc/admin/gc/`
- Manual GC trigger: `c4 gc` (CLI command that tells c4d to
  run GC now — implemented as a namespace write to a trigger
  path, or as a thin RPC since it's imperative)

**Decision:** GC trigger is inherently imperative ("run now"),
not declarative ("desired state"). Two options:

Option A: Keep `POST /admin/purge` as a traditional endpoint.
It's internal-only (CLI to local c4d), not an external
integration point.

Option B: The CLI writes a one-time token to
`/etc/admin/gc/trigger`. c4d watches, runs GC, deletes the
trigger, writes results. Pure namespace.

**Recommendation:** Option A. GC is a command, not
configuration. The result reporting moves to namespace
(`/etc/admin/gc/last_result/`), but the trigger stays as a
thin endpoint. This avoids contorting an imperative action
into a declarative pattern.

**Implementation steps:**
1. After GC runs, write results to `/etc/admin/gc/` namespace
2. Keep `POST /admin/purge` but simplify response (point
   callers to namespace for detailed results)
3. Remove JSON response body from purge endpoint (return 204)
4. Add `c4 gc` CLI command that calls purge then reads results
   from namespace

### Phase 4: Stats → `/etc/stats/`

**Current API (remove):**
```
GET    /du      → JSON disk usage breakdown
```

**Namespace-native (add):**
```
/etc/stats/
  store/
    total_blobs       → count
    total_bytes       → size
    referenced_blobs  → count
    referenced_bytes  → size
    unreferenced_blobs → count
    unreferenced_bytes → size
    max_bytes         → configured limit (0 = unlimited)
  pressure/
    state             → "normal" | "elevated" | "high" | "critical"
    utilization       → "0.73" (ratio)
  gc/
    last_run          → timestamp
    ...               → (from Phase 3)
```

**How it works:**
- c4d periodically updates `/etc/stats/` (on pressure monitor
  tick, every 10-60s depending on state)
- `c4 ls etc/stats/store/` shows storage stats
- `c4 ls etc/stats/pressure/` shows pressure state
- External monitoring reads these the same way — they're
  namespace paths, accessible via `GET /etc/stats/store/`
  which returns c4m

**Note:** `/health` endpoint stays traditional (HTTP status
codes matter for health checks). `/etc/stats/` is the detailed
view; `/health` is the integration-friendly summary.

**Implementation steps:**
1. Add periodic stats writer in c4d (writes to namespace on
   pressure monitor tick)
2. Remove `handleDU` REST handler
3. Update CLI to read stats from namespace
4. Wire pressure state into `/etc/stats/pressure/`

### Phase 5: Peer Management → `/etc/peers/`

**From mesh-trust.md design. Not a migration — new feature.**

```
/etc/peers/
  {name}/
    address         → host:port
    ca              → CA name
    fingerprint     → cert fingerprint (TOFU)
    enabled         → "true" | absent
```

**How it works:**
- c4d watches `/etc/peers/` for changes
- New peer entry → connect to peer
- Removed entry → disconnect
- Current config.yaml peers migrate to namespace on first run

**Implementation steps:**
1. Add namespace watcher for `/etc/peers/` in c4d
2. On peer entry change: update peer client pool
3. One-time migration from config.yaml peers to namespace
4. Update CLI peer commands to write namespace entries

### Phase 6: Identity → `/etc/identity/`, `/etc/ca/`

**From mesh-trust.md design. Not a migration — new feature.**

```
/etc/identity/
  cert              → node certificate (PEM)
  name              → node name
  owner             → owner identity
/etc/ca/
  {name}            → trusted CA certificate (PEM)
```

**Implementation steps:**
1. `c4 init` writes identity and CA entries to namespace
2. c4d reads TLS trust pool from `/etc/ca/*`
3. c4d reads node cert from `/etc/identity/cert`
4. One-time migration from config.yaml TLS paths

### Phase 7: Location Proxy → Namespace Mounts

**Current:** `/~{location}/{path}` proxies to named peer.

**Target:** Remote locations are transparent namespace mounts.
`/mnt/nas/renders/` works regardless of whether `nas` is local
or remote. c4d resolves the location and proxies transparently.

The `~` prefix routing is an implementation detail that should
be invisible to the user. When the CLI does
`c4 ls nas:renders/`, it resolves to `GET /mnt/nas/renders/`
on the local c4d, which proxies to the nas peer.

**Implementation steps:**
1. Location resolver integrated into namespace GET handler
2. When GET hits a path under a remote location mount, proxy
   to the peer (current proxy logic, different dispatch)
3. Same for PUT and DELETE
4. Remove `/~{location}/` prefix routing
5. Update CLI to use `/mnt/{location}/` paths

**Note:** This is already mostly how the CLI thinks about it.
The `~` prefix was a server-side routing shortcut. Making
locations first-class namespace mounts removes the shortcut
in favor of the real thing.

---

## Namespace Watcher Architecture

Multiple phases depend on c4d watching namespace subtrees.
This needs a single, clean mechanism.

### Design

```go
// Watcher subscribes to namespace mutations under a prefix.
type Watcher struct {
    prefix  []string          // e.g. ["etc", "channels"]
    handler func(tx *db.Tx)   // called inside the commit
}
```

c4d registers watchers at startup:
```go
db.Watch("etc/channels", channelWatcher)
db.Watch("etc/scans", scanWatcher)
db.Watch("etc/peers", peerWatcher)
db.Watch("etc/ca", tlsWatcher)
db.Watch("etc/admin/gc", gcWatcher)
```

On every namespace commit, c4d checks which prefixes were
affected (efficient: Merkle tree tells you exactly which
subtree changed). Only matching watchers fire.

### Implementation

The DB already has `Watch() <-chan struct{}` for commit
notifications. Extend this to prefix-filtered watches:

```go
func (db *DB) WatchPrefix(prefix string) <-chan struct{}
```

Or, since watchers need to read the new state:

```go
func (db *DB) OnCommit(prefix string, fn func(snap *Snapshot))
```

The watcher receives a snapshot of the committed state scoped
to its prefix. It reads the subtree and acts. This is the
same pattern as Unix inotify but for the namespace Merkle tree.

---

## Phase Ordering

```
Phase 1: Channels → /etc/channels/
  (removes channels.go, channel JSON store)

Phase 2: Scans → /etc/scans/
  (removes scan.go, scan manager internal state)

Phase 3: Admin/GC → /etc/admin/
  (simplifies purge.go, adds result reporting)

Phase 4: Stats → /etc/stats/
  (removes du.go, adds periodic stat writer)

Phase 5: Peers → /etc/peers/
  (new: namespace-driven peer management)

Phase 6: Identity → /etc/identity/, /etc/ca/
  (new: namespace-driven trust, from mesh-trust.md)

Phase 7: Location proxy → namespace mounts
  (removes proxy.go, integrates into namespace handler)
```

Phases 1-4 are independent migrations of existing REST
endpoints. They can be done in any order but this ordering
minimizes risk (channels are most isolated, stats are most
visible).

Phase 5-6 are new features from mesh-trust.md. They depend
on the namespace watcher infrastructure built in Phases 1-4.

Phase 7 depends on Phase 5 (peer management must be in
namespace before location mounts work).

---

## What Gets Deleted

After all phases complete, these files/features are removed:

| File | What it was |
|------|-------------|
| `internal/server/channels.go` | REST channel handlers |
| `internal/server/scan.go` | REST scan handlers |
| `internal/server/proxy.go` | `/~location/` proxy routing |
| `internal/server/du.go` | REST disk usage handler |
| `internal/server/purge.go` | Simplified (keep trigger, remove JSON response) |
| `internal/channel/store.go` | JSON file channel persistence |
| `internal/scan/manager.go` | Internal scan job tracking |

The `internal/server/server.go` dispatch shrinks from ~15
route patterns to:

```
1. /health, /metrics, /version, /versions  (traditional)
2. /join, /join/{name}                      (pre-auth bootstrap)
3. PUT /                                    (store content)
4. {c4id} routes                            (content plane)
5. {path} routes                            (namespace plane)
```

Five dispatch categories. Everything else is namespace content
that c4d watches and reacts to.

## What Gets Added

| Component | Purpose |
|-----------|---------|
| Namespace watcher | Prefix-filtered commit notifications |
| Channel watcher | Starts/stops flow workers on `/etc/channels/` changes |
| Scan watcher | Starts/stops scan jobs on `/etc/scans/` changes |
| Peer watcher | Updates peer connections on `/etc/peers/` changes |
| TLS watcher | Rebuilds trust pool on `/etc/ca/` changes |
| Stats writer | Periodic namespace updates at `/etc/stats/` |
| Prometheus exporter | `/metrics` endpoint (new) |
| Join handler | Pre-auth CSR exchange (from mesh-trust.md) |

## Metrics (Prometheus)

New `/metrics` endpoint exposing:

```
c4d_blobs_total{state="referenced|unreferenced"}
c4d_blobs_bytes{state="referenced|unreferenced"}
c4d_store_max_bytes
c4d_pressure_state   (gauge: 0=normal, 1=elevated, 2=high, 3=critical)
c4d_namespace_entries_total
c4d_peers_connected
c4d_channels_active
c4d_gc_runs_total
c4d_gc_collected_total
c4d_gc_freed_bytes_total
c4d_requests_total{method, path_prefix}
c4d_request_duration_seconds{method, path_prefix}
```

Standard Prometheus client library. Scraped at `/metrics`.
This is the monitoring integration point — everything else
is readable from the namespace, but Prometheus needs its
native format.

## Summary

Nothing is lost, everything is gained. Channels still work.
Scans still work. GC still works. Peer management still
works. Every feature keeps its full implementation — the
flow engine, the scan walker, the pressure monitor, the
content resolution cascade. None of that changes.

What changes is the control plane. Instead of REST endpoints
with JSON bodies, every feature is controlled by documents
at namespace paths under `/etc/`. c4d watches those paths
and acts on changes.

The c4d API surface collapses from ~25 endpoint patterns
to 5 dispatch categories + namespace watchers. External
integrations (health, metrics, version, join) keep their
traditional endpoints because those consumers can't speak
c4m.

The payoff: **the mesh becomes asynchronous.** Configuration
documents propagate through flow channels the same way
content does. A channel created on one node arrives at its
peer whenever the peer next syncs — no real-time
coordination, no "are you online?", no retry loops. Nodes
that are offline catch up. Partitioned nodes converge. The
namespace IS the coordination protocol.

Unix figured this out fifty years ago: configuration is
files, daemons watch files, changes propagate. c4d extends
the pattern across the mesh with content-addressing providing
the consistency guarantee that local filesystems never had.

## Deferred: Sneakernet Sync Workflow

Sneakernet is the ultimate async mesh — latency measured in
shoe leather. The namespace-document model handles it
naturally, but the PKI and trust workflow needs a structured
design.

**The scenario:** Two c4d meshes that cannot or should not be
network-connected. A shuttle drive carries content between
them. Both meshes have their own CAs.

**What needs design:**

- **Cross-CA trust for bundles**: When mesh A exports a
  bundle for mesh B, the bundle needs to carry enough trust
  material for B to verify the content came from A. Options:
  A's CA cert in the bundle (B trusts it for import), a
  shared signing key, or a pre-exchanged trust anchor.

- **Bundle signing**: The c4m in the bundle should be signed
  by the exporting node's cert. The receiver verifies the
  signature before importing. This is chain of custody — the
  c4m is a signed shipping manifest.

- **Structured ceremony**: `c4 bundle` and `c4 import` need
  a clear PKI workflow. Who signs? What does the receiver
  verify? What if the receiver doesn't trust the sender's
  CA? What's the approval step?

- **Incremental sneakernet**: First drive carries the full
  project + trust material. Subsequent drives carry only
  deltas. The c4m diff is the delta manifest. CAS
  deduplication handles the rest.

- **Air-gapped mesh join**: A new node joins a mesh that has
  no network. The invite + CA cert + signed node cert travel
  on a USB stick. `c4 join --bundle /mnt/usb/` instead of
  `c4 join host:port`.

- **Bidirectional shuttle**: Drive goes out with content,
  comes back with updated content. Both directions carry
  namespace state. Merge semantics for `/etc/` when the
  drive returns.

This is a natural extension of mesh-trust.md Phase 1 and the
bundle/import design in mesh-implementation.md Phase 4. The
hard part isn't the content transfer (CAS handles that) —
it's the trust ceremony for meshes that can never handshake
over a network.

**Tracked as deferred work.** Design when bundle/import
(mesh-implementation Phase 4) and trust bootstrap
(mesh-trust Phase 1) are both implemented.
