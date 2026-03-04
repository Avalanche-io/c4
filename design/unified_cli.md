# Unified CLI Architecture: c4 and c4d

## The Problem

Two problems are one problem:

1. **The CLI split is wrong.** `c4d` has user-facing operations (`push`,
   `checkout`) that don't belong in a daemon binary. Nobody types `sshd connect`
   or `dockerd run`. The daemon should be invisible.

2. **There's no naming layer.** Users think in places: "my laptop", "the cloud
   bucket", "the studio NAS." But c4d speaks in hostnames and ports. Setting up
   two machines and a policy to move data between them requires plumbing the user
   shouldn't see.

The fix is one design: **c4** is the command you type, **c4d** is the process
that runs, and **locations** are how you name the places where c4d runs.

## The Model

| Client | Daemon | What the daemon does |
|--------|--------|---------------------|
| `ssh`  | `sshd` | Accepts connections, authenticates, runs shells |
| `kubectl` | `kubelet` | Manages containers on a node |
| `docker` | `dockerd` | Manages images and containers |

In every case: the user only types the client command. The daemon runs, quietly,
in the background. You configure it once, maybe never touch it again.

**c4** is the command you type. **c4d** is the process that runs.

## Two-Layer Architecture

The location system has two layers, like file sharing on a computer: you have
many internal resources but expose specific share points.

**Layer 1 — Internal Mesh (Private).** All of a user's c4d instances see each
other and route freely. Alice's laptop, workstation, NAS, cloud, render farm —
they're all part of her personal mesh. Each has a private name only she sees.
Internal routing is automatic.

**Layer 2 — Published Interfaces (Public).** Users explicitly choose what to
expose to the outside world. Alice might have 5 internal nodes but only publish
2 interfaces: "Alice's Cloud" and "Alice's Studio." Outsiders see only the
published names. The internal topology stays private.

The privacy boundary: your internal mesh is yours. You publish what you choose.

## Layer 1: The Internal Mesh

### Locations

A location is a human-friendly name for a c4d instance. `laptop`. `cloud-vm`.
`home-nas`. `render-farm`. Not hostnames. Not IPs. Not ports. Names.

Every c4d instance has a name. Every name maps to exactly one content store.
Users think in places; locations are how the system speaks the same language.

### Three Layers of Identity

c4d currently has one identity concept: the TLS certificate email
(`alice@example.com`). The location model adds two more:

| Layer | Example | Purpose |
|-------|---------|---------|
| **Who** | `alice@example.com` | The person (TLS cert, exists today) |
| **Where** | `laptop`, `cloud-vm` | The place (new) |
| **What group** | `studio`, `archive` | A collection of places (new) |

These compose naturally:

```
c4 push . --to cloud-vm              # store at a specific place
c4 push . --to archive               # replicate to every place in a group
c4 push . --to alice@example.com     # relay delivery to a person
```

The `@` disambiguates. No `@` = location or group. Has `@` = person.

### Where the Name Lives

The name is set at init time and lives in three places:

1. **c4d config** (`~/.c4d/config.yaml`):
   ```yaml
   name: laptop
   identity: alice@example.com
   listen: ":7433"
   store: ~/.c4d/store
   ```

2. **TLS certificate** — The node cert includes a URI SAN:
   `c4://alice@example.com/laptop`. When two c4d instances connect via mTLS,
   they prove both WHO and WHERE they are. The name is cryptographically
   authenticated. No spoofing.

   The URI SAN is the **advertised name** — the name the machine presents to
   peers. Users can also define **local aliases** in their client config (the
   SSH model). Alice's workstation advertises "workstation" in its cert, but
   Bob might alias it as "alice-edit-bay" in his config. The certificate
   fingerprint resolves ambiguity.

3. **c4 client config** (`~/.c4/config.yaml`):
   ```yaml
   identity: alice@example.com
   local: laptop

   locations:
     laptop:
       address: localhost:7434
     cloud-vm:
       address: cloud.example.com:7433
     home-nas:
       address: 192.168.1.50:7433

   groups:
     studio: [laptop, cloud-vm]
     archive: [cloud-vm, home-nas]
   ```

### URI SAN Format

`c4://alice@example.com/laptop` — Go's `url.Parse` splits this into:

- `Scheme`: `c4`
- `User`: `alice`
- `Host`: `example.com`
- `Path`: `/laptop`

Reconstruction:
```go
identity := fmt.Sprintf("%s@%s", u.User.Username(), u.Host)  // "alice@example.com"
name := strings.TrimPrefix(u.Path, "/")                       // "laptop"
```

Tested and validated. Standard RFC 3986 behavior. The identity owner IS the
URI authority, the location IS the path. These store cleanly in x509 URI SANs
(`cert.URIs`).

### Groups

Groups are a client-side concept — a named set of locations. c4d doesn't know
about groups. The c4 CLI resolves a group name to its member locations and acts
on each.

Groups enable policy without complicating the daemon.

## The Setup Experience

### First Node

```bash
c4d init alice@example.com --name laptop
c4d start
```

After this: `~/.c4d/` exists with CA, cert, key, config, and content store.
`~/.c4/config.yaml` exists with the local location entry. The node is running
and ready for local use.

Two commands. Done.

### Second Node (Same Owner)

On the new machine:

```bash
# Copy CA from existing node (the trust anchor)
scp laptop:~/.c4d/ca.key ~/.c4d/ca.key
scp laptop:~/.c4d/ca.crt ~/.c4d/ca.crt

c4d init alice@example.com --name cloud-vm --store s3://my-bucket/c4
c4d start
```

On the laptop:

```bash
c4 location add cloud-vm cloud.example.com:7433
```

The CA copy is the only ceremony. Both nodes share a CA, so they trust each
other's certs automatically. `c4 location add` registers the name-to-address
mapping in the client config.

### Adding an S3 Location

An S3 bucket joins the mesh as a c4d instance backed by S3 storage. The user
sees a name and doesn't think about S3 — they think about a place:

```bash
c4d init alice@example.com --name cloud-storage --store s3://my-c4-bucket/
c4d start
```

That's it. `cloud-storage` is now a named place. The c4d instance is a thin
protocol node fronting S3. It provides TLS authentication, the push/checkout
protocol, namespace management, and content-addressing — all for free. The
user never touches the S3 API directly.

### Future: Pairing (the "USB feel")

For the wow-me experience, pairing replaces CA copying:

On the new machine:
```bash
c4d init alice@example.com --name cloud-vm
c4d start
c4 pair
# Displays: "Pairing code: BLUE-FISH-DAWN-7432"
```

On the laptop:
```bash
c4 pair BLUE-FISH-DAWN-7432
```

Symmetric — either machine can generate or consume a code. `c4 pair` talks to
the local c4d, which handles the certificate exchange. The user never types
`c4d` commands. Under the hood: a TOFU-style CA exchange verified by a short
authentication string (like Bluetooth pairing or Signal safety numbers).
For MVP, CA copy is sufficient.

### Future: mDNS Discovery (zero-config local)

On the same network, c4d instances discover each other automatically via mDNS
(service type `_c4d._tcp`). Like AirDrop or Bonjour printers.

```
$ c4d init --name workstation
  Initialized "workstation"
  Discovered on local network:
    laptop   192.168.1.5   (trust? Y/n) y
  Exchanged certificates with "laptop".
```

Trust is always explicit — mDNS discovers, the user decides. Once trusted,
instances appear in locations automatically. IP changes re-resolve via mDNS.

## Layer 2: Published Interfaces

Layer 1 handles "managing your own machines." Layer 2 handles "collaborating
with others." They're additive — Layer 2 builds on Layer 1 without changing it.

### The Core Insight

Publishing = registration, not routing. A published interface isn't a new kind
of endpoint. It's a registration — telling the relay "I exist, and outsiders
should see me as this name." The relay bridges trust between meshes.

### How It Works

Alice has 5 internal nodes: laptop, workstation, NAS, render-farm, cloud-vm.
She publishes 2:

```bash
c4 publish cloud-vm --as cloud --description "Alice's Cloud Storage"
c4 publish workstation --as studio --description "Alice's Studio"
```

The relay now has a directory:
- `alice@example.com` → [cloud: "Alice's Cloud Storage", studio: "Alice's Studio"]

Bob discovers Alice's published interfaces:
```
$ c4 discover alice@example.com
  cloud    - Alice's Cloud Storage
  studio   - Alice's Studio
```

Bob sees 2 names. He doesn't know about Alice's laptop, NAS, or render farm.
The privacy boundary is enforced by design — the relay only knows what Alice
registered, and Bob can only query the relay.

Bob sends content:
```bash
c4 push . --to alice@example.com:cloud
```

Content flows: Bob's c4d → relay → Alice's cloud-vm. The relay routes by
published name to the authenticated connection from Alice's cloud-vm.

### Published vs. Internal Names

The internal name (`cloud-vm`) and published name (`cloud`) can differ. Alice
thinks "cloud-vm" because that's what it is. Bob sees "cloud" because that's
what it means to him. The published name carries intent — "cloud" means
persistence/archive, "studio" means active work.

### What Changes in c4d

**Config gets a `publish` section:**
```yaml
name: cloud-vm              # internal name (in cert URI SAN)
identity: alice@example.com
publish:                     # public-facing
  name: cloud
  description: "Alice's Cloud Storage"
relay: relay.avalanche.io:7433
```

**Relay registration flow.** When c4d starts with a `publish` section and a
relay configured, it registers with the relay:

```
POST /.c4d/register
{"published_name": "cloud", "description": "Alice's Cloud Storage"}
```

The relay verifies the caller's cert says `alice@example.com` (existing mTLS
auth), then records the registration. Authenticated — you can only publish
under your own identity.

**Relay discovery endpoint:**

```
GET /.c4d/endpoints/alice@example.com
→ {"endpoints": [
    {"name": "cloud", "description": "Alice's Cloud Storage"},
    {"name": "studio", "description": "Alice's Studio"}
  ]}
```

Public — anyone can discover what Alice has published. Names and descriptions
only, no addresses. The relay is the intermediary.

**Endpoint-targeted delivery.** Currently the relay delivers to identities:
`To: bob@example.com` → writes to `/home/bob@example.com/inbox/`. With
published interfaces: `To: bob@example.com:studio` → routes to Bob's studio
endpoint specifically.

**No TLS changes for publishing.** The cert already proves identity (email SAN)
and internal name (URI SAN). Publishing is a relay-side registration — an
application-layer concept. No new certs, no SAN changes, no re-issuing.

### Cross-Mesh Trust

Within a mesh: same CA, mutual TLS, direct connections.

Between meshes: each side talks to the relay using their own CA. The relay
trusts both CAs. The relay is the trust anchor for cross-mesh communication.

```
Bob's mesh                    Relay                        Alice's mesh
[laptop]                   [relay.avalanche.io]            [cloud-vm]
[workstation]                                              [laptop]
                                                           [NAS]
  Bob's CA ──── trusts ────  Relay CA  ──── trusts ──── Alice's CA
```

For direct peer-to-peer between meshes (future): cross-CA trust or a shared CA.
But the relay pattern handles MVP cleanly.

### The `c4 publish` Command

`c4 publish` talks to the local c4d, which handles both config update and relay
registration. The daemon owns its config — the client just says "publish this."

```bash
c4 publish cloud-vm --as cloud --description "Cloud Storage"
# c4 → local c4d → updates config + registers with relay
```

This keeps the seam clean: c4 is the user interface, c4d handles the protocol.

## The c4 CLI

After the unification, c4 has three kinds of operations:

### Local (no daemon needed)

These work today and don't change:

```
c4 file.txt                       # C4 ID of a file
c4 .                              # C4 ID of a directory
c4 -m .                           # Show capsule
c4 -mr .                          # Full recursive capsule
c4 diff old.c4m new.c4m           # Compare capsules
c4 union a.c4m b.c4m              # Combine capsules
c4 subtract need.c4m have.c4m     # What's missing?
c4 fmt manifest.c4m               # Format capsule
c4 validate manifest.c4m          # Validate capsule
```

### Network (talks to local c4d)

Moved from c4d, now with location names:

```
c4 push . --to cloud-vm            # store content at a named location
c4 push . --to archive             # replicate to a group
c4 push . --to alice@example.com   # relay delivery to a person
c4 push . --to alice@example.com:cloud  # relay delivery to a published endpoint
c4 checkout <id> ./output          # materialize content locally
c4 checkout <id> ./output --from home-nas  # fetch from a specific location
```

### Establishment and location management

```
c4 mk <name>: <address>            # establish a location for writing
c4 mk <file>.c4m:                  # establish a capsule for writing
c4 rm <name>:                      # remove a location
c4 locations                       # list all locations with status
c4 groups                          # list groups
c4 group create <name> <loc>...    # create a group
c4 group delete <name>             # delete a group
```

### Content location

```
c4 where <id>                       # show which locations have content
```

### Publishing and discovery

```
c4 publish <location> --as <name> [--description <desc>]  # publish an endpoint
c4 unpublish <name>                 # remove a published endpoint
c4 discover <identity>              # list someone's published endpoints
```

### How c4 finds c4d

c4 connects to the local c4d over HTTP (localhost:7434 by default). Discovery:

1. Environment variable `C4D_ADDR` (if set)
2. Config file `~/.c4/config.yaml` with `local` location (if exists)
3. Default `localhost:7434`

If c4d isn't running, c4 says so and exits. No fallback, no auto-start.

## The c4d CLI

c4d is a daemon with a minimal operator interface:

```
c4d                               # run daemon (foreground)
c4d init <identity> --name <name> # initialize store + config
c4d init <identity> --name <name> --store s3://bucket/  # S3-backed
c4d status                        # show daemon info
c4d pki ca                        # generate CA
c4d pki cert --name alice         # generate client cert
```

That's it. Configuration lives in `~/.c4d/config.yaml`. The config file is the
interface for operators; the CLI is the interface for users.

Running `c4d` starts the daemon. No `serve` subcommand. Like `sshd`.

### Info endpoint

c4d exposes a minimal info endpoint for `c4 locations` to query:

```
GET /.c4d/info → {"name":"laptop","identity":"alice@example.com","store":{"objects":42000,"size":"42.3 GB"}}
```

Read-only. Used by the client for status display.

## The Mesh Story

"Every computer runs c4d." Your laptop, your workstation, your render farm
nodes, your NAS — they all run c4d. Each is a node in your personal mesh.
Content flows between them based on capsules and intent.

Within your mesh, you think in places:

- `c4 push . --to cloud-vm` — store it there
- `c4 push . --to archive` — replicate to the archive group
- `c4 checkout <id> ./output` — get it here (from wherever it lives)

Across meshes, you think in people and published endpoints:

- `c4 push . --to alice@example.com` — deliver to Alice
- `c4 push . --to alice@example.com:cloud` — deliver to Alice's cloud endpoint
- `c4 discover alice@example.com` — see what Alice has published

This is intention-based, not plumbing-based. "Store this in the cloud" not
"upload 47,000 files to s3://bucket-name/prefix via the daemon at
relay.example.com:7433."

## Colon Syntax: Unified Path Addressing

The colon is the portal. Everything after it is just a path.

```
c4 ls renders/                      # local directory
c4 ls project.c4m:renders/          # described directory (capsule)
c4 ls studio:project/renders/       # remote directory (location)
```

Three different kinds of filesystem. One syntax. One mental model.

### PathSpec Grammar

```
PathSpec     ::= LocalPath | CapsulePath | LocationPath

LocalPath    ::= path                          (no colon, or starts with ./ or /)
CapsulePath  ::= capsule_file ':' subpath?
LocationPath ::= location_name ':' subpath?

capsule_file    ::= path ending in '.c4m'
location_name   ::= registered name (no '/', no '.c4m' suffix)
subpath         ::= path within target (may contain sequence notation)
```

### Resolution Rules

Parse the first colon. What's to its left determines the type:

1. **No colon** → Local path. `c4 ls renders/`
2. **Starts with `./` or `/`** → Local path. `c4 ls ./file:with:colons`
3. **Left side contains `/`** → Local path (colon is inside a path component).
4. **Left side ends with `.c4m`** → Capsule path. Syntactic — no lookup needed.
5. **Left side is a registered location** → Location path. Semantic — checks registry.
6. **Otherwise** → Error: `"foo" is not a capsule (.c4m) or known location.`

Capsule detection is syntactic (works offline, no daemon). Location detection is
semantic (requires registry lookup, but the lookup is fast and definitive since
location names are explicitly registered).

### The Go Type

```go
type PathSpec struct {
    Type     PathType
    Source   string  // capsule path, location name, or local path
    SubPath  string  // path within capsule/location (empty = root)
}

type PathType int
const (
    PathLocal    PathType = iota
    PathCapsule
    PathLocation
)
```

`ParsePathSpec` lives in the c4 CLI layer (`cmd/c4/`), not in c4m. The c4m
package deals with manifest structure; it doesn't know about locations or CLI
argument conventions.

### `c4 cp` — The Universal Verb

`cp` works between any combination of source types:

| Source → Dest | Semantics | Speed |
|------|-----------|-------|
| Local → Local | File copy with C4 identity | Normal (bytes move) |
| Local → Capsule | Capture — build/extend c4m, optionally store in c4d | Fast (hash + write) |
| Local → Location | Push intent | Instant (description now, bytes later) |
| Capsule → Local | Materialize — extract described content | Slow (bytes move) |
| Capsule → Capsule | Manifest editing | Instant (description only) |
| Capsule → Location | Push described state | Instant |
| Location → Local | Pull content | Slow (bytes move) |
| Location → Location | Orchestrate transfer | Instant (locations negotiate) |

"Instant" means the user gets their prompt back immediately. Content flow
happens asynchronously per policy.

### How It Maps to c4d Protocol

c4d never sees colons. The c4 CLI translates colon paths into standard HTTP:

**Single file:** `c4 cp file.txt studio:project/`
1. `PUT /` with file data → C4 ID (store content)
2. `PUT /project/file.txt` with C4 ID body (write namespace)

**Directory (capsule expansion):** `c4 cp dailies/ studio:project/dailies/`
1. c4 walks `dailies/`, creates capsule — fast, just hashing + metadata
2. `PUT /` → capsule ID (store capsule)
3. `PUT /` for each file (concurrent, streamed)
4. `PUT /project/dailies/` with `Content-Type: application/c4m` → capsule expansion
5. c4d reads capsule from store, expands entries into namespace under target path

**Cross-location:** `c4 cp studio:project/ archive:backup/`
1. `GET /project/` on studio's c4d → manifest
2. `PUT /` on archive's c4d → push content
3. `PUT /backup/` on archive's c4d → capsule expansion

The user's machine orchestrates; actual bytes flow between locations directly.

### One Protocol Addition: Capsule Expansion

When `PUT /path` receives `Content-Type: application/c4m`, c4d expands the
capsule's entries into the namespace under the target path. ~20 lines of
server code. No new endpoints.

### Instant Completion Model

1. Capsule creation is fast — walking files + hashing produces kilobytes
2. Capsule push is instant — kilobytes over the network
3. Namespace expansion is instant — writing path→ID entries
4. User sees "done" — the directory structure exists at the destination
5. Content streams in background — actual file data transfers asynchronously

This IS "Email a Petabyte" applied to `cp`.

### Sequence Ranges

No ambiguity with sequence notation (`plate.[0001-0200].exr`):
- We split on the FIRST colon only
- Sequence brackets appear in the subpath (right side)
- Step notation colons (`[0001-0200:2]`) are inside brackets, protected

### Nested Colon Syntax

`studio:myfiles.c4m:renders/` — a capsule inside a location. Two-pass parse:

**First colon** resolves the location:
1. c4 resolves `studio` → c4d address
2. Fetches `myfiles.c4m` from studio's namespace

**Second colon** navigates into the capsule:
3. Parse capsule locally
4. Navigate to `renders/` within the virtual filesystem

c4d serves the capsule as a blob. "Looking inside" happens in the c4 CLI.
Clean separation between transport (c4d) and interpretation (c4).

The key invariant: each `.c4m` segment in the path can introduce another colon
boundary. The parser scans left-to-right, resolving each layer before proceeding
to the next.

### The Colon as "Look Inside"

The trailing colon is the universal "look inside" operator. Its absence means
"treat as literal file."

```
myfiles.c4m          → the literal .c4m file (an object)
myfiles.c4m:         → the virtual filesystem it describes (a view into the object)
myfiles.c4m:renders/ → a path inside that virtual filesystem
```

Applied to commands:
```
c4 ls project.c4m           # shows the capsule file itself (size, C4 ID)
c4 ls project.c4m:          # lists the capsule's described contents
c4 ls project.c4m:renders/  # lists a subtree inside the capsule
```

Applied to cp — materialization without a separate command:
```
c4 cp myfiles.c4m studio:           # copies the literal .c4m file to studio
c4 cp myfiles.c4m: studio:          # materializes the c4m's contents at studio
c4 cp myfiles.c4m:renders/ studio:incoming/  # copies just the renders subtree
```

Recursive across locations:
```
c4 ls studio:myfiles.c4m:           # navigates into a remote c4m's virtual contents
```

The colon means "open the portal." Without it, the capsule is opaque.

### Establishment: `mk` and `rm`

Write access through colon syntax requires prior establishment. This is a
safety gate, not ceremony.

**The problem:** `c4 cp a.c4m b.c4m` vs `c4 cp a.c4m b.c4m:` — one character.
Without a safety gate, a trailing colon typo silently changes "copy literal
file" to "write into namespace." If b.c4m is important, this corrupts it.

**The rule:** Read is implicit. Write requires `mk`.

```
c4 ls project.c4m:              # works — read-only, safe
c4 cp project.c4m:renders/ ./   # works — reading from capsule
c4 cp files/ project.c4m:       # ERROR unless c4 mk project.c4m: was run first
c4 cp files/ studio:            # ERROR unless c4 mk studio: addr:port was run first
```

**Why `mk`:** Unix lineage — `mkdir` makes directories, `mkfifo` makes FIFOs,
`mk` makes colon endpoints. Short, imperative, no ambiguity.

**For capsules:**
```
c4 mk project.c4m:              # establish capsule for writing
c4 cp dailies/ project.c4m:     # now this works
```

Establishment is local-only (creates a marker alongside the c4m file or in a
registry). No daemon needed. Establishment persists — you `mk` once.

**For locations:**
```
c4 mk studio: cloud.example.com:7433    # establish location for writing
c4 cp dailies/ studio:                  # now this works
```

**Teardown is asymmetric:**
```
c4 rm studio:                    # removes the location registration
rm project.c4m                   # OS rm removes the file (and its establishment)
```

Locations use `c4 rm` because the registration is in c4's registry.
Capsules use OS `rm` because the file is just a file.

**Same verb, both types.** The colon suffix on `mk` makes both types look the
same: `c4 mk thing:`. What follows the name distinguishes them — `.c4m` suffix
means capsule, otherwise location (with address argument).

### Bidirectional Capture: Writing Into Capsules

The colon syntax is fully symmetric — read and write:

```
c4 cp myfiles.c4m: dest/           # Read: content flows OUT of capsule
c4 cp /path/to/files myfiles.c4m:  # Write: content flows IN, capsule gets built
```

Write requires prior establishment: `c4 mk myfiles.c4m:` — this is the safety
gate that prevents accidental writes from colon typos. Read does not.

This eliminates `c4 scan` as a separate concept. Capturing files IS just `cp`
into a `c4m:` path.

**Capture examples:**
```
c4 mk today.c4m:                          # establish for writing (once)
c4 cp dailies/ today.c4m:                 # capture dailies, build capsule
c4 cp dailies/ today.c4m:renders/         # capture into a subtree
c4 cp shot_010/ project.c4m:shots/010/    # add to an existing capsule (after mk)
```

**Create-on-write.** If the target c4m file doesn't exist, `cp` creates it
(after establishment). Like how `cp` creates destination files.

**Merge semantics.** When writing into an existing capsule:
- New paths → added
- Existing paths with different content → updated
- Existing paths with same content → no-op (idempotent)

The c4m file on disk is a **working document** — a mutable workspace for
building capsules. The C4 ID of any given snapshot is immutable (change the
file, change the ID), but the file itself evolves. This is the git model:
working tree is mutable, commits are immutable. Want a clean start? Delete the
file and rebuild.

**Two modes of operation:**
- **Local-only (no c4d):** Walks source, hashes files, writes c4m entries.
  Content stays on disk where it is. The capsule accurately describes the
  filesystem. This is what `c4 -m .` already does — cp is the universal entry.
- **With c4d running:** Builds the c4m AND stores content in c4d. Content is
  now tracked, transferable, and subject to policies.

The capsule is useful either way — it's a description. But to move content to
other locations, you need c4d.

**Protocol flow for `c4 cp source/ file.c4m:`:**
1. Walk source directory, hash each file → C4 IDs
2. Build c4m entries with relative paths
3. Write (or merge into) c4m file on disk
4. If c4d running: `PUT /` for each file → store content

**Protocol flow for `c4 cp source/ file.c4m:subdir/`:**
1. Walk source, hash files → C4 IDs
2. Read existing file.c4m (if exists)
3. Add new entries under `subdir/` prefix
4. Write updated c4m to disk
5. If c4d running: store new content

### Capsule Identity vs. Capsule Mutability

A capsule's **C4 ID** is immutable — it's a hash of the content. Change the
file, get a new ID. But the c4m **file** on disk is mutable: you keep adding to
it with successive `cp` operations. When you push a capsule to a location, you
push a specific snapshot — THAT snapshot's identity is permanent.

Capsule transforms (`c4 union`, `c4 subtract`) remain the tools for combining
or diffing capsules structurally. `cp` handles the capture (building from real
files) and materialization (extracting to real files) directions.

### Colon Syntax Replaces Flags

The `--to` and `--from` flags from push/checkout become unnecessary:

| Flag syntax | Colon syntax |
|------|------|
| `c4 push . --to cloud-vm` | `c4 cp . cloud-vm:` (after `c4 mk cloud-vm: addr`) |
| `c4 push . --to archive` | `c4 cp . archive:` (after `c4 mk archive: addr`) |
| `c4 checkout <id> ./out --from nas` | `c4 cp nas:<path> ./out` (read — no `mk` needed) |
| `c4 push . --to alice@example.com:cloud` | `c4 cp . alice@example.com:cloud:` (after `mk`) |

`push` and `checkout` may survive as semantic shortcuts (push = always instant,
checkout = blocks until content materializes), but `cp` subsumes both.

### Location Registry as Filesystem

```
~/.c4d/locations/
├── laptop           # file: address + TLS config
├── cloud-vm         # file: address + TLS config
├── nas              # file: address + TLS config
├── studio/          # directory = group
│   ├── laptop → ../laptop   # symlink
│   └── cloud-vm → ../cloud-vm
└── archive/         # directory = group
    ├── cloud-vm → ../cloud-vm
    └── nas → ../nas
```

Immediately `ls`-able, `tree`-able. No config parsing.

## The @intent Connection

Capsules have `@intent` directives that describe what should happen with the
content. The c4 client interprets these. A capsule emailed to someone can carry
its own delivery instructions:

```
@intent deliver --to cloud-vm
```

**Open question:** How much @intent interpretation belongs in c4 vs. being
purely application-layer? The ssh analogy suggests the client should understand
the protocol, but @intent is extensible and might grow beyond what a core CLI
should handle. This needs separate design work.

## Implementation Plan

### Phase 1: Internal mesh (Layer 1)

1. Add `name` field to c4d config; add `--name` flag to `c4d init`
2. Include location URI SAN (`c4://identity/name`) in generated TLS certs
3. Add `LocationFromPeer()` to extract name from cert URI SAN on connect
4. Make bare `c4d` run the daemon (no `serve` subcommand)
5. Add `c4d info` endpoint (`GET /.c4d/info`)
6. Add `~/.c4/config.yaml` with locations and groups
7. Add `c4 mk`/`c4 rm` establishment commands and `c4 locations`
8. Move `push` and `checkout` from c4d to c4
9. Resolve location names via registry (name → address)
10. Tests: push/checkout via c4 against running c4d, with location names

Steps 1–5 are daemon-only (no c4 client changes). Steps 6–9 are client-only.
Step 10 validates the integration.

### Phase 2: Simplify c4d + published interfaces (Layer 2)

1. Remove `push` and `checkout` subcommands from c4d
2. Remove orphaned `enroll` command
3. Add `c4d status` for operator diagnostics
4. Add `publish` config section to c4d
5. Add relay registration endpoint (`POST /.c4d/register`)
6. Add relay discovery endpoint (`GET /.c4d/endpoints/<identity>`)
7. Add endpoint-targeted inbox routing (`To: identity:endpoint`)
8. Add `c4 publish`, `c4 unpublish`, `c4 discover` commands
9. Tests: cross-mesh delivery via relay with published endpoints

### Phase 3: Package boundary

The dependency direction: `c4` depends on nothing from `c4d`. The client HTTP
code lives in `c4` (or a shared package in `c4`). The server code lives in
`c4d`. This keeps c4d as pure infrastructure and c4 as the user tool.

### Future

- **Pairing:** `c4 pair` / `c4 pair CODE` for zero-config trust
- **mDNS:** Automatic local network discovery (`c4 locations --discover`)
- **Policies:** `c4 policy add "replicate deliverables to archive"`
- **Sync agent:** Background `c4 sync` that executes policies
- **Service install:** `c4d install` as launchd/systemd/Windows service
- **Multi-node published interfaces:** One published name fronting multiple nodes

## What This Doesn't Address

- Multi-user shared locations (Bob and Alice both accessing the same c4d) —
  future, builds on CRL mechanism
- Auto-start of c4d when c4 needs it — explicitly rejected
- Tiered storage within one location (local cache + S3 backing) — internal
  c4d concern, not visible in the location model
- Multi-store locations — one name = one store, keeps the model simple
- Direct S3 access from c4 client — rejected; S3 is always a c4d-backed
  location, keeping protocol and auth consistent
