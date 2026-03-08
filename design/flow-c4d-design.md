# Flow Links and Location:Path in c4d

**Status**: Design Research
**Date**: 2026-03-08

---

## A. Current Namespace Model

c4d's namespace is a single c4m manifest persisted to disk (`root.c4m`). It is a flat-but-positional tree: entries have a `Depth` field and are stored in order, with children following their parent directory entry. The tree is walked by matching name components at each depth level.

### Default Root Structure

On first run (`namespace.New`), c4d creates five top-level directories:

```
bin/
etc/
home/
mnt/
tmp/
```

These are reserved system paths. User content lives under `mnt/`. Identity-scoped content lives under `home/{identity}/`. The function `IsSystemPath` gates write access — only `mnt/` is open to user PUT/DELETE without identity checks. `home/` paths require the caller's mTLS identity to match the path's identity segment (or the server's own identity, for relay writes).

### How c4m References Work

Each non-directory entry in the namespace carries a `c4.ID` — the C4 ID of the c4m it points to. This is a pointer, not inline content. The namespace maps `path -> c4.ID`, and the blob behind that ID lives in the content store.

When a PUT arrives at a namespace path, the body is a bare C4 ID string. The namespace records it. When a GET arrives, the namespace resolves the path and returns the C4 ID. The caller then fetches the actual blob separately via `GET /{c4id}`.

### Persistence

Every mutation (Put, Delete) atomically rewrites the entire `root.c4m` file via temp-file-then-rename. This is simple and correct but means the namespace state is a single serialized file. There is no journal, no WAL, no incremental update.

### Subscription / Long-Poll

The namespace supports `Subscribe(prefix)` which returns a channel closed when any Put occurs under that prefix. The HTTP server uses this for `?wait=` long-poll on directory listings. This is the existing event mechanism — important for flow channels later.

### TTL-Bearing Paths

A `TTLStore` (persisted as `root.ttl.json`) maps directory paths to retention durations. Entries placed under a TTL-bearing directory automatically get expiration timestamps. A background goroutine (`StartExpirer`) periodically deletes expired entries. This is the "trash can" pattern — the directory is permanent, entries drain through it.

### Reference Graph

The `refgraph` package maintains a forward index (c4m ID -> referenced blob IDs) and a derived backward index (blob ID -> referencing c4m IDs). When a namespace entry is added or removed, the graph updates. Blobs that lose all references enter purgatory. A pressure-curve reclaimer deletes purgatory blobs when storage approaches configured limits.

---

## B. Current Peer Model

### Configuration

Peers are configured as a list of `host:port` strings in `config.yaml`:

```yaml
peers:
  - "nas.local:7433"
  - "relay.example.com:7433"
```

Each peer becomes a `peer.Client` — an HTTP client with optional mTLS. There is no concept of a peer name, peer identity, or peer capability. A peer is an address.

### Router

The `peer.Router` holds the list of `Client` instances. Its single method, `Fetch(id)`, tries each peer sequentially: HEAD to check existence, then GET to retrieve. First success wins. This is used for content fallback — when the local store doesn't have a blob, the router tries peers.

### Peer Client Capabilities

`peer.Client` supports:
- `Put(reader)` — store content, returns C4 ID
- `Get(id)` — retrieve content by C4 ID
- `Has(id)` — check existence by C4 ID
- `PutPath(path, body)` — write a namespace path (body is C4 ID string)
- `GetPath(path)` — read a namespace path

This is already sufficient for basic cross-node operations. The client can read and write both content and namespace paths on a remote peer. But there is no location abstraction on top of this — the caller must know the address.

### What's Missing

1. **Named locations.** No mapping from a human name ("nas", "studio") to a peer address.
2. **Discovery.** No mDNS, no directory lookup, no signal server. Peers are hardcoded.
3. **Peer metadata.** No identity, no capabilities, no health tracking.
4. **Bidirectional awareness.** The peer list is one-directional — "I know about these peers." There is no mutual registration.

---

## C. Location Resolution

### What `location:path` Means

`location:path` is the addressing scheme for the c4 CLI and c4d API. It separates *where* (the location) from *what* (the path within that location's namespace).

- `nas:` — the root namespace of the "nas" location
- `footage:raw/shot01.mov` — the path `raw/shot01.mov` within the "footage" location's namespace
- `:` or `:path` — the local c4d (unnamed/default location)

The colon is the separator. Everything before it is a location name. Everything after is a namespace path. The empty location name means "local."

### Resolution Order

Location resolution should follow a layered strategy, from most specific to most general:

1. **Explicit config.** A `locations:` section in `config.yaml` mapping names to addresses:
   ```yaml
   locations:
     nas: "nas.local:7433"
     studio: "studio.example.com:7433"
     cloud: "relay.avalanche.io:7433"
   ```
   This is the SSH config model. Deterministic, user-controlled, zero ambiguity.

2. **mDNS discovery.** c4d nodes advertise `_c4d._tcp` with their cert CN as the instance name. A location name matches if it equals a discovered peer's CN. This gives zero-config resolution for LAN peers.

3. **Relay/directory lookup.** For Avalanche.io users, `c4 login` provisions a cert and registers with the directory. Location names that are email addresses (`sarah@gmail.com`) resolve through the directory to a relay-mediated route.

4. **Unresolved.** If no resolution succeeds, the location name is *unbound*. This is not an error — it means the location exists as a concept but has no reachable peer. Flow channels to unbound locations accumulate locally (see Section D).

### What Happens When a Location Is Unknown

The location name is recorded. Any flow declarations targeting it are stored. Content accumulates in local storage. When the location becomes bound (config change, mDNS discovery, manual bind command), accumulated state propagates. This is the "eventual connectivity" model — the system works before the network does.

### Config Data Structure

```go
type LocationConfig struct {
    Name     string   // human name: "nas", "studio"
    Address  string   // host:port (empty if unbound)
    TLS      *TLSRef  // cert/key/CA override (nil = use node default)
    AutoBind bool     // if true, accept mDNS matches for this name
}
```

The `config.Config` struct gains:
```go
Locations []LocationConfig `yaml:"locations"`
```

Existing `Peers []string` remains for backward compatibility but is deprecated. Each peer entry without a name is an anonymous peer (used only for content fallback routing, not addressable as a location).

---

## D. Flow Channel Lifecycle

### How c4d Discovers Flow Declarations

Flow links are entries in a c4m file. They are not file entries — they are metadata entries with special syntax:

```
-> studio:incoming/
<- nas:footage/raw/
<> partner:shared/
```

When c4d processes a c4m (either receiving it via namespace PUT, or parsing it for the reference graph), it scans entries for flow link syntax. The flow link parser extracts:
- Direction: outbound (`->`), inbound (`<-`), bidirectional (`<>`)
- Target: `location:path`

### Discovery Entry Points

Flow declarations are discovered at two moments:

1. **Namespace PUT.** When a c4m is placed at a namespace path, c4d parses the c4m's content to build the reference graph (forward index). At this same moment, it can extract flow declarations. The namespace path where the c4m is mounted becomes the "local anchor" of the flow channel.

2. **Startup rebuild.** When c4d starts and rebuilds the reference graph from the namespace, it also re-discovers all flow declarations from all active c4m files.

### Local Representation for Unbound Channels

When a flow declaration targets a location that isn't currently bound to a peer, c4d creates a **channel record** — not a virtual namespace, not a real c4m, but a lightweight entry in a channel registry:

```go
type Channel struct {
    ID          string    // unique channel ID
    LocalPath   string    // namespace path where the c4m with the flow is mounted
    Direction   Direction // Outbound, Inbound, Bidirectional
    Location    string    // target location name
    RemotePath  string    // path within the target location
    Bound       bool      // whether the location is currently resolved
    PeerAddr    string    // resolved address (empty if unbound)
    CreatedAt   time.Time
    LastSyncAt  time.Time // last successful propagation
}
```

The channel registry persists as `channels.json` alongside `root.c4m`. This is separate from the namespace — channels are infrastructure metadata, not user content.

For **outbound** channels to unbound locations: content accumulates normally in the local namespace. The channel records "I need to push this to `studio:incoming/` when studio becomes available." Each namespace mutation under the local anchor path is noted as pending.

For **inbound** channels from unbound locations: nothing happens locally until the location binds. The channel record exists as a declaration of intent — "when nas becomes available, subscribe to changes at `nas:footage/raw/`."

### Connecting Channels When a Peer Becomes Available

When a location binds (mDNS discovery, config reload, admin command):

1. c4d resolves the location name to a peer address.
2. For each channel targeting that location:
   - **Outbound:** Push the current c4m at the local anchor path to the remote `location:path`. If the remote path already exists, compare C4 IDs — if different, the outbound channel pushes the local version (conflict resolution per policy).
   - **Inbound:** Fetch the current c4m at the remote `location:path`. Place it at the local anchor path (or a configured local path). Subscribe to changes via long-poll.
   - **Bidirectional:** Compare both sides. If one is ahead, propagate. If diverged, apply conflict resolution policy.
3. Start the reconciliation loop for this channel (see Section F).

### Handling Disconnection/Reconnection

When a peer becomes unreachable (connection failure, timeout):

1. The channel's `Bound` flag flips to false.
2. Outbound changes continue accumulating locally. Each mutation is logged in the channel's pending queue.
3. Inbound subscriptions (long-poll connections) break. c4d notes the last known remote state (C4 ID).
4. When the peer returns, c4d resumes from the last known state — diff and propagate the delta, not the full state. This is efficient because comparing two C4 IDs tells you instantly if anything changed, and diffing two c4m files tells you exactly what.

---

## E. Channel Visibility

### Not Namespace Entries

Channels should NOT appear as regular namespace entries. They are orthogonal to content — a channel is a relationship between paths, not content at a path. Mixing them into the namespace would create confusion: `ls /mnt/` would show both projects and infrastructure metadata.

### Dedicated API Surface

Channels are visible through a dedicated API under a reserved path:

```
GET  /etc/channels/              — list all channels
GET  /etc/channels/{id}          — get channel detail
POST /etc/channels/              — create channel (admin)
DELETE /etc/channels/{id}        — remove channel (admin)
```

The `/etc/` prefix is already a reserved system path in the namespace. Using it for channel metadata is natural — it's configuration, not content.

### Channel Detail Response

```json
{
  "id": "ch-a1b2c3",
  "local_path": "/mnt/dailies/",
  "direction": "outbound",
  "location": "studio",
  "remote_path": "incoming/dailies/",
  "bound": true,
  "peer_addr": "studio.local:7433",
  "last_sync": "2026-03-08T10:30:00Z",
  "pending": 0,
  "status": "synced"
}
```

### Admin Visibility

`c4d status` already shows peers. It should also show channels:

```
Channels: 3 active
  -> studio:incoming/dailies/     synced (0 pending)
  <- nas:footage/raw/             synced
  <> partner:shared/project/      2 pending (partner offline)
```

### c4m-Level Visibility

Flow declarations remain visible in the c4m itself — they're entries in the manifest. When you `c4 ls project.c4m:`, the flow links appear alongside file entries. This is by design: the c4m is human-readable and self-describing. The flow declaration travels with the description.

---

## F. Reconciliation Engine

### Architecture

The reconciliation engine is a set of per-channel goroutines managed by a `FlowEngine`:

```go
type FlowEngine struct {
    channels   *ChannelStore    // persistent channel registry
    namespace  *namespace.Namespace
    store      store.Store
    resolver   *LocationResolver  // resolves location -> peer.Client
    logger     *log.Logger

    mu         sync.Mutex
    workers    map[string]chan struct{} // channel ID -> stop channel
}
```

Each channel gets its own goroutine. The goroutine's behavior depends on the direction.

### Outbound Reconciliation

```
loop:
    wait for namespace change under local_path (Subscribe)
    resolve location -> peer client
    if unbound: record pending, continue

    local_id = namespace.Resolve(local_path)
    remote_id = peer.GetPath(remote_path)  // may 404

    if local_id == remote_id: continue  // already synced

    // Push the c4m
    c4m_data = store.Get(local_id)
    peer.Put(c4m_data)

    // Push referenced blobs the remote doesn't have
    for blob_id in parse_c4m_refs(local_id):
        if not peer.Has(blob_id):
            peer.Put(store.Get(blob_id))

    // Update remote namespace
    peer.PutPath(remote_path, local_id)

    update last_sync
```

This is "push intent, pull content" — but from c4d's perspective, it pushes both the c4m and the content to the peer. The peer's c4d then has everything it needs. The key insight: c4d pushes the c4m (intent) AND makes the content available. The receiving c4d doesn't need to pull from the sender because the sender proactively pushed.

### Inbound Reconciliation

```
loop:
    resolve location -> peer client
    if unbound: sleep, retry

    // Long-poll for changes at remote path
    response = peer.GetPath(remote_path + "/?wait=30s")
    if timeout (304): continue

    remote_id = peer.GetPath(remote_path)
    local_id = namespace.Resolve(local_path)

    if remote_id == local_id: continue

    // Fetch the c4m
    c4m_data = peer.Get(remote_id)
    store.Put(c4m_data)

    // Fetch referenced blobs we don't have
    for blob_id in parse_c4m_refs(remote_id):
        if not store.Has(blob_id):
            blob = peer.Get(blob_id)
            store.Put(blob)

    // Update local namespace
    namespace.Put(local_path, remote_id)

    update last_sync
```

Inbound uses the existing long-poll mechanism (`?wait=`) on the remote's directory listing. When the remote namespace changes, the long-poll unblocks and the local side fetches the update.

### Bidirectional Reconciliation

Bidirectional is the hard case. Two approaches:

**Last-writer-wins (simple, lossy):** Compare timestamps. The newer c4m wins. This is appropriate for cases where one side is authoritative and bidirectional is really "sync to whichever is newer."

**Three-way merge (correct, complex):** Track a common ancestor C4 ID. When both sides diverge from the ancestor:
1. Fetch ancestor c4m, local c4m, remote c4m.
2. Diff ancestor->local and ancestor->remote.
3. If changes touch disjoint paths, merge automatically.
4. If changes touch the same path with the same new C4 ID, convergent — no conflict.
5. If changes touch the same path with different new C4 IDs, real conflict — flag it.

This aligns with the CAS-based merge strategy described in `CLOUD_RELAY_AND_WORKSPACES.md`. The ancestor tracking requires storing the last-synced C4 ID per channel.

### Interaction with Refgraph/Retention

When the reconciliation engine places a new c4m at a namespace path:
- `namespace.Put(path, id)` fires, which the server already wires to `refGraph.OnNamespacePut`.
- All referenced blobs become active (removed from purgatory if they were there).
- The old c4m's blobs may enter purgatory if no other namespace path references them.

When inbound flow brings in new blobs via `store.Put`:
- The blobs land in the store but have no namespace reference yet.
- They are technically purgatory-eligible, but the reconciliation engine immediately follows with a `namespace.Put` that makes them active.
- Race condition mitigation: the reconciliation engine should store all blobs BEFORE updating the namespace path. This ensures the refgraph never sees a c4m reference to a blob that isn't in the store.

TTL-bearing paths compose naturally with flow: a TTL-bearing directory with an inbound flow channel is a "recent arrivals" cache that auto-drains. Content flows in, lingers for the TTL duration, then expires. The flow replenishes it.

---

## G. Auto-Accumulation

### The Problem

When a c4m declares `-> partner:shared/`, but "partner" isn't bound to any peer, the outbound content needs somewhere to go. It can't be pushed to a peer that doesn't exist yet.

### The Solution: No Special Mechanism Needed

Content already lives in the local store. The c4m already lives in the local namespace. The channel record already tracks that this content should eventually go to `partner:shared/`. When the partner binds, the reconciliation engine reads the current local state and pushes it.

There is no "accumulation" in a separate structure. The local namespace IS the accumulation. The c4m at the local anchor path describes the current state. The content store holds the blobs. When the peer appears, the reconciliation engine pushes the current state — it doesn't need to replay a log of changes. The current c4m IS the accumulated result.

This is a direct consequence of the content-addressed model: every c4m is a complete snapshot. There is no diff to accumulate. The current snapshot is the accumulation.

### Edge Case: Inbound from Unbound

For `<- partner:data/` where partner is unbound: nothing accumulates locally because there's nothing to receive. The channel record exists as a declaration. When partner binds, the first inbound reconciliation fetches whatever is currently at `partner:data/` — the full current state, not a backlog.

---

## H. Configuration Surface

### Per-Location Config

```yaml
locations:
  nas:
    address: "nas.local:7433"
    tls:
      cert: "~/.c4d/nas-client.crt"
      key: "~/.c4d/nas-client.key"
      ca: "~/.c4d/nas-ca.crt"

  studio:
    address: "studio.example.com:7433"
    # uses node default TLS

  partner:
    # no address — unbound, will be resolved via mDNS or manual bind
    auto_bind: true  # accept mDNS matches for "partner"
```

### Flow Defaults

```yaml
flow:
  # Default reconciliation interval when not using long-poll
  check_interval: "30s"

  # Default conflict strategy for bidirectional channels
  conflict_strategy: "last-writer-wins"  # or "flag-conflict"

  # Maximum concurrent blob transfers per channel
  max_concurrent_transfers: 4

  # Per-location overrides
  overrides:
    nas:
      check_interval: "5s"    # LAN peer, check frequently
    partner:
      conflict_strategy: "flag-conflict"  # don't auto-resolve with external partners
      max_concurrent_transfers: 2         # bandwidth-limited link
```

### Admin Commands

```
c4d location list                     # show all locations and their state
c4d location bind studio 10.0.1.5:7433   # manually bind a location
c4d location unbind studio            # disconnect a location
c4d location discover                 # trigger mDNS discovery scan
```

These would be new subcommands of the c4d binary (alongside serve, init, status, etc.).

---

## I. What location:path Enables Beyond Flow

The `location:path` construct is independently valuable even without flow links. It gives the CLI a uniform way to address content across the mesh.

### Cross-Location Queries

```
c4 ls nas:projects/
c4 ls studio:incoming/
c4 ls :mnt/local-project/
```

Implementation: the CLI sends a request to the local c4d. c4d resolves the location to a peer client and proxies the request. The HTTP API gains a location prefix:

```
GET /location/{name}/{path}   ->  proxy to peer.GetPath(path)
PUT /location/{name}/{path}   ->  proxy to peer.PutPath(path, body)
```

Alternatively, the CLI could parse `location:path` client-side and route directly. But routing through the local c4d is correct — the CLI should not maintain peer connections. c4d is the gateway.

### Cross-Location Copy

```
c4 cp nas:footage/raw/ :mnt/local-footage/
```

This is the fundamental operation. The CLI tells local c4d: "Resolve `nas:footage/raw/`, get the c4m there, fetch all referenced blobs, and place the c4m at `:mnt/local-footage/`."

Steps inside c4d:
1. Resolve "nas" to a peer client.
2. `peer.GetPath("/footage/raw/")` to get the C4 ID.
3. `peer.Get(c4mID)` to fetch the c4m blob.
4. Parse the c4m, fetch all referenced blobs the local store doesn't have.
5. `namespace.Put("/mnt/local-footage/", c4mID)`.

This is push-intent-pull-content at the API level. The c4m (intent) arrives first. The blobs (content) follow.

### Cross-Location Diff

```
c4 diff nas:project.c4m: local:project.c4m:
```

Resolve both locations to C4 IDs, fetch both c4m blobs, diff them using the c4m diff machinery. This requires no new infrastructure — just location resolution and the existing c4m operations.

### Peer Namespace Browsing

```
c4 ls nas:
c4 ls nas:mnt/
c4 ls nas:home/josh@example.com/
```

The local c4d proxies directory listing requests to the peer. The peer returns its namespace listing. Access control is enforced by the peer's server — the peer checks the caller's mTLS identity against its own ACL.

### Required c4d API Changes

The server currently routes based on path prefix: C4 IDs go to content handlers, everything else goes to namespace handlers. Adding location-qualified requests requires a new routing layer:

```
GET /~{location}/{path}      — proxy to location's peer
PUT /~{location}/{path}      — proxy to location's peer
```

The `~` prefix disambiguates from namespace paths. The server resolves the location, creates or reuses a peer client, and proxies the request. The response flows back unchanged.

---

## J. Security Considerations

### The Threat

A c4m file with flow declarations is user-authored content. Anyone who can place a c4m in the namespace can declare flow links. This means:

- **Data exfiltration:** A c4m with `-> attacker:stolen/` would cause c4d to push content to an attacker-controlled location whenever that location binds.
- **Data injection:** A c4m with `<- attacker:malware/` would cause c4d to pull content from an attacker-controlled location.
- **Amplification:** A c4m with `<> everywhere:` on every path would flood the network.

### Defense: Flow Declarations Require Admin Approval

Flow links in a c4m are declarations, not imperatives. c4d does NOT automatically act on flow declarations found in arbitrary c4m files. Instead:

1. **Discovery is passive.** When c4d parses a c4m and finds flow declarations, it records them as *proposed channels* — visible in the channel API but not active.
2. **Activation requires explicit approval.** An admin (or the node's configured policy) must approve a proposed channel before c4d starts reconciliation.
3. **Auto-approval for trusted paths.** c4d can be configured to auto-approve flow declarations from specific namespace paths. For example: "auto-approve all flow declarations in c4m files under `/mnt/` that target locations listed in config." This allows trusted workflows to operate without manual approval while blocking arbitrary c4m files from creating channels.

### Defense: Location Allowlist

c4d should maintain an allowlist of location names that flow declarations can target. Any flow declaration targeting a location not on the allowlist is rejected (or held for approval).

```yaml
flow:
  allowed_locations:
    - nas
    - studio
    - partner
  # Flow declarations targeting any other location name are held for admin approval
```

### Defense: Direction Restrictions

Per-location, the admin can restrict allowed flow directions:

```yaml
locations:
  partner:
    allowed_flow: ["outbound"]  # partner can receive, never inject
  nas:
    allowed_flow: ["inbound", "outbound", "bidirectional"]  # full trust
```

### Defense: Content Scope

A flow channel should only propagate content that the c4m actually describes. An outbound channel for a c4m at `/mnt/project/` should only push blobs referenced by that c4m, not arbitrary content from the store. The reconciliation engine must scope its blob push to the c4m's reference graph.

### Defense: Rate Limiting

Per-channel rate limits prevent a compromised or misconfigured flow from consuming all bandwidth:

```yaml
flow:
  overrides:
    partner:
      max_bytes_per_hour: 10737418240  # 10 GB
```

### Identity Interaction

Flow declarations in c4m files placed under `/home/{identity}/` are scoped to that identity's trust boundary. A c4m in Alice's home directory can declare flows, but those flows execute with Alice's permissions. If Alice's cert doesn't have access to the target location, the flow fails at the peer's ACL check.

---

## K. Information-Theoretic Operations

The paper establishes three results that should be built into c4d's reconciliation engine, not bolted on later.

### Information Staleness

The paper defines staleness as Σ(t) = H(S_t | D_t) — the conditional entropy of the source given the destination. This is the right metric for prioritization, not "time since last sync."

c4d should track per-channel:

```go
type ChannelMetrics struct {
    LastSyncAt       time.Time
    LastSourceChange time.Time  // when the source last mutated
    SourceChangeRate float64    // mutations per hour (rolling average)
    StalenessEstimate float64  // approximate Σ(t), derived from rate × partition duration
}
```

After a partition heals and multiple channels need reconciliation, c4d should prioritize by estimated staleness: a rapidly-changing working directory that's been offline for an hour is more stale than a static archive offline for a week. The reconciliation scheduler uses `SourceChangeRate × TimeSinceLastSync` as a proxy for Σ(t).

### Chain Detection and the DPI Bound

The Data Processing Inequality proves that in a chain A→B→C, C's knowledge of A can never exceed B's knowledge. c4d should:

1. **Detect chains.** When c4d discovers that a namespace path has both an inbound flow (from location X) and an outbound flow (to location Y), this path is a relay node in a chain. Record the chain topology.

2. **Measure chain depth.** If c4d can query its peers for their own flow declarations (via the `/etc/channels/` API), it can trace the full chain. Each hop degrades information.

3. **Suggest direct links.** If A can reach C directly (even intermittently), a direct flow link is provably superior to relaying through B. c4d should surface this as a recommendation in the channel API:

```json
{
  "id": "ch-xyz",
  "chain_depth": 2,
  "chain": ["studio -> nas -> archive"],
  "recommendation": "Direct link studio -> archive would reduce staleness"
}
```

### State-Sufficient vs History-Dependent Content

c4d's content model is naturally state-sufficient: each c4m is a complete snapshot. The reconciliation engine pushes/pulls the current c4m, not a log of changes. This means:

- **Partition erasure is safe for c4m content.** If the source transitions through states s_0, s_1, ..., s_k during a partition, the destination receives s_k. The intermediate states are lost, but for filesystem state (which files exist, what their content is), only the current state matters.

- **Snapshot history is history-dependent.** The managed directory snapshot chain (undo/redo history) IS a sequence where intermediate states matter. If snapshots are being flowed across locations, the flow must ensure gap-free delivery. c4d should distinguish between "flow the current state" (state-sufficient, default) and "flow the snapshot chain" (history-dependent, opt-in).

This suggests a channel configuration flag:

```yaml
flow:
  overrides:
    nas:
      delivery: "state"      # default: push current c4m only
    archive:
      delivery: "history"    # push all snapshots in order, no gaps
```

History-dependent delivery requires a different reconciliation loop: instead of comparing current C4 IDs and pushing the latest, it maintains a cursor into the snapshot chain and pushes each snapshot in sequence.

---

## Summary of What Needs to Change

### New Packages

- `internal/location/` — Location resolution: config-based, mDNS, directory lookup. Maps names to `peer.Client` instances. Manages bind/unbind lifecycle.
- `internal/flow/` — Flow engine: channel registry, per-channel reconciliation goroutines, conflict resolution policies. Depends on location, namespace, store, and peer.

### Modified Packages

- `internal/config/` — Add `Locations` and `Flow` config sections.
- `internal/server/` — Add location-proxy routes (`/~{location}/...`), channel API (`/etc/channels/`), flow declaration discovery on namespace PUT.
- `internal/peer/` — No changes needed. The existing Client/Router already support all required operations.
- `internal/namespace/` — No changes to core logic. The Subscribe mechanism already provides the event primitive that outbound reconciliation needs.

### New CLI Commands (in c4d binary)

- `c4d location list|bind|unbind|discover`
- `c4d channel list|approve|reject|remove`

### Config Additions

```yaml
locations:
  name:
    address: "host:port"
    tls: { cert, key, ca }
    auto_bind: bool
    allowed_flow: ["inbound", "outbound", "bidirectional"]

flow:
  check_interval: "30s"
  conflict_strategy: "last-writer-wins"
  allowed_locations: [...]
  overrides:
    location_name:
      check_interval: "5s"
      conflict_strategy: "flag-conflict"
      max_concurrent_transfers: 4
      max_bytes_per_hour: 10737418240
```

### Implementation Order

1. **Location resolution** — config-based only (no mDNS yet). Map names to peer clients. Add `locations:` to config. This is the foundation everything else depends on.

2. **Location proxy** — Add `/~{location}/{path}` routes to the server. The CLI can now `c4 ls nas:` through the local c4d.

3. **Channel registry** — Persistent channel store. CRUD API under `/etc/channels/`. No reconciliation yet — just the data model.

4. **Flow declaration parser** — Extract `->`, `<-`, `<>` entries from c4m files during namespace PUT and refgraph rebuild. Create proposed channels in the registry.

5. **Outbound reconciliation** — Per-channel goroutine that watches local namespace and pushes to the target peer. The first useful flow direction.

6. **Inbound reconciliation** — Per-channel goroutine that long-polls the remote peer and pulls changes. Uses existing `?wait=` mechanism.

7. **Bidirectional reconciliation** — Three-way merge with ancestor tracking. The hardest piece.

8. **Security hardening** — Allowlists, approval workflow, rate limiting, direction restrictions.

9. **mDNS discovery** — Auto-bind locations from LAN peers. Adds the zero-config experience.
