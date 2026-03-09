# Flow Links Implementation Plan

**Status**: Ready for implementation
**Date**: 2026-03-08
**Deferred work**: See `flow-deferred.md`

---

## Overview

This plan implements flow links across two repositories (c4, c4d) in
6 phases. Each phase produces testable, shippable artifacts. The result
is outbound and inbound flow with location resolution and CLI support.

Bidirectional reconciliation, visibility/monitoring, and advanced
configuration are explicitly deferred to `flow-deferred.md`.

Critical path:
```
c4m format (c4) → location config (c4d) → channel registry (c4d) →
  outbound (c4d) → inbound (c4d)
```

CLI integration starts after c4m format lands and iterates as c4d
features arrive.

---

## Design Decision: Format Representation

**Resolved**: Flow links use the **inline representation** from
`flow-c4m-format.md`. The flow operator and target appear on the
same line as the entry, between name and C4 ID:

```
drwxr-xr-x 2024-01-01T00:00:00Z 0 footage/ -> nas:raw/ -
dr-xr-xr-x 2024-01-01T00:00:00Z 0 plates/ <- studio:plates/ -
```

Rationale: Flow is a property of a path, like permissions or a symlink
target. It belongs on the entry, not as a phantom child. The inline
model is consistent with how hard links and symlinks already work in
c4m — same syntactic slot, same structural role.

**Fan-out**: A single entry supports one flow target. In practice
this is not a limitation — different destinations imply different
deliverables, which naturally live in separate directories with
their own flow links. When identical content must reach multiple
locations, two directories with the same C4 ID cost nothing in
storage (content-addressed dedup) and each carries its own flow.

**Mutual exclusivity**: Flow links, symlink targets, and hard link
markers occupy the same syntactic slot. An entry cannot be both
flow-linked and symlinked. This is semantically correct — combining
"follow this path" with "propagate this content" on a single entry
is incoherent.

---

## Design Decision: Channel Registry as Derived State

The channel registry is strictly derived from c4m declarations in the
namespace. The c4m is authoritative for "what flows are declared."
The registry adds only:
- Approval status (proposed → active)
- Sync state (last sync time, pending count)
- Bound/unbound tracking

There is no independent channel creation. Channels appear when c4m
files containing flow declarations are placed in the namespace.
Channels disappear when those c4m files are removed. The only admin
actions are approve/reject on proposed channels.

---

## Design Decision: Outbound Content Push

Outbound reconciliation pushes both the c4m (intent) and the
referenced blobs (content) to the peer. This departs from the
general "push intent, pull content" principle.

Justification: The flow declaration IS the intent. Once the sending
c4d knows the receiving c4d needs content (because the flow link says
so), making the receiver discover the need and pull back is an
unnecessary round-trip. The sender has the content and knows the
destination — pushing is correct here.

The receiving c4d does not need the sender in its peer list. The
sender proactively delivers. This is the one context where c4d
pushes content, and it is intentional.

---

## Design Decision: Refgraph Interaction

During inbound reconciliation, blobs must be stored BEFORE the
namespace is updated. Otherwise, the refgraph sees references to
blobs that don't exist in the store. If the pressure-curve reclaimer
runs between blob storage and namespace update, newly stored blobs
(with no namespace reference yet) could be reclaimed.

Mitigation: The reconciliation worker acquires a "hold" on the
refgraph that suppresses reclamation for the blobs it is actively
receiving. The hold releases after the namespace update completes.
This is a lightweight semaphore, not a complex locking scheme.

---

## Phase 1: c4m Format Support (c4 repo)

**Branch**: `flow-c4m-format`
**Depends on**: nothing
**Design doc**: `flow-c4m-format.md`

### 1.1 Entry struct changes (`c4m/entry.go`)

Add FlowDirection type and constants:
```go
type FlowDirection int

const (
    FlowNone        FlowDirection = iota
    FlowOutbound                         // ->
    FlowInbound                          // <-
    FlowBidirectional                    // <>
)
```

Add fields to Entry:
```go
FlowDirection FlowDirection
FlowTarget    string
```

Add helper methods:
- `IsFlowLinked() bool`
- `FlowOperator() string` — returns `->`, `<-`, `<>`, or `""`

Extend `Format()` and `Canonical()` to emit flow operator + target
between name and C4 ID:
```go
} else if e.FlowDirection != FlowNone {
    parts = append(parts, e.FlowOperator(), e.FlowTarget)
}
```

### 1.2 Decoder changes (`c4m/decoder.go`)

At the link marker detection point (~line 269), extend to three-way:

```
After name, skip whitespace, check:
  "->" : existing logic, plus flow if target matches location pattern
  "<-" : new, always flow
  "<>" : new, always flow
```

For `->` disambiguation (precedence order):
1. `->N` (no space, digit 1-9): hard link group
2. `-> c4...` or `-> -`: hard link
3. `-> WORD:...` (matches `[a-zA-Z][a-zA-Z0-9_-]*:`): flow target
4. Otherwise: symlink target

Extend `parseNameOrTarget` boundary detection:
```go
if strings.HasPrefix(rest, " -> ") ||
   strings.HasPrefix(rest, " <- ") ||
   strings.HasPrefix(rest, " <> ") {
    return ...
}
```

Add `parseFlowTarget(line, pos)` — reads `LOCATION:path`, validates
location label, returns target string and new position.

### 1.3 Encoder changes (`c4m/encoder.go`)

Extend `formatEntryPretty` to handle flow links. Flow operator and
target contribute to column width calculation for alignment.

### 1.4 Builder changes (`c4m/builder.go`)

Add EntryOption functions:
- `WithFlowOutbound(target string) EntryOption`
- `WithFlowInbound(target string) EntryOption`
- `WithFlowSync(target string) EntryOption`

Each sets FlowDirection and FlowTarget on the entry.

### 1.5 Validator changes (`c4m/validator.go`)

Add validation:
- Flow target has valid location label (`[a-zA-Z][a-zA-Z0-9_-]*`)
  followed by `:`
- Flow target path (after `:`) has no `..` or leading `/`
- Flow links mutually exclusive with symlink target and hard link marker
- `<-` and `<>` not combined with symlink mode bit

Add `ErrInvalidFlowTarget` to `errors.go`.

### 1.6 Tests

- Decoder: parse all three operators, re-encode, verify identical
- Decoder: `->` disambiguation — symlink vs flow vs hard link group
- Decoder: quoted names containing ` <- ` and ` <> ` (not operators)
- Decoder: location names with hyphens and underscores
- Encoder: canonical and pretty output for flow entries
- Builder: construct entries with each flow option
- Validator: invalid targets (missing colon, `..` in path, combined
  with symlink), valid targets
- Round-trip: parse → encode → parse produces bit-identical output
- All existing tests pass unchanged

### 1.7 Acceptance criteria

- `go test ./c4m/...` all green (new + existing)
- Round-trip fidelity for all flow operator variants
- Flow entries in Canonical() affect c4m C4 ID
- Zero changes to existing behavior

---

## Phase 2: Location Configuration (c4d repo)

**Branch**: `flow-locations`
**Depends on**: nothing (parallel with Phase 1)
**Design doc**: `flow-c4d-design.md` §C

### 2.1 Config changes (`internal/config/config.go`)

Add to Config:
```go
Locations map[string]LocationConfig `yaml:"locations"`
```

```go
type LocationConfig struct {
    Address string    `yaml:"address"`
    TLS     *TLSRef  `yaml:"tls,omitempty"`
}
```

The map key is the location name. No FlowConfig, no overrides, no
allowlists — just name-to-address mapping. Defaults handle everything
else.

Existing `Peers []string` remains for backward compatibility (anonymous
peers used for content fallback routing).

### 2.2 Location resolver (`internal/location/`)

New package:

```go
type Resolver struct {
    configs    map[string]LocationConfig
    clients    map[string]*peer.Client
    defaultTLS *tls.Config
    mu         sync.RWMutex
}
```

Methods:
- `New(configs map[string]LocationConfig, defaultTLS *tls.Config) *Resolver`
- `Resolve(name string) (*peer.Client, error)` — lazy client creation
- `IsBound(name string) bool`
- `Bind(name, address string) error` — runtime bind (for future mDNS)
- `Unbind(name string) error`
- `List() []LocationStatus`

`ErrUnbound` for locations with no address. The resolver creates
`peer.Client` instances on first Resolve() and caches them.

### 2.3 Location proxy routes (`internal/server/`)

New route pattern: `/~{location}/{path...}`

Handler:
1. Extract location name from URL path
2. `resolver.Resolve(name)` → peer.Client
3. Proxy the request (GET/PUT/DELETE) to the peer
4. Return peer response to caller

This enables the CLI to do `c4 ls nas:` through local c4d.

### 2.4 Wire into server

- Server constructor takes `*location.Resolver`
- Resolver stored on server struct
- Proxy routes registered alongside existing namespace routes

### 2.5 Tests

- Config: parse locations with/without TLS, empty locations section
- Resolver: Resolve bound location, Resolve unbound returns ErrUnbound
- Resolver: Bind/Unbind lifecycle, concurrent Resolve safety
- Proxy: mock peer server, verify GET/PUT forwarded correctly
- Proxy: unbound location returns 502 or similar
- Integration: two c4d instances, configure location, proxy request

### 2.6 Acceptance criteria

- `go test ./...` all green in c4d
- `locations:` config section parses correctly
- Resolver maps names to peer.Client instances
- `/~nas/mnt/project/` proxies to the configured nas peer
- Unbound location returns clear error

---

## Phase 3: Channel Registry (c4d repo)

**Branch**: `flow-channels`
**Depends on**: Phase 1 (c4m flow types via go.mod update), Phase 2
**Design doc**: `flow-c4d-design.md` §D, §E

**Note**: This phase requires updating c4d's `go.mod` to reference
the c4 module version that includes flow types from Phase 1. Phase 1
must be merged and tagged before this phase can begin the scanner work
(3.4). Phases 3.1-3.3 (channel data model, store, API) can proceed
in parallel with Phase 1 since they don't import c4m flow types
directly.

### 3.1 Channel data model (`internal/flow/`)

New package:

```go
type Direction int

const (
    Outbound      Direction = iota + 1
    Inbound
    Bidirectional
)

type Channel struct {
    ID         string    `json:"id"`
    LocalPath  string    `json:"local_path"`
    Direction  Direction `json:"direction"`
    Location   string    `json:"location"`
    RemotePath string    `json:"remote_path"`
    Status     string    `json:"status"`     // proposed, active
    Bound      bool      `json:"bound"`
    PeerAddr   string    `json:"peer_addr,omitempty"`
    CreatedAt  time.Time `json:"created_at"`
    LastSyncAt time.Time `json:"last_sync_at,omitempty"`
    SourceC4M  string    `json:"source_c4m"` // namespace path of declaring c4m
}
```

### 3.2 Channel store (`internal/flow/store.go`)

```go
type ChannelStore struct {
    path     string           // channels.json
    channels map[string]*Channel
    mu       sync.RWMutex
}
```

Methods:
- `NewChannelStore(path string) (*ChannelStore, error)`
- `Propose(ch *Channel) error` — add with status "proposed"
- `Approve(id string) error` — set status to "active"
- `Reject(id string) error` — remove the channel
- `Get(id string) (*Channel, error)`
- `List() []*Channel`
- `ListByLocation(name string) []*Channel`
- `RemoveBySource(c4mPath string)` — remove all channels from a c4m
- `UpdateSync(id string, t time.Time) error`

No independent `Add` or `Create` — channels only enter via `Propose`,
which is called by the flow declaration scanner. This enforces the
"derived from c4m" principle.

Persistence: atomic write of `channels.json` (temp-then-rename,
same pattern as root.c4m).

### 3.3 Channel API routes (`internal/server/`)

Under `/etc/channels/`:
- `GET /etc/channels/` — list all channels with status
- `GET /etc/channels/{id}` — single channel detail
- `POST /etc/channels/{id}/approve` — activate proposed channel
- `POST /etc/channels/{id}/reject` — reject and remove

No DELETE or POST for creating channels — channels are derived from
c4m state, not independently managed.

### 3.4 Flow declaration scanner

```go
func ScanFlowDeclarations(entries []c4m.Entry, namespacePath string) []Channel
```

Walks entries, filters for `IsFlowLinked()`, creates Channel records.
Called from:

1. **Namespace PUT handler** — after parsing the c4m being stored,
   scan it for flow declarations. Propose any new channels. Remove
   channels whose source c4m path matches but whose declarations
   changed.

2. **Startup rebuild** — walk all namespace entries, fetch and parse
   each c4m, scan for flow declarations. Reconcile with persisted
   channel store (add missing, remove stale).

3. **Namespace DELETE handler** — when a c4m is removed from the
   namespace, call `RemoveBySource(path)` to clean up channels.

### 3.5 Wire into server

- Server constructor takes `*flow.ChannelStore`
- Namespace PUT/DELETE handlers call scanner/cleanup
- Startup calls rebuild
- Channel API routes registered under /etc/

### 3.6 Tests

- ChannelStore: Propose/Approve/Reject lifecycle
- ChannelStore: persistence round-trip (save, reload, verify)
- ChannelStore: RemoveBySource cleans up correctly
- ChannelStore: concurrent access safety
- Scanner: c4m with outbound/inbound/bidirectional entries
- Scanner: c4m with no flow entries produces empty result
- Scanner: c4m with mixed regular + flow entries
- Channel API: HTTP tests for list, get, approve, reject
- Integration: PUT c4m with flow entries → channels appear as proposed
- Integration: DELETE c4m → channels removed
- Integration: re-PUT c4m with changed flows → channels updated

### 3.7 Acceptance criteria

- Channel registry persists across c4d restarts
- c4m PUT creates proposed channels
- c4m DELETE removes associated channels
- Approve/reject works via API
- No way to create channels except through c4m declarations

---

## Phase 4: Outbound Reconciliation (c4d repo)

**Branch**: `flow-outbound`
**Depends on**: Phase 3
**Design doc**: `flow-c4d-design.md` §F (outbound), §G

### 4.1 Flow engine (`internal/flow/engine.go`)

```go
type Engine struct {
    channels  *ChannelStore
    namespace *namespace.Namespace
    store     store.Store
    resolver  *location.Resolver
    refGraph  *refgraph.Graph

    mu       sync.Mutex
    workers  map[string]context.CancelFunc // channel ID → cancel
}
```

Methods:
- `NewEngine(channels, namespace, store, resolver, refGraph) *Engine`
- `Start()` — start workers for all active channels
- `Stop()` — cancel all workers, wait for shutdown
- `StartChannel(id string) error` — launch one worker
- `StopChannel(id string)` — cancel one worker

### 4.2 Outbound worker (`internal/flow/outbound.go`)

Per-channel goroutine lifecycle:

```
1. Subscribe to namespace changes under channel.LocalPath
2. Loop:
   a. Wait for namespace change (or periodic check at 30s)
   b. Resolve channel.Location → peer.Client
      - If unbound: log, continue (content accumulates locally)
   c. local_id = namespace.Resolve(channel.LocalPath)
   d. remote_id = peer.GetPath(channel.RemotePath)
      - May 404 (first push)
   e. If local_id == remote_id: continue (synced)
   f. Push c4m blob: peer.Put(store.Get(local_id))
   g. Parse c4m, for each referenced blob:
      - If !peer.Has(blob_id): peer.Put(store.Get(blob_id))
   h. Update remote namespace: peer.PutPath(channel.RemotePath, local_id)
   i. channels.UpdateSync(channel.ID, time.Now())
3. On context cancel: return
```

Error handling:
- Peer unreachable: exponential backoff (1s, 2s, 4s, ... 60s max)
- Individual blob push failure: retry, then skip channel cycle
- Namespace subscribe re-issues on any error

### 4.3 Wire into server

- Server creates Engine after all dependencies are initialized
- `Engine.Start()` called during server startup
- Channel approve → `Engine.StartChannel(id)`
- Channel reject → `Engine.StopChannel(id)`
- Server shutdown → `Engine.Stop()`

### 4.4 Tests

- Unit: outbound worker with mock peer client
  - Verify push sequence: c4m first, then blobs, then namespace update
  - Verify skip when IDs match (already synced)
  - Verify backoff on peer failure
  - Verify graceful shutdown on context cancel
- Integration: two c4d instances on localhost
  - Instance A: PUT c4m with `-> B:path/`, approve channel
  - Verify: content appears at instance B's namespace path
  - Instance A: mutate content, update c4m
  - Verify: B updates to match
  - Instance B: take offline, mutate A, bring B back
  - Verify: B catches up after reconnection

### 4.5 Acceptance criteria

- Active outbound channel pushes c4m + content to peer
- Changes propagate within 30s of namespace mutation
- Unbound location: content accumulates, pushes on bind
- Peer offline: worker backs off, resumes on reconnect
- Worker shuts down cleanly on Engine.Stop()

---

## Phase 5: Inbound Reconciliation (c4d repo)

**Branch**: `flow-inbound`
**Depends on**: Phase 4
**Design doc**: `flow-c4d-design.md` §F (inbound)

### 5.1 Refgraph hold mechanism

Add to refgraph:
```go
func (g *Graph) Hold(ids []c4.ID) func()
```

Returns a release function. While held, the listed IDs are exempt
from pressure-curve reclamation. Used by inbound reconciliation to
protect blobs between storage and namespace update.

### 5.2 Inbound worker (`internal/flow/inbound.go`)

Per-channel goroutine:

```
1. Loop:
   a. Resolve channel.Location → peer.Client
      - If unbound: sleep 30s, retry
   b. Long-poll: peer.GetPath(channel.RemotePath + "?wait=30s")
      - Timeout (304): continue loop
   c. remote_id = response
   d. local_id = namespace.Resolve(channel.LocalPath)
   e. If remote_id == local_id: continue (synced)
   f. Fetch c4m: c4m_data = peer.Get(remote_id)
   g. Parse c4m, collect all referenced blob IDs
   h. hold = refGraph.Hold(blob_ids)  // protect from reclamation
   i. Store c4m: store.Put(c4m_data)
   j. For each referenced blob:
      - If !store.Has(blob_id): store.Put(peer.Get(blob_id))
   k. Update local namespace: namespace.Put(channel.LocalPath, remote_id)
   l. hold()  // release
   m. channels.UpdateSync(channel.ID, time.Now())
2. On context cancel: return
```

The critical ordering is: store blobs → update namespace → release
hold. This ensures the refgraph never sees dangling references and
the reclaimer never purges blobs that are about to become active.

Error handling:
- Peer unreachable: exponential backoff
- Long-poll connection drop: immediate re-issue
- Blob fetch failure: retry, then skip cycle

### 5.3 Tests

- Unit: inbound worker with mock peer
  - Verify pull sequence: c4m first, then blobs, then namespace update
  - Verify hold acquired before store, released after namespace update
  - Verify skip when IDs match
  - Verify backoff on peer failure
  - Verify graceful shutdown
- Refgraph hold: verify held IDs survive reclamation pressure
- Integration: two c4d instances
  - Instance A: `<- B:path/`, approve channel
  - Instance B: PUT content at path/
  - Verify: content appears at A's local path
  - Instance B: mutate content
  - Verify: A updates
  - Instance B: go offline, come back
  - Verify: A reconnects and catches up

### 5.4 Acceptance criteria

- Active inbound channel pulls c4m + content from peer
- Long-poll detects remote changes within seconds
- Blobs protected from reclamation during inbound transfer
- Blobs stored before namespace updated (no dangling refs)
- Peer offline: graceful backoff and reconnect

---

## Phase 6: CLI Integration (c4 repo)

**Branch**: `flow-cli`
**Depends on**: Phase 1 (c4m flow types)
**Design doc**: `flow-cli-design.md`

This phase starts after Phase 1 merges. Some features (6.4) require
c4d phases to be complete for full testing, but c4m-local operations
(6.1, 6.2, 6.3) can be built and tested immediately.

### 6.1 `c4 ln` flow mode

Extend `runLn` to detect flow direction as first argument:
```bash
c4 ln -> nas: project.c4m:footage/       # outbound
c4 ln <- incoming: :backups/             # inbound
c4 ln '<>' desktop: :projects/           # bidirectional
```

Implementation:
1. Check if first arg is `->`, `<-`, or `<>`
2. Parse second arg as location reference (must contain `:`)
3. Parse third arg as local target (c4m path or managed dir path)
4. Resolve the target to a c4m + entry path
5. Set FlowDirection and FlowTarget on the entry
6. Write updated c4m

Shell note: `<>` must be quoted in most shells. `->` and `<-` pass
through unquoted as command arguments (they are not shell operators
in argument position).

### 6.2 `c4 ls` flow display

Flow entries displayed inline:

Canonical:
```
drwxr-xr-x 2026-03-04T14:22:10Z - footage/ -> nas:raw/ c4xyz...
```

Pretty:
```
drwxr-xr-x  Mar 04 14:22:10 2026 CST  footage/ -> nas:raw/  c4xyz...
```

The flow operator and target appear between name and C4 ID,
consistent with how symlink targets are displayed. No special
rendering needed — the encoder handles it (Phase 1).

### 6.3 `c4 rm` flow entries

Removing a flow link means clearing FlowDirection and FlowTarget on
the entry. The entry itself (the directory) remains.

```bash
c4 rm --flow project.c4m:footage/       # remove flow from footage/
c4 rm --flow :renders/                  # remove flow from managed dir
```

A `--flow` flag distinguishes "remove the flow declaration" from
"remove the entry." Without `--flow`, `c4 rm project.c4m:footage/`
removes the directory entry as it does today.

### 6.4 Location pathspec wiring

Wire the existing `pathspec.Location` case in `getManifest` to call
local c4d's location proxy:

```
c4 ls nas:projects/     →  GET http://localhost:7433/~nas/mnt/projects/
c4 cp project.c4m: studio:  →  PUT http://localhost:7433/~studio/...
```

This enables all location-addressed operations: `ls`, `cp`, `diff`,
`patch` through existing verb implementations with no verb-specific
changes.

Requires c4d with Phase 2 (proxy routes) running. When c4d is not
available, location pathspecs return a clear error ("c4d not running"
or "location not configured").

### 6.5 Tests

- `c4 ln -> nas: project.c4m:footage/`: verify entry gets flow fields
- `c4 ln <- studio:dailies/ :incoming/`: verify inbound flow
- `c4 ln '<>' nas: :`: verify bidirectional (shell-quoted)
- `c4 ls` with flow entries: canonical and pretty output
- `c4 rm --flow project.c4m:footage/`: verify flow cleared, entry remains
- Round-trip: ln → ls → rm → ls shows flow added then removed
- Location pathspec: mock c4d server, verify proxy requests
- Error case: location pathspec without c4d running

### 6.6 Acceptance criteria

- `c4 ln -> nas: :footage/` creates outbound flow on footage/
- `c4 ls :` shows flow inline on annotated entries
- `c4 rm --flow :footage/` clears the flow declaration
- `c4 ls nas:` works through c4d proxy
- All existing CLI tests pass unchanged

---

## Parallelism Map

```
Time →

Phase 1 (c4m format)  ████████████
Phase 2 (locations)   ████████████
                                  ↓
Phase 3 (channels)                ████████████
                                              ↓
Phase 4 (outbound)                            ████████
                                                      ↓
Phase 5 (inbound)                                     ████████

Phase 6 (CLI)                     ████████████████████████████████
         starts after Phase 1, iterates as c4d features land
```

Phases 1 and 2 are fully parallel (different repos, no dependency).
Phase 3 needs Phase 1 tagged (for go.mod) + Phase 2 merged.
Phase 6 starts after Phase 1, with 6.4 gated on Phase 2.

---

## Agent Assignment

### Agent A: c4m Format (Phase 1)
- Repo: c4 (`oss/c4`)
- Branch: `flow-c4m-format`
- Scope: c4m package only
- Files: entry.go, decoder.go, encoder.go, builder.go, validator.go, errors.go
- Tag the c4 module after merge (needed for c4d go.mod)

### Agent B: c4d Infrastructure (Phases 2, 3, 4, 5)
- Repo: c4d (`oss/c4d`)
- Sequential branches: `flow-locations` → `flow-channels` → `flow-outbound` → `flow-inbound`
- Each phase merges to main before next begins
- Phase 3 updates go.mod to pull c4 with flow types

### Agent C: CLI Integration (Phase 6)
- Repo: c4 (`oss/c4`)
- Branch: `flow-cli`
- Starts after Agent A completes
- Phases 6.1-6.3 testable without c4d
- Phase 6.4 testable after Agent B completes Phase 2

### Coordination Points

1. **Phase 1 complete → Phase 3 can update go.mod, Phase 6 can start**
2. **Phase 2 complete → Phase 6.4 can be tested end-to-end**
3. **Phase 3 complete → Agents B and C can verify channel discovery**

### Module Dependency

c4d imports c4 as a Go module. Phase 3.4 (flow declaration scanner)
calls `c4m.Entry.IsFlowLinked()` which is added in Phase 1. The
sequence is:

1. Agent A merges Phase 1 to c4 main
2. Tag c4 module (e.g., `v1.1.0` or appropriate version)
3. Agent B runs `go get github.com/Avalanche-io/c4@v1.1.0` in c4d
4. Agent B proceeds with Phase 3.4

---

## Risk Assessment

| Phase | Risk | Concern |
|-------|------|---------|
| 1 (c4m format) | Low | Additive, isolated package, clear spec |
| 2 (locations) | Low | Straightforward config + resolver |
| 3 (channels) | Low | Simple derived store + CRUD API |
| 4 (outbound) | Medium | First distributed behavior, needs integration tests |
| 5 (inbound) | Medium | Long-poll reliability, refgraph hold correctness |
| 6 (CLI) | Medium | Shell escaping for `<>`, pathspec extension |

No high-risk phases. Bidirectional (the only high-risk work) is
deferred.

### Integration Test Infrastructure

Phases 4 and 5 need two c4d instances running in the same test
process. Pattern: start two `httptest.Server` instances with separate
stores, namespaces, and configs. Configure each as a location of the
other. This avoids TLS complexity in tests while exercising the full
request path.

---

## Definition of Done

Each phase is done when:
1. All tests pass (`go test ./...`) in the relevant repo
2. No regressions in existing tests
3. Branch merged to main (fast-forward)
4. No TODOs in new code (deferred work lives in `flow-deferred.md`)
