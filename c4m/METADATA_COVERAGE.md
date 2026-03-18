# C4M Metadata Coverage

This document explains what filesystem metadata c4m captures, what it omits, and how omitted metadata can be associated with c4m entries using external systems.

## What c4m captures

A c4m file represents the complete POSIX-like filesystem tree:

- **Regular files** with permissions, timestamps, sizes, and content C4 IDs
- **Directories** with recursive structure and computed content identity
- **Symbolic links** with targets and target content identity
- **Hard links** with group tracking
- **Named pipes, sockets, block and character devices** (type and permissions)
- **Arbitrary filenames** including non-printable bytes, control characters, and invalid UTF-8 (via [Universal Filename Encoding](../design/filename-encoding.md))
- **Media file sequences** like `frame.[0001-0100].exr` in compact notation

This covers the structural and content information of any filesystem you are likely to encounter.

## What c4m does not capture

c4m does not include:

- **Ownership** (UID/GID)
- **Extended attributes** (xattrs, ACLs, SELinux contexts, macOS resource forks)
- **Multiple timestamps** (only one; not separate mtime/atime/ctime/btime)
- **Device major/minor numbers**
- **File flags** (immutable, append-only, etc.)
- **Sparse file hole maps**

## Perspective: what existing tools capture

Before concluding that the absence of this metadata is a limitation unique to c4m, consider what actually happens today without c4m. Most tools people rely on for filesystem transfer and comparison lose the same details:

| Tool       | Ownership | xattrs | Content verification   | Arbitrary filenames |
| ---------- | --------- | ------ | ---------------------- | ------------------- |
| `rsync`    | yes       | flag   | checksum (optional)    | yes                 |
| `tar`      | yes       | flag   | no                     | yes                 |
| `zip`      | partial   | no     | CRC-32                 | limited             |
| `git`      | no        | no     | SHA (tree only)        | limited             |
| `scp/sftp` | no        | no     | no                     | yes                 |
| **c4m**    | no        | no     | SHA-512 (every object) | yes (all bytes)     |

No single tool captures everything. The difference is that c4m gives you something none of them do: **cryptographic content identity for every file and every directory**, in a format that is human-readable, diffable, and composable.

The metadata c4m omits is the same metadata that most transfer and comparison workflows already lose. c4m is not uniquely limited here — it is simply not attempting to solve that problem along with the problems it does solve.

## Associating additional metadata

Every c4m entry has two stable identifiers:

1. **Path** — the entry's location within the tree
2. **C4 ID** — the entry's content identity

Either can serve as a join key for external metadata. This means any system that can store key-value pairs can extend c4m with arbitrary metadata without modifying the format itself.

### Path-keyed metadata

Metadata that depends on *where* a file lives — ownership, ACLs, SELinux contexts — is naturally keyed by path:

```
# Ownership sidecar
readme.txt    uid=1000 gid=1000
src/main.go   uid=1000 gid=1000

# Extended attributes sidecar
src/main.go   security.selinux=unconfined_u:object_r:user_home_t:s0
```

### Content-keyed metadata

Metadata that depends on *what* a file contains — MIME type, encoding, provenance — is naturally keyed by C4 ID. This has the advantage of applying everywhere that content appears, across all trees:

```
# Content metadata (keyed by C4 ID)
c41j3C6Jqga...   mime=text/plain charset=utf-8
c43pCP9e69E...   mime=application/x-go-source
```

### Why this separation works

c4m provides the identity layer. External systems provide the metadata. The two compose cleanly because:

- **Paths are unique within a c4m** — every entry has exactly one path
- **C4 IDs are globally unique** — the same content always has the same ID
- **c4m is stable** — the format doesn't change when metadata requirements change
- **Metadata systems are independent** — different environments can track different metadata without affecting the c4m

This is the same principle that makes filesystems work: the filesystem provides naming and structure, and extended attributes, ACLs, and ownership are layered on top by independent subsystems. c4m applies the same separation at the description layer.
