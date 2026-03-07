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

### 1.1 Remote client package

Create `cmd/c4/internal/remote/client.go` — a thin wrapper
around `peer.Client` logic, configured from the CLI's TLS
settings.

```go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

func NewFromLocation(loc *establish.LocationEntry) (*Client, error)
func (c *Client) PutBlob(r io.Reader) (c4.ID, error)
func (c *Client) GetBlob(id c4.ID) (io.ReadCloser, error)
func (c *Client) HasBlob(id c4.ID) (bool, error)
func (c *Client) HasBatch(ids []c4.ID) (missing []c4.ID, error)
func (c *Client) PutNamespace(path string, c4mID c4.ID) error
func (c *Client) GetNamespace(path string) (c4.ID, error)
func (c *Client) ListNamespace(path string) ([]NamespaceEntry, error)
```

`NewFromLocation` reads TLS config from `~/.c4d/config.yaml`
(same certs the local c4d uses). Falls back to plain HTTP if
no TLS configured (development mode).

### 1.2 Push: local → location

Wire `pathspec.Location` as a destination in `cp.go`:

```
case src.Type == pathspec.Local && dst.Type == pathspec.Location:
    cpLocalToLocation(src, dst)
case src.Type == pathspec.C4m && dst.Type == pathspec.Location:
    cpC4mToLocation(src, dst)
case src.Type == pathspec.Managed && dst.Type == pathspec.Location:
    cpManagedToLocation(src, dst)
```

`cpC4mToLocation(src, dst)`:
1. Load local c4m file
2. Create remote client from location
3. Walk c4m entries, collect all C4 IDs
4. `HasBatch` → get missing set (falls back to individual Has
   if batch endpoint not available yet)
5. For each missing blob: read from local c4d, `PutBlob` to remote
6. `PutBlob` the c4m file itself
7. `PutNamespace` to register the c4m at `dst.SubPath`

### 1.3 Pull: location → local

```
case src.Type == pathspec.Location && dst.Type == pathspec.C4m:
    cpLocationToC4m(src, dst)
case src.Type == pathspec.Location && dst.Type == pathspec.Local:
    cpLocationToLocal(src, dst)
```

`cpLocationToC4m(src, dst)`:
1. `GetNamespace` to resolve the remote path to a C4 ID
2. `GetBlob` the c4m content
3. Decode the c4m
4. Walk entries, `HasBatch` against local c4d
5. For each missing blob: `GetBlob` from remote, `PutBlob` to
   local c4d
6. Write the c4m file locally
7. Establish and register if needed

### 1.4 `c4 ls location:`

Wire `pathspec.Location` in `ls.go`:
1. Create remote client from location
2. `ListNamespace(path)` → get entries
3. Display as usual

Requires a list endpoint on c4d — currently namespace only
supports GET (returns C4 ID for a path) and PUT/DELETE. Need
GET on a directory path to return child entries.

**New c4d endpoint:** `GET /path/` (trailing slash) returns a
JSON list of child namespace entries. Distinct from `GET /path`
which returns the C4 ID for that exact path.

### 1.5 Tests

- Integration test: start two c4d instances (different ports,
  shared CA), push c4m from one to the other via CLI remote
  client, verify content resolves on both.
- Unit tests for remote client methods.
- Test push with some blobs already present (deduplication).
- Test pull with partial local content.

### Files to create/modify

**Create:**
- `c4/cmd/c4/internal/remote/client.go`
- `c4/cmd/c4/internal/remote/client_test.go`

**Modify:**
- `c4/cmd/c4/cp.go` — add Location cases to switch
- `c4/cmd/c4/ls.go` — add Location case
- `c4d/internal/server/namespace.go` — directory listing endpoint
- `c4d/internal/server/server.go` — route directory GET

---

## Phase 2: Batch Has + Efficient Transfer

**Goal:** Large transfers are efficient. One round-trip to
determine what needs to move.

### 2.1 Batch `POST /has` endpoint

`c4d/internal/server/content.go`:

```go
// POST /has — batch existence check
// Request body: JSON array of C4 ID strings
// Response: JSON array of C4 ID strings that are NOT present
func (s *Server) handleBatchHas(w http.ResponseWriter, r *http.Request)
```

Route in `server.go`: `POST` to `/has`.

### 2.2 Wire batch into remote client

`HasBatch` prefers `POST /has` if available, falls back to
individual `HEAD /{c4id}` calls. Version detection: try batch
once, if 404, fall back permanently for this client instance.

### 2.3 Transfer progress

For large transfers, show progress:
```
pushing project.c4m: → nas:
  142/500 blobs (28%) — 2.3 GB / 8.1 GB
```

Progress callback in the remote client, wired to stderr output
in cp.go.

### Files to create/modify

**Modify:**
- `c4d/internal/server/content.go` — handleBatchHas
- `c4d/internal/server/server.go` — route POST /has
- `c4/cmd/c4/internal/remote/client.go` — HasBatch with fallback

---

## Phase 3: LAN Discovery

**Goal:** `c4 find` shows c4d nodes on the local network.
c4d advertises itself via mDNS.

### 3.1 mDNS advertisement in c4d

Add `internal/discovery/mdns.go`:

```go
func Advertise(port int, identity string, stopCh <-chan struct{}) error
```

Uses `_c4d._tcp` service type. TXT record carries identity
(from TLS cert CN/SAN). Advertise on startup, stop on shutdown.

Wire into `serve.go` after TLS config is loaded.

### 3.2 `c4 find` command

Scan for `_c4d._tcp` services on the local network. Display:
```
c4 find
  nas        (josh@home)     nas.local:7433
  desktop    (josh@home)     10.0.1.10:7433
```

Timeout: 3 seconds by default, configurable.

### 3.3 Auto-discovery for locations

When `c4 mk name: address` is called without an address, try
mDNS lookup for a node with matching identity. If exactly one
match, use it. This enables:

```
c4 find                    # see what's on the network
c4 mk nas: nas.local:7433  # establish (explicit)
```

Future: `c4 mk nas:` (no address) could auto-resolve via mDNS
if a node advertising identity "nas" is found.

### Dependencies

Go mDNS library: `github.com/grandcat/zeroconf` (well-maintained,
supports both advertising and browsing).

### Files to create/modify

**Create:**
- `c4d/internal/discovery/mdns.go`
- `c4d/internal/discovery/mdns_test.go`
- `c4/cmd/c4/find.go`

**Modify:**
- `c4d/serve.go` — start mDNS advertisement
- `c4/cmd/c4/main.go` — add "find" case

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
    identity: josh@home
  - address: cloud.example.com:7433
    identity: josh@cloud
```

`config.PeerConfig` struct with Address + Identity fields.
Backward-compatible: plain strings still work (identity derived
from TLS handshake).

### 5.2 Peer announcement

On startup, c4d announces itself to configured peers:

```
POST /peer/announce
{
  "identity": "sarah@home",
  "address": "10.42.5.7:7433"
}
```

Peers store the announcement in a routing table (in-memory,
TTL-based). Re-announce periodically (heartbeat).

### 5.3 Peer routing

When content is addressed to an identity:

```
POST /peer/route
{
  "identity": "sarah@home"
}
→ {
  "reachable": true,
  "via": "self"          // or "peer" with next-hop info
}
```

The query cascades through peers (with hop limit). The first
peer that can reach the target becomes the route.

### 5.4 Identity-based cp

```
c4 cp project.c4m: sarah:
```

If "sarah" is not an established location, resolve it:
1. Check mDNS (LAN)
2. Check peer routing (mesh)
3. Check directory (Avalanche.io, future)

If resolved, create a transient connection and push.

### 5.5 Store-and-forward

When the target identity is not currently reachable, content
lands on the intermediary (the peer that's "closest" to the
target). When the target reconnects and re-announces, the
intermediary forwards queued content.

Storage: intermediary's namespace under a queue path, e.g.
`/queue/{target-identity}/`. Delivered on next announcement.

### Files to create/modify

**Create:**
- `c4d/internal/peer/announce.go`
- `c4d/internal/peer/routing.go`
- `c4d/internal/peer/routing_test.go`
- `c4d/internal/server/peer.go` — announce/route handlers

**Modify:**
- `c4d/internal/config/config.go` — PeerConfig struct
- `c4d/internal/peer/client.go` — identity field
- `c4d/internal/peer/router.go` — identity-based routing
- `c4d/serve.go` — announcement on startup
- `c4/cmd/c4/cp.go` — identity resolution fallback
- `c4/cmd/c4/internal/remote/client.go` — identity resolution

---

## Phase 6: Sync Policy

**Goal:** Managed directories stay in sync with declared
locations automatically on every mutation.

### 6.1 Sync target declaration

```
c4 mk : --sync nas: desktop:
```

Stores sync targets in `.c4/sync` (JSON list of location names).

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

OAuth2 device flow with Avalanche.io:
1. CLI displays authorization URL + user code
2. User authenticates in browser
3. CLI polls for completion
4. Avalanche.io issues client cert (signed by its CA)
5. Cert stored in `~/.c4/auth/`

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
Phase 2 (Batch Has) ← optimization, can ship Phase 1 without it
  ↓
Phase 3 (LAN Discovery) ← independent of 1-2, can parallelize
  ↓
Phase 4 (Bundle/Import) ← independent of 1-3, can parallelize
  ↓
Phase 5 (Peer Routing) ← requires Phase 1
  ↓
Phase 6 (Sync Policy) ← requires Phase 1
  ↓
Phase 7 (Identity/Login) ← requires Phase 5
```

Phases 3 and 4 are independent and can be done at any time.
Phase 1 is the critical path — everything else builds on the
remote client and location-based cp.

## Implementation Notes

### mTLS reuse

The CLI already reads `~/.c4d/config.yaml` for TLS config (see
`c4dVersionClient()` in `version.go`). The remote client reuses
this same TLS config. No new cert provisioning needed for
self-hosted meshes.

### Location entry evolution

Current `LocationEntry` has Address + CreatedAt. Future fields:
- `Identity string` — identity of the remote node
- `LastSeen time.Time` — from mDNS or announcement
- `SyncPolicy string` — eager/lazy/none

These are additive (JSON, zero values are no-ops).

### Namespace listing protocol

c4d namespace currently stores paths → C4 IDs in a flat c4m file.
Listing a directory means scanning the c4m for entries under that
prefix. The `ListNamespace` endpoint returns JSON:

```json
[
  {"path": "/home/josh/project.c4m", "c4id": "c4abc..."},
  {"path": "/home/josh/backup.c4m", "c4id": "c4def..."}
]
```

### Error handling

Remote operations can fail (network, auth, disk). The CLI should:
- Report progress before failure ("pushed 142/500 blobs")
- Be resumable (re-run same command, batch has skips existing)
- Distinguish "remote unreachable" from "auth failed" from
  "content not found"
