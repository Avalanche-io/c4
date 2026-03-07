# Mesh: Multi-Node Content Distribution

## The Model

Every c4d node is sovereign. It manages its own store, its own
namespace, its own retention. Nodes connect to other nodes to
exchange content — but no node controls another.

A mesh is not a hierarchy. It's a set of nodes that can reach each
other. A single user might have three nodes (laptop, NAS, cloud).
A team might share a cloud node while each member also has local
nodes. An organization might have dozens. The topology is whatever
the user builds — star, chain, full mesh, or something ad hoc.

The primitives are simple:
- **Locations**: a named reference to a remote c4d endpoint
- **Push**: send content from here to there
- **Pull**: fetch content from there to here
- **Sync**: bidirectional — push what they're missing, pull what
  we're missing

Everything else — backup, sharing, collaboration, cloud relay — is
a pattern built from these primitives.

## User Scenarios

### Solo user, two machines

Alice has a laptop and a NAS. She wants her c4m files backed up.

```
# On laptop: establish the NAS as a location
c4 mk nas: nas.local:7433

# Push a project to the NAS
c4 cp project.c4m: nas:

# Later, on a different machine, pull it back
c4 cp nas:project.c4m local.c4m:
```

Both nodes have mTLS certs from the same CA. No accounts, no cloud,
no subscription. This is the OSS baseline — two c4d instances
talking to each other.

### Solo user, cloud relay

Bob wants his content accessible from anywhere without running a
server at home.

```
# Authenticate with Avalanche.io
c4 login

# His cloud node is automatically available as a location
c4 cp project.c4m: cloud:

# From another device, after login
c4 cp cloud:project.c4m local.c4m:
```

`c4 login` establishes a cloud-hosted c4d node as the user's
default remote. The cloud node is just another c4d — same protocol,
same namespace model. The only difference is Avalanche.io manages
the infrastructure and the identity.

### Sending content to someone

Alice wants to send dailies to Bob.

```
# Alice sends
c4 cp dailies.c4m: bob@example.com:

# Bob receives (checks inbox)
c4 ls inbox:
c4 cp inbox:dailies.c4m local-dailies.c4m:
```

The relay handles delivery. Alice's content is pushed to the relay,
placed in Bob's inbox namespace path, and Bob pulls it. Both Alice
and Bob need Avalanche.io accounts (the relay must know both
identities).

### Team with shared storage

A VFX team shares a cloud node for their project.

```
# Team admin creates a shared location
c4 mk vfx-project: cloud.avalanche.io:7433

# Each team member establishes it
c4 mk vfx-project: cloud.avalanche.io:7433

# Anyone can push/pull
c4 cp shots.c4m: vfx-project:
c4 cp vfx-project:shots.c4m local-shots.c4m:
```

The shared node has access control — who can read, who can write,
managed through the Avalanche.io team admin. The c4d on the shared
node enforces permissions via mTLS identity.

### Organization with local + remote

A studio has on-prem storage for active work, cloud for archive and
remote access.

```
# Local high-speed node
c4 mk render-farm: render.internal:7433

# Cloud archive
c4 mk archive: cloud.avalanche.io:7433

# Sync between them
c4 cp render-farm:finished/ archive:
```

Each node has its own retention policy. The render farm might have
aggressive TTLs (clean up after 14 days). The archive keeps
everything. Content flows between them via push/pull.

## Design Questions

### Transfer Protocol

How does content move between nodes?

**Option A: Blob-level transfer.** Push/pull individual blobs by
C4 ID. The sender walks a c4m, checks which blobs the receiver
already has (`HEAD /{c4id}`), and sends only the missing ones.
Simple, uses existing store API.

**Option B: c4m-level transfer.** Push/pull the c4m itself plus a
manifest of what's needed. The receiver diffs the c4m against its
store and requests missing blobs. More efficient for large c4m
files (one round-trip to determine what's needed).

**Option C: Lazy transfer.** Register the c4m in the remote
namespace immediately. Blobs are fetched on demand when accessed.
The remote node becomes a "thin" mirror that fills in content as
needed.

These aren't mutually exclusive. Blob-level is the foundation.
c4m-level is an optimization. Lazy is a mode.

### Content Discovery

When you `c4 cp project.c4m: nas:`, how does the remote know what
blobs are needed?

The sender has the c4m and can enumerate its blobs. For each blob,
it checks whether the remote already has it. This is the
Has/HEAD check — already in the c4d API. The remaining set is
what gets transferred.

For large c4m files, this is many HEAD requests. A batch endpoint
(`POST /has` with a list of IDs, returns which ones are missing)
would collapse this to one round-trip.

### Namespace Model for Shared Locations

When Alice pushes `project.c4m:` to a shared location, where does
it land in the remote namespace?

Options:
- Under Alice's identity: `/home/alice@example.com/project.c4m`
- Under a shared path: `/mnt/team-project/project.c4m`
- Caller specifies: `c4 cp project.c4m: team:renders/project.c4m`

The `/home/` scoping already exists for identity-isolated paths.
Shared paths under `/mnt/` need team-level access control — who
can write to `/mnt/team-project/`.

### Identity and Authentication

**Self-hosted nodes:** mTLS with certs from a shared CA. Already
implemented. No accounts needed.

**Avalanche.io cloud nodes:** `c4 login` authenticates and provisions
client certs. The cloud c4d uses the same mTLS model — `c4 login`
just automates the cert exchange.

**Cross-org federation:** Node A trusts CA-1, Node B trusts CA-2.
They can't talk directly. The relay (which trusts both CAs) bridges
them. This is how sending to external recipients works.

### OSS vs Paid Boundary

What's free, what requires a subscription?

**OSS (free, self-hosted):**
- c4 CLI (all local operations)
- c4d (run your own nodes)
- Node-to-node push/pull (direct mTLS)
- All retention features
- All c4m operations

**Avalanche.io subscription:**
- Cloud-hosted c4d node (storage + compute)
- `c4 login` (managed identity)
- Relay delivery (send to others by email)
- Team management (shared locations, access control)
- Cross-org federation (relay bridges trust domains)

The boundary is infrastructure and identity management, not
features. The protocol is the same. A self-hosted mesh with its own
CA has every capability except the convenience of managed
infrastructure.

### Transfer Priorities and Bandwidth

When syncing large amounts of content, how is bandwidth managed?

- Priority queue: user-initiated transfers before background sync
- Bandwidth limits: configurable per-location
- Resume: interrupted transfers pick up where they left off
- Deduplication: CAS means the same blob is never sent twice across
  any path in the mesh

### Conflict Resolution

What happens when two users modify the same c4m on different nodes?

c4m files are content-addressed. Two different modifications produce
two different c4m IDs. There's no conflict at the storage level —
both versions exist. The question is which one the namespace points
to.

Options:
- Last-writer-wins (simplest, current model)
- CAS with If-Match (optimistic concurrency, already implemented)
- Fork detection (both versions preserved, user resolves)

## What Needs to Be Built

### Protocol Layer
- Batch `POST /has` endpoint (check many IDs in one request)
- Transfer manager (queue, prioritize, resume transfers)
- c4m-aware transfer (walk c4m, diff against remote, send missing)

### Identity Layer
- `c4 login` (OAuth flow with Avalanche.io, provisions client cert)
- Token refresh / cert rotation
- Logout / revocation

### Relay Layer
- Inbox model (sender pushes, recipient pulls)
- Delivery notifications (long-poll or webhook)
- Cross-org bridging (relay trusts multiple CAs)

### Team Layer
- Shared location creation and membership
- Access control (read/write/admin per path)
- Team billing integration

### CLI Commands
- `c4 cp source: dest:` (remote-to-remote, remote-to-local, local-to-remote)
- `c4 sync location:` (bidirectional sync)
- `c4 login` / `c4 logout`
- `c4 ls location:` (list remote namespace)
- `c4 send file.c4m: user@example.com` (relay delivery)
