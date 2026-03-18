# C4 CLI Reference

## Command Philosophy

C4 commands divide into two categories:

**Observer commands** — read-only by default, no side effects unless you opt in:

| Command | Purpose |
|---------|---------|
| `c4 id` | Identify files, directories, or c4m files |
| `c4 diff` | Compare two trees, produce a patch |
| `c4 log` | List patches in a chain |
| `c4 cat` | Retrieve content by C4 ID from store |

**Actor commands** — modify the filesystem or produce transformed output:

| Command | Purpose |
|---------|---------|
| `c4 patch` | Apply target state (reconcile, resolve, revert) |
| `c4 merge` | Combine 2+ trees into one c4m |
| `c4 split` | Split a patch chain for branching |

`c4 version` prints version info.

The CLI has zero external dependencies. It requires Go 1.16+.

## Synopsis

```
c4 id [flags] <path>...         Identify files, directories, or c4m files
c4 cat <c4id>                   Retrieve content by C4 ID from store
c4 diff <old> <new>             Produce c4m diff/patch (directories or c4m files)
c4 patch [flags] <target> [<dest>]
                                Apply target state (reconcile, resolve, revert)
c4 merge <tree> <tree>...       Combine filesystem trees (c4m files or directories)
c4 log <file.c4m>...            List patches in a chain
c4 split <file> <N> <before> <after>
                                Split chain at patch N
c4 version                      Print version

echo "data" | c4               C4 ID from stdin
```

## `c4 id` — Identify

The core observer command. Reads paths and outputs their C4 representation.
No side effects by default — content is never stored unless you pass `-s`.

```bash
# Single file → c4m entry (permissions, timestamp, size, name, C4 ID)
c4 id photo.jpg

# Directory → full recursive c4m
c4 id myproject/

# Multiple files → combined c4m
c4 id *.exr

# c4m file → canonical form (normalizer)
c4 id project.c4m

# Ergonomic form (aligned columns, formatted sizes)
c4 id -e myproject/

# Stdin → bare C4 ID (no metadata available)
echo "hello" | c4 id

# Store content while identifying (opt-in, zero extra I/O)
c4 id -s myproject/ > project.c4m

# Tree ID (pipe c4m through stdin)
c4 id . | c4
```

### Flags

| Flag | Long | Description |
|------|------|-------------|
| `-s` | `--store` | Store content in the configured store (opt-in) |
| `-e` | `--ergonomic` | Output ergonomic form c4m |
| `-S` | `--sequence` | Detect and fold file sequences |
| `-m` | `--mode` | Scan mode: `s`/`m`/`f` (see Scan Modes) |
| `-c` | `--continue` | Continue from existing c4m (use as guide) |
| | `--exclude` | Glob pattern to exclude (repeatable) |
| | `--exclude-file` | File of exclude patterns (one per line) |

### Excluding Files

C4 scans everything by default — unlike git, nothing is ignored implicitly.
Three mechanisms for exclusion:

```bash
# Inline patterns (repeatable)
c4 id --exclude node_modules --exclude "*.tmp" ./project/

# Explicit exclude file
c4 id --exclude-file my-excludes.txt ./project/

# Env var: auto-load a named file from scanned directories
C4_EXCLUDE_FILE=.c4ignore c4 id ./project/
```

The `C4_EXCLUDE_FILE` env var names a file to look for in each scanned
directory. C4 itself never assumes any exclude file exists — you choose
the name, you set the convention.

Exclude patterns are simple globs matched against both the filename and
the relative path from the scan root.

## `c4 cat` — Retrieve Content

Retrieves content by C4 ID from the configured content store.

```bash
c4 cat c43zYcLni5LF... > output.exr
```

## `c4 diff` — Produce Patch

Compares two filesystem trees and outputs a c4m patch. Arguments can be
c4m files, directories, or any combination. The patch format starts with
the C4 ID of the base state, followed by changed entries, followed by
the C4 ID of the new state. This output can be appended to the original
c4m file for versioning.

```bash
# Produce a patch (c4m files)
c4 diff before.c4m after.c4m > changes.c4m

# Diff two directories directly
c4 diff ./old-project/ ./new-project/

# Mix: c4m file vs live directory
c4 diff project.c4m ./project/

# Append a patch for versioning
c4 diff project.c4m <(c4 id ./project/) >> project.c4m
```

Empty diff produces no output.

### Flags

| Flag | Long | Description |
|------|------|-------------|
| `-r` | `--reverse` | With a changeset: diff against the pre-patch state from store. With two manifests/dirs: swap old and new. |
| `-s` | `--store` | Store content from directory arguments |
| `-e` | `--ergonomic` | Output ergonomic form |
| `-m` | `--mode` | Scan mode for directory arguments: `s`/`m`/`f` |

### Reverse diff with a changeset

When `-r` is used with a changeset as the first argument, `c4 diff`
loads the pre-patch manifest from the content store and diffs the
current directory against it. This lets you preview what a revert
would look like before running `c4 patch -r`:

```bash
# Preview what reverting would change
c4 diff -r changeset.c4m ./project/

# The changeset must have been produced with -s so the pre-patch
# manifest is in the store
```

## `c4 patch` — Apply Target State

The primary actor command. Applies a target state by resolving diffs,
reconciling filesystems, or reverting to a stored prior state. Outputs
the computed diff (changeset) to stdout when reconciling a directory.

### Argument Patterns

`c4 patch` dispatches based on what its arguments are:

| Target | Dest | What it does |
|--------|------|-------------|
| `file.c4m` | *(none)* | Resolve patch chain, output final c4m to stdout |
| `dir/` | *(none)* | Scan directory, store content, output c4m to stdout |
| `file.c4m` | `file.c4m` | Resolve chain, write result to dest c4m |
| `file.c4m` | `dir/` | Reconcile directory to match c4m target state |
| `dir/` | `file.c4m` | Scan directory, store content, write c4m |
| `dir/` | `dir/` | Reconcile dest directory to match source directory |
| `file.c4m...` | *(none)* | Multi-file chain resolution (3+ args) |

When reconciling a directory (`c4m×dir`, `dir×dir`), the computed diff
is written to stdout as a changeset. This changeset can be redirected
to a file and used later for reversal with `-r`.

### Flags

| Flag | Long | Description |
|------|------|-------------|
| `-s` | `--store` | Store pre-patch manifest + removed content (enables `-r` reversal) |
| `-r` | `--reverse` | Revert: restore directory to pre-patch state using stored manifest |
| `-e` | `--ergonomic` | Output ergonomic form |
| `-n` | `--number` | Resolve to specific patch number (1-based) |
| `-m` | `--mode` | Scan mode for directory arguments: `s`/`m`/`f` |
| | `--dry-run` | Show planned operations without making changes |
| | `--no-store` | Suppress content storage |
| | `--source` | Additional content source path (repeatable) |

### Examples

```bash
# Resolve a patch chain to final state
c4 patch project.c4m

# Resolve to specific patch number
c4 patch -n 3 project.c4m

# Resolve across files (branching)
c4 patch common.c4m release.c4m

# Reconcile a directory to match a c4m, capture changeset
c4 patch target.c4m ./project/ > changeset.c4m

# Same, but store pre-patch state for later reversal
c4 patch -s target.c4m ./project/ > changeset.c4m

# Revert using the stored pre-patch state
c4 patch -r changeset.c4m ./project/

# Preview reconciliation without making changes
c4 patch --dry-run target.c4m ./project/

# Reconcile with content from an additional source directory
c4 patch --source /mnt/backup/ target.c4m ./project/

# Sync one directory to match another
c4 patch ./source/ ./dest/

# Scan a directory, store content, write c4m
c4 patch ./project/ output.c4m
```

### Reconciliation

When patching a directory, `c4 patch` computes the diff between the
directory's current state and the target, then applies operations
(create, move, remove, chmod, chtimes) to make the directory match.

If content needed by the target is missing, the command reports the
missing C4 IDs and exits non-zero. Use `--source` to provide additional
directories where content can be found, or ensure the content store has
the needed files.

The `-s` flag stores the pre-patch manifest in the content store,
keyed by its C4 ID. This is what enables `-r` reversal — the stored
manifest is the revert target.

## `c4 merge` — Combine Trees

Combines two or more filesystem trees into one. Inputs can be c4m files,
directories, or any combination. Requires at least 2 arguments. Outputs
a merged c4m to stdout.

```bash
# Merge two c4m files
c4 merge base.c4m overlay.c4m

# Merge a c4m file with a live directory
c4 merge base.c4m ./additional-files/

# Merge multiple directories
c4 merge ./assets/ ./overrides/ ./extras/
```

### Flags

| Flag | Long | Description |
|------|------|-------------|
| `-e` | `--ergonomic` | Output ergonomic form |
| `-m` | `--mode` | Scan mode for directory arguments: `s`/`m`/`f` |

Conflicts (same path, different content in both inputs) are reported to
stderr and cause a non-zero exit.

## `c4 log` — List Patches

Enumerates the patches in a c4m chain with summary statistics.

```bash
$ c4 log project.c4m
1  c4abc...  (base)  1,234 files, 45 dirs
2  c4def...  +12 -3 ~5
3  c4ghi...  +2 -0 ~1
```

## `c4 split` — Split Chain

Extracts a range from a patch chain into two files, enabling branching.

```bash
# Split at patch 3
c4 split project.c4m 3 common.c4m remainder.c4m

# Branch from common point
c4 diff common.c4m <(c4 id ./release/) >> release.c4m
c4 diff common.c4m <(c4 id ./dev/) >> dev.c4m
```

## `c4 version`

```bash
$ c4 version
c4 1.0.0 (darwin/arm64, go1.24.5)
```

## Content Store

The content store holds file content addressed by C4 ID. Configure via:

1. `C4_STORE` environment variable (path or `s3://bucket/prefix`)
2. `~/.c4/config` file (`store = /path/to/store`)

On first use of `-s` without a configured store, the CLI offers to
create `~/.c4/store`.

## Stdin Shortcut

Piping content to bare `c4` (no subcommand) outputs the C4 ID:

```bash
echo "hello" | c4
tar cf - ./dir | c4
```

## Scan Modes

The `-m` flag controls how much work `c4 id` does:

| Flag | Mode | What it does | Cost |
|------|------|-------------|------|
| `-m s` or `-m 1` | Structure | Names and hierarchy only | Instant (readdir) |
| `-m m` or `-m 2` | Metadata | + permissions, timestamps, sizes | Fast (stat) |
| `-m f` or `-m 3` | Full | + C4 IDs | Expensive (read every byte) |

Default is full (`-m f`). Lower modes are useful for previewing what
a scan will cover before committing to the expensive hashing phase.

## Continue from Existing c4m

The `-c` flag takes an existing c4m file as a guide. Only entries present
in the guide are processed — everything else is skipped. This enables a
powerful scan-filter-continue workflow:

```bash
# 1. Instant structure scan — see what's there
c4 id -m s ./project/ > project.c4m

# 2. Edit the c4m — remove what you don't want
#    To remove a directory, delete the directory line AND all indented
#    lines immediately below it (its children). In c4m, depth is
#    structural — children are indented deeper than their parent.
vi project.c4m

# 3. Continue — upgrade to full IDs, only for what survived the edit
c4 id -m f -c project.c4m ./project/
```

This avoids hashing anything you filtered out. The I/O cost of step 3
is proportional to what you kept, not what exists on disk.

### Upgrading modes

Continue works across mode transitions:

```bash
# Structure → metadata (add sizes without hashing)
c4 id -m m -c structure.c4m ./project/ > metadata.c4m

# Metadata → full (add C4 IDs)
c4 id -m f -c metadata.c4m ./project/ > full.c4m
```

## Working with c4m as Text

See [c4m Unix Recipes](./c4m-unix-recipes.md) for the full cookbook —
duplicate detection, subtree extraction, size analysis, verification, and more.

A c4m file is plain text with predictable fields. After stripping leading
spaces (which encode depth), each line has:

```
<mode> <timestamp> <size> <name> [<flow> <target>] <c4id>
```

- **Mode** (field 1): `d` prefix = directory, `-` = file, `l` = symlink
- **Timestamp** (field 2): ISO 8601 UTC
- **Size** (field 3): byte count, or `-` for null
- **Name** (field 4): filename; directories always end with `/`
- **C4 ID** (last field): 90-char ID starting with `c4`, or `-` for null

Depth is the count of leading spaces (each level indented by 2).

### Finding entries

```bash
# Find a file by name
grep 'utils.go' project.c4m

# Find all directories (mode starts with d)
grep '^[[:space:]]*d' project.c4m

# Find all EXR files
grep '\.exr ' project.c4m

# Find files larger than 1MB (size is the 3rd whitespace-separated field)
awk '{s=$0; gsub(/^ +/,"",s); split(s,f," "); if(f[3]+0 > 1000000) print}' project.c4m
```

### Listing directories with full paths

List all directories with their line numbers and reconstructed full paths.
Useful for finding where a directory lives, then using the line number
to extract or remove its subtree:

```bash
awk '{
  d=0; s=$0; while(substr(s,1,1)==" "){d++;s=substr(s,2)}
  split(s,f," "); name=f[4]
  for(i in stk) if(i+0>=d) delete stk[i]
  if(name~/\/$/) {
    stk[d]=name
    path=""; for(i=0;i<d;i++) path=path stk[i]
    print NR, path name
  }
}' project.c4m
```

Output:
```
3 src/
7 src/lib/
15 assets/
22 assets/textures/
```

Pipe through grep to find a specific directory:

```bash
# Find the line number of the textures directory
... | grep textures
# 22 assets/textures/
```

### Reconstructing full paths for all entries

The name on each line is just the basename. To get full paths for every
entry (files and directories), track the directory stack using depth:

```bash
awk '{
  d=0; s=$0; while(substr(s,1,1)==" "){d++;s=substr(s,2)}
  split(s,f," "); name=f[4]
  for(i in stk) if(i+0>=d) delete stk[i]
  if(name~/\/$/) stk[d]=name
  path=""; for(i=0;i<d;i++) path=path stk[i]
  print path name
}' project.c4m
```

### Finding a directory and all its children

A directory entry is followed by its children at deeper indentation.
To extract a subtree, find the directory line and capture everything
indented deeper until you reach the same or shallower depth:

```bash
# Extract the src/ subtree (directory + all contents)
awk '{
  d=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")d++;else break}
  if(!skip && $0 ~ /src\//) {skip=1; sd=d; print; next}
  if(skip) {if(d>sd){print;next}; skip=0}
  if(!skip) print
}' project.c4m

# Remove the node_modules/ subtree
awk '{
  d=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")d++;else break}
  if(!skip && $0 ~ /node_modules\//) {skip=1; sd=d; next}
  if(skip && d>sd) next
  skip=0; print
}' project.c4m > filtered.c4m
```

### Removing entries

Simple grep works for **individual files** by name pattern:

```bash
# Remove all .tmp files (leaf entries, no children)
grep -v '\.tmp ' project.c4m > clean.c4m
```

**Removing directories** requires removing the directory line AND all its
children — the subsequent lines at deeper indentation, until you reach a
line at the same or shallower depth. c4m uses indentation to show nesting
(leading spaces are trimmed when computing the C4 ID, so indentation is
structural, not cosmetic).

In an editor, this is straightforward — select the directory line and all
indented lines below it, delete the block:

```bash
# Structure scan — indentation shows the tree
c4 id -m s ./project/ > project.c4m

# In your editor, the tree structure is visible:
#   file.txt
#   node_modules/
#     express/
#       index.js
#       ...
#     .package-lock.json
#   src/
#     main.go
#
# Delete the node_modules/ line and everything indented under it.
vi project.c4m

# Continue — only hash what's left
c4 id -c project.c4m ./project/
```

For scripted directory removal, count leading spaces to track depth:

```bash
# Remove node_modules/ and everything inside it
awk 'BEGIN{skip=0; sd=-1}
  /node_modules\// {skip=1; sd=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")sd++;else break}; next}
  skip {d=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")d++;else break}; if(d>sd){next}; skip=0}
  {print}' project.c4m > filtered.c4m
```

Or use `--exclude` at scan time, which is simpler for known patterns:

```bash
c4 id --exclude node_modules ./project/
```

### Comparing with diff

```bash
# Text diff between two c4m snapshots (line-level changes)
diff project-v1.c4m project-v2.c4m

# Semantic diff (understands c4m structure, produces a patch)
c4 diff project-v1.c4m project-v2.c4m
```

### When to exclude at scan time vs filter after

**Exclude at scan time** (`--exclude`, or scan-filter-continue) when:
- The excluded content is large (node_modules, build artifacts) — skipping
  avoids hashing gigabytes of unwanted data
- You want the c4m to never contain certain entries

**Filter after scanning** when:
- You want the full c4m as a record of everything, then derive subsets
- You're doing ad-hoc exploration ("what EXR files are in this project?")
- You're building different views from the same scan

The c4m file is the truth. Filtering it is cheap. Scanning is where the
I/O cost lives — exclude there when it matters for performance.
