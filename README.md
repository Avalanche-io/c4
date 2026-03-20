
# C4 — Universal Content Identification

*In Unix, everything is a file. With C4, every file has an identity.*

[![CI](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Avalanche-io/c4)](https://goreportcard.com/report/github.com/Avalanche-io/c4)
[![Go Reference](https://pkg.go.dev/badge/github.com/Avalanche-io/c4.svg)](https://pkg.go.dev/github.com/Avalanche-io/c4)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

C4 gives every file an ID derived from its content — not its name,
path, or location. Two people on opposite sides of the world, who
have never met, will independently produce the same ID for the same
file. No registry, no coordination, no central authority. The content
itself is the agreement.

```bash
$ echo "hello" | c4
c447Fm3BJZQ62765jMZJH4m28hrDM7Szbj9CUmj4F4gnvyDYXYz4WfnK2nYRhFvRgYEectEXYBYWLDpLo6XGNAfKdt
```

That 90-character string is a [SMPTE ST 2114:2017](https://ieeexplore.ieee.org/document/7971777)
identifier — an international standard built on SHA-512. Run it on
any machine, in any language, ten years from now. Same input, same ID.
Unlike a UUID, which is assigned arbitrarily and requires coordination
to be meaningful, a C4 ID is discovered from the data itself. If two
people have a file with the same ID, they have identical copies of the
same file.

C4 extends this idea to entire filesystems. `c4 id` produces a c4m
file — a plain-text c4m file that captures the identity of every file
and directory in a tree:

```bash
$ c4 id ./project/
-rw-r--r-- 2026-03-18T22:22:57Z 13 README.md c44iCq6un9W47x7ydjJSWp4arMJ...
drwxr-xr-x 2026-03-18T22:22:57Z 66 src/ c44nbgL6nkBWsEBDCUCr4LufsjVhJt...
  -rw-r--r-- 2026-03-18T22:22:57Z 66 main.go c43Q4j81SxGkV9FhbeW23YrMTj6...
```

A c4m file is just text. Pipe it, grep it, diff it, email it. Each
line looks like `ls -l` with a content ID at the end. The format is
designed to compose with `awk`, `sort`, `grep`, and the rest of the
Unix toolkit — no special tools required, though C4 provides some
that are useful.

## Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

## What can you do with it?

**Know what changed.** Snapshot a directory, come back later, compare:

```bash
c4 id ./deliverables/ > monday.c4m
# ... work happens ...
c4 diff monday.c4m ./deliverables/
```

**Build history.** Append diffs to a c4m file to create a version chain:

```bash
c4 id ./project/ > project.c4m                                  # snapshot
c4 diff project.c4m ./project/ >> project.c4m                    # append changes

c4 log project.c4m          # see what changed
c4 patch -n 1 project.c4m   # recover the original state
```

**Make a directory match a description.** Declarative — you say what
the result should be, C4 figures out how to get there:

```bash
c4 patch target.c4m ./dir/
```

Creates, moves, and removes files as needed. Nothing starts until all
required content is confirmed available. Safe to re-run after
interruption.

**Store and retrieve content by ID:**

```bash
c4 id -s ./final/ > delivery.c4m     # identify + store
c4 cat c44iCq6un9W47...              # retrieve by ID
```

The store can be a local directory, an S3-compatible object store, or
both. Content goes in once, comes out by ID.

**Undo what you just did:**

```bash
c4 patch -s new_state.c4m ./dir/ > changeset.c4m    # apply + preserve
c4 patch -r changeset.c4m ./dir/                     # revert
```

**Work with Unix tools.** One entry per line, fields in predictable
positions:

```bash
awk '{print $NF}' project.c4m | sort | uniq -d       # find duplicates
grep '\.exr ' project.c4m | wc -l                    # count EXR files
```

**Scan fast, hash later.** Structure-only scans skip content hashing.
Edit the manifest, then hash only what survived:

```bash
c4 id -m s ./project/ > scan.c4m     # names only
vi scan.c4m                           # remove what you don't want
c4 id -c scan.c4m ./project/          # hash the rest
```

## Commands

| Command | What it does |
|---------|-------------|
| `c4 id` | Identify files, directories, or c4m files |
| `c4 cat` | Retrieve content by C4 ID from store |
| `c4 diff` | Compare two states (c4m files or directories) |
| `c4 patch` | Apply a target state: reconcile directories, resolve chains |
| `c4 merge` | Combine two or more trees |
| `c4 log` | Show patch history |
| `c4 split` | Split a patch chain |

Every command that takes a c4m file also takes a directory, and vice
versa. `echo "data" | c4` produces a bare ID from stdin.

## The c4m format

A c4m file is a complete filesystem description in plain text. Each
line has permissions, timestamp, size, name, and C4 ID. Readable,
editable, diffable, composable.

A 10,000-entry c4m file is about 1.4 MB. It describes the identity
of every file in the tree regardless of how large those files are.
The description is the lightweight handle; the content is the heavy
thing it refers to.

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
