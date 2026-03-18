# C4M — C4 Manifest Format

A c4m file is a complete description of a filesystem. It captures the full tree structure — every file, directory, symlink, permission, timestamp, size, and content identity — in a plain text document you can read, email, diff, and pipe through Unix tools.

```
-rw-r--r-- 2025-01-15T10:30:00Z  1,234 readme.txt   c41j3C6Jqga95PL2zmZVBW...
-rw-r--r-- 2025-01-15T10:30:00Z 12,800 main.go      c43pCP9e69EGD253L3pcwc...
drwxr-xr-x 2025-01-15T10:30:00Z 48,200 assets/      c4RgFeXFYL1FjFueMKjPjn...
  -rw-r--r-- 2025-01-15T10:30:00Z 48,200 logo.png   c42RgFeXFYL1FjFueMKjPjn...
```

Every file has a C4 ID — a cryptographic content identifier (SMPTE ST 2114:2017). Every directory has one too, computed from its contents. If two directories have the same C4 ID, they contain exactly the same data, no matter where they are or how they got there.

## What c4m captures

A c4m file represents the complete POSIX-like filesystem tree: regular files, directories, symlinks, hard links, named pipes, sockets, and device nodes — with permissions, timestamps, sizes, content C4 IDs, and support for arbitrary filenames including non-printable bytes and invalid UTF-8. Media file sequences like `frame.[0001-0100].exr` get compact notation.

This covers the structural and content information of any filesystem you are likely to encounter. Environment-specific metadata like ownership and extended attributes is not part of the format but can be associated with c4m entries through external systems using paths or C4 IDs as join keys. See [METADATA_COVERAGE.md](METADATA_COVERAGE.md) for details.

## What c4m makes possible

These are capabilities that did not exist before c4m and C4 IDs:

- **Filesystem-as-document**: A c4m file *is* the filesystem, but can email it, diff it, version it, and reconstruct the full tree from it plus the content store.
- **Content-addressed deduplication**: Identical files anywhere in the tree (or across trees) share a single C4 ID. Storage and transfer only need one copy.
- **Cryptographic diff**: Comparing two directory trees is comparing two c4m files. Changed files have different C4 IDs. Unchanged subtrees can be skipped entirely (their directory C4 IDs match).
- **Incremental patches**: The [patch format](SPECIFICATION.md#patch-format) describes changes between states as inline entries. A million-file tree with one changed file produces a patch touching only the path to that file.
- **Progressive resolution**: Start with just filenames. Add permissions and sizes later. Add C4 IDs when content is hashed. Each stage is a valid c4m file.
- **Human editability**: It's text with a familiar structure that looks a lot like `ls -l`. You can write a c4m file by hand, fix one in a text editor, or generate one from a script.
- **Composability**: c4m output is plain text with one entry per line. `grep`, `awk`, `sort`, `diff`, and every other Unix tool works on it natively.

## Quick start

```bash
# View a directory as a c4m listing
c4 id myproject/

# Save a c4m file
c4 id myproject/ > snapshot.c4m

# Get the tree's C4 ID (pipe c4m through stdin)
c4 id myproject/ | c4

# Compare two snapshots
c4 diff old.c4m new.c4m
```

## Format overview

Each entry occupies one line:

```
<mode> <timestamp> <size> <name> [link-operator target] <c4id>
```

- **Mode**: Unix permissions (`-rw-r--r--`, `drwxr-xr-x`, `lrwxrwxrwx`, etc.) or `-` for unspecified
- **Timestamp**: RFC 3339 UTC (`2025-01-15T10:30:00Z`) or `-` for unspecified
- **Size**: Bytes as integer, or `-` for unspecified
- **Name**: Bare filename (directories end with `/`), backslash-escaped for spaces and special characters
- **C4 ID**: 90-character SMPTE ST 2114 identifier, or `-` for uncomputed

Nesting is expressed through indentation:

```
drwxr-xr-x 2025-01-15T10:30:00Z 50000 project/
  -rw-r--r-- 2025-01-15T10:30:00Z  1234 readme.txt c4...
  drwxr-xr-x 2025-01-15T10:30:00Z 48766 src/
    -rw-r--r-- 2025-01-15T10:30:00Z 12800 main.go c4...
```

For the complete format definition, see [SPECIFICATION.md](SPECIFICATION.md). For the formal standard with ABNF grammar, see [C4M-STANDARD.md](C4M-STANDARD.md).

## Go package

```go
// Decode
m, err := c4m.NewDecoder(reader).Decode()

// Encode (canonical)
err = c4m.NewEncoder(writer).Encode(m)

// Encode (pretty-printed)
err = c4m.NewEncoder(writer).SetPretty(true).Encode(m)

// Build programmatically
m := c4m.NewBuilder().
    AddFile("readme.txt", c4m.WithSize(100), c4m.WithMode(0644)).
    AddDir("src", c4m.WithMode(os.ModeDir|0755)).
        AddFile("main.go", c4m.WithSize(1024)).
    End().
    MustBuild()

// Compute deterministic C4 ID
id := m.ComputeC4ID()

// Diff two manifests
patch := c4m.PatchDiff(old, new)
```

## Documentation

- **[SPECIFICATION.md](SPECIFICATION.md)** — Format specification (user-facing)
- **[C4M-STANDARD.md](C4M-STANDARD.md)** — Formal standard with ABNF grammar
- **[WORKFLOWS.md](WORKFLOWS.md)** — Common workflows: patches, sequences, extraction
- **[METADATA_COVERAGE.md](METADATA_COVERAGE.md)** — What c4m captures, what it doesn't, and how to extend it

## License

Same as the C4 project — see [LICENSE](../LICENSE).
