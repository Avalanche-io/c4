# Mesh: Content-Addressed Cache Peering

## The Physics

Every c4d node is a content-addressed cache. It stores blobs by
C4 ID and maps human-readable paths to those IDs via namespace.
A single node is already a complete system — it identifies,
stores, describes, and serves content.

The mesh is what happens when nodes can find each other. Not a
protocol built on top, not a layer added alongside — an emergent
property of content-addressed caches that can talk.

Two operations make a mesh:
- **Share descriptions** (propagate c4m files between namespaces)
- **Materialize content** (ensure blobs exist where they're needed)

Everything else — relay, sync, backup, archive, sneakernet,
multi-site collaboration — is a pattern built from these two
operations plus discovery and policy.

## Five Orthogonal Concerns

### Identity

Identity is an email address. `sarah@home`, `unit-3@studio`,
`josh@avalanche.io`. The email address serves triple duty:

- **Authentication:** Your TLS cert carries your email in the
  SAN field. Self-verifying — signed by a CA the other party
  trusts. No accounts, no tokens, no passwords.
- **Routing:** The mesh resolves your email to a route. The
  sender doesn't need to know your address or even which of
  your devices is online.
- **Fallback transport:** If no mesh route exists, the c4m can
  literally be emailed. The email address is always reachable
  — it's the transport of last resort.

A studio issues certs with `@studio` addresses. A family signs
their own with `@home`. Avalanche.io issues `@avalanche.io`
certs for strangers.

Identity is not an address. `sarah@home` is Sarah regardless of
which network she's on. `unit-3@studio` is Unit-3 whether it's
in the building or on location in Morocco.

### Discovery

Discovery resolves identity to a route — not necessarily a direct
address, but a path to reach someone. That path might be direct,
or it might go through intermediaries.

**LAN (zero-config):** Every c4d broadcasts `_c4d._tcp` via
mDNS. Identity from the TLS cert in the service record. `c4 find`
shows who's on the local network. No configuration. No internet.

```
c4 find
  nas           (josh@home)        nas.local:7433
  sarah-laptop  (sarah@home)       10.0.1.42:7433
  desktop       (josh@home)        10.0.1.10:7433
```

**Mesh (peer routing):** When c4d starts, it connects to
configured peers. The mTLS handshake IS the announcement —
identity from the cert, address from the connection. Peers
remember. When you send to `sarah@home`, your node asks its
peers "can you reach `sarah@home`?" The peer that can reach
her becomes the route.

This handles the hard cases naturally:
- Sarah behind hotel NAT? She connected outbound to the home
  NAS. The NAS can reach her. It's the route.
- Sarah offline? The intermediary materializes into a transit
  path until she reconnects.
- Sarah has a cloud VM? It's always reachable. Content lands
  there. Her laptop syncs from it later.

Every node is a potential proxy for any node it can reach.
"Sending to `sarah@home`" doesn't mean delivering to a specific
device. It means ensuring Sarah's cache network has the content.
The sender doesn't need to know which device, which network, or
even which continent. The mesh routes it.

**Email (fallback):** If no mesh route exists, the c4m can be
delivered as email. `sarah@home` is a real email address. The
c4m is small enough to attach. Sarah imports it. Her node pulls
blobs through whatever channel works later.

**Directory (Avalanche.io):** For strangers. c4d registers with
the directory on startup. You look up `sarah@example.com` and
get a route — possibly through the Avalanche.io relay if no
direct path exists. This is the only mechanism that requires
accounts.

### Description

c4m files describe content. A c4m file is simultaneously:

- A filesystem description (structure, names, sizes, identities)
- A shipping manifest (what's in this bundle)
- A sync diff (compare two c4m IDs — same means in sync)
- A transfer list (walk entries to find what blobs are needed)
- A verification checklist (recompute C4 IDs on arrival)

The c4m travels independently of the content it describes.
It's small (KB-MB for projects with TB of content). It can go
through any channel — HTTPS, email, git, QR code, shuttle drive,
written on a shipping label. Since identities are email
addresses, the c4m can always reach the recipient even when no
mesh route exists. Once the receiver has the c4m, they have
complete knowledge of the content without having a single byte
of it.

This is the separation: **knowing about data and having data are
different things.** The c4m is the knowing. The blobs are the
having.

### Policy

Rules about what content should be where, and when.

**Sync:** "This directory stays in sync with these nodes."
Declared once, fulfilled automatically on every mutation.

```
# Sync this managed directory to NAS and desktop
c4 mk : --sync nas: desktop:

# Every c4 cp, c4 ln, etc. now propagates to sync targets
```

**Materialization:** "When a c4m appears in this namespace path,
ensure all referenced blobs exist locally." Eager (for backup) or
lazy (for thin mirrors). Per-path configuration on each node.

**Migration:** "Content not accessed in 30 days moves to cold
storage." "Finished renders go to archive within 24 hours." "Raw
footage stays on-prem, proxies go to cloud." Event-driven rules
responding to namespace changes.

**Retention:** TTL-bearing paths, pressure-curve reclamation,
purgatory. Already implemented. Content-addressed caches need
cache eviction policy.

### Transport

How bytes actually move. Pluggable. Orthogonal to everything else.

**HTTPS:** The default. c4d already serves blobs via GET and
accepts them via PUT. Works for any size over any network.

**Email:** The fallback. The c4m is small enough to email (KB-MB
even for TB-scale projects). If mesh routing fails, email the
c4m. The receiver imports it and pulls blobs through whatever
channel works. Since identities are email addresses, the
fallback is always available — no special configuration.

**Bundle (sneakernet):** When the network is too slow or doesn't
exist. Export a c4m and all referenced blobs to a portable
directory. Ship the drive. Import at the destination.

```
# Export
c4 bundle project.c4m: /mnt/shuttle-drive/

# Ship the drive. Import at destination.
c4 import /mnt/shuttle-drive/
```

The c4m IS the shipping manifest. Self-describing.
Self-verifying. If the drive is damaged, the c4m tells you
exactly what's missing. Incremental shipments deduplicate
automatically — CAS means you never import the same blob twice.

**Multi-band:** A single transfer uses multiple channels
simultaneously. Small blobs over the internet, large blobs on a
shuttle drive, mid-size via dedicated link. The c4m is the
coordination point — each band chips away at the set of missing
blobs. Any band can fulfill any blob. Deduplication is automatic.

**Third-party transports:** Aspera, satellite, dedicated fiber.
The mesh doesn't care how bytes arrive. A blob is a blob. Verify
the C4 ID on receipt. Done.

## Scenarios

### Personal mesh

Josh has a laptop, desktop, and NAS at home. All three run c4d
with certs from a self-signed home CA. They discover each other
via mDNS.

```
# On laptop — NAS and desktop appear automatically
c4 find
  nas       (josh@home)    nas.local:7433
  desktop   (josh@home)    desktop.local:7433

# Sync a project directory across all machines
c4 mk : --sync nas: desktop:

# Every change propagates. Content materializes on each node.
```

No accounts. No cloud. No configuration beyond the initial CA
setup. The mesh is three caches that know about each other.

### Sending to a person

Sarah is traveling for business. Josh wants to send her project
files. Sarah's laptop is on hotel WiFi in Tokyo — behind NAT,
not directly reachable.

```
# Josh sends to sarah — not to an address, not to a device
c4 cp dailies.c4m: sarah@home:
```

Josh's node finds a route to `sarah@home`. The home NAS can
reach her — Sarah's laptop connected outbound. The c4m travels
to the NAS, which materializes the blobs into a transit path
with a short TTL. The NAS forwards to Sarah's laptop. Blobs
are now cached on the NAS (transit) and available to any other
node that routes through it.

If Sarah's laptop is off entirely, the c4m and blobs sit in
transit on the NAS. When Sarah reconnects, her node pulls from
the NAS. After the transit TTL expires, the NAS reclaims the
space through existing retention machinery.

If Sarah walks into the same room as Josh, mDNS finds her
directly — content goes peer-to-peer with no intermediary.

If no mesh route exists at all, the c4m can be delivered as
email to `sarah@home`. Sarah imports it. Her node pulls blobs
through whatever channel is available — mesh, cloud, shuttle
drive. The email address is always reachable.

### Studio on an isolated network

An MPAA-compliant studio. Air-gapped network. No internet. Named
production units, editorial bays, vendor workstations.

```
# Studio CA issues certs to every node before deployment
# mDNS discovery — no internet, no directory, no accounts

c4 find
  editorial     (editorial@studio)     10.42.1.5:7433
  unit-3        (unit-3@studio)        10.42.1.30:7433
  color-suite   (color@studio)         10.42.1.40:7433
  vendor-weta   (vendor@weta)          10.42.2.5:7433

# Send plates from unit-3 to editorial
c4 cp plates.c4m: editorial:incoming/

# Vendor on location delivers VFX shots
c4 cp shots.c4m: unit-3:vendor-delivery/
```

The vendor has a cert signed by the studio CA (provisioned before
going on location). Everything works via mDNS + mTLS on the
isolated LAN. Same protocol as the cloud mesh. No internet
required at any point.

### Vendor exchange without network

The vendor's workstations aren't on the studio network at all.
Different building, different security zone. Content moves via
shuttle drive.

```
# Studio bundles plates for vendor
c4 bundle plates.c4m: /mnt/shuttle-drive/

# Drive walks across the lot

# Vendor imports
c4 import /mnt/shuttle-drive/

# Vendor works, bundles results
c4 bundle shots.c4m: /mnt/shuttle-drive/

# Drive walks back

# Studio imports, verifies every blob against the c4m
c4 import /mnt/shuttle-drive/
```

The c4m is the chain of custody document. Human-readable — you
can open it and see exactly what was shipped. Self-verifying —
every blob's identity is checked on import. If something was
corrupted in transit, you know which files and you can re-ship
only those.

### Multi-site production

Vancouver does editorial, London does VFX, Mumbai does
compositing. Each site has a local c4d cluster for high-speed
access. A cloud node coordinates.

```
# Each site's c4d peers with the coordination node
# Content materializes locally on first access (caching)
# Subsequent access is local-speed

# Vancouver pushes editorial cuts
c4 cp edit-v42.c4m: production:editorial/

# London's node sees the namespace update, lazily materializes
# the blobs it needs for VFX work

# Mumbai pulls only the compositing layers
c4 cp production:editorial/edit-v42.c4m comp-work.c4m:
```

Each site is a localized cache. The c4m (description) propagates
instantly. Blobs materialize where they're accessed. A blob
pulled once in London stays cached in London — everyone at that
site gets local-speed access from then on.

Large initial syncs might go multi-band: first batch on shuttle
drives, incremental updates over the wire.

### Cross-organization collaboration

Studio A works with Studio B. Different CAs, different meshes.
They set up a shared relay — a c4d that trusts both CAs.

```
# Shared relay trusts Studio-A-CA and Studio-B-CA
# Both studios establish it as a location

c4 mk partner-exchange: relay.example.com:7433

# Studio A pushes
c4 cp deliverables.c4m: partner-exchange:to-studio-b/

# Studio B pulls
c4 cp partner-exchange:to-studio-b/deliverables.c4m local.c4m:
```

The relay is not special software. It's a standard c4d configured
to trust multiple CAs. Anyone can run one. Avalanche.io runs a
managed one for convenience.

## Design Decisions

### Push Intent, Pull Content

Content transfer is two phases:

1. **Push intent:** The c4m arrives at the destination (or the
   next hop). Fast — KB-MB regardless of project size.

2. **Pull content:** Blobs materialize based on policy. Eagerly
   for backup, lazily for thin mirrors, into transit paths with
   short TTLs for forwarding.

`c4 cp project.c4m: nas:` means: the NAS knows about this
project right now. The NAS has the complete description. Blobs
materialize based on the NAS's policy.

The source colon controls what moves:
- `c4 cp project.c4m nas:` — send the c4m FILE (just the blob)
- `c4 cp project.c4m: nas:` — send the DESCRIBED CONTENT (c4m
  triggers materialization along the route)

### Transit Materialization

When content is sent through the mesh, each intermediate node
materializes the c4m into a transit namespace path with a short
TTL. This serves three purposes:

1. **Forwarding:** Blobs are cached locally so the next hop can
   pull them. The transit node is temporarily a cache for this
   content.

2. **Distribution:** Multiple routes through the same node share
   cached blobs. The mesh becomes a CDN naturally — content
   spreads along routes and accumulates where traffic converges.

3. **Cleanup:** Transit TTLs are short (hours to days). When
   they expire, the existing retention machinery (purgatory,
   pressure-curve reclamation) cleans up. No special transit
   garbage collection.

Transit materialization is not a new mechanism. It is the
existing materialization policy applied to a TTL-bearing
namespace path. The combination of "materialize on arrival"
and "expire after N hours" is already implementable with
existing primitives.

### Content Resolution Cascade

When a c4d needs a blob it doesn't have locally:

1. Check local store
2. Check peers (in priority order)
3. Return to client (or 404 if nobody has it)

This is a content-addressed cache hierarchy. Local store is L1.
LAN peers are L2. Remote peers are L3. On a miss, go up the
chain. Any copy is identical (content-addressed), so any source
is as good as any other.

For large blobs, multiple peers can serve different byte ranges
simultaneously. For urgent transfers, ask all peers in parallel
and take the first response. The resolver strategy is policy.

### Efficient Sync via c4m Hierarchy

At petabyte scale, you can't enumerate all blobs. But you can
compare namespace entries. Two nodes with the same c4m ID for a
path are in sync — done. Different IDs? Diff the c4m files
(a few MB of manifest, not TB of content) to find the delta.

For nested c4m files (c4m referencing other c4m files), this
becomes a Merkle-tree walk: top-level changed → which sub-c4m
changed → expand only those → find the exact delta. The c4m
hierarchy IS the efficient sync protocol.

Blob-level discovery is implicit: the receiver has the c4m, it
checks its own store, it pulls what's missing. No explicit
"what do you have?" query needed.

### Discovery Is Relay

There is no separate relay concept. Discovery and relay are the
same operation: the node that can answer "can you reach
`sarah@home`?" can also forward content to her. The discovery
path IS the delivery path.

Every node in the mesh is a potential intermediary for any node
it can reach. When you send to `sarah@home`, your node finds a
route — possibly direct, possibly through one or more
intermediaries. Content flows along that route, materializing
into transit caches at each hop. No special relay software, no
inbox model, no delivery queue. Just nodes forwarding to nodes
they can reach, with transit TTLs cleaning up behind them.

A c4d on AWS with S3 storage isn't a "relay." It's a node that
Sarah controls, always reachable, that her other nodes sync with.
Content addressed to Sarah lands there because it's reachable.
Avalanche.io runs the same thing as managed infrastructure.

### OSS vs Paid: Self-Hostable Everything

Everything is self-hostable. A self-hosted mesh with its own CA
has every capability — relay, shared locations, discovery, sync,
bundle/import, multi-band transfer.

**OSS (free, self-hosted):**
- c4 CLI, c4d (full mesh node)
- mDNS discovery
- Peer routing (mTLS connection = announcement)
- mTLS with self-signed CA
- All sync, retention, bundle, import operations
- Full mesh topology

**Avalanche.io (managed convenience):**
- Cloud-hosted c4d nodes
- Managed CA (no PKI to run)
- Directory discovery (find anyone by email)
- Managed relay (no server to provision)
- Team admin UI

Git is self-hostable. Docker is self-hostable. Websites are
self-hostable. All of those are easier to set up than a secure,
reliable c4d mesh. The value of the paid service is not having to
build and maintain that infrastructure — while never feeling
locked into a platform.

## What Needs to Be Built

### Discovery and Routing
- mDNS/Bonjour advertisement (`_c4d._tcp` service type)
- `c4 find` (scan LAN for c4d nodes)
- Implicit peer announcement (mTLS connection = announcement)
- Peer routing ("can you reach X?" → forward through intermediary)
- Store-and-forward (transit namespace paths for offline peers)
- Email fallback (c4m delivery via SMTP when no mesh route)
- Directory registration/lookup (Avalanche.io integration)

### Content Resolution
- Peer list configuration in c4d
- Blob fallback (local miss → ask peers → ask peers' peers)
- Transit materialization (TTL-bearing paths, auto-reclaimed)
- Parallel/cascading resolver strategies

### Sync
- `--sync` flag on `c4 mk :` (declare sync targets)
- Mutation propagation (push c4m to sync targets after CLI ops)
- c4m diff for incremental sync

### Bundle/Import
- `c4 bundle` (export c4m + referenced blobs to directory)
- `c4 import` (ingest blobs + register c4m from directory)
- Verification on import (recompute C4 IDs)
- Incremental bundle (only export what destination is missing)

### Remote Operations
- `c4 cp` to/from remote locations (HTTPS client)
- `c4 ls location:` (list remote namespace)
- `c4 cp location:path local.c4m:` (pull)

### Identity
- `c4 login` (provision cert from Avalanche.io CA)
- `c4 logout` (revoke cert)
- Self-signed CA setup tooling
