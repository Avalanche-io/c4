# Mesh Implementation Plan

Phased implementation of [mesh.md](mesh.md). Each phase is a
working increment — the system is usable after every phase.

## Existing Foundation

**c4d already has:**
- Peer client (`internal/peer/client.go`) — Put, Get, Has,
  PutPath, GetPath over mTLS
- Peer router (`internal/peer/router.go`) — sequential fetch
  from peer list
- Content resolution fallback (`internal/server/content.go:55`)
  — on GET miss, ask peers, cache locally on passthrough
- Peer config in config.yaml (`peers: []string`)
- Peer wiring in serve.go — builds peer clients with mTLS,
  creates router, passes to server

**c4 CLI already has:**
- Location registry (`~/.c4/locations/`) — name → address
- Location establishment (`c4 mk name: host:port`)
- pathspec.Location type — parsed from colon syntax
- mTLS client via `c4dVersionClient()` (reads `~/.c4d/config.yaml`)
- `putToC4d()` — push blob content to local c4d
- cp handles Local, C4m, Container, Managed source/dest types

**What's missing** is connecting these two halves: the CLI can
parse `nas:` as a location, and c4d can talk to peers, but the
CLI doesn't know how to push/pull to a remote location.

---

## Phase 1: Remote Copy

**Goal:** `c4 cp project.c4m: nas:` and `c4 cp nas:project.c4m
local.c4m:` work. Push c4m + blobs to a remote c4d, pull c4m +
blobs from a remote c4d.

### 1.1 CLI talks to local c4d only

The CLI never talks to remote c4d nodes. All remote operations
go through the local c4d — like kubectl to a local API server.
The CLI authenticates to the local c4d with mTLS (client cert
from `~/.c4d/config.yaml`). c4d handles all peer-to-peer
communication.

The CLI already has `c4dVersionClient()` which builds an
`*http.Client` with TLS config from `~/.c4d/config.yaml`. All
remote operations reuse this same client, talking to the local
c4d's endpoints.

Location resolution: when the CLI sees `nas:path`, it resolves
`nas` to a local c4d namespace path. The location registry
(`~/.c4/locations/`) maps names to c4d namespace prefixes, not
to remote addresses. c4d knows how to reach the peer; the CLI
doesn't need to.

### 1.2 Push: local → location

Wire `pathspec.Location` as a destination in `cp.go`:

```
case src.Type == pathspec.C4m && dst.Type == pathspec.Location:
    cpC4mToLocation(src, dst)
```

`cpC4mToLocation(src, dst)`:
1. Load local c4m file
2. Push c4m blob to local c4d (`putToC4d`)
3. PUT to local c4d namespace at the location's path
4. c4d handles forwarding to the peer. The peer materializes
   blobs from the sender's c4d on its own schedule.

For the source-colon case (`project.c4m:`), the CLI also
pushes all referenced blobs to local c4d. For the no-colon
case (`project.c4m`), only the c4m file blob is pushed.

### 1.3 Pull: location → local

```
case src.Type == pathspec.Location && dst.Type == pathspec.C4m:
    cpLocationToC4m(src, dst)
```

`cpLocationToC4m(src, dst)`:
1. GET from local c4d namespace at the location's path
2. c4d resolves the path (proxying to the peer if needed)
3. Write the c4m file locally
4. Blobs materialize through c4d's content resolution cascade

### 1.4 `c4 ls location:`

Wire `pathspec.Location` in `ls.go`:
1. GET from local c4d at the location's listing path
2. c4d returns c4m-formatted entries (proxying if needed)
3. Display as usual

### 1.5 c4d namespace routing

c4d needs to route namespace operations to peers. When a
namespace PUT targets a path owned by a peer, c4d forwards
the operation. When a GET resolves to a peer's namespace, c4d
proxies the response.

Location entries map to c4d peer routing, not direct addresses.
`c4 mk nas: nas.local:7433` tells c4d "the peer at
nas.local:7433 is named nas" — the CLI stores the name, c4d
stores the peer relationship.

### 1.6 Tests

- Integration test: start two c4d instances (different ports,
  shared CA), push c4m from one to the other via CLI, verify
  content resolves on both.
- Test push with some blobs already present (deduplication).
- Test pull with partial local content.
- Test listing a remote peer's namespace through local c4d.

### Files to create/modify

**Modify:**
- `c4/cmd/c4/cp.go` — add Location cases to switch
- `c4/cmd/c4/ls.go` — add Location case
- `c4d/internal/server/namespace.go` — proxy to peers for
  non-local namespace paths
- `c4d/internal/peer/router.go` — namespace routing (not just
  blob routing)

---

## Phase 2: Transit Materialization + Transfer Progress

**Goal:** Content materializes through the mesh. Intermediate
nodes cache in transit. Large transfers show progress.

### 2.1 Transit namespace policy

When a c4m is registered in a transit path (e.g. `/transit/`),
the node materializes all referenced blobs from its peers.
Transit paths have short TTLs — content is cached for
forwarding, then reclaimed by existing retention machinery.

Configuration in c4d config.yaml:
```yaml
transit:
  path: /transit/
  ttl: 24h
  materialize: eager
```

Uses existing primitives: namespace PUT (with TTL), content
resolution cascade (blob fallback to peers), retention
(purgatory + pressure curve).

### 2.2 Transfer progress

For large transfers, show progress:
```
pushing project.c4m: → nas:
  142/500 blobs (28%) — 2.3 GB / 8.1 GB
```

Progress callback in the client, wired to stderr output in cp.go.

### Files to create/modify

**Modify:**
- `c4d/internal/server/namespace.go` — trigger materialization
  on PUT to transit paths
- `c4d/internal/config/config.go` — transit config
- `c4/cmd/c4/cp.go` — progress display for blob push

---

## Phase 3: LAN Discovery

**Goal:** `c4 ls net:/peers` shows c4d nodes on the local
network. No new commands — discovery is a path on a pseudo-
location.

### 3.1 mDNS advertisement in c4d

Add `internal/discovery/mdns.go`:

```go
func Advertise(port int, identity string, stopCh <-chan struct{}) error
```

Uses `_c4d._tcp` service type. TXT record carries identity
(from TLS cert CN/SAN). Advertise on startup, stop on shutdown.

Wire into `serve.go` after TLS config is loaded.

### 3.2 Peers endpoint in c4d

`GET /peers/` returns discovered peers in c4m format:

```
drwxr-xr-x - - nas/ -
drwxr-xr-x - - desktop/ -
drwxr-xr-x - - sarah-laptop/ -
```

Standard c4m directory entries. `GET /peers/nas/` proxies to
that peer's namespace root. Content-Type: `text/c4m`.

Composable: `GET /peers/nas/peers` returns nas's peers.
The mesh topology is browsable by walking deeper paths:

```
c4 ls net:/peers/nas/peers        # transitive discovery
c4 ls net:/peers/nas/peers/cloud/ # browse cloud through nas
```

This is the manual form of what Phase 5 formalizes as peer
routing — "can you reach X?" is walking the peer graph.

The peers list is maintained by the mDNS browser running in
c4d. Entries have a TTL — disappear when the peer stops
advertising.

### 3.3 `net:` pseudo-location in pathspec

`net:` is a built-in pseudo-location recognized by the pathspec
system. It resolves to the local c4d's address (from
`~/.c4d/config.yaml`). No entry in `~/.c4/locations/` — it's
implicit when c4d is configured.

```
c4 ls net:/peers          # list discovered peers
c4 ls net:/peers/nas/     # browse a peer's namespace
```

In `pathspec.go`, when `isLocation("net")` is called, return
true and resolve to the local c4d address. This is the only
special-cased location name.

### 3.4 Auto-discovery for `c4 mk`

When `c4 mk nas:` is called without an address, resolve via
`net:/peers/nas`. If the peer is discovered, use its address.
This enables:

```
c4 ls net:/peers       # see what's on the network
c4 mk nas:             # establish (auto-resolve address)
c4 cp project.c4m: nas:
```

### Dependencies

Go mDNS library: `github.com/grandcat/zeroconf` (well-maintained,
supports both advertising and browsing).

### Files to create/modify

**Create:**
- `c4d/internal/discovery/mdns.go`
- `c4d/internal/discovery/mdns_test.go`

**Modify:**
- `c4d/serve.go` — start mDNS advertisement
- `c4d/internal/server/server.go` — `/peers/` endpoint
- `c4/cmd/c4/internal/pathspec/pathspec.go` — recognize `net:`
- `c4/cmd/c4/mk.go` — auto-resolve from `net:/peers/` when
  no address given

---

## Phase 4: Bundle and Import

**Goal:** `c4 bundle project.c4m: /mnt/drive/` exports content
for physical transport. `c4 import /mnt/drive/` ingests it.

### 4.1 Bundle format

A bundle is a directory containing:
```
/mnt/drive/
  manifest.c4m          # the c4m file
  blobs/
    c4abc.../           # blob files named by C4 ID (sharded)
```

The c4m IS the shipping manifest. Self-describing. The blobs/
directory mirrors the c4d store layout (sharded by first 2
chars of ID).

### 4.2 `c4 bundle`

```
c4 bundle project.c4m: /mnt/drive/
c4 bundle : /mnt/drive/               # from managed dir
```

1. Load the c4m (or managed state)
2. Create bundle directory structure
3. Copy the c4m file as `manifest.c4m`
4. Walk entries, for each blob:
   - Check if already in bundle (incremental)
   - Copy from local c4d to `blobs/{shard}/{c4id}`
5. Report: "bundled 500 blobs (8.1 GB) into /mnt/drive/"

### 4.3 `c4 import`

```
c4 import /mnt/drive/
c4 import /mnt/drive/ project.c4m:    # import into specific c4m
```

1. Read `manifest.c4m` from bundle
2. Verify each blob (recompute C4 ID, compare)
3. Push verified blobs to local c4d
4. Optionally register c4m in local namespace
5. Report: "imported 500 blobs (8.1 GB), 0 corrupt"

### 4.4 Incremental bundle

If the bundle directory already has blobs from a previous export:
```
c4 bundle project.c4m: /mnt/drive/   # first time: 500 blobs
# ... project changes ...
c4 bundle project.c4m: /mnt/drive/   # second time: only 12 new blobs
```

CAS deduplication: check `blobs/{shard}/{c4id}` exists before
copying. Only new content is added.

### Files to create/modify

**Create:**
- `c4/cmd/c4/bundle.go`
- `c4/cmd/c4/bundle_test.go`

**Modify:**
- `c4/cmd/c4/main.go` — add "bundle" and "import" cases

---

## Phase 5: Peer Routing

**Goal:** Send content to people by identity, not address.
Intermediary nodes forward content when the recipient isn't
directly reachable.

### 5.1 Richer peer config

Currently `peers: []string` (addresses only). Change to:

```yaml
peers:
  - address: nas.local:7433
    name: nas
  - address: cloud.example.com:7433
    name: cloud
```

`config.PeerConfig` struct with Address + Identity fields.
Backward-compatible: plain strings still work (identity derived
from TLS handshake).

### 5.2 Peer announcement

The TLS cert carries identity. The TCP connection carries the
source address. Connecting IS announcing.

On startup, c4d connects to each configured peer (e.g. `HEAD /`
version check). The peer reads identity from the mTLS cert and
records the source address. This creates a routing table entry
(in-memory, TTL-based).

Any subsequent interaction refreshes the routing entry. No
special announce endpoint, no JSON. The mTLS handshake that
already happens IS the announcement.

Heartbeat: periodic version check (`HEAD /`) to each peer.
Serves double duty — liveness probe and routing refresh.

### 5.3 Peer routing

When content is addressed to an identity:

```
GET /route/sarah@gmail.com
→ 10.42.5.7:7433       (directly reachable)
→ via:nas.local:7433    (reachable through intermediary)
→ 404                   (unknown)
```

Plain text response. The query cascades through peers (with
hop limit). The first peer that can reach the target becomes
the route.

### 5.4 Identity-based cp

```
c4 cp project.c4m: sarah@gmail.com:
```

If `sarah@gmail.com` is not an established location, resolve:
1. Check peer routing (mesh — ask peers)
2. Check directory (Avalanche.io, future)
3. Email fallback (send c4m as attachment to the same address)

The sender doesn't need to know the target location. The
mesh routes it. If resolved to a mesh route, push the c4m to
the next hop's transit path. If no route exists, the c4m is
emailed — the identity IS the email address.

### 5.5 Store-and-forward

When the target identity is not currently reachable, content
lands on the intermediary (the peer that's "closest" to the
target). When the target reconnects (re-establishing its route
via mTLS handshake), the intermediary forwards pending content.

Storage: intermediary's namespace under a convention-based path,
e.g. `/pending/{target-identity}/`. No special queue mechanism —
just namespace entries pointing to c4m files, like every other
operation. Delivered when the target's route becomes active.

### Files to create/modify

**Create:**
- `c4d/internal/peer/routing.go` — routing table, identity
  extraction from mTLS cert on connect
- `c4d/internal/peer/routing_test.go`
- `c4d/internal/server/peer.go` — route query handler

**Modify:**
- `c4d/internal/config/config.go` — PeerConfig struct
- `c4d/internal/peer/client.go` — identity field
- `c4d/internal/peer/router.go` — identity-based routing
- `c4d/serve.go` — connect to peers on startup (implicit
  announcement via mTLS handshake)
- `c4/cmd/c4/cp.go` — identity resolution fallback

---

## Phase 6: Sync Policy

**Goal:** Managed directories stay in sync with declared
locations automatically on every mutation.

### 6.1 Sync target declaration

```
c4 mk : --sync nas: desktop:
```

Stores sync targets in `.c4/sync` (one location name per line).

### 6.2 Mutation propagation

After every CLI operation that mutates a managed directory
(cp, ln, mv, rm, patch), push the updated c4m to all sync
targets. Reuse the remote client from Phase 1.

In `managed.go`, add a post-mutation hook:

```go
func (d *Dir) SyncTargets() []string    // read .c4/sync
func (d *Dir) NotifySync(newC4mID c4.ID) // push to targets
```

Wire into `Snapshot()` — after snapshot, push to sync targets.

### 6.3 Bidirectional sync

`c4 sync :` pulls changes from sync targets, merges, pushes
back. Conflict: if remote c4m differs from local and neither
is an ancestor, report conflict (don't auto-merge).

### Files to create/modify

**Modify:**
- `c4/cmd/c4/mk.go` — parse --sync flag
- `c4/cmd/c4/internal/managed/managed.go` — sync target storage,
  post-mutation hook
- `c4/cmd/c4/main.go` — add "sync" case (future)

---

## Phase 7: Identity and Login

**Goal:** `c4 login` provisions a client cert from Avalanche.io
CA. Enables collaboration with strangers.

### 7.1 `c4 login`

Provision a cert from the Avalanche.io CA:
1. CLI initiates cert request with Avalanche.io
2. User verifies identity (email confirmation or browser)
3. Avalanche.io CA issues client cert
4. Cert stored in `~/.c4/auth/`

### 7.2 `c4 logout`

Revoke cert, remove from `~/.c4/auth/`.

### 7.3 Directory lookup

After login, `c4 cp project.c4m: user@example.com:` resolves
via Avalanche.io directory API.

### 7.4 Avalanche.io relay

Post-login, the user's Avalanche.io cloud node is auto-
established as a location named "cloud:". Acts as always-on
intermediary for their mesh.

### Files to create/modify

**Create:**
- `c4/cmd/c4/login.go`
- `c4/cmd/c4/internal/auth/auth.go`

**Modify:**
- `c4/cmd/c4/main.go` — add "login"/"logout" cases

---

## Phase Ordering and Dependencies

```
Phase 1 (Remote Copy)
  ↓
Phase 2 (Transit + Progress) ← builds on Phase 1 materialization
  ↓
Phase 3 (LAN Discovery) ← requires Phase 1 (Location pathspec)
  ↓
Phase 4 (Bundle/Import) ← independent of 1-3, can parallelize
  ↓
Phase 5 (Peer Routing) ← requires Phase 1
  ↓
Phase 6 (Sync Policy) ← requires Phase 1
  ↓
Phase 7 (Identity/Login) ← requires Phase 5
```

Phase 4 is independent and can be done at any time.
Phase 1 is the critical path — everything else builds on the
remote client and location-based cp. Phase 3 uses the same
Location pathspec wiring from Phase 1 (`net:` is just another
location).

## Implementation Notes

### CLI ↔ local c4d only

The CLI authenticates to the local c4d with mTLS (client cert
from `~/.c4d/config.yaml`, same as `c4dVersionClient()` in
`version.go`). The CLI never talks to remote nodes. All remote
operations are namespace operations on the local c4d, which
proxies to peers as needed. Like kubectl to a local API server.

This means no shared peer package between c4 and c4d. The CLI
uses `net/http` with TLS config to talk to one endpoint. c4d
uses its internal peer client for everything else.

### Location entries

Locations map names to c4d peer relationships. Current
`LocationEntry` has Address + CreatedAt. The address tells c4d
which peer to route to, not the CLI where to connect. Future
fields:
- `Identity string` — identity of the remote node
- `LastSeen time.Time` — from mDNS or announcement
- `SyncPolicy string` — eager/lazy/none

### Namespace listing protocol

c4d namespace stores paths → C4 IDs in a c4m file (root.c4m).
Listing a directory means scanning for entries under that prefix.
The listing endpoint (`GET /path/`) returns c4m format:

```
-rw-r--r-- - - project.c4m c41abc...
-rw-r--r-- - - backup.c4m c41def...
drwxr-xr-x - - renders/ -
```

Content-Type: `text/c4m`. Standard c4m entries. The current
implementation returns a simplified `name\tC4ID` format — this
should be upgraded to full c4m entries for consistency with
`c4 ls` output everywhere.

### Error handling

Remote operations can fail (network, auth, disk). The CLI should:
- Report progress before failure ("pushed 142/500 blobs")
- Be resumable (re-run same command, CAS dedup skips existing)
- Distinguish "remote unreachable" from "auth failed" from
  "content not found"
