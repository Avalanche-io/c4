# `c4` Command Line Interface

C4 is a command line tool for identifying files, directories, and piped data using C4 IDs (SMPTE ST 2114:2017). Output is c4m format — a human-readable, awk-friendly text format where every line is a self-contained record with permissions, timestamps, size, name, and C4 ID.

## Commands

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

## Quick Examples

```bash
# Identify a file
$ c4 id photo.jpg
-rw-r--r-- 2026-03-04T14:22:10Z 4404019 photo.jpg c4VxG8n...

# Identify a directory
$ c4 id myproject/

# Identify from stdin (bare C4 ID)
$ echo "hello" | c4

# Store content while identifying
$ c4 id -s myproject/ > project.c4m

# Retrieve stored content
$ c4 cat c4VxG8n...

# Version a directory over time
$ c4 diff project.c4m <(c4 id myproject/) >> project.c4m
$ c4 log project.c4m
```

See the [CLI Reference](../../docs/cli-reference.md) for the full command set.
