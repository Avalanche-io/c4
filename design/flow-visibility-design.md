# Flow Visibility Design

**Status**: Design Research
**Date**: 2026-03-08

---

## The Argument for Visibility

Every organization with more than one computer has data flow
relationships. The rsync cron job is an outbound flow link. The S3
replication rule is a bidirectional flow link. The cloud sync agent is
an inbound flow link. But each lives in a different tool, with a
different config format, invisible to everything else. There is no
`ip route` for data. There is no `dig` for "where does this content
come from."

Flow links as a filesystem primitive solve the declaration problem.
Visibility solves the operational problem: once flows are declared,
how do you see them, reason about them, and manage them at scale?

This document designs the observability layer — what c4d exposes, what
tools consume it, and what questions an operator can answer.

---

## 1. What Questions Must Be Answerable

### For a single node

- What channels does this node expose? (its interface)
- Which channels are bound vs unbound?
- Which channels are synced vs stale vs conflicted?
- What is the estimated staleness of each channel?
- How fast is the source changing? (source entropy rate)
- What permissions compose with each flow? (drop slot, cache, sync)

### For a mesh

- What is the flow topology? (which nodes connect to which)
- Are there chains? How deep? (DPI bound implications)
- Where are direct links possible but missing?
- Which channels are partitioned right now?
- Where is content accumulating without draining? (unbound outbound)
- Where is content expected but not arriving? (unbound inbound)

### For an administrator

- What changed since I last looked?
- What needs my attention? (conflicts, unbound channels, stale data)
- If I bind location X to peer Y, what channels activate?
- If peer Y goes offline, what channels are affected?
- What is the total data volume flowing through each channel?

---

## 2. Data Model: What c4d Exposes

### Per-Channel Metrics

Every channel in the registry exposes:

```go
type ChannelStatus struct {
    // Identity
    ID          string
    LocalPath   string
    Direction   string  // "outbound", "inbound", "bidirectional"
    Location    string
    RemotePath  string

    // Connection state
    Bound       bool
    PeerAddr    string
    Connected   bool    // currently reachable
    LastSeen    time.Time

    // Sync state
    Status      string  // "synced", "pending", "stale", "conflicted", "unbound"
    LocalID     string  // current local c4m C4 ID
    RemoteID    string  // last known remote c4m C4 ID (empty if never synced)
    AncestorID  string  // common ancestor (for bidirectional)
    PendingOps  int     // mutations awaiting propagation

    // Information metrics
    LastSyncAt        time.Time
    LastSourceChange  time.Time
    SourceChangeRate  float64  // mutations per hour
    StalenessEstimate float64  // Σ(t) proxy
    BytesTransferred  int64    // total bytes moved through this channel
    BytesPending      int64    // bytes awaiting transfer

    // Chain info
    ChainDepth  int      // 0 = direct, 1+ = relay
    ChainPath   []string // trace of location names

    // Permissions composition
    LocalPerms  string   // e.g. "rwx", "-wx", "r-x"
    Behavior    string   // derived: "sync", "drop-slot", "cache", "publish", etc.

    // Source c4m
    SourceC4M   string   // path to the c4m containing this flow declaration
}
```

### Topology Data

c4d can expose its view of the mesh topology:

```go
type MeshView struct {
    Self      NodeInfo
    Peers     []NodeInfo
    Channels  []ChannelStatus
    Chains    []ChainInfo
}

type NodeInfo struct {
    Name      string   // location name (how others refer to this node)
    Address   string
    Connected bool
    Channels  int      // number of channels involving this node
}

type ChainInfo struct {
    Path           []string  // ordered location names
    Depth          int
    HeadStaleness  float64   // estimated staleness at the tail relative to the head
    DirectPossible bool      // head and tail can reach each other directly
}
```

---

## 3. API Surface

### Channel API (per-node)

```
GET  /etc/channels/                    — list all channels with status
GET  /etc/channels/{id}               — full detail for one channel
GET  /etc/channels/{id}/metrics       — time-series metrics
GET  /etc/channels/?status=conflicted — filter by status
GET  /etc/channels/?location=nas      — filter by location
```

### Topology API (mesh-wide, assembled from peers)

```
GET  /etc/mesh/topology               — this node's view of the mesh
GET  /etc/mesh/chains                  — detected chains
GET  /etc/mesh/recommendations        — suggested improvements
```

The topology view is assembled by querying each bound peer's
`/etc/channels/` endpoint and stitching the results. This is
best-effort — offline peers contribute no data to the view.

### Events / Stream

```
GET  /etc/channels/events?stream=true  — SSE stream of channel events
```

Events include: channel_bound, channel_unbound, sync_complete,
sync_failed, conflict_detected, conflict_resolved, chain_detected.

---

## 4. CLI Surface

### c4 status (node overview)

```
$ c4 status
Node: workstation-7 (workstation.local:7433)
Peers: 3 (2 connected, 1 offline)
Channels: 5 (3 synced, 1 pending, 1 unbound)

  -> nas:backup/footage/       synced      0 pending    Σ=0.0
  -> nas:backup/projects/      synced      0 pending    Σ=0.0
  <- nas:reference/             synced                   Σ=0.1
  <> studio:shared/project-x/  3 pending   partner offline  Σ=2.4
  -> archive:completed/        unbound     42 pending   Σ=8.7
```

The Σ column is staleness estimate. High staleness = high priority
when connectivity restores.

### c4 channels (detailed channel management)

```
$ c4 channels list
$ c4 channels detail ch-a1b2c3
$ c4 channels approve ch-pending-1
$ c4 channels reject ch-suspect-1
$ c4 channels trace ch-a1b2c3     # show chain path
```

### c4 mesh (topology view)

```
$ c4 mesh
workstation-7
  -> nas          connected   3 channels  (all synced)
  -> studio       connected   1 channel   (3 pending)
  -> archive      offline     1 channel   (42 pending, Σ=8.7)

Chains detected:
  workstation -> nas -> offsite-archive  (depth 2)
    Recommendation: direct link workstation -> offsite-archive
    would reduce staleness from Σ=12.3 to Σ=4.1

$ c4 mesh dot > topology.dot   # export as graphviz
```

### c4 dig (content provenance)

Inspired by DNS `dig` — trace where content comes from:

```
$ c4 dig :projects/shot0010/
projects/shot0010/
  Local: c4abc123... (modified 2 hours ago)
  Flow:  <> studio:shared/shot0010/
  Remote: c4abc123... (synced 2 hours ago)
  Status: synced, Σ=0.0
  Chain: none (direct link)

$ c4 dig nas:reference/lut-pack/
reference/lut-pack/
  Source: nas:reference/lut-pack/
  Flow:   <- nas:reference/lut-pack/
  Local:  c4def456... (received 3 days ago)
  Remote: c4def456... (unchanged)
  Status: synced, Σ=0.0
  Perms:  r-x (read cache)
```

---

## 5. Visual Representations

### Text-Based Topology (for terminal)

```
         ┌─────────┐
    ──>──│   nas    │──>── offsite-archive
    │    └─────────┘
    │
┌─────────────┐
│ workstation  │──<>── studio
└─────────────┘
    │
    ──>── archive (offline, 42 pending)
```

Direction arrows show flow. Color (where supported):
green = synced, yellow = pending, red = conflicted, dim = unbound.

### Graphviz Export

```
$ c4 mesh dot | dot -Tsvg > mesh.svg
```

Produces a directed graph with:
- Nodes = locations
- Edges = channels, labeled with direction and status
- Edge weight = staleness estimate
- Dashed edges = unbound channels
- Red edges = conflicted channels

### Machine-Readable Export

```
$ c4 mesh json
```

Full topology, channel status, and metrics as JSON. Consumable by
dashboards, monitoring systems, alerting pipelines.

---

## 6. Monitoring Integration

### Prometheus Metrics

c4d should expose `/metrics` (Prometheus format):

```
c4d_channel_total{direction="outbound"} 3
c4d_channel_total{direction="inbound"} 1
c4d_channel_total{direction="bidirectional"} 1

c4d_channel_status{id="ch-1",status="synced"} 1
c4d_channel_staleness{id="ch-1",location="nas"} 0.0
c4d_channel_staleness{id="ch-5",location="archive"} 8.7

c4d_channel_bytes_transferred_total{id="ch-1"} 1073741824
c4d_channel_pending_ops{id="ch-4"} 3
c4d_channel_pending_bytes{id="ch-5"} 42949672960

c4d_chain_depth{chain="workstation-nas-offsite"} 2
c4d_peers_connected 2
c4d_peers_total 3
```

### Alerting Rules (examples)

```
# Channel stale for more than 1 hour with high source change rate
c4d_channel_staleness > 5.0 and c4d_channel_pending_ops > 0

# Chain depth warning
c4d_chain_depth > 2

# Unbound outbound with pending data
c4d_channel_status{status="unbound"} == 1
  and c4d_channel_pending_bytes > 0
```

---

## 7. Relationship to Existing Observability

### What c4d Already Has

- `GET /du` — storage breakdown (active, purgeable, total, limit)
- `GET /admin/purge` — manual purgatory flush
- Namespace subscription (`?wait=`) — event primitive
- Refgraph forward/backward indexes — blob reachability

### What Flow Visibility Adds

- **Channel layer** above the namespace layer — relationships between
  namespaces, not just within them
- **Staleness metrics** — information-theoretic measure of how out of
  date each channel is
- **Topology view** — the mesh as a graph, not just a list of peers
- **Chain detection** — structural analysis with DPI-derived bounds
- **Provenance** — trace where content came from and how current it is

### Integration Points

The channel layer builds on existing infrastructure:
- Namespace `Subscribe` drives outbound reconciliation events
- Refgraph tracks blob references for scoped propagation
- Peer `Client` handles all remote communication
- The content store handles blob storage and dedup

The new layer adds: channel registry, reconciliation goroutines,
metrics collection, topology assembly, and the API surface above.

---

## 8. Monetization Note

The flow link primitive (c4m format, `->` / `<-` / `<>` syntax) is
open source. The fulfillment engine in c4d is the natural open core
gate:

- **Free c4d**: outbound flow (`->`), manual `c4 cp` between
  locations, basic peer connectivity. Enough for backup and
  sneakernet workflows.
- **Paid c4d**: inbound flow (`<-`), bidirectional sync (`<>`),
  reconciliation engine, staleness metrics, `c4 mesh`, `c4 dig`,
  fleet template deployment, channel approval workflows.

The c4m format supports all three operators regardless of license.
A free c4d doesn't fulfill `<-` and `<>` — the channels appear as
"upgrade to activate" in `c4 status`. The declaration travels with
the content. The fulfillment is licensed.

This sits below the Cloud Relay ($9/mo) tier and captures value
from self-hosting organizations that would never pay for a relay.

Potential tiers:
- **Free**: c4 CLI + c4d with outbound flow only
- **Pro**: bidirectional sync, inbound flow, visibility tools
- **Enterprise**: fleet management, staleness metrics, chain
  detection, admin approval workflows, multi-site topology
- **Cloud Relay**: Avalanche.io relay service (additive to any tier)

---

## 9. Design Principles

1. **Declare in c4m, fulfill in c4d, observe everywhere.** The flow
   declaration is in the content. The reconciliation engine is in c4d.
   The visibility layer makes both inspectable.

2. **Staleness over freshness.** Don't show "last synced 5 minutes
   ago." Show "estimated staleness: low/medium/high" based on source
   change rate. A static archive synced a week ago is fresher than a
   working directory synced 5 minutes ago.

3. **Topology over lists.** A flat list of channels is less useful
   than a graph showing how they connect. Chains, fan-out, fan-in —
   these patterns are only visible in a topology view.

4. **Recommendations over alerts.** "Chain depth 3 detected —
   direct link would reduce staleness by 60%" is more actionable than
   "WARNING: chain depth exceeds threshold."

5. **Progressive disclosure.** `c4 status` shows the summary.
   `c4 channels detail` shows one channel. `c4 mesh` shows topology.
   `c4 dig` traces provenance. Each level adds detail without
   requiring the previous level to be complex.
