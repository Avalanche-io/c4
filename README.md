
# C4 — Content-Addressable Identification

[![CI](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Avalanche-io/c4)](https://goreportcard.com/report/github.com/Avalanche-io/c4)
[![Go Reference](https://pkg.go.dev/badge/github.com/Avalanche-io/c4.svg)](https://pkg.go.dev/github.com/Avalanche-io/c4)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

C4 gives every file an ID based on what it contains — not where it lives
or what it's called. Same content, same ID, always. Different content,
different ID, always.

```bash
echo "hello" | c4
# c447Fm3BJZQ62765jMZJH4m28hrDM7Szbj9CUmj4F4gnvyDYXYz4WfnK2nYRhFvRgYEectEXYBYWLDpLo6XGNAfKdt
```

That ID is a [SMPTE ST 2114:2017](https://ieeexplore.ieee.org/document/7971777)
identifier — an international standard for content identification based
on SHA-512. Run it again anywhere, on any machine, and you'll get the
same 90 characters.

## Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

## What can you do with it?

**Track what changed.** Save a snapshot, come back later, diff it:

```bash
c4 id ./deliverables/ > monday.c4m
# ... work happens ...
c4 id ./deliverables/ > friday.c4m
c4 diff monday.c4m friday.c4m
```

When both sides are c4m files, the diff compares IDs — not file
contents — so it completes in milliseconds regardless of how large the
underlying files are. Everything is a file, including the manifest.

**Build version history.** A c4m file can accumulate patches over time:

```bash
c4 id ./project/ > project.c4m                                     # snapshot
c4 diff project.c4m ./project/ >> project.c4m                      # append changes

c4 log project.c4m          # see what changed
c4 patch -n 1 project.c4m   # recover the original state
```

**Reconcile directories.** Declare the state you want. c4 figures out
the rest:

```bash
c4 patch target.c4m ./dir/
```

This diffs the current directory against the target, then applies only
what's different — creating, moving, and removing files as needed.
Nothing starts until all required content is confirmed available.
Operations are idempotent and safe to re-run after interruption.

**Store and retrieve content by ID:**

```bash
c4 id -s ./final/ > delivery.c4m     # identify + store
c4 cat c44iCq6un9W47...              # retrieve by ID
```

The store can be a local directory or an S3-compatible object store.
Content goes in once, comes out by ID. Simple as `cat`.

**Reversible operations.** Store what you're about to destroy:

```bash
# Apply a patch, but save any files that would be removed or overwritten.
c4 patch -s new_state.c4m ./dir/ > changeset.c4m

# Revert to the previous state using the saved changeset.
c4 patch -r changeset.c4m ./dir/                     # revert
```

**Compose with Unix tools.** The c4m format is one entry per line —
designed to work with the tools you already know:

```bash
awk '{print $NF}' project.c4m | sort | uniq -d       # find duplicates
grep '\.exr ' project.c4m | wc -l                    # count EXR files
comm -23 <(awk '{print $NF}' a.c4m | sort) \
         <(awk '{print $NF}' b.c4m | sort)           # IDs in a but not b
```

No special query language. No database. Just text.

**Scan fast, hash later.** Structure-only scans skip content hashing
entirely. Edit the manifest to remove what you don't need, then hash
only what survived:

```bash
c4 id -m s ./project/ > scan.c4m      # names + structure only
vi scan.c4m                           # remove what you don't want
c4 id -c scan.c4m ./project/          # continue scan, hash only what remains
```

## Commands

| Command    | What it does                                                |
| ---------- | ----------------------------------------------------------- |
| `c4 id`    | Identify files, directories, or c4m files                   |
| `c4 cat`   | Retrieve content by C4 ID from store                        |
| `c4 diff`  | Compare two states (c4m files or directories)               |
| `c4 patch` | Apply a target state: reconcile directories, resolve chains |
| `c4 merge` | Combine two or more trees                                   |
| `c4 log`   | Show patch history                                          |
| `c4 split` | Split a patch chain                                         |

Every command that accepts a c4m file also accepts a directory, and
vice versa. `echo "hello" | c4` produces a bare C4 ID from stdin.

## The c4m format

A c4m file is a complete filesystem description in plain text. Each line
has permissions, timestamp, size, name, and C4 ID — like `ls -l` with
content identity. Readable, editable, diffable, pipeable.

A 10,000-entry c4m file is about 1.4 MB. It describes the identity of
every file in the tree regardless of how large those files are. The
description is the lightweight handle; the content is the heavy thing
it refers to.

- [User Guide](./c4m/README.md)
- [Specification](./c4m/SPECIFICATION.md)
- [Unix Recipes](./docs/c4m-unix-recipes.md)

## Go library

```go
import "github.com/Avalanche-io/c4"

id := c4.Identify(strings.NewReader("hello"))
fmt.Println(id)
// c447Fm3BJZQ62765jMZJH4m28hrDM7Szbj9CUmj4F4gnvyDYXYz4WfnK2nYRhFvRgYEectEXYBYWLDpLo6XGNAfKdt
```

C4 IDs are 90-character base58 strings — SHA-512 with a `c4` prefix.
URL-safe, filename-safe, double-click selectable.

Zero external dependencies. Go 1.16+.

## Links

- [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU) (video)
- [C4 ID Whitepaper](http://www.cccc.io/c4id-whitepaper-u2.pdf)
- [CLI Reference](./docs/cli-reference.md)
- [Getting Started](./docs/getting-started.md)

## License

MIT
