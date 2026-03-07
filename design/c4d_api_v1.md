# c4d API v1.0 Design

> Push intent, pull content.

## Status

Work in progress. Covers the content plane, path plane, namespace
hierarchy, and content resolution model. Identity, send/receive
mechanics, and Avalanche.io business integration are not yet addressed.

## Principles

c4d is a content-addressed storage daemon that speaks c4m. The API has
two planes:

- **Content plane** — bytes addressed by C4 ID. Immutable, eternally
  cacheable, idempotent. This is the core.
- **Path plane** — c4m metadata addressed by path. Mutable only at
  location root pointers. Everything else derives from content.

The client pushes intent (a c4m declaring desired state). c4d pulls
content (from local storage, managed filesystem paths, or mesh peers).
The client only uploads bytes that c4d cannot find on its own.

## Namespace Hierarchy

c4d's namespace IS a filesystem. First-tier paths are system-owned,
following Unix conventions:

```
/home/{identity}/    per-user space (identity-scoped access)
/etc/                shared mesh configuration
/bin/                tools, scripts, executables
/tmp/                ephemeral (auto-GC, TTL-based)
/mnt/{name}/         user-created locations
```

All top-level paths are reserved by the system. User-created locations
live under `/mnt/`. The CLI's colon notation maps directly:

| CLI | HTTP path |
|-----|-----------|
| `studio:renders/` | `/mnt/studio/renders/` |
| `archive:project/` | `/mnt/archive/project/` |
| `:` (local managed) | local c4d only, not HTTP |

`/etc/` enables mesh-wide configuration. Push a config file to
`/etc/c4d/` on one node, the mesh replicates it. Nodes read config
from their namespace like it's a local filesystem.

`/home/{identity}/` is access-controlled. Users can read their own
home path. The system writes to home paths during message delivery
and relay operations.

### Reserved Path Rules

- First-tier paths cannot be created by users
- User locations are created under `/mnt/` via `c4 mk`
- C4 IDs are self-identifying (90 chars, `c4` prefix) and never
  collide with path segments
- Future system paths follow Unix convention (no dot-prefixed paths,
  no `/.ns/`, no `/.c4d/` — those are v0 mistakes)

## Content Plane

Bytes addressed by C4 ID. The core of c4d.

```
GET    /{c4id}     → content bytes
HEAD   /{c4id}     → exists? + Content-Length
PUT    /           → store bytes, return C4 ID
PUT    /{c4id}     → store bytes (server verifies ID matches body)
DELETE /{c4id}     → tombstone (shred — propagates to all mesh nodes)
```

Properties:
- `GET /{c4id}` is eternally cacheable (content never changes)
- `PUT /{c4id}` is idempotent (same content = same ID = no-op)
- `PUT /` auto-computes the C4 ID server-side (no client dependency
  on the `c4` CLI for ID computation)
- `HEAD` returns existence and size without transferring content
- `DELETE` creates a tombstone — the ID is permanently marked as
  purged and the content is removed from all mesh nodes

Content-Type for `GET` responses: `application/octet-stream`.
The C4 ID is returned in `PUT` responses as plain text.

## Path Plane

c4m metadata addressed by path. All path operations return c4m.

```
GET    /                → list system paths + locations (c4m)
GET    /mnt/            → list user locations (c4m)
GET    /mnt/{name}/     → c4m listing of location root
GET    /mnt/{name}/.../ → c4m listing of subtree
GET    /mnt/{name}/file → c4m entry (single line)
GET    /home/{id}/      → c4m listing (access-controlled)
GET    /etc/            → c4m listing of config
```

Content-Type for all path responses: `text/c4m` (canonical format).

The distinction between path plane and content plane is absolute:
- Path operations return c4m (metadata)
- Content operations return bytes
- They never mix

To get file content via a path, the client makes two requests:
1. `GET /mnt/studio/renders/frame.001.exr` → c4m entry with C4 ID
2. `GET /{c4id}` → content bytes

This mirrors the CLI: `c4 ls` returns c4m, `c4 cat` returns bytes.

## Location Management

Locations are named mount points under `/mnt/`. A location's state
is a single C4 ID — the root of its c4m listing, stored as a regular
content blob.

```
PUT    /mnt/{name}     → set/update location root
                         body: C4 ID (the root c4m listing ID)
                         If-Match: {prior-c4id} (CAS)
DELETE /mnt/{name}     → remove location
```

Responses:
- `200 OK` — content available, pointer updated
- `202 Accepted` — intent accepted, reconciling (content being
  fetched from mesh/filesystem). Body: list of missing C4 IDs.
- `409 Conflict` — CAS failed (concurrent update). Re-read, merge,
  retry.
- `404 Not Found` — location does not exist (for DELETE)

Creating a new location:
```
PUT /mnt/studio
```
If the location doesn't exist, it's created. If it does, it's
updated (with CAS via If-Match).

The location pointer is the ONLY mutable state in the entire system.
Everything else — content blobs, c4m listings, directory entries — is
immutable and addressed by C4 ID.

## Content Resolution

When a client pushes intent (a c4m with C4 IDs), c4d must resolve
those IDs to actual content. The resolution chain:

1. **Local object store** — already have it? Done.
2. **Managed filesystem paths** — local c4d has a `c4 mk :` path
   that contains this content? Read from disk.
3. **Mesh peers** — another c4d node has it? Fetch.
4. **Client** — nobody has it. Report as missing; client must push.

### Push Intent, Pull Content

```
Client                                c4d
  │                                    │
  │  scan files, build c4m             │
  │  store c4m listing via PUT /       │
  │  PUT /mnt/studio (root ID) ──────→ │
  │                                    │  resolve C4 IDs:
  │                                    │    local store ✓
  │                                    │    managed paths ✓
  │                                    │    mesh peers ✓
  │                                    │    missing: [c4x, c4y]
  │     ←── 202 Accepted + [c4x,c4y] ─│
  │                                    │
  │  PUT /c4x (only novel content) ──→ │  store
  │  PUT /c4y (only novel content) ──→ │  store
  │                                    │
  │     ←── 200 OK (reconciled) ───────│  pointer updated
```

The client uploads only content that c4d cannot find anywhere.
If the mesh already has everything, zero bytes transfer from client.

### Local c4d as Filesystem Bridge

`c4 mk :` registers a managed directory with the LOCAL c4d process.
This does two things:

1. Enables undo/redo/history for the user (CLI feature)
2. Tells c4d "content lives here on disk, serve it to the mesh"

The local c4d can read content from managed filesystem paths and
serve it to mesh peers. Remote c4d nodes never need direct filesystem
access — they pull through the mesh like any other transfer.

**Requirement:** A c4d process must be running on any machine where
`c4 mk :` is used. The local c4d is the bridge between the filesystem
and the content-addressed mesh.

When a remote node needs content that exists on a laptop's managed
directory:

```
laptop c4d                     cloud c4d
  │ managed: ~/renders/          │
  │ has c4abc on disk             │
  │                               │
  │   ←── need c4abc? ───────────│ resolving for /mnt/studio
  │   ─── here are the bytes ──→ │ stores, pointer updated
```

The managed path is just another content source in the mesh. No
special protocol — standard c4d peer transfer.

## Route Disambiguation

C4 IDs are self-identifying: 90 characters, starts with `c4`, base58
alphabet. The server distinguishes content requests from path requests
by pattern matching the first path segment:

```go
if isC4ID(segment) {
    // Content plane: GET/HEAD/PUT/DELETE by C4 ID
} else {
    // Path plane: namespace resolution
}
```

No prefix collision is possible. No ambiguous routes.

## Summary

```
Content (immutable, by identity):
  GET    /{c4id}           bytes
  HEAD   /{c4id}           exists + size
  PUT    /                 store, return ID
  PUT    /{c4id}           store (verified)
  DELETE /{c4id}           tombstone

Paths (c4m, by location):
  GET    /                 list top-level (c4m)
  GET    /mnt/{name}/...   listing or entry (c4m)
  GET    /home/{id}/...    per-user (c4m, access-controlled)
  GET    /etc/...          config (c4m)
  PUT    /mnt/{name}       set location root (CAS)
  DELETE /mnt/{name}       remove location
```

11 routes. 4 HTTP methods. Two planes. c4m everywhere.

## Not Yet Addressed

The following need design work before the API is complete:

- **User identity and authentication** — how users authenticate,
  mTLS vs tokens, identity derivation, access control model
- **Send/receive mechanics** — how users send content to each other,
  inbox model, relay coordination
- **Avalanche.io integration** — the business tie-in, subscription
  gating, cloud relay service
- **Mesh protocol** — peer discovery, inventory exchange, transfer
  orchestration (exists in v0, needs alignment with new API)
- **`/bin/` semantics** — what executables mean in this context,
  how they're invoked
- **`/tmp/` lifecycle** — TTL policy, auto-GC mechanics
- **Write operations on non-mnt paths** — who can write to `/etc/`,
  admin model
- **Bulk operations** — batched HEAD for existence checking,
  pipelined uploads, efficiency at scale
- **Long-poll / streaming** — real-time updates for reconciliation
  status (the current namespace.go has `?wait=` long-poll, worth
  keeping)
