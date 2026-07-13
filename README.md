# C4 — Universal Content Identification

*Everything in Unix is a file, except for the filesystem itself. Until now.*

[![CI](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Avalanche-io/c4)](https://goreportcard.com/report/github.com/Avalanche-io/c4)
[![Go Reference](https://pkg.go.dev/badge/github.com/Avalanche-io/c4.svg)](https://pkg.go.dev/github.com/Avalanche-io/c4)
[![Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

C4 is a file management system designed to simplify distributed
systems. It decouples file storage and transport from organization
and metadata by providing a standardized content-derived identifier,
and using these identifiers in place of file content in a simple,
readable, text-based filesystem format.

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
file — a plain-text description that captures the identity of every
file and directory in a tree:

```bash
$ c4 id ./project/
-rw-r--r-- 2026-03-18T22:22:57Z 13 README.md c44iCq6un9W47x7ydjJSWp4arMJ...
drwxr-xr-x 2026-03-18T22:22:57Z - src/ -
  -rw-r--r-- 2026-03-18T22:22:57Z 66 main.go c43Q4j81SxGkV9FhbeW23YrMTj6...
drwxr-xr-x 2026-03-18T22:22:57Z 66 src/ c44nbgL6nkBWsEBDCUCr4LufsjVhJt...
c45k2Jd...                                  ← the final line is the tree's ID
```

Each line looks like `ls -l` with a content ID at the end — and the
output *streams*: file lines print the moment they're hashed
(directory aggregates fill in via a refinement patch at the end,
because a folder's ID is the fingerprint of its contents). The final
line is always the whole tree's ID. Need machine-independent identity?
`c4 id -q -m c` projects modes and timestamps away, so the same bytes
give the same ID on any machine, clock, or umask.

A c4m file is just text. Pipe it, grep it, diff it, email it. The
format is designed to compose with `awk`, `sort`, `grep`, and the
rest of the Unix toolkit — no special tools required, though C4
provides some that are useful.

## Install

### Homebrew (recommended — includes c4 and c4sh)

```bash
brew install mrjoshuak/tap/c4
```

### Binary downloads

Pre-built archives for macOS, Linux, and Windows:
[c4toolkit releases](https://github.com/Avalanche-io/c4toolkit/releases)

### From source

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
go install github.com/Avalanche-io/c4sh@latest
```

### Other languages

```bash
pip install c4py                    # Python
npm install @avalanche-io/c4        # TypeScript / JavaScript
```

See [c4toolkit](https://github.com/Avalanche-io/c4toolkit) for the
full suite and version matrix.

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

**Snapshot into the store.** One command, one ID back. The ID prints
only after everything it names is on stable media — if it printed,
you can get it back, even after `kill -9` or power loss:

```bash
c4 id -s ./final/                    # snapshot; streams the listing,
                                     # ends with the durable ID
SNAP=$(c4 id -s -q ./final/)         # script capture: THE ID, one line
c4 cat "$SNAP"                       # the root listing
c4 cat "$SNAP"/src/parser.go         # extract one file, verified
c4 log                               # every snapshot the store holds
```

The store can be a local directory, an S3-compatible object store, or
both. Content goes in once, comes out by ID.

**Make a directory match a description.** Declarative — you say what
the result should be, C4 figures out how to get there. Dry run by
default; `--force` snapshots the directory first, so every restore is
undoable:

```bash
c4 restore "$SNAP" ./dir/            # dry run: print the plan
out=$(c4 restore --force "$SNAP" ./dir/)
undo=$(printf '%s\n' "$out" | sed -n 1p)
c4 restore --force "$undo" ./dir/    # undo — which prints its own undo
```

Nothing starts until all required content is confirmed available, and
the undo handle is journaled before the first destructive operation.

**Work with Unix tools.** One entry per line, fields in predictable
positions:

```bash
awk '{print $NF}' project.c4m | sort | uniq -d       # find duplicates
grep '\.exr ' project.c4m | wc -l                    # count EXR files
```

**Merge two trees.** Combine branches, deliveries, or any two c4m
files into one:

```bash
c4 merge branch-a.c4m branch-b.c4m > merged.c4m
```

Conflicts (same path, different content) are reported. Non-overlapping
entries are combined.

**Re-scan fast.** A prior full description is a reuse guide: files
whose size and mtime are unchanged (and safely older than the guide's
scan — git's racy-index rule) keep their recorded IDs without being
re-read:

```bash
c4 id ./project/ > full.c4m           # full description once
c4 id -c full.c4m -q ./project/       # fast re-scan (trusted mtimes)
c4 id --verify -c full.c4m ./project/ # audit: full re-hash, report
                                      # changes hidden under unchanged
                                      # size+mtime
```

## Commands

| Command        | What it does                                                 |
|----------------|--------------------------------------------------------------|
| `c4 id`        | Describe files, directories, or c4m files; snapshot with -s  |
| `c4 restore`   | Make a directory match a description (dry run by default)    |
| `c4 cat`       | Retrieve a store object by ID or ID/path, verified           |
| `c4 diff`      | Compare two states (directories, c4m files, or store IDs)    |
| `c4 patch`     | Resolve patch chains to c4m text (never touches directories) |
| `c4 merge`     | Combine two or more trees                                    |
| `c4 log`       | List the store journal, or a chain's sections                |
| `c4 split`     | Split a patch chain                                          |
| `c4 explain`   | Human-readable narration of what a command would do          |
| `c4 paths`     | Convert between c4m format and plain path lists              |
| `c4 intersect` | Find common entries between two c4m files                    |

Nothing is written anywhere without `-s` (or `restore --force`), and
`-s` writes only inside the store. `c4 <path>` is a read-only
shortcut for `c4 id <path>`; `c4 -s <path>` snapshots. `echo "data" |
c4` prints the ID of stdin (nothing stored); `echo "data" | c4 -s`
stores it. Every verb's `--help` is its reference page.

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
- [C4 ID Whitepaper](https://cccc.io/c4id-whitepaper-u2.pdf)
- [CLI Reference](./docs/cli-reference.md)
- [Getting Started](./docs/getting-started.md)
- [FAQ](./docs/faq.md) — design decisions (SHA-512 permanence, c4m format, store scaling)

## The C4 toolkit

C4 is part of a cross-language ecosystem. Every tool reads and writes
the same c4m format and produces identical C4 IDs:

| Tool | Language | What it does |
|------|----------|-------------|
| **[c4](https://github.com/Avalanche-io/c4)** | Go | CLI — identify, snapshot, restore, diff |
| **[c4sh](https://github.com/Avalanche-io/c4sh)** | Go | Shell — cd into c4m files, browse, copy |
| **[c4py](https://github.com/Avalanche-io/c4py)** | Python | Library — scan, diff, verify, store |
| **[c4git](https://github.com/Avalanche-io/c4git)** | Go | Git filter — version large files |
| **[c4ts](https://github.com/Avalanche-io/c4ts)** | TypeScript | Browser + Node.js — zero dependencies |
| **[c4-swift](https://github.com/Avalanche-io/c4-swift)** | Swift | Apple platforms — Sendable, Codable |
| **[libc4](https://github.com/Avalanche-io/libc4)** | C | Embed in any application |

See [c4toolkit](https://github.com/Avalanche-io/c4toolkit) for the
full suite, install options, and version matrix.

## License

Apache 2.0
