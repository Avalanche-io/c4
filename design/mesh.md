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

## Design Decisions

### Transfer Model: Push Intent, Pull Content

Content transfer follows a two-phase model:

1. **Push intent**: Register the c4m reference in the remote
   namespace. This is fast — just the c4m file itself (small) and
   a namespace PUT.

2. **Pull content**: Blobs are fetched on demand when the remote
   node (or a client of it) actually accesses them. The remote
   becomes a "thin" mirror that fills in content as needed.

Eager transfer is an optimization on top of this. After pushing
the c4m reference, the sender can proactively push blobs the
remote doesn't have. A batch `POST /has` endpoint (send a list
of C4 IDs, get back the missing set) collapses discovery to one
round-trip. But the baseline is: intent is pushed, content is
pulled.

This means `c4 cp project.c4m: nas:` registers the c4m on the
NAS immediately. The NAS can serve the c4m to clients right away.
Blob fetches happen lazily or are backfilled eagerly — either
way, the namespace is live instantly.

### Relay: Just Another Node

A relay is not a special service. It's a c4d instance that
accepts content on behalf of a recipient.

Bob can run his own c4d on AWS with S3 storage. Alice sends
content to Bob by pushing to Bob's relay. Avalanche.io's relay
is the same thing — just managed infrastructure so users don't
have to provision their own.

This keeps the protocol uniform. Every relay speaks the same c4d
API. The only difference is who operates the infrastructure and
who manages the identity/CA.

### Shared Locations: Just a Location

A shared location is not a special namespace concept. It's a
location that multiple users have established.

```
# Everyone on the team establishes the same location
c4 mk OurShare: shared.example.com:7433

# Browse it
c4 ls OurShare:/path

# Push to it
c4 cp shots.c4m: OurShare:renders/shots.c4m
```

The remote c4d handles access control — who can read, who can
write, to which paths. The namespace on the shared node is just
a namespace. Paths are caller-specified, not auto-scoped by
identity (though the remote node may enforce path restrictions
based on the caller's mTLS identity).

### Identity and Authentication

**Self-hosted mesh:** Sign and accept your own certs. A team or
studio runs its own CA, issues certs to members, and all nodes
in the mesh trust that CA. This is the fully self-hosted model —
no external dependencies.

**Avalanche.io CA:** For collaborating with strangers or avoiding
the overhead of running your own CA. `c4 login` provisions a
client cert signed by the Avalanche.io CA. Nodes that trust the
Avalanche.io CA can authenticate any logged-in user.

**CA hierarchy:** Studios, vendors, and distributed teams can
build CA hierarchies. A studio CA signs sub-CAs for departments
or vendor relationships. On-location and internet-limited teams
can use pre-provisioned certs without live CA access.

**Cross-org federation:** Node A trusts CA-1, Node B trusts CA-2.
They can't talk directly. A relay that trusts both CAs bridges
them. This is how sending to external recipients works.

Token-based authentication may be added as a lighter-weight
option for cases where mTLS cert management is too heavy. The
mTLS model remains the foundation.

### OSS vs Paid: Self-Hostable Everything

Everything is self-hostable. The protocol is open. A self-hosted
mesh with its own CA has every capability — relay, shared
locations, team access control, cross-node sync.

The paid service is convenience, not lock-in:

**OSS (free, self-hosted):**
- c4 CLI (all local operations)
- c4d (run your own nodes, relay, shared locations)
- Node-to-node push/pull (direct mTLS)
- All retention features
- All c4m operations
- Full mesh topology

**Avalanche.io subscription:**
- Cloud-hosted c4d nodes (managed storage + compute)
- `c4 login` (managed identity, no CA to run)
- Managed relay (no server to provision)
- Team admin UI (shared locations, access control)
- Cross-org federation (managed CA bridging)

The analogy: Git is self-hostable, Docker is self-hostable,
websites are self-hostable. And all of those are easier to set up
than a secure, reliable c4d + relay + identity system. The value
is that you don't have to build and maintain that infrastructure
yourself, while never feeling locked into a platform. Good will
towards the community.

### Transfer Priorities and Bandwidth

When syncing large amounts of content, how is bandwidth managed?

- Priority queue: user-initiated transfers before background sync
- Bandwidth limits: configurable per-location
- Resume: interrupted transfers pick up where they left off
- Deduplication: CAS means the same blob is never sent twice across
  any path in the mesh

### Conflict Resolution

c4m files are content-addressed. Two different modifications produce
two different c4m IDs. There's no conflict at the storage level —
both versions exist. The question is which one the namespace points
to.

Current model: last-writer-wins (simplest). CAS with If-Match
(optimistic concurrency) is already implemented in the c4d API.
Fork detection (both versions preserved, user resolves) is a
future option if needed.

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
