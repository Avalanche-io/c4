# Getting Started with C4

This guide walks through the core workflow: identifying files, creating c4m snapshots, comparing directories, and versioning with patches.

## Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

## 1. Identify Files

Every file gets a c4m entry with its metadata and C4 ID:

```bash
$ c4 id photo.jpg
-rw-r--r-- 2026-03-04T14:22:10Z 4404019 photo.jpg c43zYcLni5LF...

$ echo "hello world" | c4
c44SjyfSsNez6bqFCJeFCSurmMiQ3DFCXkG67PiB9DJobUqG2YhvMeCvig6fjuh67SmrUUYMcaHmJjNMeZCqbNkWcTP
```

Piped data has no filesystem metadata, so it outputs a bare C4 ID. A directory produces a full recursive c4m listing:

```bash
$ c4 id myproject/
-rw-r--r-- 2026-03-04T14:22:10Z 1234 README.md c4abc...
-rw-r--r-- 2026-03-04T14:22:10Z 5678 main.go c4def...
```

## 2. Create a c4m File

A c4m file is a lightweight description of a directory — every file's permissions, timestamps, size, name, and C4 ID in a human-readable text format:

```bash
# Save to a .c4m file
c4 id myproject/ > project.c4m
```

The c4m file is a small text file (typically a few KB) that fully describes a directory that could contain terabytes of data.

## 3. Store Content

Store file content in a content-addressed store for later retrieval:

```bash
# Store content while identifying
c4 id -s myproject/ > project.c4m

# Retrieve content by C4 ID
c4 cat c43zYcLni5LF... > restored-file.jpg
```

On first use of `-s`, the CLI offers to create a default store at `~/.c4/store`. You can also configure it explicitly:

```bash
C4_STORE=/path/to/store                                        # local
C4_STORE=s3://bucket/prefix?region=us-west-2                   # S3
C4_STORE=/fast/ssd,s3://bucket/c4?region=us-west-2             # multiple
```

## 4. Compare Directories

See what changed between two snapshots. Arguments can be c4m files,
directories, or any combination:

```bash
c4 diff old.c4m new.c4m
c4 diff project.c4m ./project/
c4 diff ./old-project/ ./new-project/
```

## 5. Version with Patches

Append diffs to a c4m file to build a version history:

```bash
# Initial snapshot
c4 id -s ./project/ > project.c4m

# Later: append a patch
c4 diff project.c4m <(c4 id -s ./project/) >> project.c4m

# View history
c4 log project.c4m

# Resolve to current state
c4 patch project.c4m > current.c4m

# Branch: split at any point
c4 split project.c4m 3 common.c4m rest.c4m
```

## 6. Reconcile a Directory

Apply a target state to a live directory:

```bash
# Make ./project/ match target.c4m, store pre-patch state
c4 patch -s target.c4m ./project/ > changeset.c4m

# Revert later using the stored pre-patch state
c4 patch -r changeset.c4m ./project/

# Preview without making changes
c4 patch --dry-run target.c4m ./project/
```

## Next Steps

- [CLI Reference](./cli-reference.md) — All commands and flags
- [C4M User Guide](../c4m/README.md) — c4m format details
- [C4M Specification](../c4m/SPECIFICATION.md) — Formal spec
