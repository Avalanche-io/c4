# The Missing Link: Flow as a Filesystem Primitive

## The Progression

Filesystems have two link types. Each one extended the reach of the
previous.

**Hard links** bind two names to the same content within a single volume.
A write through either name is immediately visible through the other. The
kernel enforces this — it is the same inode. The boundary is the volume:
hard links cannot cross mount points.

**Symbolic links** bind a name to a path that may resolve anywhere the
kernel can see — across mount points, across NFS, across any filesystem
the operating system can reach. The kernel follows the reference
transparently. The boundary is the visible network: the target must be
reachable at the moment of access.

Both are links — one name refers to another — but they operate at
different scales and with different consistency guarantees. Hard links are
immediate and local. Symbolic links are synchronous and network-scoped.

There is no third type. Linking stops at the edge of the visible network.

## The Gap

Modern computing does not stop at the edge of the visible network.
Filesystems span data centers, continents, organizations, and air gaps.
Content moves between systems that cannot see each other, on timescales
from milliseconds to days. The infrastructure for moving data across
these boundaries exists — rsync, object storage, CDNs, sneakernet — but
none of it is expressed in the filesystem itself. It is bolted on:
scripts, cron jobs, sync agents, pipeline configurations. The filesystem
has no way to say "this path is related to that path on that system."

This is the same gap that existed before symbolic links. Before symlinks,
cross-filesystem references were managed by convention, scripts, and
administrator knowledge. The filesystem had no way to express them. When
symbolic links were introduced, they were controversial — a file that
isn't a file, just a pointer. Edge cases abounded. Not all filesystems
supported them. But the concept was fundamental enough that every
operating system eventually adopted them, because the alternative —
managing cross-filesystem references outside the filesystem — was worse
in every way.

The same argument applies at the next scale.

## Flow

A **flow link** binds a local path to a remote path that may not be
reachable at the moment of declaration, or ever simultaneously. Unlike a
symbolic link, a flow link does not resolve synchronously — the kernel
cannot follow a reference across an air gap. Instead, a flow link
declares that content at one path should propagate to another path,
mediated by infrastructure that handles the actual movement.

Flow links have three forms, distinguished by direction:

- **Outbound** — content here propagates there.
- **Inbound** — content there propagates here.
- **Bidirectional** — content propagates both ways, converging toward
  a common state.

The consistency guarantee is **eventual**, which is not a compromise — it
is the only consistency model that works when the link spans an ocean, an
organizational boundary, or a device that is sometimes offline. Hard
links give you immediate consistency because they share an inode.
Symbolic links give you synchronous consistency because the kernel can
reach the target. Flow links give you eventual consistency because the
infrastructure propagates changes when it can. Each guarantee is the
strongest possible for its scope.

## Why This Is Fundamental

Flow is not a feature of any particular software system. It is a
property of how data relates to other data across boundaries that
prevent direct access.

Every organization that manages data across multiple locations
re-invents flow — as rsync scripts, as S3 replication rules, as
Dropbox-like sync engines, as custom pipeline configurations. These
are all implementations of the same underlying concept: a declared
relationship between paths across systems, with infrastructure mediating
the movement.

The concept is re-invented because the filesystem has no way to express
it. There is no equivalent of `ln -s` for cross-boundary references.
There is no entry in a directory listing that says "this path is related
to a path on another system." That information lives in scripts,
documentation, and institutional knowledge — exactly where cross-mount
references lived before symbolic links.

Flow links make this relationship explicit, inspectable, and portable.
A directory listing that shows flow links tells you how a path relates
to the wider world without consulting any external configuration. The
declaration travels with the filesystem description and can be
interpreted by any infrastructure that understands it.

## Properties

**Directional.** Hard links are symmetric — both names are equal.
Symbolic links are asymmetric — one name is the "real" path, the other
is a reference. Flow links are explicitly directional — content flows
from source to destination, or converges bidirectionally. The direction
is part of the declaration.

**Asynchronous.** Flow links do not require the target to be reachable.
A declared flow link on a laptop in airplane mode is not broken — it is
pending. When connectivity is restored, accumulated changes propagate.
The link outlives any particular network session.

**Policy-composable.** Flow links interact with existing filesystem
metadata. Permissions on a flow-linked path determine local access.
A write-only path with outbound flow is a drop slot — data enters and
drains elsewhere, with no local accumulation. A read-only path with
inbound flow is a read cache — data arrives from elsewhere and cannot
be locally modified. These are not special modes; they are the natural
composition of two orthogonal concepts (permissions and flow direction).

**Idempotent under content addressing.** When the content at a path is
identified by a cryptographic hash, flow propagation is inherently safe.
Duplicate propagation is a no-op — the content already has the right
identity. Interrupted propagation is resumable — partially transferred
content can be verified and completed without retransmission. Corrupted
propagation is detectable — the identity either matches or it doesn't.
Content addressing is not required for flow links, but it makes them
dramatically more robust.

## The Scale Ladder

| Primitive | Scope | Consistency | Boundary |
|-----------|-------|-------------|----------|
| Hard link | Volume | Immediate | Mount point |
| Symbolic link | Visible network | Synchronous | Network reachability |
| Flow link | Any | Eventual | None |

Each row extends the same concept — one path refers to another — to a
wider scope, with a consistency guarantee appropriate to that scope.

The progression is not accidental. It follows from the structure of
computing itself: single machine, then networked machines, then machines
that cannot always see each other. At each stage, the filesystem needs a
way to express cross-boundary relationships. Hard links and symbolic
links addressed the first two stages. Flow links address the third.

## CAP Theorem and the Link Spectrum

The CAP theorem states that a distributed system can guarantee at most
two of three properties: **Consistency** (every read sees the latest
write), **Availability** (every request gets a response), and
**Partition tolerance** (the system operates despite network failures).

The three link types map directly onto this tradeoff space:

| Primitive | Partitions? | CAP tradeoff | Consequence |
|-----------|------------|--------------|-------------|
| Hard link | Impossible (same volume) | C + A | No tradeoff needed |
| Symbolic link | Possible (NFS, network) | C over A | Fails on partition |
| Flow link | Expected (air gaps, orgs) | A over C | Eventual consistency |

**Hard links** operate within a single volume. Partitions cannot occur.
The CAP tradeoff does not apply, and both consistency and availability
are guaranteed by the kernel.

**Symbolic links** operate across networks where partitions are possible
but exceptional. When a partition occurs — the NFS mount goes down, the
network drops — the symlink **fails**. It returns an error rather than
stale data. This is the correct choice: consistency over availability.
When partitions are rare, it is better to fail loudly than to silently
serve outdated content.

**Flow links** operate across boundaries where partitions are the normal
condition. A laptop is usually offline relative to the data center. Two
organizations are always partitioned relative to each other — there is
no shared filesystem, no direct mount. Flow links **keep working**
through partitions: you continue reading and writing locally, and
changes propagate when connectivity exists. This is the correct choice:
availability over consistency. When partitions are the norm, a system
that fails on every partition is a system that is always failing.

The consequence of choosing availability over consistency is that
concurrent writes at different locations can diverge. This is not a
flaw in the design — it is a theorem. Any system that operates across
partition boundaries and remains available will have consistency
conflicts. Bidirectional flow links acknowledge this explicitly:
convergence requires conflict resolution, which is a policy decision
made by the infrastructure, not a property of the link.

This mapping is not a post-hoc rationalization. The scale ladder — from
volume-local to network-visible to partition-tolerant — follows the same
structure as the CAP theorem's tradeoff space. Each link type exists
because the tradeoff at its scale demands it. Hard links exist because
intra-volume linking needs no tradeoffs. Symbolic links exist because
network-scoped linking can afford to sacrifice availability for
consistency. Flow links exist because partition-scoped linking must
sacrifice consistency for availability.

The theorem does not merely permit a third link type. It **requires**
one — or demands that we accept the alternative: managing cross-partition
relationships entirely outside the filesystem, with no formal semantics,
no inspectability, and no portability. That is the current state of
affairs, and it is precisely analogous to the state of cross-filesystem
references before symbolic links were introduced.

## What Flow Is Not

Flow is not synchronization software. It is not a replication protocol.
It is not a backup system. These are all implementations that could
fulfill a flow declaration, just as NFS is an implementation that makes
symbolic links work across machines. The flow link itself is just the
declaration: this path relates to that path, in this direction.

Flow is not a transport mechanism. How content moves — over TCP, on a
hard drive in a shipping container, through a cloud relay — is an
infrastructure concern. The flow link does not specify transport any more
than a symbolic link specifies whether the target is on a local disk or
an NFS mount. The link declares the relationship; the infrastructure
fulfills it.

Flow is not orchestration. A flow link does not specify when propagation
happens, what to do on conflict, or how to prioritize competing updates.
These are policy decisions made by the infrastructure interpreting the
link, not properties of the link itself. A symbolic link does not specify
how NFS should cache; a flow link does not specify how sync should
converge.
