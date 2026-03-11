#set document(
  title: "C4M — The C4 Manifest Format",
  author: "Avalanche",
)

#set page(
  paper: "us-letter",
  margin: (x: 1.2in, y: 1.2in),
  numbering: "1",
  number-align: center,
)

#set text(
  font: "Charter",
  size: 11pt,
  lang: "en",
)

#show heading.where(level: 1): it => {
  set text(size: 24pt, weight: "bold")
  v(0.3em)
  it
  v(0.3em)
}

#show heading.where(level: 2): it => {
  set text(size: 14pt, weight: "bold")
  v(0.6em)
  it
  v(0.2em)
}

#show heading.where(level: 3): it => {
  set text(size: 12pt, weight: "bold")
  v(0.4em)
  it
  v(0.1em)
}

#show raw.where(block: true): it => {
  set text(font: "JetBrains Mono NL", size: 9pt)
  block(
    fill: luma(245),
    inset: 12pt,
    radius: 4pt,
    width: 100%,
    it
  )
}

#show raw.where(block: false): it => {
  set text(font: "JetBrains Mono NL", size: 9.5pt)
  it
}

// Title page
#v(2.5in)

#align(center)[
  #text(size: 36pt, weight: "bold")[C4M]

  #v(0.3em)

  #text(size: 16pt, fill: luma(80))[The C4 Manifest Format]

  #v(0.6em)

  #text(size: 12pt, fill: luma(120))[A Complete Filesystem in a Text File]

  #v(1.5in)

  #text(size: 10pt, fill: luma(100))[
    Version 1.0 --- March 2026 \
    SMPTE ST 2114:2017 Content Identification \
    #link("https://github.com/Avalanche-io/c4")[github.com/Avalanche-io/c4]
  ]
]

#pagebreak()

// Body

= A Filesystem You Can Read

A c4m file is a complete description of a filesystem --- every file, directory, symlink, permission, timestamp, size, and content identity --- in a plain text document you can read, email, diff, and pipe through Unix tools.

```
-rw-r--r-- 2025-01-15T10:30:00Z  1,234 readme.txt    c41j3C6Jqga95PL...
-rw-r--r-- 2025-01-15T10:30:00Z 12,800 main.go       c43pCP9e69EGD25...
drwxr-xr-x 2025-01-15T10:30:00Z 48,200 assets/       c4RgFeXFYL1FjFu...
  -rw-r--r-- 2025-01-15T10:30:00Z 48,200 logo.png    c42RgFeXFYL1FjF...
```

Every file has a C4 ID --- a cryptographic content identifier defined by SMPTE ST 2114:2017. Every directory has one too, computed from its contents. If two directories anywhere in the world have the same C4 ID, they contain exactly the same data.

This is not compression. It is _description_. A c4m file is small enough to email, yet it fully describes a filesystem of any size. The actual file content lives elsewhere, addressed by ID. The c4m tells you what exists, where it is, and whether it has changed.

= What c4m Captures

A c4m file represents the complete POSIX-like filesystem tree:

- *Regular files* with permissions, timestamps, sizes, and content C4 IDs
- *Directories* with recursive structure and computed content identity
- *Symbolic links* with targets and target content identity
- *Hard links* with group tracking
- *Named pipes, sockets, block and character devices* (type and permissions)
- *Arbitrary filenames* including non-printable bytes, control characters, and invalid UTF-8
- *Media file sequences* like `frame.[0001-0100].exr` in compact notation

This covers the structural and content information of any filesystem you are likely to encounter. Every entry type that exists on a POSIX filesystem has a representation in c4m.

Environment-specific metadata like ownership (UID/GID) and extended attributes (xattrs, ACLs) is not part of the format. This is deliberate: ownership is machine-local (UID 501 means different things on different systems), and no existing tool reliably transfers all extended metadata across environments anyway. The c4m provides stable identity --- paths and C4 IDs --- that external systems can use as join keys for any additional metadata they need to track.

= The Format

Each entry occupies one line:

```
<mode> <timestamp> <size> <name> <c4id>
```

- *Mode*: Standard Unix permissions (`-rw-r--r--`, `drwxr-xr-x`, `lrwxrwxrwx`)
- *Timestamp*: RFC 3339 UTC (`2025-01-15T10:30:00Z`)
- *Size*: File size in bytes
- *Name*: Bare filename; directories end with `/`
- *C4 ID*: 90-character SMPTE ST 2114 identifier

Any field can be `-` (null) when unknown. This enables progressive resolution: a c4m file can be created with just filenames, then enriched with metadata as it becomes available.

Nesting is expressed through indentation:

```
drwxr-xr-x 2025-01-15T10:30:00Z 50000 project/
  -rw-r--r-- 2025-01-15T10:30:00Z  1234 readme.txt     c41j3...
  drwxr-xr-x 2025-01-15T10:30:00Z 48766 src/
    -rw-r--r-- 2025-01-15T10:30:00Z 12800 main.go      c43pC...
    -rw-r--r-- 2025-01-15T10:30:00Z 35966 render.go    c4Xkm...
```

The format is designed to look like `ls -l` output. Anyone who has used a Unix terminal can read a c4m file without documentation.

=== Links

Between the name and C4 ID, an optional operator declares relationships. Symlinks and hard links work as expected:

```
lrwxrwxrwx 2025-01-01T00:00:00Z    0 link.txt -> target.txt    c4...
-rw-r--r-- 2025-01-01T00:00:00Z 1024 copy.txt ->2              c4...
```

Flow links are described in their own section below.

=== Media File Sequences

Numbered file sequences common in media workflows get compact representation:

```
-rw-r--r-- 2025-01-15T10:30:00Z 2,400,000 frame.[0001-0100].exr c4...
-rw-r--r-- 2025-01-15T10:30:00Z 1,200,000 frame.[0001-0100:2].exr c4...
```

A hundred-frame EXR sequence is one line, not a hundred. The C4 ID covers the entire sequence. Stepped ranges, discontinuous ranges, and individual frame lists are all supported.

=== Filename Encoding

Real-world filesystems contain filenames with spaces, control characters, non-printable bytes, and invalid UTF-8. The Universal Filename Encoding handles all of them:

- Printable UTF-8 passes through unchanged
- Control characters use standard backslash escapes (`\t`, `\n`, `\r`, `\\`)
- Non-printable bytes are encoded as braille codepoints between `¤` delimiters

Every possible filename round-trips faithfully. No information is lost.

= What c4m Makes Possible

These are capabilities that emerge from having a human-readable, content-addressed filesystem description.

== Filesystem as Document

A c4m file _is_ the filesystem. Not a compressed archive --- a readable description. You can email a c4m file to a colleague, and they can see exactly what files exist, how large they are, when they were modified, and verify every byte of content by ID. Two people looking at the same c4m file are looking at the same filesystem, provably.

== Cryptographic Diff

Comparing two directory trees reduces to comparing two c4m files. Changed files have different C4 IDs. Unchanged subtrees can be skipped entirely --- if a directory's C4 ID matches, everything inside it is identical, no matter how deep the tree goes.

For a million-file project where one texture changed, the comparison touches only the entries along the path to that file. Everything else is verified by a single ID comparison at the directory level.

== Content-Addressed Deduplication

Identical files anywhere in the tree --- or across trees, across facilities, across continents --- share a single C4 ID. Storage and transfer only need one copy of the content. The c4m tracks where each ID appears; the content itself is location-independent.

This is particularly powerful in media pipelines where the same plate, texture, or LUT appears in dozens of shots. The C4 ID means you store it once and reference it everywhere.

== Incremental Patches

The c4m patch format describes changes between filesystem states as inline entries. A bare C4 ID line acts as a checkpoint, cryptographically verifying the state at that point. Subsequent entries describe additions, modifications, and removals.

```
-rw-r--r-- 2025-03-06T12:00:00Z 100 readme.txt c4abc...
c449ByTh8Hkx...
-rw-r--r-- 2025-03-06T12:00:00Z 200 changelog.txt c4def...
```

This stream says: here is a filesystem with `readme.txt`, here is its verified identity, now add `changelog.txt`. The receiver can verify every intermediate state. For real-time collaboration on large filesystems, only the delta needs to move.

== Progressive Resolution

Not every workflow needs full metadata immediately. A c4m file can start with just names:

```
- - - readme.txt -
- - - src/ -
```

Then gain permissions and sizes:

```
-rw-r--r-- 2025-01-15T10:30:00Z 1234 readme.txt -
drwxr-xr-x 2025-01-15T10:30:00Z 48766 src/ -
```

Then gain content identity:

```
-rw-r--r-- 2025-01-15T10:30:00Z 1234 readme.txt c41j3...
drwxr-xr-x 2025-01-15T10:30:00Z 48766 src/ c43pC...
```

Each stage is a valid c4m file. A fast scan can produce the skeleton; background hashing fills in the C4 IDs. Users see structure immediately while verification catches up.

== Human Editability

It is text. You can write a c4m file by hand, edit one in any text editor, generate one from a shell script, or process one with `grep`, `awk`, `sort`, `diff`, or any other Unix tool. There is no binary format to decode, no special tooling required to inspect the contents.

This is the escape valve. When automation fails or a tool does not exist yet for your workflow, you can always fall back to editing the description directly. The format is simple enough that a human can write it correctly from memory.

== Composability

Because c4m is one entry per line in plain text, the entire Unix tool ecosystem works on it natively:

```bash
# Find all EXR files over 10MB
c4 project/ | awk '$3+0 > 10485760' | grep '\.exr'

# Extract all unique C4 IDs
c4 project/ | awk '{ print $NF }' | sort -u

# Compare two snapshots
diff <(c4 dir1/) <(c4 dir2/)
```

No API, no SDK, no library dependency. If your system can read text, it can read c4m.

= Flow Links: The Third Filesystem Primitive

Filesystems have two kinds of links. *Hard links* bind two names to the same content within a single volume --- immediate consistency, enforced by the kernel. *Symbolic links* bind a name to a path that may cross mount points and network filesystems --- synchronous consistency, as long as the target is reachable.

Both stop at the edge of the visible network. There is no way for a filesystem to say "this directory is related to that directory on a system I cannot currently reach."

But modern data doesn't stop at the network edge. Content moves between data centers, between facilities, between continents, between devices that are sometimes offline. The infrastructure for moving it exists --- rsync, object storage, CDNs, courier drives --- but none of it is expressed in the filesystem. It is bolted on: scripts, cron jobs, sync agents, pipeline configurations that live outside the data they describe.

=== The Third Link

A *flow link* binds a local path to a remote path that may not be reachable at the moment of declaration, or ever simultaneously. It declares that content at one location should propagate to another, mediated by infrastructure that handles the actual movement.

```
drwxr-xr-x 2025-01-01T00:00:00Z 4096 outbox/  -> studio:inbox/ c4...
drwxr-xr-x 2025-01-01T00:00:00Z 4096 inbox/   <- nas:renders/  c4...
drwxr-xr-x 2025-01-01T00:00:00Z 4096 shared/  <> peer:shared/  c4...
```

Three operators express three directions:

#table(
  columns: (auto, auto, 1fr),
  inset: 8pt,
  stroke: 0.5pt + luma(200),
  table.header(
    [*Operator*], [*Direction*], [*Meaning*],
  ),
  [`->`], [Outbound], [Content here propagates there],
  [`<-`], [Inbound], [Content there propagates here],
  [`<>`], [Bidirectional], [Two-way sync, converging toward common state],
)

The target is a location reference: a named location followed by a path (`studio:inbox/`, `nas:renders/`). Location names are human-readable identifiers for machines, facilities, or storage systems.

In practical terms, a flow link takes a directory buried deep in a filesystem hierarchy and surfaces it to a meaningful destination. Without flow links, the relationship between `/mnt/projects/show_xyz/sequences/sq010/shots/sh0020/comp/renders/v003/` and wherever those renders need to go next lives in someone's head, or in a script on a server, or nowhere at all. With a flow link, that directory simply says `-> review:sh0020/renders/` --- the intent is right there in the filesystem description, readable by humans and machines alike.

=== Eventual Consistency Is Not a Compromise

Hard links give immediate consistency because they share an inode. Symbolic links give synchronous consistency because the kernel can reach the target. Flow links give *eventual* consistency because the infrastructure propagates changes when it can.

This is not weaker --- it is the only consistency model that works when the link spans an ocean, an organizational boundary, or a device that is sometimes offline. Each link type provides the strongest possible guarantee for its scope.

=== What Flow Links Enable

*Declarative data distribution.* Instead of writing scripts that copy files between machines, you declare the relationship in the filesystem itself. The c4m file says "this directory's content should appear at that location." The infrastructure reads the declaration and makes it happen.

*Visible topology.* Flow links are entries in a c4m file, so the data flow topology is visible, diffable, and versionable. You can see where content goes by reading the c4m. You can change where it goes by editing the c4m. The routing is in the data, not in a configuration file on a server somewhere.

*Content-addressed transfer.* Because every file has a C4 ID, flow propagation only moves content the destination doesn't already have. If the same plate exists at both ends, it is recognized by ID and skipped. Deduplication is automatic across the entire flow network.

*Separation of intent and mechanism.* The flow link declares _what_ should propagate _where_. It says nothing about _how_ --- that is the job of the infrastructure layer (the c4d daemon, or any system that reads flow declarations). The mechanism can be rsync, S3 replication, a courier drive, or a satellite uplink. The declaration doesn't change.

Every organization that manages data across multiple locations reinvents flow --- as rsync scripts, as replication rules, as pipeline configurations. Flow links make it a first-class concept in the filesystem description itself, where it can be seen, tracked, and reasoned about alongside the data it governs.

= For Media Workflows

C4M was designed with media production in mind. The problems it solves --- tracking thousands of versioned assets across facilities, verifying delivery integrity, detecting what changed between iterations --- are daily realities in VFX, animation, and post-production.

*Delivery verification.* A facility sends a c4m file alongside a delivery. The recipient generates their own c4m and compares. If the C4 IDs match, every frame arrived intact. If they differ, the diff shows exactly which files are wrong.

*Asset tracking.* A c4m file of a show's asset library is a complete inventory with cryptographic content identity. When the same texture appears in multiple shots, it has the same C4 ID everywhere. Deduplication is automatic and provable.

*Sequence handling.* A thousand-frame EXR sequence is one line in a c4m file, with a single C4 ID covering the entire sequence. Missing frames, corrupted frames, and version mismatches are immediately visible.

*Multi-site sync.* Flow links make cross-facility data relationships a first-class part of the filesystem description. A c4m file doesn't just describe what exists --- it describes where content should go. The infrastructure reads the flow declarations and converges toward the desired state automatically.

*Pipeline integration.* c4m is plain text. Any pipeline tool that can read a file can read a c4m. No SDK integration required. Write a c4m from Python, parse one in a shell script, diff two from a web interface.

= Technical Foundation

*C4 IDs* are defined by SMPTE ST 2114:2017. They are SHA-512 hashes encoded in base58, producing a 90-character string starting with `c4`. The probability of a collision is approximately 1 in 2#super[256] --- effectively zero.

*Directory identity* forms a Merkle tree. Each directory's C4 ID is computed from the canonical representation of its immediate children. A change to any file anywhere in the tree propagates up to the root. This means the root C4 ID is a single value that verifies the entire filesystem.

*Canonical form* ensures deterministic identity. The same filesystem always produces the same c4m, regardless of which tool generates it or on which platform. Entries are sorted (files before directories, then natural sort), timestamps are UTC, sizes are bare integers, and there is no optional formatting.

*The c4m format is an open standard.* The specification, reference implementation, and all tooling are open source under the MIT license. There is no vendor lock-in, no proprietary component, and no fee.

#v(1.5em)

#align(center)[
  #text(size: 10pt, fill: luma(120))[
    C4 Project --- #link("https://github.com/Avalanche-io/c4")[github.com/Avalanche-io/c4] \
    Open source under the MIT License
  ]
]
