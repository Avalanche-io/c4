# C4 CLI Reference

Every verb's `--help` prints its full reference page — this document
is the tour. `c4 --help` prints the one-page contract, including the
scripting rules and exit codes.

## Command Philosophy

Nothing is written anywhere without `-s` (or `restore --force`), and
`-s` writes only inside the store.

**Observer commands** — read-only:

| Command        | Purpose                                                      |
|----------------|--------------------------------------------------------------|
| `c4 id`        | Describe files, directories, or c4m files                    |
| `c4 diff`      | Compare two states, produce a patch                          |
| `c4 patch`     | Resolve patch chains to c4m text (never touches directories) |
| `c4 log`       | List the store journal, or a chain's sections                |
| `c4 cat`       | Retrieve a store object by ID or ID/path, verified           |
| `c4 explain`   | Human-readable narration of what a command would do          |
| `c4 paths`     | Convert between c4m format and plain path lists              |
| `c4 intersect` | Find common entries between two c4m files                    |
| `c4 merge`     | Combine 2+ trees into one c4m (text out, nothing written)    |
| `c4 split`     | Split a patch chain (writes the two named output files)      |

**Writers** — exactly two ways bytes land anywhere:

| Command            | Purpose                                                  |
|--------------------|----------------------------------------------------------|
| `c4 id -s`         | Snapshot into the store; print the snapshot ID           |
| `c4 restore --force` | Make a directory match a description, undo-safely     |

`c4 version` prints version info.

The CLI has zero external dependencies. It requires Go 1.16+.

## Synopsis

```
c4 id [flags] <path>...              Describe files/directories/c4m files (c4m text to stdout)
c4 id -s <path>...                   Snapshot into the store; print one snapshot ID per path
c4 restore [--force] <target> <dir>  Make dir match a description (dry run by default)
c4 cat [-e] [-r] <id[/path]|f.c4m>   Retrieve a store object by ID or ID/path (verified)
c4 diff [-e] <old> <new>             Produce a c4m diff; sides: dirs, c4m files, or IDs
c4 patch [-n N] [-e] <chain.c4m>...  Resolve patch chains to c4m text
c4 log [<chain.c4m>...]              List a chain's sections (default: the store journal)
c4 merge <tree> <tree>...            Combine filesystem trees (c4m files or directories)
c4 split <file> <N> <before> <after> Split chain at section N
c4 explain <command> [args]          Human-readable command narration
c4 paths [<file.c4m> | -]            Convert between c4m and path lists
c4 intersect <id|path> <a> <b>       Find common entries between c4m files
c4 version                           Print version

c4 <path>                            Read-only shortcut for c4 id <path>
c4 -s <path>                         Snapshot shortcut for c4 id -s <path>
echo "data" | c4                     ID of stdin (nothing stored)
echo "data" | c4 -s                  Store stdin, print its ID
... | c4 id -                        Parse stdin as a c4m description
```

## Identity — two concepts

**Content ID** — the ID of bytes alone: a file's contents; a
directory's child listing with mode and timestamp nulled entirely
(the exec bit included). Equal exactly when contents are
byte-identical, on any machine, clock, or umask. This is the default
level for `c4 id`; get the bare ID with `-q`.

**Snapshot ID** — the ID of a full description: everything observed
at a moment (modes, times, sizes, names, content). Printed by
`c4 id -s`; the store alone returns all of it.

An ID never says which kind it is: compare like with like. A
store-resolved ID may take `/path` to descend by recorded entry name
(never following symlinks); store addresses are accepted by `c4 cat`,
restore targets, and diff sides.

## Scripting — the machine contract

stdout is data, byte-pure; stderr is narration. Parse nothing from
stderr, and never scrape an ID out of entry text or prose.

```bash
SNAP=$(c4 id -s dir/)              # THE snapshot ID: one line, nothing else
CID=$(c4 id -q dir/)               # THE content ID of a file or directory
c4 cat "$ID" >/dev/null            # in the store? exit 0 = present AND byte-verified
out=$(c4 restore --force "$T" d/)  # two lines: the undo handle, then the as-built ID
c4 log | awk '{print $NF}'         # every ID the journal lists, oldest first
c4 cat -r "$ID" | c4 id -q -       # recompute any claim instead of trusting it
```

Exit codes: 0 ok; 1 error — lines already printed remain true claims;
2 completed with declared partiality on stderr; 3 (restore only)
curable — cure, then re-run. In scripts, snapshot one path per
invocation: a multi-path call prints one line per path that
SUCCEEDED, so pairing lines to paths is guesswork unless exit is 0.

## `c4 id` — Describe and Snapshot

The core verb. Writes a plain-text c4m description of each path to
stdout. Without `-s`, id reads only; nothing is written anywhere.

```bash
# Directory → content-level c4m (machine-independent; the default)
c4 id myproject/

# Full observed detail: permissions, timestamps, sizes, names, IDs
c4 id -m f myproject/

# THE identity, one bare line (content ID)
c4 id -q myproject/

# Snapshot into the store; prints THE snapshot ID (implies -q)
SNAP=$(c4 id -s myproject/)

# c4m file → canonical form (normalizer); -q prints its own ID
c4 id project.c4m
c4 id -q project.c4m

# Ergonomic form (aligned columns, formatted sizes)
c4 id -e myproject/

# Stdin → bare C4 ID (nothing stored)
echo "hello" | c4

# Parse stdin as a c4m description
c4 cat -r "$SNAP" | c4 id -q -
```

### Snapshots are self-capturing

`c4 id -s` stores a complete snapshot of each path: every file's
bytes, every directory's one-level listing (root included), and one
journal entry per path. The snapshot ID prints only after everything
it stored is on stable media — a killed process (`kill -9` included),
a kernel panic, and power loss are one case. If it printed, you can
get it back:

```bash
SNAP=$(c4 id -s ./project/)
# ...lose the tree entirely...
c4 restore --force "$SNAP" ./project/     # rebuilt from the store alone
c4 cat -r "$SNAP" | c4 id -q -            # recompute: verifies the claim
```

`-s` writes only inside the store — no `.c4m` file, nothing in the
scanned tree or working directory. To keep a description file, use
the read-only form: `c4 id -m f ./project/ > project.c4m`.

### Flags

| Flag | Long             | Description                                                       |
|------|------------------|-------------------------------------------------------------------|
| `-q` | `--quiet`        | Bare ID only, one line per path (needs an ID-bearing level: c or f) |
| `-s` | `--store`        | Snapshot into the store; print the snapshot ID (implies `-q`; always full detail — conflicts with `-m`) |
| `-m` | `--mode`         | Scan detail: `s` structure, `c` content (default), `m` metadata, `f` full |
| `-c` | `--continue`     | Reuse guide: trust unchanged size+mtime from this description     |
|      | `--verify`       | Re-hash everything; with `-c`, report changes hidden under unchanged metadata |
| `-e` | `--ergonomic`    | Column-aligned output                                             |
| `-S` | `--sequence`     | Detect and fold file sequences                                    |
|      | `--exclude`      | Glob pattern to exclude (repeatable)                              |
|      | `--exclude-file` | File of exclude patterns (one per line)                           |

### Fast re-scans (`-c` and `--verify`)

A prior full description is a reuse guide. A file reuses the guide's
recorded ID — without being read — exactly when its path is in the
guide, its size and mtime match at second precision, and its mtime is
strictly older than the guide's scan start (git's racy-index rule; no
tunable window). Everything else re-hashes. The stderr summary counts
the split: `reuse: R reused, H rehashed`.

```bash
c4 id -m f ./project/ > full.c4m        # the guide
c4 id -c full.c4m -q ./project/         # fast re-scan
c4 id -s -c full.c4m ./project/         # fast re-snapshot into the store
```

The documented, accepted risk: a same-size byte change under a
deliberately restored older mtime is invisible to a `-c` re-scan.
Bytes already in the store are immune (every read re-verifies), and
`--verify` is the audit — it ignores every reuse rule, re-hashes
everything, and reports each file whose bytes changed under unchanged
size+mtime on stderr.

### Excluding Files

C4 scans everything by default — nothing is ignored implicitly, no
ignore file (`.gitignore` included) is ever read, and no environment
variable adds patterns. `--exclude GLOB` (repeatable) and
`--exclude-file FILE` are the only exclusion mechanism:

```bash
c4 id --exclude node_modules --exclude "*.tmp" ./project/
c4 id --exclude-file my-excludes.txt ./project/
```

Patterns are portable globs (`*`, `?`, `[...]`, no `**`); a pattern
containing `/` matches the slash-separated path relative to the scan
root, otherwise the bare name; a matched directory is pruned whole.
Exclusion is part of what an ID names — reproducing an ID needs the
same exclusions, supplied each run. Right after a snapshot,
`c4 diff <SNAP> <dir>` lists exactly what was left out.

### Partial scans

An unreadable entry is reported on stderr and recorded with null
fields for everything unread. The description is still produced; id
exits 2. Partial knowledge is a valid state, not an error.

## `c4 restore` — Make a Directory Match a Description

The one verb that writes to the filesystem. Dry run by default.

```bash
c4 restore "$SNAP" ./dir/                 # dry run: print the plan
out=$(c4 restore --force "$SNAP" ./dir/)  # apply; two bare-ID lines
```

The target is a store-resolved C4 ID (optionally with `/path`
descent — it must land on a directory), or a c4m file (chains
resolve). A snapshot-ID target restores recorded modes and times; a
content-ID target restores byte-exact content with platform-default
metadata.

With `--force`:

1. Every ID the target names must resolve from the store, else the
   missing IDs are listed and nothing is touched (exit 1).
2. The destination's pre-image is snapshotted — complete, durable,
   and journaled before the first destructive operation — and printed
   as stdout **line 1**: the undo handle.
3. The directory is reconciled: entries created, moved, updated,
   removed; content pulled by ID from the store and from bytes
   already present (moves detected).
4. The result is verified by recomputation and printed as **line 2**.
   Exit 0 exactly when line 2 equals the target ID.

Undo is line 1 fed straight back:

```bash
undo=$(printf '%s\n' "$out" | sed -n 1p)
c4 restore --force "$undo" ./dir/     # undo — prints its own undo handle
```

Handles are IDs, never refs: journaled permanently (`c4 log` is the
reflog), so a lost terminal loses nothing. No c4 verb deletes store
objects, so every undo handle keeps working for as long as the store
directory exists.

Exit codes: 0 verified; 1 refused before any modification; 2 declared
omissions (listed on stderr, line 2 printed and true); 3 curable
failure after line 1 (cure, then re-run — the undo stands).

## `c4 cat` — Retrieve Store Objects

```bash
c4 cat "$SNAP"                             # the root listing, one level
c4 cat -r "$SNAP"                          # the whole tree, expanded
c4 cat "$SNAP"/src/parser.go > parser.go   # extract by recorded name
c4 cat "$ID" >/dev/null && echo verified   # membership test, checked
```

Every read is verified: the bytes must hash to the requested ID or
cat writes nothing and exits 1 — exit 0 means present AND intact.
`<ID>/a/b` descends by recorded entry name, byte-exact, each level
rehash-verified, never following symlinks; a trailing slash asserts a
listing. A `.c4m` file path displays the file, chains resolved.

## `c4 diff` — Produce Patch

Compares two states — directories, c4m files, or store IDs — and
emits a c4m patch on stdout. Directories scan at content level, or at
the other side's level when that side is a description (using it as a
guide so only changed files rehash). Equal states emit nothing.

```bash
c4 diff before.c4m after.c4m > changes.c4m
c4 diff ./old-project/ ./new-project/
c4 diff project.c4m ./project/
c4 diff "$SNAP" ./project/                 # what changed since the snapshot
c4 diff project.c4m <(c4 id ./project/) >> project.c4m   # append for versioning
```

The only flag is `-e` (column-aligned output).

## `c4 patch` — Resolve Chains

Text algebra: c4m chains in, c4m text out. It never touches
directories — that is restore's job, behind restore's safety machine.

```bash
c4 patch project.c4m           # resolve the chain to its final state
c4 patch -n 3 project.c4m      # resolve to section 3
c4 patch common.c4m release.c4m   # multiple files concatenate into one chain
```

A directory argument exits 1 with:

```
patch composes descriptions; to change a directory: c4 restore <target> <dir>
```

Flags: `-n N` (1-based section; 0 = final state), `-e` (aligned output).

## `c4 merge` — Combine Trees

Combines two or more filesystem trees into one c4m on stdout. Inputs
can be c4m files, directories, or any combination.

```bash
c4 merge base.c4m overlay.c4m
c4 merge ./assets/ ./overrides/ ./extras/
```

Conflicts (same path, different content) are reported to stderr and
cause a non-zero exit.

## `c4 log` — The Journal, and Chain Sections

With no arguments, log lists the store journal `<store>/log.c4m` —
an ordinary c4m patch chain in which the store records every claim it
makes: each `c4 id -s` snapshot, each stdin blob, every pre-image
taken by `restore --force`. One line per claim, append order, oldest
first: the 1-based section index, a space, then the entry exactly as
recorded. The last field is always the ID:

```bash
c4 log | tail -1              # the latest claim
c4 log | awk '{print $NF}'    # the IDs — fields, never regex
```

Chain-file arguments list journal files the same way (a copied
store's journal included); other chains list with summary statistics:

```bash
$ c4 log project.c4m
1  c4abc...  (base)  1,234 files, 45 dirs
2  c4def...  +12 -3 ~5
```

Nothing expires: every journaled ID and everything it names stays
restorable for as long as the store directory exists. To shorten the
listing, `c4 split` the journal and install the kept part — splitting
changes what `c4 log` lists, never what the store retains.

## `c4 split` — Split Chain

Extracts a range from a patch chain into two files, enabling branching.

```bash
c4 split project.c4m 3 common.c4m remainder.c4m
c4 diff common.c4m <(c4 id ./release/) >> release.c4m
c4 diff common.c4m <(c4 id ./dev/) >> dev.c4m
```

## `c4 explain` — Human-readable Narration

A read-only command that describes what another command would do, in
plain language. Never modifies any files.

| Subcommand                            | What it describes                                     |
|---------------------------------------|-------------------------------------------------------|
| `c4 explain id <path>`                | What a directory or c4m file contains                 |
| `c4 explain diff <old> <new>`         | What changed between two states                       |
| `c4 explain patch <chain.c4m>...`     | What state a chain resolves to                        |
| `c4 explain restore <target> [<dest>]` | What a restore would change, and how to apply safely |

## `c4 paths` — Convert Between c4m and Path Lists

Bidirectional converter between c4m format and plain path lists.
Detects the input format automatically. Reads from a file argument or
stdin.

```bash
c4 paths project.c4m           # c4m → paths
find . -type f | c4 paths      # paths → c4m
c4 cat -r "$SNAP" | c4 paths   # every path in a snapshot
```

## `c4 intersect` — Find Common Entries

Finds entries that appear in both of two c4m files (or directories).
Output is a valid c4m from the second argument's perspective.

| Subcommand                  | Match criterion                                 |
|-----------------------------|-------------------------------------------------|
| `c4 intersect id <a> <b>`   | Content identity (same C4 ID, regardless of path) |
| `c4 intersect path <a> <b>` | Full path (same location in the tree)           |

```bash
c4 intersect id ./dir-a/ ./dir-b/
c4 intersect path monday.c4m friday.c4m
```

## Content Store

The content store holds objects addressed by C4 ID, plus one journal
file (`log.c4m`). Copy a store anywhere and every printed ID resolves
there identically; the history travels inside it. Configure via:

1. `C4_STORE` environment variable — a path, `s3://` URI, or comma-separated list
2. `~/.c4/config` file — one or more `store = ...` lines

Multiple stores can be configured. Writes go to the first store (it
holds the journal). Reads check all stores in order:

```bash
C4_STORE=/data/store
C4_STORE=/fast/ssd,s3://bucket/c4?region=us-west-2,/mnt/archive
```

On first use of `-s` without a configured store, the CLI offers to
create `~/.c4/store`.

### Durability — the print barrier

If a snapshot ID printed, the snapshot — content, listings, journal
entry — survives power loss, even one microsecond after printing.
The ordering is the contract on every platform: staged bytes reach
stable media before any object appears at its hash name; every object
a claim depends on is stable before its journal entry; the entry is
stable before the ID prints. If nothing printed, nothing was claimed;
re-running continues where it left off. There is no flag to skip or
strengthen this. No c4 verb deletes store objects.

## Scan Modes

The `-m` flag controls how much a description records:

| Flag         | Mode      | What it records                                        | Cost                    |
|--------------|-----------|--------------------------------------------------------|-------------------------|
| `-m s`       | Structure | Names and hierarchy only                               | Fast (readdir only)     |
| `-m m`       | Metadata  | + permissions, timestamps, sizes (no IDs)              | Fast (stat)             |
| `-m c`       | Content   | Sizes, names, IDs — mode and timestamp null            | Reads every byte        |
| `-m f`       | Full      | Everything observed: modes, times, sizes, names, IDs   | Reads every byte        |

Default is content (`-m c`): the machine-independent projection —
same bytes, same IDs, on any machine, clock, or umask. Use `-m f`
when observed metadata matters (it is what `-s` snapshots always
record). `-m` projects downward only: a description computes at its
own knowledge level.

## Working with c4m as Text

See [c4m Unix Recipes](./c4m-unix-recipes.md) for the full cookbook —
duplicate detection, subtree extraction, size analysis, verification, and more.

A c4m file is plain text with predictable fields. After stripping leading
spaces (which encode depth), each line has:

```
<mode> <timestamp> <size> <name> [<flow> <target>] <c4id>
```

- **Mode** (field 1): `d` prefix = directory, `-` = file or null (content level)
- **Timestamp** (field 2): ISO 8601 UTC, or `-` for null (content level)
- **Size** (field 3): byte count, or `-` for null
- **Name** (field 4): filename; directories always end with `/`
- **C4 ID** (last field): 90-char ID starting with `c4`, or `-` for null

Depth is the count of leading spaces (each level indented by 2).
Directories are identified by the trailing `/` on the name — that
holds at every level, content projection included.

### Finding entries

```bash
# Find a file by name
grep 'utils.go' project.c4m

# Find all directories (names end with /)
grep -E '(^| )[^ ]+/ ' project.c4m

# Find all EXR files
grep '\.exr ' project.c4m

# Find files larger than 1MB (size is the 3rd whitespace-separated field)
awk '{s=$0; gsub(/^ +/,"",s); split(s,f," "); if(f[3]+0 > 1000000) print}' project.c4m
```

### Listing directories with full paths

List all directories with their line numbers and reconstructed full paths:

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

### Removing entries

Simple grep works for **individual files** by name pattern:

```bash
# Remove all .tmp files (leaf entries, no children)
grep -v '\.tmp ' project.c4m > clean.c4m
```

**Removing directories** requires removing the directory line AND all
its children — the subsequent lines at deeper indentation, until you
reach a line at the same or shallower depth (indentation is
structural, not cosmetic). In an editor, select the directory line
and the indented block below it. For scripted removal:

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

**Exclude at scan time** (`--exclude`) when the excluded content is
large (node_modules, build artifacts) — skipping avoids hashing
gigabytes of unwanted data — or when the c4m must never contain
certain entries.

**Filter after scanning** when you want the full c4m as a record of
everything, then derive subsets; ad-hoc exploration; or building
different views from the same scan.

The c4m file is the truth. Filtering it is cheap. Scanning is where
the I/O cost lives — exclude there when it matters for performance.
