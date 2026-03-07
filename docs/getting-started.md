# Getting Started with C4

This guide walks through the core workflow: scanning files, creating c4m files, comparing directories, and transferring content.

## Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
go install github.com/Avalanche-io/c4d@latest
```

## 1. Identify Files

Every file gets a c4m entry with its metadata and C4 ID:

```bash
$ c4 photo.jpg
-rw-r--r-- 2026-03-04T14:22:10Z 4404019 photo.jpg c43zYcLni5LF...

$ echo "hello world" | c4
c44SjyfSsNez6bqFCJeFCSurmMiQ3DFCXkG67PiB9DJobUqG2YhvMeCvig6fjuh67SmrUUYMcaHmJjNMeZCqbNkWcTP
```

Piped data has no filesystem metadata, so it outputs a bare C4 ID. A directory produces a full recursive c4m listing. Use `-i` for just the ID:

```bash
$ c4 -i my-project/
c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq
```

## 2. Create a c4m File

A c4m file is a lightweight description of a directory — every file's permissions, timestamps, size, name, and C4 ID in a human-readable, awk-friendly text format:

```bash
# Scan a directory (full recursive c4m output)
$ c4 my-project/

# Save to a .c4m file
$ c4 my-project/ > my-project.c4m
```

The c4m file is a small text file (typically a few KB) that fully describes a directory that could contain terabytes of data. Every line is a self-contained record you can process with `grep`, `awk`, `sort`, and `diff`.

## 3. Compare Directories

See what changed between two snapshots:

```bash
$ c4 diff old.c4m: new.c4m:
```

## 4. Transfer Content with c4d

Start a c4d node to store and serve content:

```bash
# First-time setup
$ c4d init

# Start the daemon
$ c4d serve
```

Use the `c4` CLI to copy content to and from c4m files backed by c4d:

```bash
# Create a c4m file and push content to c4d
$ c4 mk project.c4m:
$ c4 cp ./src/ project.c4m:src/

# Materialize content from a c4m file
$ c4 cp project.c4m:src/ ./restored/
```

## Next Steps

- [C4M User Guide](../c4m/README.md) — c4m format details
- [C4M Specification](../c4m/SPECIFICATION.md) — Formal spec
- [CLI Reference](../README.md) — All commands and flags
