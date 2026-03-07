# `c4` Command Line Interface

C4 is a command line tool for identifying files, directories, and piped data using C4 IDs (SMPTE ST 2114:2017). Output is c4m format — a human-readable, awk-friendly text format where every line is a self-contained record with permissions, timestamps, size, name, and C4 ID.

## Usage

```bash
# Identify a file (outputs a c4m entry)
$ c4 photo.jpg
-rw-r--r-- 2026-03-04T14:22:10Z 4404019 photo.jpg c4VxG8n...

# Identify a directory (full recursive c4m listing)
$ c4 myproject/

# Just the C4 ID
$ c4 -i myproject/

# Pretty-print with aligned columns
$ c4 -p myproject/

# Identify from stdin (bare C4 ID, no metadata)
$ echo "hello" | c4

# Copy content between locations
$ c4 cp ./src/ project.c4m:src/

# Compare two directories
$ c4 diff dir1/ dir2/
```

### Global Flags

| Flag | Long | Description |
|------|------|-------------|
| `-i` | `--id` | Output bare C4 ID(s) instead of c4m |
| `-p` | `--pretty` | Pretty-print (aligned columns, local time, comma sizes) |

See the [CLI Reference](../../docs/cli-reference.md) for the full command vocabulary.

