
# C4 — Content-Addressable Identification

[![CI](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Avalanche-io/c4)](https://goreportcard.com/report/github.com/Avalanche-io/c4)
[![Go Reference](https://pkg.go.dev/badge/github.com/Avalanche-io/c4.svg)](https://pkg.go.dev/github.com/Avalanche-io/c4)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

C4 gives every file an ID based on what it contains — not where it lives
or what it's called. Same content, same ID, always. Different content,
different ID, always.

```
$ c4 id .
-rw-r--r-- Mar 18 13:38:54 2026 CDT   1,846 CHANGELOG.md                c424v...
-rw-r--r-- Aug 29 17:29:44 2025 CDT   3,215 CODE_OF_CONDUCT.md          c43WS...
-rw-r--r-- Mar 18 13:38:54 2026 CDT   4,214 CONTRIBUTING.md             c414p...
-rw-r--r-- Aug 29 17:29:44 2025 CDT     197 CONTRIBUTORS                c44nd...
-rw-r--r-- Aug 29 17:29:44 2025 CDT   1,132 LICENSE                     c42js...
-rw-r--r-- Mar 18 17:28:25 2026 CDT   5,744 README.md                   c42xS...
-rw-r--r-- Aug 29 17:29:44 2025 CDT   2,500 SECURITY.md                 c44GH...
-rw-r--r-- Mar 16 18:06:52 2026 CDT     565 doc.go                      c43yb...
-rw-r--r-- Mar 18 13:38:54 2026 CDT      43 go.mod                      c44nz...
-rw-r--r-- Mar 18 13:38:54 2026 CDT       0 go.sum                      c459d...
-rw-r--r-- Mar 18 13:38:54 2026 CDT   5,069 id.go                       c45MH...
-rw-r--r-- Mar 18 13:38:54 2026 CDT  19,439 id_test.go                  c42U9...
-rw-r--r-- Mar 18 13:38:54 2026 CDT   4,014 internals_test.go           c45m2...
-rw-r--r-- Aug 29 17:29:44 2025 CDT   4,299 tree.go                     c447F...
-rw-r--r-- Mar 18 13:38:54 2026 CDT   2,043 tree_test.go                c43Md...
-rw-r--r-- Aug 29 17:29:44 2025 CDT   1,928 treeslice_bench_test.go     c45vX...
drwxr-xr-x Mar 18 13:38:54 2026 CDT 642,792 c4m/                        c443Z...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT  34,799 C4M-STANDARD.md           c421X...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT   4,492 METADATA_COVERAGE.md      c43Ep...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT   5,179 README.md                 c418B...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT  17,910 SPECIFICATION.md          c41Ep...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT   6,408 WORKFLOWS.md              c4189...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT   8,137 adversarial_test.go       c41iW...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT   5,732 benchmarks_test.go        c41Cq...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT   4,928 builder.go                c4621...
  -rw-r--r-- Mar 18 13:38:54 2026 CDT  13,005 builder_test.go           c43ui...
#...
```

The output is a c4m file — a plain-text description of the filesystem.
One line per entry. Pipe it, grep it, diff it, email it. It's just text.

C4 IDs implement [SMPTE ST 2114:2017](https://ieeexplore.ieee.org/document/7971777),
an international standard for content identification based on SHA-512.

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
