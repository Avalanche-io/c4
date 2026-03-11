# Mesh Trust: Identity, Bootstrap, and Security Scopes

Trust is the foundation beneath the mesh. Before nodes can share
descriptions, materialize content, or propagate configuration,
they need to know who they're talking to and whether to believe
them. This document designs that foundation.

## Core Principle: Trust Is Namespace Content

All trust configuration lives in the c4d namespace as content-
addressed documents. c4d watches namespace paths under `/etc/`
for changes and reconfigures itself accordingly. There is no
separate trust API, no config file format for peers, no out-of-
band coordination protocol.

This means:
- Flow channels that sync `/etc/` propagate trust configuration
  automatically. Adding a peer on one node can propagate to the
  mesh without manual configuration on every node.
- Trust changes are versioned (Merkle tree commits). You can
  diff two namespace states to see exactly what trust changed.
- The same `c4 cp`, `c4 ls`, `c4 rm` operations that manage
  content also manage trust. No new commands for trust admin.

The one exception: **private keys never enter the namespace.**
Keys live in the local filesystem (`~/.c4d/`) and are never
content-addressed, never propagated, never stored in the blob
store. The namespace holds public certificates and configuration
only.

## Identity Hierarchy

```
Person/Org (CA)
  └── Node (cert signed by CA)
        └── Scope (namespace subtree access)
```

**Person/Org** is the root of trust. Represented by a CA
certificate. A person running their own mesh is their own CA.
An organization has an organizational CA. Avalanche.io runs a
managed CA for users who don't want to manage infrastructure.

**Node** is a c4d instance. Identified by a certificate signed
by its owner's CA. The cert CN is the node name (`abyss`,
`beast2`, `nas`). The cert SAN carries the owner identity
(email address or organizational identifier).

**Scope** is a namespace subtree that a node or identity has
access to. Scopes are Phase 4, but the namespace paths are
reserved in Phase 1 so the structure is forward-compatible.

## Phase Overview

### Phase 1: Manual Trust (Self-Sovereign)

CLI-driven trust establishment. `c4 init` creates identity.
`c4 join` connects to a peer. All ceremony is explicit and
requires the operator to be at the keyboard. No automation,
no discovery, no cloud dependency.

This phase defines the namespace conventions that all future
phases build on. Getting these paths right is critical.

**Enables:** Two or more machines under one person's control,
connected over a known network. The current abyss + beast2
setup, but with namespace-native trust instead of manually
copied cert files.

### Phase 2: Proximity Trust (LAN Discovery)

mDNS advertisement + approval gesture. Nodes discover each
other on the local network. Joining requires a confirmation
step (PIN code or approval notification) to prevent blind
trust of anything on the LAN.

**Enables:** The "Apple new device" experience. Power on a
new machine, it finds the mesh, you approve it from an
existing node, done.

### Phase 3: Brokered Trust (Avalanche.io)

Avalanche.io (or a self-hosted equivalent) acts as a CA and
directory. `c4 login` provisions a cert. Nodes with the same
account identity are automatically meshed. NAT traversal via
relay nodes.

**Enables:** Distributed meshes across the internet. Adding
a cloud VM. Collaborating with someone in another city.

### Phase 4: Scoped Trust (Multi-Party Collaboration)

Security scopes grant partial namespace access to external
identities. A studio shares `/projects/current/` with a
vendor. A family shares `/photos/` across households. Each
scope is a namespace subtree with an access control list.

**Enables:** Collaboration between different trust domains.
Shared workspaces. Per-subtree access control.

---

## Phase 1: Manual Trust — Detailed Design

### Namespace Convention: `/etc/`

c4d watches these paths in its namespace Merkle tree. Changes
trigger reconfiguration. The paths mirror Unix `/etc/` by
analogy — system configuration that the daemon reads.

```
/etc/
  identity/
    cert            → this node's certificate (PEM)
    name            → human-readable node name
    owner           → owner identity string
  ca/
    {name}          → trusted CA certificate (PEM)
  peers/
    {name}/
      address       → host:port
      ca            → which CA signed this peer (name, matches /etc/ca/{name})
      fingerprint   → cert fingerprint (TOFU mode, optional)
      enabled       → "true" or absent (for soft-disable without deleting)
  scopes/           → reserved for Phase 4
```

Each leaf points to a C4 ID of a content blob in the store.
The blob contains the value (PEM text, address string, etc.).
Directory nodes are Merkle tree interior nodes.

### What c4d Watches

c4d subscribes to namespace mutations under `/etc/`. On commit:

| Path pattern | Action |
|---|---|
| `/etc/ca/*` | Rebuild TLS trust pool from all CA certs |
| `/etc/peers/*/address` | Connect to new peer / update endpoint |
| `/etc/peers/*/enabled` | Enable or disable peer connection |
| `/etc/identity/cert` | Reload server TLS certificate |

Watching is efficient: the Merkle tree commit tells c4d exactly
which subtree changed. If `/etc/peers/nas/address` changed but
`/etc/ca/` didn't, only the peer connection is reconfigured —
the TLS trust pool is untouched.

### Private Key Storage

Private keys are **never** in the namespace:

```
~/.c4d/
  key.pem           → node private key
  ca-key.pem        → CA private key (only on the CA node)
```

The config file (`~/.c4d/config.yaml` or equivalent) points to
these paths. The private key is loaded at startup and held in
memory. It never enters the content-addressed store.

### `c4 init` — First Node

Creates identity and becomes the CA for a new mesh. The CLI
talks to the local c4d to populate the namespace and store
the CA cert. The private keys are written to `~/.c4d/` on
the local filesystem.

```bash
c4 init --name abyss --owner josh@example.com
```

Steps:
1. Generate Ed25519 CA key pair → `~/.c4d/ca-key.pem`
2. Generate self-signed CA cert (CN: `josh@example.com`,
   validity: 10 years) → blob in store
3. Generate node key pair → `~/.c4d/key.pem`
4. Generate node cert signed by CA (CN: `abyss`,
   SAN: `josh@example.com`, SAN: local IPs) → blob in store
5. Populate namespace:
   - `/etc/ca/local` → CA cert blob ID
   - `/etc/identity/cert` → node cert blob ID
   - `/etc/identity/name` → blob containing "abyss"
   - `/etc/identity/owner` → blob containing "josh@example.com"
6. Write `~/.c4d/config.yaml` with key paths
7. Start listening with mTLS

If `--owner` is omitted, the mesh is anonymous (self-sovereign,
no email identity). The CA CN becomes the node name instead.

### `c4 join` — Additional Nodes

Adds a node to an existing mesh by requesting a cert from the
CA node. The CLI drives the entire ceremony — generating the
CSR, submitting it, and storing the signed cert.

```bash
c4 init --name beast2
c4 join abyss.local:7433
```

Steps:
1. `c4 init --name beast2` generates a temporary self-signed
   cert and key (bootstrap identity, not yet trusted)
2. `c4 join abyss.local:7433`:
   a. Connect to abyss over TLS (skip verify — bootstrap)
   b. Send a Certificate Signing Request (CSR) with CN: `beast2`
   c. Abyss displays: `"beast2 wants to join. Approve? [y/n]"`
   d. On approval, abyss signs beast2's CSR with the CA key
   e. Abyss returns: signed cert + CA cert
   f. Beast2 stores the signed cert and CA cert
   g. Abyss adds beast2 to its `/etc/peers/`:
      - `/etc/peers/beast2/address` → beast2's address
      - `/etc/peers/beast2/ca` → "local"
   h. Beast2 populates its own namespace:
      - `/etc/ca/local` → CA cert (same as abyss)
      - `/etc/identity/cert` → signed node cert
      - `/etc/identity/name` → "beast2"
      - `/etc/identity/owner` → owner from CA cert SAN
      - `/etc/peers/abyss/address` → "abyss.local:7433"
      - `/etc/peers/abyss/ca` → "local"
3. Both nodes reconnect with mTLS. Handshake succeeds because
   both trust the same CA.

The bootstrap connection (step 2a) is the only moment where
TLS verification is skipped. After the join completes, all
connections use fully verified mTLS.

### Approval Surface

The join approval (step 2c) needs a "face" — the daemon needs
a way to present the approval prompt to the operator. Options
for Phase 1 (in order of implementation simplicity):

1. **CLI poll**: `c4 requests` shows pending join requests.
   `c4 approve beast2` approves. Simple, works everywhere.
2. **Stdout/log**: c4d prints the request to its log. Operator
   runs a CLI command to approve. Minimal daemon changes.
3. **Notification**: macOS `osascript` / freedesktop notify /
   Windows toast. Nice UX, platform-specific.

Phase 1 uses option 1 (CLI poll). Phase 2 adds option 3.

The pending request lives in the namespace too:

```
/etc/pending/
  beast2/
    csr           → the CSR blob
    address       → requester's address
    fingerprint   → requester's bootstrap cert fingerprint
    timestamp     → when the request arrived
```

`c4 approve beast2` tells c4d to sign the CSR, move the peer
entry from `/etc/pending/beast2/` to `/etc/peers/beast2/`, and
deliver the signed cert. `c4 deny beast2` deletes the pending
entry.

### Adding a Peer Manually (No Join Ceremony)

For advanced users or automation, skip the join handshake
entirely. Just place the right content at the right paths:

```bash
# On abyss — trust beast2's CA and add it as a peer
c4 cp beast2-ca.pem etc/ca/beast2-mesh
c4 cp - etc/peers/beast2/address <<< "192.168.1.170:7433"
c4 cp - etc/peers/beast2/ca <<< "beast2-mesh"
```

c4d sees the namespace change, rebuilds its trust pool and
peer list, connects to beast2. No join command needed. This
is the escape hatch for pre-provisioned environments (CI,
studios with existing PKI, air-gapped deployments).

### Removing a Peer

```bash
# Soft-disable (keeps config, stops connecting)
c4 cp - etc/peers/beast2/enabled <<< "false"

# Hard remove (delete peer entry entirely)
c4 rm etc/peers/beast2
```

Removing a peer from `/etc/peers/` disconnects c4d from that
peer. It does NOT revoke the peer's certificate — the peer
can still connect if it knows the address and the CA is still
trusted.

### Certificate Revocation

Full CRL/OCSP is Phase 3 (needs infrastructure). Phase 1
revocation is blunt but effective:

**Remove the CA**: If a CA is compromised, remove it from
`/etc/ca/`. All peers signed by that CA become untrusted.
c4d rebuilds its trust pool and drops those connections.

**Node-level block**: Add a fingerprint blocklist:

```
/etc/blocked/
  {fingerprint}     → empty blob (presence = blocked)
```

c4d checks incoming cert fingerprints against `/etc/blocked/`
during the TLS handshake. Blocked certs are rejected even if
signed by a trusted CA.

This is coarse-grained. Phase 3 adds proper revocation lists
distributed through the namespace.

### Configuration Migration

The current setup uses `~/.c4d/config.yaml` for peer addresses,
TLS paths, and listen address. Phase 1 migrates peer and CA
configuration into the namespace:

**Stays in config.yaml:**
- `listen` — bind address (local to this machine)
- `key` — path to private key file (local filesystem)
- `cert` — path to certificate file (can also be in namespace)

**Moves to namespace:**
- `peers` → `/etc/peers/`
- `ca` → `/etc/ca/`
- TLS trust pool built from `/etc/ca/*` instead of config

The migration path: c4d reads `config.yaml` at startup. If
`/etc/peers/` is empty but config.yaml has peers, c4d
populates `/etc/peers/` from config (one-time migration).
After that, the namespace is authoritative.

### Wire Protocol for Join

The join handshake needs a minimal protocol on top of TLS.
Phase 1 uses HTTP endpoints on c4d:

```
POST /join
  Body: PEM-encoded CSR
  Response: 202 Accepted (pending approval)

GET /join/{name}
  Response: 200 + signed cert PEM (if approved)
            202 (still pending)
            403 (denied)
```

The `/join` endpoint is only accessible without client cert
verification (the joiner doesn't have a trusted cert yet).
All other endpoints require mTLS. This is the only endpoint
that relaxes the TLS requirement, and only for the CSR
submission step.

After receiving the signed cert, the joiner reconnects with
full mTLS using the new cert. All subsequent communication
uses the normal namespace API.

### What Phase 1 Does NOT Include

- mDNS discovery (Phase 2)
- Automatic approval / PIN-based pairing (Phase 2)
- Avalanche.io CA integration (Phase 3)
- NAT traversal (Phase 3)
- Security scopes / ACLs (Phase 4)
- Certificate revocation lists (Phase 3)
- Multi-CA trust (Phase 4 — cross-org collaboration)

These are explicitly deferred. Phase 1 establishes the
namespace conventions and the manual ceremony. Future phases
automate the creation of documents at these same paths.

---

## Phase 2: Proximity Trust — Summary

**Prerequisite:** Phase 1 namespace conventions.

**New capability:** mDNS discovery + approval gesture.

- c4d advertises `_c4d._tcp` via mDNS on startup
- Discovered peers appear at `/etc/discovered/{name}/`
  (separate from `/etc/peers/` — discovered but not yet trusted)
- Approval flow: discovered node sends CSR, approval uses
  a 4-digit PIN displayed on both sides (like Bluetooth pairing)
- On approval, `/etc/discovered/{name}/` moves to `/etc/peers/{name}/`
- Platform notifications for approval prompts (macOS, Linux, Windows)
- PIN verification prevents blind trust of LAN neighbors

**The Apple moment:** Power on a new machine running c4d. Your
existing node shows a notification: "beast2 wants to join.
Code: 7742." You see 7742 on the new machine's screen. Tap
approve. Done. The new node has a CA-signed cert, knows its
peers, and is part of the mesh.

---

## Phase 3: Brokered Trust — Summary

**Prerequisite:** Phase 1 namespace conventions, Phase 2 discovery.

**New capability:** Avalanche.io (or self-hosted) CA + directory.

- `c4 login josh@example.com` provisions a cert from the
  Avalanche.io CA via browser-based OAuth
- The CA cert goes in `/etc/ca/avalanche` automatically
- Nodes with the same owner identity auto-discover via the
  Avalanche.io directory service
- NAT traversal: Avalanche.io relay nodes act as always-on
  intermediaries (standard c4d instances in the cloud)
- `c4 join` works across the internet, not just LAN
- Certificate revocation via Avalanche.io (CRL distributed
  as namespace content at `/etc/crl/avalanche`)
- Self-hosted equivalent: run your own CA server with the
  same protocol. `c4 login --ca https://ca.studio.internal/`

**The key insight:** `c4 login` is just a convenient way to
populate `/etc/ca/`, `/etc/identity/`, and `/etc/peers/`. The
underlying namespace convention is identical to Phase 1.

---

## Phase 4: Scoped Trust — Summary

**Prerequisite:** All previous phases.

**New capability:** Per-subtree access control for multi-party
collaboration.

A scope is a namespace subtree with an access control list:

```
/etc/scopes/
  dailies-review/
    path            → "/projects/film/dailies"
    acl/
      josh@example.com    → "rw"
      sarah@gmail.com     → "r"
      vendor@studio.com   → "r"
    ca/
      studio-ca     → CA cert for the studio (cross-org trust)
```

When an external identity connects, c4d:
1. Validates their cert against CAs listed in the scope
2. Checks their identity against the scope ACL
3. Presents only the scoped subtree as their namespace root

From the external user's perspective, they see a namespace
that starts at the scoped path. They can't see or traverse
outside it. The scope IS a namespace — a restricted view of
the full tree.

**Shared namespaces:** Two organizations create matching scopes
pointing at a shared subtree. Flow channels sync the shared
subtree between them. Each side controls their own ACL. The
shared content is content-addressed — identical on both sides,
verified by C4 ID.

**Scope propagation:** Scopes are namespace content. They
propagate through flow channels like everything else. When
the studio adds a vendor to a scope, that change propagates
to all nodes in the studio's mesh automatically.

---

## Namespace Path Summary

```
/etc/
  identity/
    cert              Phase 1   Node certificate (PEM)
    name              Phase 1   Human-readable node name
    owner             Phase 1   Owner identity (email)
  ca/
    {name}            Phase 1   Trusted CA certificate (PEM)
  peers/
    {name}/
      address         Phase 1   Peer endpoint (host:port)
      ca              Phase 1   CA name that signed this peer
      fingerprint     Phase 1   Cert fingerprint (TOFU, optional)
      enabled         Phase 1   Soft enable/disable
  pending/
    {name}/
      csr             Phase 1   Pending join request CSR
      address         Phase 1   Requester address
      fingerprint     Phase 1   Requester bootstrap fingerprint
      timestamp       Phase 1   Request time
  discovered/
    {name}/
      address         Phase 2   mDNS-discovered peer
      fingerprint     Phase 2   Discovered cert fingerprint
  blocked/
    {fingerprint}     Phase 1   Cert fingerprint blocklist
  crl/
    {ca-name}         Phase 3   Certificate revocation list
  scopes/
    {scope-name}/
      path            Phase 4   Namespace subtree path
      acl/
        {identity}    Phase 4   Access level (r, rw, admin)
      ca/
        {ca-name}     Phase 4   Trusted CAs for this scope
```

## Design Decisions

### Why `/etc/` and not a config file?

Config files are local. Namespace content propagates. When you
add a peer on your laptop, that peer entry can flow to your NAS
through an existing flow channel. The NAS now knows about the
peer too — without you logging into the NAS and editing its
config.

This is the self-hosting property: flow channels propagate
namespace changes. Trust configuration is namespace content.
Therefore trust configuration propagates through flow channels.
The mesh configures itself.

### Why CA-based, not just TOFU?

TOFU (trust on first use, like SSH) works for a single person
managing a few machines. But it doesn't scale to:
- Adding the 10th machine (verify 9 fingerprints?)
- Organizations (who approves which fingerprints?)
- Cross-org collaboration (whose fingerprints?)

CA-based trust scales: sign a cert once, trusted everywhere the
CA is trusted. The CA is just a blob in `/etc/ca/`. Trust
propagation is content propagation.

Phase 1 supports both: TOFU via fingerprint pinning in
`/etc/peers/{name}/fingerprint`, and CA via `/etc/ca/`.

### Why not just mTLS from day one?

We already have mTLS. The question is how certificates get
created and distributed. Phase 1 makes that process explicit
and CLI-driven. Phase 2 makes it proximity-based. Phase 3
makes it cloud-brokered. The mTLS mechanism is the same in
all phases — only the certificate provisioning changes.

### Why are private keys outside the namespace?

Content in the namespace is content-addressed and potentially
shared. A private key in the namespace could propagate through
a flow channel to another node — catastrophic. Private keys
are local filesystem state, period.

### Why separate `/etc/discovered/` from `/etc/peers/`?

Discovery is observation ("I can see you on the network").
Trust is a decision ("I choose to connect to you"). Merging
them would mean mDNS discovery automatically creates trusted
peers — a security hole. The approval step is the explicit
bridge from discovered to trusted.

### Why not use the existing config.yaml?

config.yaml stays for truly local settings (listen address,
key file path). But peer configuration and trust belong in
the namespace because they're shared state. A peer added on
one node should be visible (and optionally auto-configured)
on other nodes in the mesh. config.yaml can't do that.

The migration is gradual: Phase 1 reads config.yaml as
fallback, populates the namespace on first run. After that,
namespace is authoritative.

## Relationship to Other Designs

- **mesh.md**: This document details the Identity section
  of mesh.md. The five concerns (Identity, Discovery,
  Description, Policy, Transport) are orthogonal; this
  document handles Identity with awareness of how it
  connects to Discovery (Phase 2) and Policy (Phase 4).

- **mesh-implementation.md**: That document's Phase 7
  (Identity and Login) is actually a prerequisite for all
  other phases. This design replaces Phase 7's brief sketch
  with the detailed phased approach.

- **flow-c4d-design.md**: Flow channels sync namespace
  subtrees. `/etc/` is a namespace subtree. Flow channels
  can sync trust configuration. This is intentional and
  powerful but must be opt-in — you don't want a compromised
  peer injecting entries into your `/etc/ca/`.

- **c4d-lan-bootstrap.md** and **c4d-tls-enforcement.md**:
  These parked plans are subsumed by this design. Phase 1
  handles TLS enforcement (everything is mTLS). Phase 2
  handles LAN bootstrap (mDNS + approval).
