# The c4m Format

**Filesystem Organization Without Storage**

Joshua Kolden
joshua@avalanche.io

Avalanche.io, Inc. / Cinema Content Creation Cloud (C4) Project

*(March 2026)*

## Abstract

The C4 ID whitepaper[^1] described a system for identifying any digital asset universally and permanently, without requiring a registry or any communication between parties. This paper introduces c4m (pronounced "cam"), a plain-text file format that extends C4 IDs from identifying individual files to describing entire filesystems.

A c4m file captures every filename, directory, permission, timestamp, file size, and content identity in a filesystem -- without containing any of the content itself. A 2 KB c4m file can completely describe an 8 TB project. Every operation that normally requires the actual files -- diff, merge, verify, version control -- can operate on the c4m description instead. The files are needed only at the moment of use.

We call the information captured in a c4m file *indelible metadata*. Unlike filenames, paths, and other conventional metadata, the bond between a c4m description and the content it describes is permanent. It does not rely on naming conventions, storage locations, or any external registry. It is derived from the content itself, and this bond cannot be broken by any operation that preserves the bytes.

c4m is plain text, one line per file, and requires no special tools to read. It works with grep, diff, email, and version control out of the box. Despite this simplicity, the format supports cryptographic diff and merge operations, incremental patch chains for version history, and Merkle tree verification of arbitrary directory hierarchies. It is implemented in Go, Python, TypeScript, Swift, and C, with every implementation producing byte-identical output.

## Introduction

The C4 ID whitepaper solved the problem of identification. Given any file, anywhere in the world, two people who have never met will independently produce the same C4 ID without the need for communication, coordination, or a central registry. This *agreement without communication* is the foundation on which everything else is built.

But identification alone answers only one question: *what is this thing?*

In practice, the harder questions are organizational. What files does this project contain? What changed since last week? Does this backup match the original? Are these two copies identical? Every tool that answers these questions -- git, rsync, tar, Docker, digital asset management systems -- requires the actual files to function. To organize files, you must store them. To compare two versions of a project, you need both versions. To verify a backup, you need the backup and the original accessible in the same place at the same time.

This requirement is so deeply assumed that it is rarely stated. But it is a requirement, not a law of nature. The C4 ID system demonstrated that identification does not require a registry. c4m demonstrates that organization does not require storage.

A c4m file is a complete description of a filesystem. You can examine a project's structure, compare it to another version, verify that a backup is intact, or plan a reorganization -- all without having the files. The description is small (a few kilobytes for hundreds of files), human-readable (plain text), and permanently bound to the content it describes (through C4 IDs).

This paper describes the c4m format, the operations it enables, the design decisions behind it, and the practical problems it solves.

## Indelible Metadata

The term *metadata* ordinarily refers to information about data -- filenames, timestamps, permissions, folder structures. In conventional systems, metadata is fragile. Rename a file and its identity changes. Move it to another folder and its path changes. Copy it to another machine and its metadata may not survive the transfer. The connection between metadata and the data it describes depends on convention, proximity, and luck.

c4m inverts this relationship. In a c4m file, every piece of metadata is anchored to a C4 ID -- a content-derived identity that is immutable and universal. The filename `README.md`, the permission `-rw-r--r--`, the size `5,069 bytes`, and the C4 ID `c45xZeXwKjQ2...` all appear on the same line, and the C4 ID permanently identifies the specific content that those attributes describe. Rename the file, move it to another continent, archive it to tape -- the C4 ID still points to the same bytes, and the c4m entry still describes the same file.

We call this *indelible metadata*: a description that is permanently bound to the content it describes, not by convention or storage location, but by the content itself. The bond is mathematical. It cannot be broken by any operation that preserves the bytes.

This property has a profound practical consequence: a c4m file is not merely a record of a filesystem. It is functionally equivalent to the filesystem it describes, for every purpose except actually reading the bytes. You can diff two c4m files and see exactly what changed between two versions of a project. You can merge two independently modified c4m files and resolve conflicts. You can verify a backup against its c4m description and know with cryptographic certainty whether it is complete and intact. All of this without the files themselves.

The files are needed only at the moment of use -- when a human wants to open a document, or a program needs to process a frame, or a render farm needs to read a texture. Everything else -- the organizing, the comparing, the versioning, the verifying -- operates on the description.

## The Format

A c4m file is a UTF-8 text file containing one line per filesystem entry. Each line records five fields separated by spaces:

```
MODE TIMESTAMP SIZE NAME C4ID
```

A small project might produce a c4m file that looks like this:

```
-rw-r--r-- 2026-03-20T12:00:00Z  5,069 README.md              c45xZeXwKjQ2...
-rw-r--r-- 2026-03-20T12:01:00Z 41,023 model.bin               c43zYcLnRtWx...
drwxr-xr-x 2026-03-20T13:38:00Z  7,112 src/                    c47mNqPvWxYz...
  -rw-r--r-- 2026-03-20T13:38:00Z 5,069 main.go                c44aBcDeFgHi...
  -rw-r--r-- 2026-03-18T13:38:00Z 2,043 util.go                c45jKlMnOpQr...
  drwxr-xr-x 2026-03-19T19:13:00Z 1,280 internal/              c46StUvWxYzA...
    -rw-r--r-- 2026-03-19T19:13:00Z 1,280 parser.go            c48BcDeFgHiJ...
drwxr-xr-x 2026-03-20T10:00:00Z 12,800 tests/                  c49KlMnOpQrS...
  -rw-r--r-- 2026-03-20T10:00:00Z 12,800 main_test.go         c4AtUvWxYzAb...
```

That is a complete description of the project. Every file, every directory, every permission, every timestamp, every size, and every content identity -- captured in nine lines of plain text that anyone can read.

### Fields

**Mode** is the 10-character Unix permission string familiar from `ls -l`. The first character indicates the entry type: `-` for regular files, `d` for directories, `l` for symbolic links. The remaining nine characters encode read, write, and execute permissions for owner, group, and others.

**Timestamp** is the file's modification time in UTC, formatted as RFC 3339 with second precision. UTC is the only canonical representation; local times may be displayed for human readability but are stored as UTC internally.

**Size** is the file size in bytes. For directories, the size is the recursive sum of all contained file sizes -- a useful at-a-glance measure of how much content a directory holds.

**Name** is the bare filename -- no path components. Directory hierarchy is expressed entirely through indentation: two spaces per level. Directory names end with a trailing slash. This means every entry's name is a simple string, never a path that requires parsing or interpretation.

**C4 ID** is the content-derived identity. For files, it is the C4 hash of the file's raw bytes. For directories, it is the C4 hash of the directory's *canonical representation* -- a deterministic text rendering of the directory's immediate children, including their names, metadata, and C4 IDs. This creates a Merkle tree[^2]: changing one byte in one file changes the C4 ID of every ancestor directory up to the root. Two directory trees with identical contents always produce the same C4 ID, regardless of where or when they exist.

### Hierarchy

Directory structure is expressed through indentation rather than path names. Children of a directory appear at one additional indent level immediately following their parent. Within each level, files are listed before directories, and entries within each group are sorted using natural ordering -- so `file2.txt` sorts before `file10.txt`, as a human would expect.

### Progressive Resolution

Any field except the name may be null, represented by a single dash (`-`). This enables *progressive resolution*: a c4m file can describe a filesystem at varying levels of detail. A structure-only scan captures names and hierarchy. A metadata scan adds permissions, timestamps, and sizes. A full scan adds C4 IDs by reading and hashing every byte of every file.

Each level is useful on its own. A structure-only scan tells you what files exist. A metadata scan tells you what has likely changed (by comparing sizes and timestamps). A full scan gives you cryptographic certainty. You choose the level of certainty you need based on the questions you are asking and the IO cost you are willing to pay.

### Sequences

In media production, numbered file sequences are the norm. A single shot might consist of ten thousand individually numbered EXR frames. Listing each one would be unwieldy, so c4m provides sequence notation:

```
-rw-r--r-- 2026-03-20T12:00:00Z 41,943,040 frame.[0001-10000].exr c4SeqId...
```

One line describes ten thousand files. The sequence's C4 ID is computed from the ordered concatenation of the individual frame C4 IDs, so it captures the identity of every frame without listing them individually. The individual identities can be stored alongside the c4m file or inlined within it for expansion when needed.

The range notation supports gaps (`[0001-0050,0075-0100]`), steps (`[0001-0100:2]` for every other frame), and arbitrary discontinuous ranges -- the kinds of irregular sequences that arise naturally in production when frames are dropped, re-rendered, or delivered in batches. Ten thousand lines collapse to one without losing any information.

## Links

Filesystems provide two link primitives. Symbolic links give a file an alternate name that points to a target path. Hard links give two directory entries the same underlying storage, so that changes through one name are immediately visible through the other. Both are important filesystem features, and both are surprisingly difficult to capture faithfully in archival or description formats. c4m records both, and introduces a third kind of link for a scope that existing primitives do not reach.

### Symbolic Links

A symbolic link in a c4m file is recorded with the `->` operator and a symlink mode:

```
lrwxrwxrwx 2026-03-20T12:00:00Z 0 latest -> v23/comp_final.exr c4abc...
```

The target path is stored as-is. The C4 ID, when present, is that of the target file's content -- the same ID you would get by hashing the file the symlink points to. Broken symlinks and symlinks that chain through other symlinks record a null C4 ID.

This is straightforward, and it is worth noting precisely because many archival formats handle symlinks poorly or not at all. A tar archive preserves them. A zip file does not. rsync follows them by default, silently replacing the link with a copy of the target. A c4m file records the link as what it is: a named reference to a path.

### Hard Links and the Independence of Identical Files

Hard links present a subtler problem, one that is directly relevant to content-addressed systems.

Content addressing makes deduplication trivially discoverable. If two files in a project have the same C4 ID, they have identical content. It is tempting to conclude that they should be stored as a single copy -- and in a content store, they should be. But the reverse operation -- reconstructing a filesystem from a content store -- must not conflate *identity* with *interchangeability*.

Consider a VFX project where two shots use the same placeholder texture. The files are identical today -- same bytes, same C4 ID. But they belong to different shots and will diverge as work progresses. If a naive tool hard-links them together to save space, editing the texture for one shot silently changes it in the other. The files had the same content, but they were independent entities, and the independence mattered.

C4 IDs reveal that content is identical. c4m understands that identical content does not imply identical intent. A content-addressed store can safely collapse duplicates for storage, because the store is read-only with respect to content -- you retrieve bytes by ID, you do not edit them in place. But the filesystem that is reconstructed from a c4m description must preserve the original independence of files that merely happened to share content.

When c4m scans a filesystem, it records which files are actually hard-linked -- sharing a single inode, so that modifying one genuinely modifies the other -- and which are independent copies that happen to have the same C4 ID. Hard links are marked with the `->` operator and a group number:

```
-rw-r--r-- 2026-03-20T12:00:00Z 4096 shared_lut.cube  ->1 c4abc...
-rw-r--r-- 2026-03-20T12:00:00Z 4096 grade_ref.cube   ->1 c4abc...
-rw-r--r-- 2026-03-20T12:00:00Z 4096 master_lut.cube      c4abc...
```

The first two entries share hard link group 1 -- they are the same inode, and modifying one modifies the other. The third entry has the same C4 ID but no group annotation. It is an independent copy. When the filesystem is reconstructed, the hard-linked files will be hard-linked, and the independent copy will remain independent. The distinction is preserved exactly.

This is an aspect of filesystems that very few archival or description formats capture correctly. Most either ignore hard links entirely (zip, most asset management systems), collapse all duplicates indiscriminately (some deduplication tools), or record hard links but lose track of which groups they belong to. c4m preserves the filesystem's own decisions about what is linked and what is independent, because those decisions carry meaning that the format has no right to discard.

### Flow Links

Symbolic links and hard links address two scopes of the filesystem. Hard links operate within a single volume, with immediate consistency -- two names for the same bytes on the same disk. Symbolic links reach across volumes and network mounts, with synchronous consistency -- the link resolves now, or it fails now. But there is a third scope that neither primitive reaches: systems separated by organizational boundaries, air gaps, intermittent connectivity, and geographic distance. In this regime, partitions are not exceptional events to be handled as errors. They are the normal state of affairs.

The CAP theorem[^6] tells us that a system operating where partitions are normal must choose between consistency and availability. Hard links choose immediate consistency within a volume where partitions cannot occur. Symbolic links choose synchronous consistency across a reachable network where partitions are rare. For the third scope -- where partitions are the default -- a primitive must accept eventual consistency, or else fail continuously, which is no primitive at all.

c4m introduces the *flow link* to fill this structural gap. A flow link declares that a local path is related to a remote path, in a specified direction, with eventual consistency semantics:

```
drwxr-xr-x 2026-03-20T12:00:00Z 4096 outbox/ -> studio:inbox/  c4abc...
drwxr-xr-x 2026-03-20T12:00:00Z 4096 inbox/  <- nas:renders/   c4def...
drwxr-xr-x 2026-03-20T12:00:00Z 4096 shared/ <> peer:shared/   c4ghi...
```

The `->` operator declares an outbound flow: content in `outbox/` should propagate to `studio:inbox/`. The `<-` operator declares an inbound flow: content from `nas:renders/` should propagate to the local `inbox/`. The `<>` operator declares bidirectional synchronization. The target is a location reference -- a named destination that infrastructure daemons know how to reach.

Flow links are not aliasing primitives. Hard links and symbolic links are both forms of aliasing -- they give two names to the same resource. Flow links are *communication* primitives. They declare that two distinct resources, on two distinct systems, should converge over time through an asynchronous protocol. This distinction is not a design choice -- it is forced by the partition regime. Aliasing requires shared access to a common referent. When access is partitioned, aliasing is impossible. Communication -- message passing across a boundary -- is the only mechanism that works.

This shift from aliasing to communication explains properties that would be anomalous in a traditional link but are natural in a flow link. Flow links have direction, because communication channels have direction. They operate asynchronously, because the remote system may not be reachable at the moment the declaration is made. And they compose with filesystem permissions in useful ways: a directory with write permission and an outbound flow becomes an ingest point; a directory with read permission and an inbound flow becomes a distribution point. These semantics emerge from the combination of standard Unix permission bits with flow direction, without any special-case logic.

Like a symbolic link, a flow link is a declaration of relationship, not a mechanism of fulfillment. Just as NFS and the VFS layer make symlinks work across machines, flow links rely on infrastructure daemons to propagate content between locations. The flow link itself records the intent in the filesystem description, visible to any tool that reads the c4m file. This means cross-partition data relationships are no longer invisible. A directory listing reveals which paths are receiving data from remote sources, which are sending, and where. The relationships that are currently buried in rsync scripts, cron jobs, replication configurations, and institutional knowledge become first-class filesystem metadata.

The historical pattern is consistent. Hard links arrived in the early 1970s when computing happened on a single machine. Symbolic links arrived in 4.2BSD in 1983 when machines gained multiple volumes and network mounts. Each extended the filesystem's naming reach to match the scope of contemporary computing. The third scope -- partition-normal operation across organizational and geographic boundaries -- arrived decades ago. The corresponding primitive has not, until now. The formal argument that the CAP theorem requires exactly three link primitives with exactly three consistency models is developed in a companion paper.[^7]

## Properties

The C4 ID whitepaper defined ten properties of perfect identification. c4m inherits all of them for individual files and extends them to entire filesystems. But c4m also has properties of its own that go beyond identification.

**Human readable.** A c4m file is plain text. Anyone with a text editor can open it and understand what it describes. This is not a convenience feature -- it is a design constraint. Any format that requires specialized tooling to inspect creates a dependency on that tooling. Dependencies are liabilities. They break, they become unsupported, they require installation. Plain text requires nothing.

**Tool compatible.** Because c4m files are plain text with one entry per line and predictable field positions, they work with every text-processing tool that already exists. You can `grep` a c4m file for all `.exr` files. You can `sort` it by size. You can `diff` two c4m files with the standard `diff` command. You can pipe it through `awk` for ad-hoc analysis. You can attach it to an email, commit it to git, or print it on paper. None of this requires special software.

**Content-size independent.** A c4m file's size is proportional to the *number of files* it describes, not the total data volume. Three files totaling 8 TB produce a c4m file of roughly the same size as three files totaling 8 KB. This independence from content size is what makes it practical to pass around descriptions of very large projects -- because the description is always small.

**Self-verifying.** Every entry includes a content-derived C4 ID, and every directory C4 ID is a Merkle hash of its children. A c4m file is a cryptographic commitment to the exact state of the filesystem it describes. You can verify any individual file by hashing it and comparing. You can verify an entire subtree by checking a single directory C4 ID. Tampering with any entry -- changing a filename, modifying a permission, swapping one file's content -- changes the C4 ID of every ancestor directory up to the root.

**Composable.** c4m files are documents. A subtree can be extracted as a standalone c4m file. A standalone c4m file can be injected into a parent tree. Patch chains are c4m files concatenated with cryptographic separator lines. This composability follows directly from the text format and the Merkle tree structure -- there is no binary layout to break, no offsets to recalculate, no schema to violate.

**Indelible.** The link between a c4m file and the content it describes is derived from the content itself. It does not depend on filenames, storage locations, registries, databases, or any external system. Given a c4m file and a pool of content blobs identified by C4 ID, you can reconstruct the original filesystem -- the right files with the right names in the right directories with the right permissions -- regardless of how the content was stored, where it was moved, or how much time has passed.

**Progressively resolvable.** A c4m file can describe a filesystem at any level of detail, from names only to full cryptographic identity. Each level is useful on its own and can be refined independently. You can start working with a c4m file immediately -- browsing, searching, planning -- and fill in the expensive parts only when and where you need them.

**Implementation independent.** The format is implemented in five languages -- Go, Python, TypeScript, Swift, and C -- and every implementation produces byte-identical output from the same input. This is not a heroic engineering achievement. It is a direct consequence of the format's simplicity. There are no ambiguities to resolve differently, no optional features to support variably, no floating-point encodings to round differently. The canonical form is fully specified and deterministic. A c4m file produced by any implementation is valid input to every other.

## Operations

Because a c4m file is a complete description of a filesystem, operations that normally require the filesystem can instead operate on the description. This is the practical payoff of decoupling organization from storage.

### Diff

Given two c4m files -- or a c4m file and a live directory -- a diff produces a patch describing exactly what changed. The diff leverages the Merkle tree structure: any subtree whose root C4 ID is unchanged is skipped entirely. In a million-file project where one file changed, the diff examines only the entries along the path from the root to that file.

```
$ c4 diff monday.c4m friday.c4m > week.c4m.patch
```

The patch itself is a c4m file. You can open it in a text editor and read exactly what changed.

### Patch

A patch can be applied to a c4m file to produce a new c4m file representing the target state, or applied to a live directory to reconcile its contents with the target:

```
$ c4 patch monday.c4m week.c4m.patch > friday.c4m
```

When reconciling a directory, the process identifies which files are missing (by C4 ID), which are unchanged, and which need to be updated. Files that need to be fetched are identified by C4 ID and can be retrieved from any source that has them -- a content store, a peer, a cloud bucket. The operation is idempotent: running it again produces no changes.

### Merge

When two parties have independently modified the same base, a three-way merge combines their changes:

```
$ c4 merge base.c4m alice.c4m bob.c4m > merged.c4m
```

Changes to different files are applied independently. Additions from both sides are combined. When both sides modified the same file differently, the conflict is resolved by timestamp -- the newer version takes the original name, the older is preserved under a `.conflict` suffix -- producing a merged result that can be reviewed and adjusted rather than a process that halts and demands immediate resolution.

### Version History

c4m files support *patch chains*: a base state followed by a sequence of incremental patches, all in a single file. Each patch section is separated by a bare C4 ID line -- a 90-character cryptographic checkpoint that must match the hash of the accumulated state above it.

```
-rw-r--r-- 2026-03-18T10:00:00Z 5069 README.md  c4abc...
-rw-r--r-- 2026-03-18T10:00:00Z 2048 main.go    c4def...
c4xyz...
-rw-r--r-- 2026-03-19T14:30:00Z 5188 README.md  c4ghi...
```

The first section is the base state. The bare C4 ID `c4xyz...` commits that state cryptographically. The entries that follow are a patch containing only what changed. Applying the patch to the base reconstructs the second version. Additional patches can be appended indefinitely, each verified by its own checkpoint.

This gives you version history in a single text file. The `c4 log` command lists the versions. The `c4 split` command extracts a specific one. The chain is append-only -- previous states are never modified -- and the cryptographic checkpoints make the entire history tamper-evident.

This is not a replacement for git. Git excels at versioning source code: text files measured in kilobytes, with merge semantics tuned for line-by-line changes. But git cannot version an 8 TB video project, because it needs a copy of the files. A c4m patch chain can version that project in a file small enough to email.

## Cost

### Plain Text

The most natural objection to a text-based format is overhead. A binary format could encode the same information more compactly. This is true, and the overhead is approximately 2%.

The C4 ID dominates every c4m entry at 90 characters. In raw binary, the same identity would be 64 bytes. The remaining fields -- mode, timestamp, size, name -- add a few dozen bytes each. For a typical entry of roughly 160 bytes, the binary equivalent would be around 120. The overhead is real, and it is small.

In exchange for this 2%, you get a format that works with every text tool in existence. You can grep it, diff it, sort it, email it, commit it to git, read it on a machine that has never heard of C4. The 2% buys complete independence from specialized tooling. We consider this a bargain.

There is a deeper reason the overhead is so small. The C4 ID is dominated by entropy. SHA-512 produces 512 bits of output that are, by design, indistinguishable from random. Random data does not compress. Base58 encoding adds approximately 37% relative to raw binary, but neither representation compresses further. The format is within a small constant factor of the theoretical minimum, and that constant buys a great deal of practical utility.

### SHA-512

The C4 ID whitepaper committed to SHA-512 with no algorithm agility -- no version flags, no negotiation, no upgrade path. c4m inherits this commitment.

Algorithm agility solves a real problem: what happens when a hash function is broken? But it creates a worse one: every ID now requires context to interpret. Is this a SHA-256 ID or a SHA-512 ID? Which version of the scheme produced it? Can these two IDs be compared directly?

C4 chose permanence over flexibility. A C4 ID found in a database, email, filename, or c4m file is always SHA-512, always base58, always 90 characters starting with `c4`. It needs no version prefix, no algorithm tag, no context. A c4m file written today will be readable and verifiable decades from now with no knowledge of what software produced it.

SHA-512 has been in continuous use since 2001 and has been analyzed more thoroughly than any hash function in history. Even under Grover's algorithm, its collision resistance degrades from 2^256 to 2^128 -- still stronger than any classical attack on any hash function in widespread use today.[^3] If SHA-512 is ever meaningfully broken, the entire industry will be rebuilding. This is not a problem that C4 can solve alone, and it is not a problem worth burdening every C4 ID with a version flag to preemptively address.

## Applications

### Media Production

c4m was designed for the workflows of media production, where projects routinely span millions of files across many terabytes. A visual effects facility might manage thousands of EXR frames, texture maps, geometry caches, and renders distributed across departments and facilities. The standard practice is to coordinate these with hard drives, large shared filesystems, and considerable manual effort.

With c4m, a supervisor can describe the entire project in a file small enough to email, send it to a remote facility, and the facility can see exactly what files they need -- and which ones they already have -- before any content is transferred. Only the missing bytes move across the wire, identified unambiguously by their C4 IDs. The c4m file is the shipping manifest. The content follows on demand.

Sequence notation makes this practical at production scale. Ten thousand individually numbered EXR frames become a single line. The individual frame identities are preserved but do not clutter the description.

### Backup and Archive Verification

A c4m file produced before a backup is a cryptographic commitment to the original state. After the backup, producing a c4m of the backup and diffing the two provides mathematical certainty about what matches and what does not. No sampling, no spot-checking, no hoping. Every file is either identical to the original or it is not, and the diff says which.

For long-term archives, c4m's properties are particularly valuable. The description is as valid decades later as the day it was created. There is no format version to negotiate, no software to maintain, no schema to update. Open it in a text editor. The archive is described, completely, in plain text.

### Distributed and Disconnected Workflows

The most powerful consequence of decoupling organization from storage is that organization can happen anywhere, even without network access. A c4m file on a laptop can describe a project on a server the laptop has never connected to. You can browse its structure, compare it to another version, plan a reorganization, or merge it with someone else's changes -- all offline, all on a file measured in kilobytes.

When connectivity is available, only the structural changes need to be communicated. The content follows by C4 ID from whatever source has it. The c4m file does not care where the content lives. It cares only what the content is.

### Content-Addressed Storage

When content is stored by C4 ID rather than by name, deduplication is automatic and free. If the same file appears in ten projects, it is stored once. All ten c4m files reference the same C4 ID, and the storage system needs only one copy. This is the same principle git uses for object storage, extended to files of any size and type.

## Implementation

The format specification is documented in the c4m package[^4] and is deliberately minimal. An entry is one line with five space-separated fields. Hierarchy is indentation. The canonical form -- the representation used for computing C4 IDs -- is fully specified: no indentation, single spaces, UTC timestamps, raw integer sizes, natural sort order. Any parser that produces this canonical form will compute the same C4 IDs as every other parser.

The reference implementation is in Go:

```
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

Additional implementations are available in Python, TypeScript (including browser environments), Swift, and C. All produce byte-identical output, a property that has been verified through cross-language test suites.

The C4 ID itself is an SMPTE standard (ST 2114:2017).[^5] The c4m format builds on this foundation using only plain text and deterministic rules. There is no binary to parse, no header to validate, no schema to negotiate. The format is simple enough that an independent implementation from the specification alone -- without reference to any existing code -- is straightforward. This has been demonstrated in practice.

## Conclusion

The C4 ID system answered the question: *how do we identify digital assets universally and permanently?* The c4m format answers the follow-up: *what can we build on that foundation?*

The answer turns out to be everything that currently requires the files themselves. Compare, merge, verify, version, organize, deduplicate -- all of these operations can be performed on a small text file instead of on the terabytes it describes. The content is needed only at the moment of use, and it can be retrieved from any source that has it, identified unambiguously by its C4 ID.

This is not a theoretical capability. The format is specified, implemented in five languages, and in use. It is deliberately simple -- one line per file, plain text, no dependencies -- because simplicity is the only reliable foundation for a format intended to outlast the tools that created it.

Think of it like the black-and-white film of digital archive. Black-and-white film has lasted more than a hundred years as an archival format because of stable chemistry and the simplicity of holding a strip up to a light to know what it contains. C4 IDs are the stable chemistry. Opening a c4m file in a text editor is holding it up to the light.

---

[^1]: J. Kolden, "The C4 Identification System: Universally Consistent Identification Without Communication," ETC@USC, 2017. Available at https://cccc.io

[^2]: A Merkle tree is a tree in which every non-leaf node is labeled with the hash of its children. This allows efficient verification of large data structures -- a single root hash commits to the identity of every element in the tree. See https://en.wikipedia.org/wiki/Merkle_tree

[^3]: Grover's algorithm provides a quadratic speedup for unstructured search on quantum computers. Applied to SHA-512, pre-image resistance degrades from 2^512 to 2^256 and collision resistance from 2^256 to 2^128 -- both well beyond any projected computational capability. See NIST Post-Quantum Cryptography assessments for current status.

[^4]: The c4m format specification is maintained at https://github.com/Avalanche-io/c4 in the c4m package. The specification includes the canonical form, entry format, patch chain semantics, sequence notation, and validation requirements.

[^5]: SMPTE ST 2114:2017 "Content Identification" defines the C4 ID algorithm: SHA-512 hashing with base58 encoding. Available at https://ieeexplore.ieee.org/document/7971777

[^6]: E. Brewer, "Towards robust distributed systems," Keynote at ACM Symposium on Principles of Distributed Computing (PODC), 2000. Formally proved by S. Gilbert and N. Lynch, "Brewer's conjecture and the feasibility of consistent, available, partition-tolerant web services," ACM SIGACT News, 2002. The CAP theorem states that a distributed system can provide at most two of three guarantees: consistency, availability, and partition tolerance.

[^7]: J. Kolden, "The Missing Link: Flow as a Filesystem Primitive for Partition-Tolerant Environments," Avalanche.io, Inc., 2026. The paper proves, using sheaf-theoretic analysis, that the three consistency models (immediate, synchronous, eventual) arise as the unique sheaf conditions at each partition scope, and that no alternative model exists between them.
