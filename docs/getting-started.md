# Getting Started with C4

This guide walks through the core workflow: scanning files, creating capsules, comparing directories, and transferring content.

## Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
go install github.com/Avalanche-io/c4d@latest
```

## 1. Identify Files

Every file gets a universally unique C4 ID:

```bash
$ c4 photo.jpg
c43zYcLni5LF9rR4Lg4B8h3Jp8SBwjcnyyeh4bc6gTPHndKuKdjUWx1kJPYhZxYt3zV6tQXpDs2shPsPYjgG81wZM1

$ echo "hello world" | c4
c44SjyfSsNez6bqFCJeFCSurmMiQ3DFCXkG67PiB9DJobUqG2YhvMeCvig6fjuh67SmrUUYMcaHmJjNMeZCqbNkWcTP
```

A directory gets a single ID representing all its contents:

```bash
$ c4 my-project/
c435RzTWWsjWD1Fi7dxS3idJ7vFgPVR96oE95RfDDT5ue7hRSPENePDjPDJdnV46g7emDzWK8LzJUjGESMG5qzuXqq
```

## 2. Create a Capsule

A capsule is a lightweight description of a directory — every file's path, size, and C4 ID:

```bash
# One level deep
$ c4 -m my-project/

# Full recursive scan
$ c4 -mr my-project/

# Save to a .c4m file
$ c4 -mr my-project/ > my-project.c4m
```

The capsule is a small text file (typically a few KB) that fully describes a directory that could contain terabytes of data.

## 3. Compare Directories

See what changed between two snapshots:

```bash
$ c4 diff old.c4m new.c4m
```

Find what files you're missing:

```bash
$ c4 subtract needed.c4m local/ > todo.c4m
```

## 4. Transfer Content with c4d

Start a c4d node to store and serve content:

```bash
# First-time setup
$ c4d init

# Start the daemon
$ c4d serve
```

Use the `c4` CLI to copy content to and from capsules backed by c4d:

```bash
# Create a capsule and push content to c4d
$ c4 mk project.c4m:
$ c4 cp -r ./src/ project.c4m:src/

# Materialize content from a capsule
$ c4 cp project.c4m:src/ ./restored/
```

## 5. Progressive Scanning

For large directories, use progressive scanning — you can interrupt and resume:

```bash
# Start a progressive scan (Ctrl+C to stop, Ctrl+T for status on macOS)
$ c4 --progressive --bundle large-project/

# Resume where you left off
$ c4 --bundle --resume large-project.c4m_bundle
```

## Next Steps

- [C4M User Guide](../c4m/README.md) — Capsule format details
- [C4M Specification](../c4m/SPECIFICATION.md) — Formal spec
- [CLI Reference](../README.md) — All commands and flags
