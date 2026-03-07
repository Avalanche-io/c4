# c4 CLI v1.0 Design

> Everything in Unix is a file. Except for the filesystem. Until now.

## Principle

c4m is the universal output format. Every c4 command that describes
filesystem state speaks c4m. The colon is the portal between local paths,
c4m files, and remote locations. The command vocabulary is Unix filesystem
verbs — cp, mv, ls, ln, mkdir, rm, mk — applied uniformly to local files,
c4m contents, and remote locations via colon notation.

Canonical c4m is the default. Pretty is available via `--pretty` / `-p`.

The full vocabulary: 14 verbs (`c4`, `cat`, `ls`, `cp`, `mv`, `ln`,
`mkdir`, `mk`, `rm`, `diff`, `patch`, `undo`, `redo`, `unrm`),
1 operator (the colon), 1 format (c4m), 2 global flags (`-i`, `-p`).
Everything emerges from combining these primitives.

Nothing enters this CLI without a fight.

## c4m Format

### Entry Format

Every c4m entry is a single line with space-delimited fields:

```
mode timestamp size name c4id
```

Fields are always single-space delimited. Indentation (two spaces per
level) precedes the mode field for nested entries.

| Field | Format | Null |
|-------|--------|------|
| mode | 10-char Unix (`-rw-r--r--`, `drwxr-xr-x`) | `-` |
| timestamp | ISO 8601 UTC (`2026-03-04T14:22:10Z`) | `-` |
| size | bytes as integer | `-` |
| name | filename (quoted if spaces) | — |
| c4id | 90-char base58 starting with `c4` | `-` |

**The last field is always the C4 ID or `-`.** This means `$NF` in awk is
always the identity of the entry. No exceptions.

Examples:
```
-rw-r--r-- 2026-03-04T14:22:10Z 108 main.go c4abc...
drwxr-xr-x 2026-03-04T14:22:10Z - src/ c4xyz...
lrwxr-xr-x 2026-03-04T14:22:10Z - config.yaml -> ../shared/config.yaml -
```

Symlinks have `-> target` between the name and the C4 ID. The C4 ID
(or `-`) is still the last field.

**Line endings are LF on all platforms.** No CRLF, ever. c4m
normalizes to Unix line endings and Unix path separators regardless
of the host OS.

### All-Nil Entries

An entry with `-` for every field except the name is an all-nil entry:

```
- - - pattern -
```

All-nil entries represent exclusion patterns (see Ignore Patterns).
No real filesystem entry has nil mode AND nil timestamp AND nil size,
so all-nil entries are unambiguous.

### No Headers, No Directives

A c4m file is just entry lines. No `@c4m` header, no `@` directives.
The format is self-describing — the entry grammar (mode, ISO 8601
timestamp, C4 ID) is recognizable on sight.

This means:
- `cat a.c4m b.c4m` concatenates entries (needs `c4 sort` or re-scan for canonical order)
- `grep`, `awk`, `sort` work without filtering header lines
- Nesting is composition: indent a listing and prepend the directory entry

### Nesting

Two-space indent per depth level. Directory names end with `/`.
Concatenation builds paths naturally:

```
drwxr-xr-x 2026-03-04T14:22:10Z - src/ c4xyz...
  -rw-r--r-- 2026-03-04T14:22:10Z 108 main.go c4abc...
  drwxr-xr-x 2026-03-04T14:22:10Z - internal/ c4def...
    -rw-r--r-- 2026-03-04T14:22:11Z 42 parser.go c4ghi...
-rw-r--r-- 2026-03-04T14:22:11Z 42 README.md c4jkl...
```

Entries are sorted within each directory: files before directories,
then case-sensitive natural sort by name within each group. Natural
sort handles numeric sequences intelligently (`file2` before `file10`).
Sorted entries give binary search for path lookup without indexing.

**Natural sort algorithm:**

1. Split each name into alternating text and numeric segments
   (`render.` `12` `.exr` → text, numeric, text)
2. Compare corresponding segments left to right
3. Both text → byte comparison (case-sensitive, C locale)
4. Both numeric → compare by integer value; equal value → shorter
   representation first (`1` before `001`)
5. Text vs numeric → text sorts before numeric
6. All segments equal → fewer segments first

This is essentially `strverscmp` (glibc) without the confusing
fractional/integral quirk — the right algorithm for filenames.

A directory's C4 ID is the C4 ID of its children's canonical listing.
The format is self-referential: every directory entry's identity is
verifiable from its contents.

**Nil propagation:** If any child has a nil value for a field, the
parent directory's corresponding field is also nil. This is infectious
upward — the moment a nil is encountered during aggregation, stop
computing and propagate nil. This saves compute and maintains
consistency: a directory's timestamp is only meaningful if all
children have timestamps.

### Pretty Format

`--pretty` / `-p` enables human-friendly display:
- Aligned columns
- Sizes with commas (e.g., `43,783`)
- Timestamps in local time (e.g., `Mar 04 14:22:10 2026 CST`)
- C4 IDs right-aligned at column 80+

Canonical is the default because:
- Every field is positional — native to `awk`, `cut`, `sort`
- ISO 8601 sorts lexicographically = chronologically
- Sizes are integers — arithmetic works directly (`$3+0 > 1048576`)
- `c4 . > snapshot.c4m` is lossless (no timezone, no alignment padding)
- `diff <(c4 dirA/) <(c4 dirB/)` gives clean output

### Scan Everything

`c4 .` scans the entire directory — no filtering, no `.c4ignore` file,
no `.gitignore` integration by default. Unlike git, c4 has no large-file
or binary-file management issues, so the things that typically go into
`.gitignore` should not be ignored by c4.

Exclusions for unmanaged scans follow the rsync model: runtime flags
(`--exclude` on specific commands), not dotfiles. Users can also edit
the c4m output after the fact — the human-editable format IS the
escape valve. For managed directories, exclusions live at `:~.ignore`
(see Ignore Patterns).

### Hard Links

An entry with `->` after the name and NO link target path is a hard link
marker. It declares that this entry is write-bound with other entries
sharing the same C4 ID and the same link marker.

```
hardlink_marker ::= '->' group_number?
group_number    ::= [1-9][0-9]*
```

Rules:
1. `->` without a number: linked with all other `->` entries sharing the
   same C4 ID
2. `->N` with a number: linked only with other `->N` entries sharing the
   same C4 ID AND the same group number
3. No `->` marker: independent, even if C4 ID matches other entries

Distinction from symlinks — a symlink has a target path after `->`:
```
-rw-r--r-- ... 50M backup.exr -> c4abc...            (hard link)
-rw-r--r-- ... 50M backup.exr ->2 c4abc...           (hard link group 2)
lrwxr-xr-x ... - config.yaml -> ../shared/config.yaml -  (symlink)
```

### Sequence Identity

The C4 ID of each entry type points to fundamentally different content:

| Entry type | C4 ID points to |
|---|---|
| File | File content bytes |
| Directory | Canonical c4m listing of children (metadata + IDs, recursive) |
| Sequence (range) | Ordered list of frame C4 IDs (content, not c4m) |

A sequence range entry is file-like, not directory-like. Its C4 ID is
the C4 ID of the ordered frame-ID list — the bare C4 IDs concatenated
in range order with no delimiters, no newlines, no separators. Each
C4 ID is a fixed 90 characters, so the list is self-framing. This is
opaque content from c4m's perspective: the frame-ID list cannot be
inlined in the c4m stream. It is referenced content, fetchable from
storage like any other blob.

**Folding requires complete identity.** A range cannot be folded if
any frame has a nil C4 ID. The range's identity IS the ordered list
of frame identities — a gap breaks the chain. Unfolded entries with
nil IDs remain as individual lines.

**Verification:** To verify a range, fetch the frame-ID list content
by the range's C4 ID, then verify each individual frame against its
ID in the list. The range's C4 ID guarantees both completeness and
order of the sequence.

### Unix Tool Compatibility

Canonical c4m is awk-native. Every line is a self-contained record.

**Extract fields:**
```bash
c4 . | awk '{ print $NF }'             # all C4 IDs
c4 . | awk '{ print $4, $NF }'         # name + ID pairs
c4 . | awk '$3+0 > 1048576'            # files over 1MB
c4 . | awk '$2 > "2026-03-01"'         # modified after date
```

**Extract C4 IDs by regex** (self-delimiting, unique pattern):
```bash
grep -oE 'c4[1-9A-HJ-NP-Za-km-z]{88}' file.c4m
```

**Reconstruct full paths:**
```bash
c4 . | awk '{
  match($0, /^ */)
  d = RLENGTH / 2
  if ($1 ~ /^d/) dir[d] = $4
  p = ""; for (i = 0; i < d; i++) p = p dir[i]
  print p $4
}'
```

**10 most recently modified files with full paths:**
```bash
c4 . | awk '{
  match($0,/^ */); d=RLENGTH/2
  if($1~/^d/) dir[d]=$4
  p=""; for(i=0;i<d;i++) p=p dir[i]
  if($1!~/^d/) print $2, p $4
}' | sort -r | head -10
```

### Containers

tar and tar.gz (tgz) archives are first-class locations behind the
colon. The same verbs that work on c4m files, remote locations, and
managed directories work on archives:

```
c4 ls archive.tar:                     # list tar contents as c4m
c4 ls archive.tar:renders/             # list subtree inside tar
c4 ls archive.tar.gz:                  # compressed tar, same syntax
c4 cp ./files/ archive.tar:            # create tar (like tar cf)
c4 cp archive.tar: ./output/           # extract (like tar xf)
c4 cp archive.tar:renders/ studio:     # push tar contents to location
c4 diff archive.tar: .                 # verify extraction matches
c4 diff old.tar: new.tar:              # diff two archives
c4 cat archive.tar:src/main.go         # single file content to stdout
```

No new verbs. The colon is the portal — tar is just another container
it can address. Unlike `tar tf`, `c4 ls archive.tar:` gives you C4
IDs, so you can verify content without extracting.

Supported container formats: `.tar`, `.tar.gz` / `.tgz`,
`.tar.bz2`, `.tar.xz`, `.tar.zst`.

### Pipes and `-`

c4 commands consume c4m from stdin via `-`:

```bash
# Filter with Unix tools, then feed back into c4
c4 . | grep '\.go$' | c4 cp - project.tar:
c4 . | awk '$3+0 > 1048576' | c4 cp - big-files.tar:
c4 . | sort -k2 -r | head -20 | c4 cp - recent.tar:

# Chain c4 commands through pipes
c4 ls archive.tar: | c4 diff - .
c4 ls :~release-v1 | c4 diff - :
```

`-` always means c4m (metadata stream). For content bytes, use
`c4 cat`. This separation is clean — no flags, no modal behavior:

- **c4m flows through `-`** — metadata piped between c4 commands
  and Unix text tools (awk, grep, sort, head)
- **Content flows through `cat`** — bytes piped to non-c4 tools
  (diff, wc, less, hexdump)

```bash
# What changed since release, filtered to Go files, packed into tar
c4 diff :~release-v1 : | grep '\.go$' | c4 cp - patch.tar:

# Compare historical file against current without extracting
diff <(c4 cat :~release-v1/config.yaml) config.yaml

# Line count of a file inside a tar
c4 cat archive.tar:src/main.go | wc -l

# 5 largest files
c4 . | sort -k3 -rn | head -5
```

The principle: **c4 produces text, consumes text, and addresses
containers through `:`**. It transforms between the filesystem world
and the text world. Every c4 command is a filter in the Unix sense.

## Patch Format

The patch format extends c4m for progressive discovery and change sets.
It uses one structural element: a bare C4 ID on a line by itself.

### Pages

A bare C4 ID on its own line (90 chars, starts with `c4`, unmistakable)
acts as a page boundary. It is the C4 ID of the cumulative state up to
that point — a chain link and integrity check.

```
-rw-r--r-- 2026-03-04T14:22:10Z 108 main.go c4abc...
-rw-r--r-- 2026-03-04T14:22:11Z 42 util.go c4def...
c4page1...
-rw-r--r-- 2026-03-04T14:22:12Z 900 parser.go c4ghi...
drwxr-xr-x 2026-03-04T14:22:10Z - tests/ c4jkl...
  -rw-r--r-- 2026-03-04T14:22:13Z 200 main_test.go c4mno...
c4page2...
```

Page 1 has no prior — it is a base listing. Page 2 references `c4page1`
as its prior. The C4 ID of page 2 includes the prior reference, so the
chain is verifiable.

### Additions

Entries after a page boundary that do not appear in the accumulated
prior state are additions.

### Removals

An entry that is an exact duplicate of a line already in the accumulated
state is a removal. This is safe because:

1. **The chain enforces completeness.** If the prior C4 ID doesn't
   resolve, you know you're missing pages. You cannot apply a partial
   chain — the content addressing prevents it.
2. **True duplicates cannot occur naturally.** A modification changes at
   least the C4 ID (different content = different hash). Even touching a
   file changes its timestamp. An exact duplicate across pages means
   deliberate re-emission = removal.
3. **Same-path replacement (clobber).** A new entry with the same name
   at the same depth replaces the prior entry. Explicit re-emission of
   the old entry is only needed for pure deletion (removing without
   replacement).

### Modifications

A modification is never a true duplicate — at minimum the C4 ID changes.
Emit the new entry; it clobbers the old one at the same path. No need to
explicitly remove the old entry first.

```
c4page1...
drwxr-xr-x 2026-03-04T14:22:10Z - src/ c4srcnew...
  -rw-r--r-- 2026-03-05T10:00:00Z 55 util.go c4newutil...
c4page2...
```

`src/` gets a new C4 ID (a descendant changed), clobbers the old entry.
`util.go` gets a new C4 ID and timestamp, clobbers. Everything else in
the prior state is untouched.

### Nesting in Patches

After a page boundary, depth resets to zero — it is a fresh c4m context.
To add, modify, or remove a nested entry, re-introduce all directory
entries along the path to establish the proper depth:

```
c4page1...
drwxr-xr-x 2026-03-04T14:22:10Z - src/ c4srcnew...
  drwxr-xr-x 2026-03-04T14:22:10Z - cmd/ c4cmdnew...
    -rw-r--r-- 2026-03-05T10:00:00Z 120 main.go c4newmain...
c4page2...
```

Directory entries with new C4 IDs clobber their predecessors. Files
nested under them are additions or clobbers as normal.

### Target State Mode

`c4 patch` auto-detects its input. If the input contains bare C4 ID
page boundaries, it applies as a patch chain. If the input is plain c4m
(entry lines only, no page boundaries), it treats the input as a desired
end-state — internally diffs against the target's current state and
applies the delta.

```
c4 patch : changes.c4m      # apply a delta (patch chain)
c4 patch : desired.c4m      # converge to target state (plain c4m)
```

Same verb, same syntax, same undo semantics. The user doesn't need to
know which mode they're in.

### Diff Computation

`c4 diff` compares two c4m states and produces a patch page. The
algorithm exploits content-addressed directories: if two directories
share the same C4 ID, their entire subtree is identical and can be
skipped.

```
diff(old, new):
  for each name in union(old.children, new.children):
    if name only in new     -> emit entry (addition)
    if name only in old     -> re-emit entry (removal)
    if same name, same ID   -> skip entirely
    if same name, diff ID:
      if both directories   -> emit new dir entry, recurse
      if file               -> emit new entry (clobber)
```

For a million-file tree where one file changed, this touches only the
entries along the path from root to the changed file — typically single
digits of comparisons.

### Streaming

In a streaming context, entries arrive continuously. The bare C4 ID
serves as synchronization:

1. Accumulate entries
2. When a bare C4 ID arrives, verify it matches the hash of accumulated
   content
3. The verified page becomes the new prior state
4. Continue accumulating the next page

If the C4 ID doesn't match, the stream is corrupt or incomplete.

## Managed Directories

### The `:` Location

`:` (bare colon) refers to the current directory as a c4-managed
location. It is the "here" meta-location — the local filesystem seen
through c4's tracking layer.

The distinction: `./` is the raw filesystem. `:` is the managed view.
Operations through `:` are tracked, snapshotted, and undoable.
Without `:`  there is no mutation — c4 does not own unmanaged
filesystem operations. That's OS `rm` territory.

| | Raw (`./`, no colon) | Managed (`:`) |
|---|---|---|
| `c4 ls` | Live disk scan, everything | Tracked state (may lag reality) |
| `c4 rm` | Not a c4 operation | Tracked removal, snapshot, undoable |
| `c4 cp` | Additive copy, no tracking | Tracked copy, snapshot |
| `c4 diff` | Compare two sources | Compare managed vs anything |

```
c4 ls :                    # read managed state (works immediately)
c4 :                       # identify managed state (read-only)
c4 mk :                   # establish for tracking
c4 cp files/ :renders/     # tracked write
c4 patch : changes.c4m     # tracked, snapshotted
c4 rm :                    # stop tracking, clean up history
```

Read through `:` is always implicit — `c4 ls :` works without
establishment, reading directly from disk. Write through `:` requires
`c4 mk :`, same as any other location.

Note: `c4 ls :` reflects the last tracked state, which may be
incomplete if ingestion is still in progress. `c4 ls .` always scans
live disk. `c4 diff : .` shows what has changed on disk since the
last c4 operation — sync progress, not drift.

### Snapshots and Undo

`c4 mk :` captures the initial snapshot — the first moment c4 records
the full directory state. From that point, every mutating c4 operation
through `:` automatically snapshots the before-state.

The snapshot chain is a sequence of c4m states, each identified by its
C4 ID. This gives filesystem undo for free:

```
c4 patch : restructure.c4m      # auto-snapshots before applying
c4 undo :                        # reverts to prior snapshot
c4 redo :                        # re-applies the undone change
```

`c4 undo` is `c4 diff` (current vs prior snapshot) + `c4 patch` (apply
reverse). No new machinery — undo/redo is an emergent property of
content-addressed snapshots.

Storage cost is negligible: snapshots are c4m text (metadata, not file
content). A million-file tree snapshots in a few dozen megabytes.

### History Navigation

The `~N` suffix on `:` accesses the Nth ancestor in the snapshot chain:

```
c4 ls :           # current state
c4 ls :~1         # one operation ago
c4 ls :~5         # five operations ago
c4 diff :~1 :     # what changed in the last operation
c4 diff :~3 :     # cumulative changes over last 3 operations
c4 cp :~2 rollback.c4m:   # export a historical state
```

`c4 ls :~` (tilde, no number) lists the snapshots themselves:

```
$ c4 ls :~
drwxr-xr-x 2026-03-06T14:22:10Z - 0/ c4snap0...
drwxr-xr-x 2026-03-06T13:15:00Z - 1/ c4snap1...
drwxr-xr-x 2026-03-06T11:30:45Z - 2/ c4snap2...
drwxr-xr-x 2026-03-05T17:00:00Z - 3/ c4snap3...
```

Each snapshot is a directory entry (it's a version of `.`) with a C4 ID.
Numbered in reverse chronological order (0 = current).

### Tags

The snapshot history is itself c4m. Tags are created with `c4 ln`:

```
c4 ln :~2 :~release-v1
```

This names snapshot 2 as `release-v1` in the history. The tag is a
named entry pointing to the same C4 ID, surviving undo/redo because it
references a specific C4 ID, not a position.

```
c4 ls :~release-v1          # browse the tagged state
c4 diff :~release-v1 :      # what changed since release
c4 cp :~release-v1 archive:project/v1   # push tagged state
```

No tagging command. No bookmarking feature. `ln` in the history
namespace. Same verb, same semantics.

### Ignore Patterns

Ignore patterns control what c4 excludes from snapshots and tracking.
They are stored at `:~.ignore` — outside the managed filesystem, as
configuration of the observer, not content of the observed.

Ignore entries are all-nil c4m entries: every field is `-` except the
name, which is a filename or glob pattern. Nesting provides scoped
exclusions:

```
$ c4 ls :~.ignore
- - - data/ -
- - - *.tmp -
- - - *.sqlite -
- - - src/ -
  - - - *.test.js -
```

`data/` is excluded entirely. `*.tmp` and `*.sqlite` are excluded
everywhere. `*.test.js` is excluded only under `src/`. The tree
structure IS the scoping mechanism — same indentation rules as any
c4m listing.

**Adding and removing exclusions:**
```
c4 mk : --exclude data/ --exclude '*.tmp'  # at establishment
c4 mk : --exclude '*.sqlite'               # add to existing
c4 rm :~.ignore/data/                       # remove an exclusion
```

**Why outside the filesystem:**

Ignore patterns are a policy about what to track, not content being
tracked. Placing them at `:~.ignore` (in the history/config namespace)
rather than in the filesystem means:

- Undo/redo doesn't toggle exclusions on and off
- Adding an exclusion doesn't create a diff in history
- The policy applies uniformly across all snapshots
- Unlike `.gitignore`, which is a point-in-time file pretending to be a
  timeless policy, `:~.ignore` is genuinely timeless

**Retroactive application:** Adding an exclusion applies retroactively
to existing snapshots. If you exclude `data/` on snapshot 50, snapshots
1-49 also stop tracking `data/`. This prevents accidental undo of
untracked content (e.g., a database that was never safe to snapshot
outside its own transaction boundaries).

**GC integration:** When a newly added pattern matches content already
stored in c4d, that content moves to the reclaim queue for garbage
collection. The exclusion is both a tracking directive and a storage
cleanup signal.

### Preservation and Cleanup

History is implicit, preservation is explicit. Undo is a linear chain.
If you undo and then make a new change, the forward history detaches.

To retain a state before diverging:
```
c4 cp : before_refactor.c4m:    # save current state as c4m file
```

The c4m file IS the tag. It can be diffed, patched, shared, archived —
first-class citizen.

`c4 rm :` tears down tracking: stops snapshotting, cleans up history,
but leaves the filesystem untouched. Same as `c4 rm studio:` removes
the registration, not the data.

### LLM Workflow

c4m is a text representation of a filesystem — an LLM's native medium.
Instead of sequences of mkdir, mv, rm, cp tool calls, an LLM edits a
text document:

```bash
c4 . > current.c4m                            # snapshot
# LLM edits current.c4m -> desired.c4m        # text editing
c4 patch : desired.c4m                         # converge (tracked, undoable)
```

The entire restructuring is reviewable text before anything touches
disk. Rollback is `c4 undo :`. Context cost is one c4m document instead
of twenty tool-call round trips.

Empty files use the C4 ID of the empty string (same ID as an empty
directory — both are the hash of zero bytes of content). An LLM can
generate a project template as pure c4m text — `c4 patch : template.c4m`
creates the whole structure in one operation, including empty
placeholder files.

## Commands

### `c4` — Identify

Bare invocation computes a C4 ID or c4m entry.

**Stdin (no file metadata):**
```
echo -n hello | c4
c447Fm3BJZQ62765jMZJH4m28hrDM7Szbj9CUmj4F4gnvyDYXYz4WfnK2nYRhFvRgYEectEXYBYWLDpLo6XGNAfKdt
```

Stdin has no filename, no mode, no timestamps — there is no metadata
source. Output is a bare C4 ID.

**File (has metadata):**
```
$ c4 photo.jpg
-rw-r--r-- 2026-03-04T14:22:10Z 4404019 photo.jpg c4VxG8n...
```

A file has real metadata (mode, timestamp, size, name). Output is a
complete standalone c4m entry.

**Multiple files:**
```
$ c4 a.txt b.txt
-rw-r--r-- 2026-03-04T14:22:10Z 42 a.txt c4abc...
-rw-r--r-- 2026-03-04T14:22:11Z 108 b.txt c4def...
```

**Directory:**
```
$ c4 myproject/
drwxr-xr-x 2026-03-04T14:22:10Z - src/ c4xyz...
  -rw-r--r-- 2026-03-04T14:22:10Z 108 main.go c4abc...
-rw-r--r-- 2026-03-04T14:22:11Z 42 README.md c4def...
```

Full recursive c4m.

**ID-only flag:**
```
$ c4 -i photo.jpg
c4VxG8n...

$ c4 --id myproject/
c4xyz...
```

`-i` / `--id` suppresses c4m output and prints only the C4 ID. For
files, it's the content ID. For directories, it's the ID of the
canonical c4m (the identity of the described filesystem).

### `c4 ls` — List

Read the contents of any location via colon notation. Output is c4m.

```
c4 ls project.c4m:               # list c4m file root
c4 ls project.c4m:renders/       # list subtree
c4 ls studio:                    # list remote namespace
c4 ls studio:project/renders/    # list remote subtree
c4 ls :                          # list managed current directory
c4 ls :~1                        # list previous snapshot
c4 ls :~                         # list snapshot history
c4 ls :~release-v1               # list tagged snapshot
c4 ls :~.ignore                  # list ignore patterns
c4 ls archive.tar:               # list tar contents
c4 ls archive.tar.gz:renders/    # list subtree in compressed tar
```

`c4 ls` without a colon target is equivalent to `c4 .` — it describes
the current directory as c4m.

### `c4 cat` — Content

Output file content bytes to stdout. The one command that speaks bytes
instead of c4m.

```
c4 cat archive.tar:src/main.go          # file from tar to stdout
c4 cat project.c4m:README.md            # file from c4m to stdout
c4 cat :~release-v1/config.yaml         # historical version to stdout
c4 cat c4abc...                         # content by bare C4 ID
```

`cat` resolves the content through local disk, c4d storage, or the
container itself. A bare C4 ID as argument fetches directly from
storage — universal content retrieval by identity.

```bash
diff <(c4 cat :~1/config.yaml) config.yaml    # diff against history
c4 cat archive.tar:data.csv | wc -l           # line count, no extract
c4 cat c4abc... | sha512sum                    # verify independently
```

### `c4 cp` — Copy

The universal verb. Copies content between any combination of local
paths, c4m files, and remote locations. Always additive — never removes
content from the target.

```
c4 cp files/ project.c4m:             # capture local -> c4m
c4 cp files/ project.c4m:renders/     # capture into subtree
c4 cp project.c4m: ./output/          # materialize c4m -> local
c4 cp project.c4m:renders/ ./out/     # materialize subtree
c4 cp project.c4m: studio:            # push c4m -> location
c4 cp studio:renders/ ./local/        # pull location -> local
c4 cp files.c4m:/path/* bucket:/backups  # wildcards
c4 cp : backup.c4m:                   # export managed state
c4 cp :~release-v1 archive:project/   # push tagged snapshot
c4 cp ./files/ release.tar:           # create tar from local
c4 cp archive.tar: ./output/          # extract tar to local
c4 cp archive.tar:renders/ studio:    # tar contents to location
c4 cp - project.tar:                  # c4m from stdin selects files
```

Write targets require prior establishment (`c4 mk`). Read targets do
not. Scan is always recursive. `-r` is accepted and ignored for muscle
memory.

**Sequence copy convention:**
```
c4 cp frames.*.exr :frames.[].exr
```

`[]` in the target name is the collapse marker — it tells c4 to
collect matching source files into a sequence entry. The source glob
expands normally; the target's `[]` absorbs the varying part into
sequence notation.

**Range folding is intentionally lossy.** Individual per-frame metadata
differences are lost when compacted into a single range entry. The
folded and unfolded forms produce different C4 IDs because they are
genuinely different descriptions — the folded form's C4 ID is the ID
of the ordered frame-ID list (see Sequence Identity), while unfolded
entries each carry their own metadata. Hydrating a range back to
individual files and re-scanning produces an idempotent result.

### `c4 mv` — Move / Rename

Move or rename entries within or between c4m files and locations.

```
c4 mv project.c4m:old_name.txt project.c4m:new_name.txt
c4 mv project.c4m:renders/draft/ project.c4m:renders/final/
c4 mv studio:staging/ studio:live/
```

`mv` within a single c4m is a rename (changes the entry name, same
content ID). Between locations, it's copy + remove from source.

### `c4 ln` — Link

Create a hard link — two entries sharing the same C4 ID.

```
c4 ln project.c4m:master.exr project.c4m:backup.exr
c4 ln :~2 :~release-v1      # tag a snapshot
```

**Symlinks:**
```
c4 ln -s ../shared/config.yaml project.c4m:config.yaml
```

### `c4 mkdir` — Make Directory

Create a directory entry in a c4m file.

```
c4 mkdir project.c4m:renders/
c4 mkdir -p project.c4m:renders/shots/final/
```

`-p` creates intermediate directories. Write target must be established.

### `c4 mk` — Establish

Establish a c4m file, location, or managed directory for writing. The
safety gate that prevents accidental writes.

```
c4 mk project.c4m:                       # establish c4m file
c4 mk studio: cloud.example.com:7433     # establish remote location
c4 mk :                                  # establish current dir for tracking
c4 mk : --exclude data/ --exclude '*.tmp'  # establish with exclusions
```

Read is always implicit. Write requires `mk`.

`c4 mk :` captures the initial snapshot and begins tracking. This is the
entry point for undo/redo, history, and tags. On an already-established
directory, `c4 mk : --exclude pattern` adds exclusion patterns.

### `c4 rm` — Remove

Remove establishment or entries.

```
c4 rm studio:                   # remove location registration
c4 rm project.c4m:renders/old/  # remove entry from c4m file
c4 rm :                         # stop tracking, clean up history
c4 rm :~.ignore/data/           # remove an ignore pattern
```

c4m files themselves are removed with OS `rm` — they're just files.
`c4 rm :` removes tracking and history but leaves the filesystem intact.

### `c4 diff` — Diff

Compare two sources. Output is a c4m patch (entries + page boundary).

```
c4 diff monday.c4m: friday.c4m:
c4 diff project.c4m: ./actual/
c4 diff studio:project/ archive:project/
c4 diff :~1 :                    # what changed in last operation
c4 diff :~release-v1 :           # changes since tagged state
c4 diff : .                      # sync progress (managed vs reality)
```

The last form — `c4 diff : .` — compares the managed state (last
snapshot) against the live filesystem. This shows what has changed on
disk since the last c4 operation: files edited outside c4, new files
added by other tools, etc. It is not "what c4 missed" — it is
current sync progress.

Output is a patch page: the first operand's C4 ID as the prior, followed
by entries representing additions, clobbers, and removals. The page
boundary C4 ID at the end is the identity of the second operand's state.

### `c4 patch` — Patch

Apply a c4m patch or target state to a target.

```
c4 patch project.c4m: changes.c4m
c4 patch studio:project/ friday_delta.c4m
c4 patch : changes.c4m           # apply delta (tracked, undoable)
c4 patch : desired.c4m           # converge to target state
c4 patch : .                     # re-sync managed state to match disk
```

Auto-detects mode: if the input contains bare C4 ID page boundaries,
applies as a patch chain. If the input is plain c4m (entry lines only),
treats it as a desired end-state and internally diffs against the
target's current state.

When the target is `:`, the operation is tracked — before-state is
snapshotted, and `c4 undo :` can revert it.

When the target is a local filesystem path, c4 uses C4 IDs to resolve
operations: content already on disk is moved (same ID, new path), not
copied. No content creation from thin air — every file in the desired
state must exist locally or be available from c4d storage.

### `c4 undo` / `c4 redo` — Undo / Redo

Revert or re-apply the last operation on a managed directory.

```
c4 undo :                        # revert last operation
c4 redo :                        # re-apply undone operation
```

Undo is linear. If you undo and then make a new change, the forward
history detaches. To preserve a state before diverging, export it:
`c4 cp : save.c4m:`.

### `c4 unrm` — Selective Recovery

List or recover individual items removed from a managed directory.

```
c4 unrm :                       # list recoverable items by snapshot
c4 unrm :~1/draft.exr           # recover a specific item
```

Without arguments, lists items that exist in prior snapshots but not
in the current state, organized by snapshot. With a target path,
recovers that item into the current state.

`unrm` is selective — recover one file without reverting everything
else. `undo` reverts the whole last operation. `cp` from history
(e.g., `c4 cp :~3/file.txt :`) gives full precision when needed.

```
c4 unrm :                       # "what can I recover?"
c4 unrm :~1/draft.exr           # "bring this back"
c4 undo :                       # "revert everything from last op"
c4 cp :~3/old.txt :             # "grab this specific version"
```

### `c4 rm --shred` — Permanent Deletion

`c4 rm` on a managed directory is always soft — the item remains
recoverable from history. `--shred` is the nuclear option:

```
c4 rm --shred :secret.doc
```

This purges the content everywhere: removes from current state, scrubs
from all snapshots, deletes from c4d storage, and signals all mesh
nodes to purge their copies. The C4 ID becomes a tombstone.

### `c4 version` — Version

Show c4 version and all c4d nodes in the mesh.

```
$ c4 version
c4 1.0.0 (darwin/arm64)

  laptop                 c4d 1.0.0 (abc123) darwin/arm64
  cloud-vm               c4d 1.0.0 (abc123) linux/amd64
  render-farm            offline
```

Already implemented.

## Flags

Global flags that apply uniformly:

| Flag | Long | Meaning |
|------|------|---------|
| `-i` | `--id` | Output bare C4 ID(s) instead of c4m |
| `-p` | `--pretty` | Use pretty c4m format (aligned, local time, comma sizes) |

No other global flags. Command-specific flags (like `mkdir -p`) are
documented with their commands.

## Colon Notation

The colon is the portal.

```
PathSpec     ::= LocalPath | CapsulePath | LocationPath | HerePath

LocalPath    ::= path                    (no colon, or starts with ./ or /)
CapsulePath  ::= file.c4m ':' subpath?
LocationPath ::= name ':' subpath?
HerePath     ::= ':' modifier?

modifier     ::= subpath | '~' histref?
histref      ::= number | name
```

**Trailing colon = look inside.** Absence = treat as literal file.

```
project.c4m          -> the literal file
project.c4m:         -> the virtual filesystem it describes
project.c4m:renders/ -> a path inside that virtual filesystem
:                    -> the managed current directory
:~1                  -> one snapshot ago
:~release-v1         -> tagged snapshot
:~                   -> the snapshot history itself
:~.ignore            -> ignore patterns (tracking config)
```

## What's Out

Everything below is removed from v1.0. It may return if someone fights
for it, but it must earn its place by fitting the colon-notation
vocabulary.

- `-m`, `-mr`, `-v`, `-q`, `-a` flags
- `--progressive`, `--bundle`, `--resume`
- `--exclude`, `--exclude-from`, `-g` as global flags (gitignore-style
  filtering on every command). `--exclude` survives only on `c4 mk :`
  where it adds patterns to `:~.ignore`.
- `--depth`, `--follow`, `--no-ids`, `--format`
- `c4 fmt` (canonical is the default; `--pretty` covers the other case)
- `c4 validate` (may return — needs to fit the language)
- `c4 union`, `c4 intersect`, `c4 subtract` (diff+patch subsumes)
- `c4 extract` (c4 cp from a bundle replaces this)
- `@c4m`, `@intent`, `@base`, `@layer`, `@remove`, `@data` directives
  (removed from base format; patch format uses bare C4 IDs instead)

## Reserved for Future Consideration

These may enter the CLI if justified:

- `c4 cd` / `c4 pwd` — set/show working location context
- `c4 validate` — structural and content verification
- `c4 du` — storage visibility
- `c4 gc` — garbage collection
- Additional c4d control commands via `c4`
