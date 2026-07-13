# Getting Started with C4

This guide walks through the core loop: identifying files, snapshotting
into the store, comparing states, restoring, and versioning with patches.

## Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

## 1. Identify Files

```bash
$ c4 id photo.jpg
- - 4404019 photo.jpg c43zYcLni5LF...

$ echo "hello world" | c4
c44SjyfSsNez6bqFCJeFCSurmMiQ3DFCXkG67PiB9DJobUqG2YhvMeCvig6fjuh67SmrUUYMcaHmJjNMeZCqbNkWcTP
```

By default a description records everything observed (the `ls -l`
view). For machine-independent identity — same bytes, same ID on any
machine, clock, or umask — project to *content* level:

```bash
$ c4 id photo.jpg
-rw-r--r-- 2026-03-04T14:22:10Z 4404019 photo.jpg c43zYcLni5LF...
$ c4 id -q -m c photo.jpg
c43zYcLni5LF...
```

Nothing here writes anything — `c4 id` (and bare `c4 <path>`) is
read-only.

## 2. Create a c4m File

A c4m file is a lightweight description of a directory — a small text
file (typically a few KB) that fully describes a directory that could
contain terabytes of data:

```bash
c4 id myproject/ > project.c4m          # streams; a valid c4m chain
                                        # whose final line is the ID
```

## 3. Snapshot into the Store

`-s` stores a complete snapshot — every file's bytes, every
directory's listing — and prints one thing: THE snapshot ID. The ID
prints only after everything it names is on stable media; if it
printed, you can get it back, even after `kill -9` or power loss.

```bash
$ c4 id -s myproject/                   # streams the listing; the
                                        # final line is the durable ID
$ SNAP=$(c4 id -s -q myproject/)        # script capture: one line
$ echo "$SNAP"
c43k2Jd...

$ c4 cat "$SNAP"                        # the root listing
$ c4 cat "$SNAP"/src/main.go > main.go  # extract one file, verified
$ c4 log                                # every claim the store holds
```

On first use of `-s`, the CLI offers to create a default store at
`~/.c4/store`. You can also configure it explicitly:

```bash
C4_STORE=/path/to/store                                        # local
C4_STORE=s3://bucket/prefix?region=us-west-2                   # S3
C4_STORE=/fast/ssd,s3://bucket/c4?region=us-west-2             # multiple
```

## 4. Compare States

See what changed. Sides can be c4m files, directories, or store IDs:

```bash
c4 diff old.c4m new.c4m
c4 diff project.c4m ./project/
c4 diff "$SNAP" ./project/            # what changed since the snapshot
```

## 5. Restore — with Undo

Make a directory match a description. Dry run by default; `--force`
snapshots the directory first, so every restore is undoable:

```bash
c4 restore "$SNAP" ./project/               # dry run: print the plan
out=$(c4 restore --force "$SNAP" ./project/)

# stdout is two bare-ID lines: the undo handle, then the verified result
undo=$(printf '%s\n' "$out" | sed -n 1p)
c4 restore --force "$undo" ./project/       # undo — prints its own undo
```

The undo handle is journaled before anything is destroyed — `c4 log`
is the reflog, and no c4 verb deletes store objects.

## 6. Version with Patches

Append diffs to a c4m file to build a version history:

```bash
c4 id ./project/ > project.c4m                            # snapshot
c4 diff project.c4m <(c4 id ./project/) >> project.c4m    # append changes

c4 log project.c4m               # view history
c4 patch project.c4m             # resolve to final state (text out)
c4 patch -n 1 project.c4m        # recover the original state
c4 split project.c4m 3 common.c4m rest.c4m   # branch at any point
```

`patch` composes descriptions — text in, text out. To change a
directory, use `restore`.

## Next Steps

- [CLI Reference](./cli-reference.md) — All commands and flags
- [C4M User Guide](../c4m/README.md) — c4m format details
- [C4M Specification](../c4m/SPECIFICATION.md) — Formal spec
