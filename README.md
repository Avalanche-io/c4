
# C4 - Universal Content Identification

[![CI](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml/badge.svg)](https://github.com/Avalanche-io/c4/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Avalanche-io/c4)](https://goreportcard.com/report/github.com/Avalanche-io/c4)
[![Go Reference](https://pkg.go.dev/badge/github.com/Avalanche-io/c4.svg)](https://pkg.go.dev/github.com/Avalanche-io/c4)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

```go
import "github.com/Avalanche-io/c4"
```

This is a Go package that implements the C4 ID system **SMPTE standard ST 2114:2017**.
C4 IDs are universally unique and consistent identifiers that standardize the
derivation and formatting of data identification so that all users independently
agree on the identification of any block or set of blocks of data.

C4 IDs are 90 character long strings suitable for use in filenames, URLs,
database fields, or anywhere else that a string identifier might normally be
used. In ram C4 IDs are represented in a 64 byte "digest" format.

### Install

```bash
go install github.com/Avalanche-io/c4/cmd/c4@latest
```

### Features

- Universally unique, unforgeable identification of any data (SMPTE ST 2114:2017)
- Single ID represents one file or millions of files
- Same content always produces the same ID, regardless of location or time
- Works offline — no network connection required
- IDs are URL-safe, filename-safe, double-click selectable
- Regex: `c4[1-9A-HJ-NP-Za-km-z]{88}`

### C4M Format

A **c4m file** (`.c4m`) describes a filesystem — a small, human-readable text
file that behaves like the full directory without needing the actual file
content.

- [User Guide](./c4m/README.md) — Quick start and examples
- [Specification](./c4m/SPECIFICATION.md) — Formal C4M v1.0 spec
- [Implementation Notes](./c4m/IMPLEMENTATION_NOTES.md) — Edge cases

### Comparison of Encodings

C4 is the shortest self identifying SHA-512 encoding and is the only standardized encoding.
To illustrate, the following is the SHA-512 of "foo" in hex, base64 and c4 encodings:

```yaml
# encoding     length   id
  hex          135:     sha512-f7fbba6e0636f890e56fbbf3283e524c6fa3204ae298382d624741d0dc6638326e282c41be5e4254d8820772c5518a2c5a8c0c7f7eda19594a7eb539453e1ed7
  base64        95:     sha512-9/u6bgY2+JDlb7vzKD5STG+jIErimDgtYkdB0NxmODJuKCxBvl5CVNiCB3LFUYosWowMf37aGVlKfrU5RT4e1w==
  c4            90:     c43inc2qGhSWQUMRvDMW6GAjJnRFY5sxq399wcUcWLTuPai84A2QWTfYu1gAW8f5FmZFGeYpLsSPyrSUh9Ao3J68Cc
```

### CLI Usage

```bash
# Identify a file (outputs a c4m entry with metadata + C4 ID)
c4 file.txt

# Identify a directory (full recursive c4m listing)
c4 myproject/

# Just the C4 ID
c4 -i myproject/

# Compare two directories
c4 diff old/ new/

# Pipe data (bare C4 ID, no metadata)
echo "hello" | c4
```

### Go Library

```go
import "github.com/Avalanche-io/c4"

// Identify a single block of data
id := c4.Identify(strings.NewReader("alfa"))

// Identify a set of blocks (order-independent)
var ids c4.IDs
for _, input := range inputs {
    ids = append(ids, c4.Identify(strings.NewReader(input)))
}
setID := ids.ID()
```

---

### Links

- [C4 Framework Universal Asset ID](https://youtu.be/ZHQY0WYmGYU) (video)
- [The Magic of C4](https://youtu.be/vzh0JzKhY4o) (video)
- [C4 ID Whitepaper](http://www.cccc.io/c4id-whitepaper-u2.pdf)

### Contributing

Contributions welcome. Branch from `dev`, submit PRs against `dev`. The `master` branch holds tagged releases.

### License

MIT. See [LICENSE](./LICENSE).
