# c4m Unix Recipes

A c4m file is plain text. Every line is a self-contained record with
predictable fields. This makes c4m files natural targets for `grep`,
`awk`, `sort`, `diff`, and the rest of the Unix toolkit.

## Field Reference

After stripping leading spaces (which encode nesting depth):

```
<mode> <timestamp> <size> <name> [<flow> <target>] <c4id>
```

| Field | Position | Example | Notes |
|-------|----------|---------|-------|
| Mode | 1 | `-rw-r--r--`, `drwxr-xr-x` | `d` = dir, `l` = symlink |
| Timestamp | 2 | `2026-03-17T06:18:00Z` | UTC, ISO 8601 |
| Size | 3 | `4404019`, `-` | Bytes, or `-` for null |
| Name | 4 | `photo.jpg`, `src/` | Dirs end with `/` |
| C4 ID | last | `c4abc...` (90 chars), `-` | Always starts with `c4` |

Depth = number of leading spaces. Each nesting level adds 2 spaces.

To get the trimmed line and depth in awk:
```bash
d=0; s=$0; while(substr(s,1,1)==" "){d++;s=substr(s,2)}
split(s,f," ")
# f[1]=mode, f[2]=timestamp, f[3]=size, f[4]=name
```

---

## Finding the Full Path of Any Entry

The name on each c4m line is just the basename. The full path comes from
the parent directories above it. To find the full path of any entry, walk
backwards from that line and collect the first directory at each shallower
indentation level:

```bash
# Get the full path for a given line number
c4m_path() {
  head -"$2" "$1" | tac | awk '
    NR==1 {d=0;s=$0;while(substr(s,1,1)==" "){d++;s=substr(s,2)}
           split(s,f," ");p=f[4];nd=d;next}
    {d=0;s=$0;while(substr(s,1,1)==" "){d++;s=substr(s,2)};split(s,f," ")
     if(f[4]~/\/$/ && d<nd){p=f[4] p;nd=d}}
    END{print p}'
}
```

Usage — find a file, then resolve its path:

```bash
$ grep -n 'main.go' project.c4m
47:    -rw-r--r-- 2026-03-17T06:18:00Z 1234 main.go c4abc...

$ c4m_path project.c4m 47
src/lib/main.go
```

This works because c4m nesting is strictly hierarchical — each entry's
parents are the nearest directory entries above it at shallower depths.
Walking backwards is a linear scan that stops as soon as you reach
depth 0.

### Find and resolve in one step

```bash
# Search for a file and show its full path
grep -n 'main.go' project.c4m | while IFS=: read n rest; do
  echo "$(c4m_path project.c4m $n)  (line $n)"
done
```

Output:
```
src/lib/main.go  (line 47)
tests/test_main.go  (line 203)
```

### Alternate approach: finding parents without awk

If you prefer to avoid awk, you can find the parent directories of any
line using just head, tac, grep, and sed. The idea: from the target
line's indentation, step backwards through decreasing indent levels.
At each level, `tac | grep -m1` finds the nearest directory above —
which is always the correct parent because c4m is ordered.

```bash
LINE=47
FILE=project.c4m
INDENT=$(sed -n "${LINE}p" "$FILE" | sed 's/[^ ].*//' | wc -c)
INDENT=$((INDENT-1))
while [ $INDENT -gt 0 ]; do
  INDENT=$((INDENT-2))
  head -$((LINE-1)) "$FILE" | tac | grep -m1 "^$(printf '%*s' $INDENT '')d"
done
```

This outputs the raw c4m directory lines, deepest parent first:

```
    drwxr-xr-x 2026-03-17T06:18:00Z - lib/ c4def...
  drwxr-xr-x 2026-03-17T06:18:00Z - src/ c4ghi...
drwxr-xr-x 2026-03-17T06:18:00Z - project/ c4jkl...
```

The indentation of the output itself shows the nesting. No parsing
needed — you can see the path is `project/src/lib/` just from reading
the directory names in the output.

---

## Discovery

### Find files by name

```bash
grep 'README.md' project.c4m
```

### Find files by extension

```bash
grep '\.exr ' project.c4m
grep '\.mov \|\.mp4 ' project.c4m
```

### Find all directories

```bash
grep '^[[:space:]]*d' project.c4m
```

### List directories with full paths and line numbers

Uses forward tracking (builds a directory stack as it reads):

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
... | grep textures
# 22 assets/textures/
```

### List all entries with full paths

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

### Find files larger than a threshold

```bash
# Files over 100MB
awk '{s=$0; gsub(/^ +/,"",s); split(s,f," ");
  if(f[3]+0 > 100000000) print}' project.c4m
```

### Find files modified after a date

Timestamps are ISO 8601, so string comparison works:

```bash
awk '{s=$0; gsub(/^ +/,"",s); split(s,f," ");
  if(f[2] > "2026-03-01") print}' project.c4m
```

### Count files by extension

```bash
awk '{s=$0; gsub(/^ +/,"",s); split(s,f," "); name=f[4]
  if(name !~ /\/$/) {
    n=split(name,p,"."); ext=(n>1)?p[n]:"none"
    count[ext]++
  }
} END {for(e in count) print count[e], e}' project.c4m | sort -rn
```

Output:
```
847 exr
234 jpg
45 mov
12 py
1 json
```

### Total size by top-level directory

```bash
awk '{
  d=0; s=$0; while(substr(s,1,1)==" "){d++;s=substr(s,2)}
  split(s,f," "); name=f[4]; size=f[3]+0
  for(i in stk) if(i+0>=d) delete stk[i]
  if(name~/\/$/) stk[d]=name
  if(d>=2 && !(name~/\/$/)) dir[stk[0]]+=size
  if(d<2 && !(name~/\/$/)) dir["(root)"]+=size
} END {for(d in dir) printf "%12d %s\n", dir[d], d}' project.c4m | sort -rn
```

---

## Finding Duplicates

Every file with the same content has the same C4 ID. This makes
duplicate detection trivial.

### Find duplicate content

```bash
# Extract C4 IDs, find duplicates
awk '{s=$0; gsub(/^ +/,"",s); n=split(s,f," ");
  id=f[n]; if(id!="-" && id~/^c4/) print id}' project.c4m \
  | sort | uniq -d
```

### Show all files sharing the same content

```bash
# For each duplicate ID, show all entries that have it
awk '{s=$0; gsub(/^ +/,"",s); n=split(s,f," ");
  id=f[n]; if(id!="-" && id~/^c4/) {count[id]++; lines[id]=lines[id] "\n  " $0}
} END {for(id in count) if(count[id]>1) print id ":" lines[id]}' project.c4m
```

### Total wasted space from duplicates

```bash
awk '{s=$0; gsub(/^ +/,"",s); n=split(s,f," ");
  id=f[n]; size=f[3]+0
  if(id!="-" && id~/^c4/) {count[id]++; sz[id]=size}
} END {waste=0; for(id in count) if(count[id]>1) waste+=sz[id]*(count[id]-1);
  printf "%d bytes wasted in duplicates\n", waste}' project.c4m
```

---

## Subtree Operations

### Extract a subtree

Find a directory and all its children (lines indented deeper):

```bash
# Extract the assets/ subtree
awk '{
  d=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")d++;else break}
  if(!cap && $0~/assets\//) {cap=1; cd=d; print; next}
  if(cap) {if(d>cd){print;next}; cap=0}
}' project.c4m
```

### Remove a subtree

```bash
# Remove node_modules/ and everything inside it
awk '{
  d=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")d++;else break}
  if(!skip && $0~/node_modules\//) {skip=1; sd=d; next}
  if(skip && d>sd) next
  skip=0; print
}' project.c4m > filtered.c4m
```

### Remove multiple directories

```bash
# Remove build/, dist/, and node_modules/
awk '{
  d=0; for(i=1;i<=length($0);i++){if(substr($0,i,1)==" ")d++;else break}
  if(!skip && ($0~/build\// || $0~/dist\// || $0~/node_modules\//)) {skip=1; sd=d; next}
  if(skip && d>sd) next
  skip=0; print
}' project.c4m > filtered.c4m
```

---

## Diff and Patch Workflows

### Snapshot and diff a directory over time

```bash
# Take a snapshot
c4 id ./project/ > project.c4m

# Later: diff against current state, append patch
c4 diff project.c4m <(c4 id ./project/) >> project.c4m

# View the change history
c4 log project.c4m
```

### See what changed between two snapshots

```bash
c4 diff monday.c4m friday.c4m
```

The output is a c4m patch: entries that were added, modified, or removed.
Pipe through `awk '{print $4}'` to see just the names.

### Apply diffs to reconstruct state

```bash
# Resolve the full patch chain to final state
c4 patch project.c4m

# Resolve up to patch N
c4 patch -n 3 project.c4m
```

### Reconcile a directory to match a target

```bash
# Apply target state, capturing the changeset
c4 patch target.c4m ./project/ > changeset.c4m

# Store pre-patch state for later reversal
c4 patch -s target.c4m ./project/ > changeset.c4m
```

### Revert to pre-patch state

If the forward patch was run with `-s`, the pre-patch manifest is in the
content store. Use `-r` to revert:

```bash
# Revert the directory to its pre-patch state
c4 patch -r changeset.c4m ./project/
```

### Preview without making changes

```bash
# Dry-run a reconciliation
c4 patch --dry-run target.c4m ./project/

# Preview what reverting would change (diff against stored pre-patch state)
c4 diff -r changeset.c4m ./project/
```

### Split history at a branch point

```bash
# Split after patch 3
c4 split project.c4m 3 common.c4m remainder.c4m
```

### Create a patch chain from separate files

Since patches are appendable, you can build a chain from separate diffs:

```bash
c4 id ./project/ > v1.c4m
# ... make changes ...
c4 diff v1.c4m <(c4 id ./project/) > patch1.c4m
# ... make more changes ...
c4 diff <(c4 patch v1.c4m patch1.c4m) <(c4 id ./project/) > patch2.c4m

# Combine into single file
cat v1.c4m patch1.c4m patch2.c4m > history.c4m
c4 log history.c4m
```

### Verify a patch chain

Each patch boundary line validates the state above it. If the chain
is corrupt, `c4 patch` will report a mismatch:

```bash
c4 patch project.c4m > /dev/null  # exits non-zero on mismatch
```

### Extract just the changes from a patch

```bash
# Show only added/modified files (patch entries that aren't removals)
c4 diff old.c4m new.c4m | grep -v '^c4'
```

---

## Comparing c4m Files

### Files present in A but not in B (by C4 ID)

```bash
# Extract IDs from each, find what's in A but not B
comm -23 \
  <(awk '{s=$0;gsub(/^ +/,"",s);n=split(s,f," ");if(f[n]~/^c4/)print f[n]}' a.c4m | sort) \
  <(awk '{s=$0;gsub(/^ +/,"",s);n=split(s,f," ");if(f[n]~/^c4/)print f[n]}' b.c4m | sort)
```

### Files with same name but different content

```bash
# Join on name field, show where IDs differ
join -j1 \
  <(awk '{s=$0;gsub(/^ +/,"",s);split(s,f," ");n=split(s,g," ");if(f[4]!~/\/$/)print f[4],g[n]}' a.c4m | sort) \
  <(awk '{s=$0;gsub(/^ +/,"",s);split(s,f," ");n=split(s,g," ");if(f[4]!~/\/$/)print f[4],g[n]}' b.c4m | sort) \
  | awk '$2!=$3 {print $1, $2, "->", $3}'
```

### Quick identity check

Two c4m files describe identical content if their C4 IDs match:

```bash
# Pipe each through c4 to get the tree ID, compare
A=$(c4 id a.c4m | c4)
B=$(c4 id b.c4m | c4)
[ "$A" = "$B" ] && echo "identical" || echo "different"
```

---

## Integration with Other Tools

### Verify files on disk match a c4m

```bash
# For each entry, compute the actual C4 ID and compare
awk '{
  d=0; s=$0; while(substr(s,1,1)==" "){d++;s=substr(s,2)}
  split(s,f," "); name=f[4]; n=split(s,g," "); id=g[n]
  for(i in stk) if(i+0>=d) delete stk[i]
  if(name~/\/$/) {stk[d]=name; next}
  path=""; for(i=0;i<d;i++) path=path stk[i]
  if(id~/^c4/) print id, path name
}' project.c4m | while read expected fpath; do
  actual=$(c4 id "$fpath" | awk '{s=$0;gsub(/^ +/,"",s);n=split(s,f," ");print f[n]}')
  [ "$expected" != "$actual" ] && echo "MISMATCH: $fpath"
done
```

### Feed paths to xargs

```bash
# Delete all .log files listed in a c4m
awk '{s=$0;gsub(/^ +/,"",s);split(s,f," ");
  if(f[4]~/.log$/) print f[4]}' project.c4m | xargs rm -f
```

### Pipe to sort for different orderings

```bash
# Sort by size (largest first)
awk '{s=$0;gsub(/^ +/,"",s);split(s,f," ");
  if(f[3]+0>0) printf "%12d %s\n",f[3],f[4]}' project.c4m | sort -rn

# Sort by timestamp (newest first)
awk '{s=$0;gsub(/^ +/,"",s);split(s,f," ");
  if(f[4]!~/\/$/) print f[2],f[4]}' project.c4m | sort -r
```

---

## Display

C4 IDs are 90 characters, which makes raw c4m lines wide. A few ways
to make them easier to read in a terminal.

### Browse with horizontal scrolling

```bash
c4 id ./project/ | less -S
```

`less -S` chops lines at terminal width and lets you scroll
left/right with the arrow keys.

### Crop to terminal width

```bash
c4 id ./project/ | cut -c1-$(tput cols)
```

### Replace C4 IDs with null

Replace the 90-character ID with `-` to see just the metadata.
Matches exactly 88 base58 characters after `c4`:

```bash
sed 's/ c4[1-9A-HJ-NP-Za-km-z]\{88\}$/ -/' project.c4m
```

### Truncate C4 IDs

Show just enough of the ID to be recognizable:

```bash
# First 12 characters + ellipsis
sed 's/ c4\([1-9A-HJ-NP-Za-km-z]\{10\}\)[1-9A-HJ-NP-Za-km-z]\{78\}$/ c4\1../' project.c4m
```

### Show just names

```bash
awk '{print $4}' project.c4m
```

Awk's field splitting ignores leading whitespace, so `$4` is the name
at any depth — no stripping needed. Similarly:

```bash
awk '{print $NF}' project.c4m  # C4 IDs (last field)
awk '{print $3}' project.c4m   # sizes
awk '{print $1}' project.c4m   # modes
awk '{print $4, $3}' project.c4m  # name and size
```

### Show names with sizes, human-readable

```bash
awk '$3+0>0 {
  s=$3
  if(s>1099511627776) printf "%6.1fT %s\n",s/1099511627776,$4
  else if(s>1073741824) printf "%6.1fG %s\n",s/1073741824,$4
  else if(s>1048576) printf "%6.1fM %s\n",s/1048576,$4
  else if(s>1024) printf "%6.1fK %s\n",s/1024,$4
  else printf "%6d  %s\n",s,$4
}' project.c4m
```

Output:
```
  4.2M render_001.exr
  4.2M render_002.exr
  1.3K config.yaml
   287  README.md
```
