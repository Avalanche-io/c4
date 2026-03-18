
# C4 - Universal Content Identification

[![CI](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Avalanche-io/c4)](https://goreportcard.com/report/github.com/Avalanche-io/c4)
[![Go Reference](https://pkg.go.dev/badge/github.com/Avalanche-io/c4.svg)](https://pkg.go.dev/github.com/Avalanche-io/c4)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

C4 implements **SMPTE ST 2114:2017** — the international standard for
content-addressable asset identification. It gives every file a
universally unique, unforgeable ID derived from its content. Two files
with the same content always produce the same ID, regardless of where
they live or what they're named.

C4 works with any file type: source code, documents, images, video,
render passes, genomic data, build artifacts — anything that's bytes.
A single ID can represent one file or an entire directory tree.

Zero external dependencies. Go 1.16+.

### Install

```bash
brew install Avalanche-io/tap/c4
```

Or with Go (1.16+):

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

### Quick Start

```bash
# Identify a file
c4 id document.pdf

# Identify a directory tree
c4 id myproject/ > project.c4m

# Pipe data (bare C4 ID)
echo "hello" | c4

# Compare two snapshots
c4 diff before.c4m after.c4m

# Version a directory over time
c4 diff project.c4m <(c4 id myproject/) >> project.c4m
c4 log project.c4m
```

### What Can You Do With It?

**Track what changed.** Compare two directory trees — or two snapshots
of the same tree taken at different times. Changed files have different
C4 IDs. Unchanged subtrees are skipped entirely.

```bash
c4 id ./deliverables/ > monday.c4m
# ... time passes ...
c4 diff monday.c4m <(c4 id ./deliverables/)
```

**Know what you have.** Identify a render farm output, a code release,
a dataset, a backup — and know with cryptographic certainty whether it
matches what you expect.

```bash
c4 id -s ./final_delivery/ > delivery.c4m
# The c4m file is a receipt. The store holds the content.
# Verify later: c4 cat <id> recovers any file by its ID.
```

**Deduplicate.** If two files anywhere in the tree have the same content,
they have the same C4 ID. Find duplicates across millions of files:

```bash
awk '{print $NF}' project.c4m | sort | uniq -d
```

**Scan fast, hash later.** Structure-only scans are instant. Filter
the c4m to remove what you don't need, then hash only what matters:

```bash
c4 id -m s ./project/ > scan.c4m     # instant structure scan
vi scan.c4m                           # remove unwanted dirs
c4 id -c scan.c4m ./project/          # hash only what's left
```

### C4M Format

A **c4m file** (`.c4m`) is a complete description of a filesystem in
plain text. Each line is a self-contained record with permissions,
timestamp, size, name, and C4 ID — readable by humans and parseable
with `grep`, `awk`, `sort`, and `diff`.

- [User Guide](./c4m/README.md) — Quick start and examples
- [Specification](./c4m/SPECIFICATION.md) — Formal C4M v1.0 spec
- [Unix Recipes](./docs/c4m-unix-recipes.md) — grep/awk/sed tricks

### Commands

| Command | Description |
|---------|-------------|
| `c4 id` | Identify files, directories, or c4m files |
| `c4 cat` | Retrieve content by C4 ID from store |
| `c4 diff` | Compare two trees, produce a c4m patch |
| `c4 patch` | Apply target state: reconcile dirs, resolve chains, revert |
| `c4 merge` | Combine 2+ filesystem trees (c4m files or directories) |
| `c4 log` | List patches in a chain |
| `c4 split` | Split a patch chain for branching |
| `c4 version` | Print version |

See the [CLI Reference](./docs/cli-reference.md) for flags and workflows.

### Go Library

```go
import "github.com/Avalanche-io/c4"

// Identify content
id := c4.Identify(strings.NewReader("alfa"))

// Identify a set of blocks (order-independent)
var ids c4.IDs
for _, input := range inputs {
    ids = append(ids, c4.Identify(strings.NewReader(input)))
}
setID := ids.ID()
```

### Encoding

C4 is the shortest self-identifying SHA-512 encoding and the only
standardized encoding (SMPTE ST 2114:2017).

```yaml
# encoding     length   id
  hex          135:     sha512-f7fbba6e0636f890e56fbbf3283e524c...
  base64        95:     sha512-9/u6bgY2+JDlb7vzKD5STG+jIErimD...
  c4            90:     c43inc2qGhSWQUMRvDMW6GAjJnRFY5sxq399...
```

IDs are URL-safe, filename-safe, and double-click selectable.
Regex: `c4[1-9A-HJ-NP-Za-km-z]{88}`

---

### Links

- [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU) (video)
- [The Magic of C4](https://youtu.be/vzh0JzKhY4o) (video)
- [C4 ID Whitepaper](http://www.cccc.io/c4id-whitepaper-u2.pdf)

### Issues

Report bugs and request features at the [issue tracker](https://github.com/Avalanche-io/c4/issues).

### Contributing

Contributions welcome. See [CONTRIBUTING.md](./CONTRIBUTING.md).

### License

MIT. See [LICENSE](./LICENSE).
