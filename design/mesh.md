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

Your TLS cert says who you are. It's self-verifying — signed by
a CA the other party trusts. No accounts, no tokens, no
passwords. A studio issues certs to its people. A family signs
their own. Avalanche.io runs a CA for strangers.

Identity is not an address. "Sarah" is Sarah regardless of which
network she's on. "Unit-3" is Unit-3 whether it's in the studio
or on location in Morocco.

### Discovery

Discovery resolves identity to network address. Three mechanisms,
same identity model:

**LAN (zero-config):** Every c4d broadcasts `_c4d._tcp` via
mDNS. Identity from the TLS cert in the service record. `c4 find`
shows who's on the local network. No configuration. No internet.

```
c4 find
  nas           (josh@home)        nas.local:7433
  sarah-laptop  (sarah@home)       10.0.1.42:7433
  desktop       (josh@home)        10.0.1.10:7433
```

**Mesh (peer announcement):** When c4d starts, it announces
itself to configured peers — "I'm Sarah, I'm currently at this
address." Peers remember. When you send to Sarah, your node asks
its peers until someone knows where she is. She moved to hotel
WiFi? Her node re-announced. The mesh tracks her.

**Directory (Avalanche.io):** For strangers. c4d registers with
the directory on startup. You look up sarah@example.com and get
her current endpoint. This is the only mechanism that requires
accounts.

All three resolve identity to address. Once resolved, it's
HTTPS + mTLS. The protocol doesn't care how you found the node.

### Description

c4m files describe content. A c4m file is simultaneously:

- A filesystem description (structure, names, sizes, identities)
- A shipping manifest (what's in this bundle)
- A sync diff (compare two c4m IDs — same means in sync)
- A transfer list (walk entries to find what blobs are needed)
- A verification checklist (recompute C4 IDs on arrival)

The c4m travels independently of the content it describes.
It's small (KB-MB for projects with TB of content). It can go
through any channel — HTTPS, email, git, QR code, written on a
shipping label. Once the receiver has the c4m, they have complete
knowledge of the content without having a single byte of it.

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
files. Sarah's laptop is on hotel WiFi in Tokyo. Her c4d
announced itself to the home NAS (a shared mesh peer) when she
connected.

```
# Josh's node resolves "sarah" through the mesh
c4 cp dailies.c4m: sarah:

# Sarah's node got the c4m instantly (small).
# Blobs materialize as she accesses them, or eagerly if
# her node's policy says so.
```

If Sarah walks into the same room as Josh, mDNS finds her
directly — no mesh resolution needed. If she's offline, the c4m
queues on the shared peer (the NAS, a cloud node) and delivers
when she reconnects. The transport adapts; the intent is the same.

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

1. **Push intent:** Register the c4m in the remote namespace.
   Fast — just the c4m file (small) and a namespace PUT.

2. **Pull content:** Blobs materialize on demand or by policy.
   The remote can serve the c4m immediately. Blobs follow.

`c4 cp project.c4m: nas:` means: the NAS knows about this
project right now. The NAS has the complete description. Blobs
materialize based on the NAS's policy — eagerly for backup,
lazily for thin mirrors.

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

A batch `POST /has` endpoint (send a list of C4 IDs, get back the
missing set) collapses blob-level discovery to one round-trip.

### The Relay Is Just a Node

A relay is a c4d that accepts content on behalf of others. Bob
runs his own on AWS with S3 storage. Alice pushes to Bob's relay.
Avalanche.io's relay is the same thing — managed infrastructure.

The protocol is uniform. Every relay speaks the c4d API. The only
difference is who operates it.

### OSS vs Paid: Self-Hostable Everything

Everything is self-hostable. A self-hosted mesh with its own CA
has every capability — relay, shared locations, discovery, sync,
bundle/import, multi-band transfer.

**OSS (free, self-hosted):**
- c4 CLI, c4d (full mesh node)
- mDNS discovery
- Peer announcement
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

### Discovery
- mDNS/Bonjour advertisement (`_c4d._tcp` service type)
- `c4 find` (scan LAN for c4d nodes)
- Peer announcement (c4d → peer "I'm online at this address")
- Peer resolution (c4 → peer "where is Sarah?")
- Directory registration/lookup (Avalanche.io integration)

### Content Resolution
- Peer list configuration in c4d
- Blob fallback (local miss → ask peers)
- Batch `POST /has` endpoint
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
