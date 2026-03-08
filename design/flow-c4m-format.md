# Flow Links in the c4m Format

Design analysis for integrating flow link syntax into the c4m parser,
encoder, and Entry struct. Based on a thorough reading of the c4m package
code as of 2026-03-08.

---

## 1. Current Parsing Architecture

The entry format is:

```
[indent] MODE TIMESTAMP SIZE NAME [LINK_MARKER] [C4ID]
```

The parser (`decoder.go:parseEntryFields`) processes fields left to right:

1. **Size** -- digits/commas or `-`
2. **Name** -- quoted or unquoted, terminated by boundary detection
3. **Link marker** -- optional `->` with group number or target
4. **C4 ID** -- `c4...` or `-`, always last

The `->` marker is parsed at line 269 of `decoder.go`:

```go
if pos+1 < n && line[pos] == '-' && line[pos+1] == '>' {
```

After detecting `->`, the parser distinguishes three cases:
- `->N` (no space, digit 1-9): hard link group number
- `-> c4...` or `-> -`: ungrouped hard link (HardLink = -1)
- `-> anything_else`: symlink target (parsed via `parseTarget`)

Name boundary detection in `parseNameOrTarget` (line 410) specifically
looks for ` -> ` (space-arrow-greater-space) to terminate unquoted names.

---

## 2. Parsing Rules for Flow Operators

### 2a. The Three Operators

| Operator | Meaning | Syntax in line |
|----------|---------|----------------|
| `->` | outbound flow | `NAME -> location:path` |
| `<-` | inbound flow | `NAME <- location:path` |
| `<>` | bidirectional | `NAME <> location:path` |

### 2b. Disambiguation from Existing `->` Uses

The `->` operator already serves three roles (symlink, ungrouped hard
link, grouped hard link). Adding outbound flow as a fourth use of `->` is
possible because flow targets are syntactically distinct:

**A flow target always contains `:`** -- the location separator. Neither
symlink targets (paths) nor hard link markers (c4 IDs or `-`) contain
a colon in their parsed form. This is the disambiguation rule:

| After `->` | Contains `:` | Interpretation |
|-------------|-------------|----------------|
| `footage:` | yes | outbound flow to `footage` root |
| `backup:raw/plates/` | yes | outbound flow to path in `backup` |
| `../other/file` | no | symlink target |
| `c41j3C6J...` | no | hard link (ungrouped) |
| `-` | no | hard link (ungrouped, null ID) |
| `3` (no space) | n/a | hard link group 3 |

The `<-` and `<>` operators are entirely new -- they have no existing
meaning. No ambiguity arises.

### 2c. Parser Changes

The current `->` detection point (decoder.go line 269) becomes a
three-way check:

```
After name, skip whitespace, then check:
  "->" : existing logic, plus flow detection (if target contains ":")
  "<-" : new, always a flow link
  "<>" : new, always a flow link
```

For `<-`, the parser must check `line[pos] == '<' && line[pos+1] == '-'`.
For `<>`, the parser must check `line[pos] == '<' && line[pos+1] == '>'`.

After detecting any flow operator, the parser reads the flow target
(everything up to the next space + c4 prefix, or end of line). The flow
target is always a location reference containing `:`.

### 2d. Name Boundary Detection

The `parseNameOrTarget` function (line 407-419) detects name boundaries
by looking for ` -> ` in the remaining line. This must be extended to
also recognize ` <- ` and ` <> ` as boundaries:

```go
// Current:
if strings.HasPrefix(rest, " -> ") {
    return ...
}

// Extended:
if strings.HasPrefix(rest, " -> ") ||
   strings.HasPrefix(rest, " <- ") ||
   strings.HasPrefix(rest, " <> ") {
    return ...
}
```

### 2e. Field Order

Flow links occupy the same position as existing link markers -- between
name and C4 ID:

```
MODE TIMESTAMP SIZE NAME FLOW_OP LOCATION [C4ID]
```

The C4 ID remains the last field. A flow-linked entry can have a C4 ID
(the content's identity) alongside the flow declaration (where it goes).

---

## 3. Entry Struct Changes

### 3a. New Fields

```go
type FlowDirection int

const (
    FlowNone        FlowDirection = iota
    FlowOutbound                         // ->
    FlowInbound                          // <-
    FlowBidirectional                    // <>
)

type Entry struct {
    // ... existing fields ...

    // Flow link: declares a cross-location data relationship.
    // FlowDirection is FlowNone for entries without flow links.
    FlowDirection FlowDirection
    FlowTarget    string        // Location reference (e.g., "footage:", "nas:raw/")
}
```

### 3b. Why Separate Fields (Not Overloading Target)

The existing `Target` field holds symlink targets. It would be tempting
to reuse it for flow targets, but:

- A symlink and a flow link are different concepts. A symlink is a kernel
  construct; a flow link is an infrastructure declaration.
- An entry could theoretically be a symlink *to* a local path while also
  having a flow declaration (see Section 7). Separate fields keep this
  clean.
- `FlowDirection` needs its own field regardless -- it cannot be inferred
  from the target string alone.

### 3c. Helper Methods

```go
func (e *Entry) IsFlowLinked() bool {
    return e.FlowDirection != FlowNone
}

func (e *Entry) FlowOperator() string {
    switch e.FlowDirection {
    case FlowOutbound:
        return "->"
    case FlowInbound:
        return "<-"
    case FlowBidirectional:
        return "<>"
    default:
        return ""
    }
}
```

---

## 4. Ambiguity Analysis

### 4a. `<-` in Filenames

Can a filename contain the literal characters `<-`? Yes -- filenames can
contain `<` and `-`. However, the parser only recognizes ` <- ` (with
surrounding spaces) as a flow operator, because name boundary detection
requires a space before the operator.

A file literally named `a<-b` parses correctly:
```
-rw-r--r-- 2024-01-01T00:00:00Z 100 a<-b -
```
The `<-` here is not preceded by a space, so it is part of the name.

A file named `a <- b` would be ambiguous in unquoted form. But the
existing parser already requires quoting for names containing spaces:
```
-rw-r--r-- 2024-01-01T00:00:00Z 100 "a <- b" -
```
Quoted names are parsed character by character with escape handling --
the `<-` inside quotes is never interpreted as an operator.

### 4b. `<>` in Filenames

Same analysis as `<-`. The characters `<` and `>` are legal in Unix
filenames (though forbidden on Windows). A filename containing `<>` only
conflicts if surrounded by spaces, which forces quoting.

### 4c. `:` in Filenames and Flow Targets

The colon in the flow target is the key disambiguator. This raises the
question: what if a symlink target path contains `:`?

On Unix, `:` is legal in filenames. A symlink target like
`/mnt/nas:vol1/file` contains `:` but is NOT a flow target. The
disambiguation rule is:

**Flow targets are only expected after the new `<-` and `<>` operators.**
For the existing `->` operator, the parser must distinguish:
- Symlink target: a path (may contain `:`)
- Hard link: starts with `c4` or is `-`
- Flow target: a location reference (contains `:`)

The critical case is `-> path:with:colons` vs `-> location:path`. The
resolution: **a flow location name cannot contain `/` or `.`**, while a
filesystem path almost always does before any `:`. The location syntax is
`LABEL:` or `LABEL:subpath/`, where LABEL is a simple identifier
(alphanumeric plus `-` and `_`). If the token after `->` matches
`[a-zA-Z0-9_-]+:.*`, it is a flow target. If it starts with `/`, `.`,
or `..`, it is a symlink target.

In practice, symlink targets containing `:` are rare on Unix and
impossible on Windows. A symlink to `host:port/path` is not a valid
symlink -- that is URL syntax, not filesystem syntax.

For robustness, the parser should use this precedence for `->`:
1. If immediately followed by digit (no space): hard link group
2. If next token is `c4...` or `-`: hard link
3. If next token matches location pattern (`WORD:`): flow target
4. Otherwise: symlink target

### 4d. Quoted Names with Flow Operators Inside

```
-rw-r--r-- 2024-01-01T00:00:00Z 100 "report -> draft" <- edits: -
```

The quoted name `"report -> draft"` is parsed correctly -- the `->` inside
quotes is never reached by boundary detection. The `<-` after the closing
quote is correctly parsed as a flow operator.

---

## 5. Encoder Output

### 5a. Inline with Entry

Flow links appear inline, in the same position as symlink targets and
hard link markers. This is consistent with how all link types are
currently formatted.

```
MODE TIMESTAMP SIZE NAME FLOW_OP FLOW_TARGET [C4ID]
```

### 5b. Format and Canonical Methods

In `entry.go`, the `Format()` and `Canonical()` methods currently handle
symlinks and hard links in the same block (lines 91-99, 139-147). The
flow link case is added as a third branch:

```go
// Add symlink target, hard link marker, or flow link
if e.Target != "" {
    parts = append(parts, "->", formatTarget(e.Target))
} else if e.HardLink != 0 {
    if e.HardLink < 0 {
        parts = append(parts, "->")
    } else {
        parts = append(parts, fmt.Sprintf("->%d", e.HardLink))
    }
} else if e.FlowDirection != FlowNone {
    parts = append(parts, e.FlowOperator(), e.FlowTarget)
}
```

### 5c. Flow Target Formatting

Flow targets follow the same quoting rules as symlink targets -- if the
location path portion contains spaces (unlikely but possible), it gets
quoted. The location label itself should be restricted to
`[a-zA-Z0-9_-]+`.

### 5d. Pretty Printing

In `encoder.go`, the `formatEntryPretty` method adds the same flow link
handling. The flow operator and target contribute to line length for
column alignment calculation.

### 5e. Examples

Canonical form:
```
-rw-r--r-- 2024-01-01T00:00:00Z 1048576 plate_001.exr -> footage: c41j3C6J...
drwxr-xr-x 2024-01-01T00:00:00Z 0 inbox/ <- dailies:review/ -
drwxrwxrwx 2024-01-01T00:00:00Z 0 shared/ <> nas:project/shared/ -
--w------- 2024-01-01T00:00:00Z 0 dropbox/ -> ingest: -
-r--r--r-- 2024-01-01T00:00:00Z 524288 cache.dat <- cdn:assets/cache.dat c41j3C6J...
```

---

## 6. Round-Trip Behavior

### 6a. What Round-Trips Through What

| Medium | Flow links preserved? | Notes |
|--------|----------------------|-------|
| c4m file (text) | Yes | Flow operators are part of the format |
| c4d (daemon) | Yes | c4d understands and stores flow metadata |
| Bare filesystem | **No** | No kernel construct for flow links |
| tar/zip | **No** | No metadata field for flow declarations |
| git | **No** | Git tracks content, not flow relationships |

### 6b. The Filesystem Gap

Flow links are prescriptive declarations -- they describe how things
*should* behave, not how the kernel presents them. When a c4m file is
materialized to a bare filesystem, flow link information is lost because
the filesystem has no way to store it.

This is analogous to how c4m hard link groups work: the `->N` marker
declares write-binding between entries, but a bare filesystem does not
preserve the group number -- it only preserves the inode linkage.

### 6c. Lossless Round-Trip Path

The lossless round-trip path for flow links is:

```
c4m file -> c4d -> c4m file
```

c4d stores the full c4m including flow declarations. When a c4m is
re-exported from c4d, all flow links are preserved.

### 6d. Scan-from-Filesystem Behavior

When `c4 scan` generates a c4m from a live filesystem, it cannot discover
flow links -- they do not exist in the filesystem. The resulting c4m will
not contain flow declarations.

Flow links are created by:
1. Hand-editing a c4m file (human-editable is the escape valve)
2. CLI commands that add flow declarations to an existing c4m
3. c4d API calls that establish flow relationships
4. Exporting a c4m from c4d that already has flow links

### 6e. Merge Semantics

When merging two c4m files (e.g., a scanned c4m with no flows and a
hand-edited c4m with flows), flow links from the flow-bearing c4m are
preserved if the entries still exist. This is a c4m merge operation,
not a parser concern.

---

## 7. Interaction with Existing Features

### 7a. Hard Link Groups + Flow Links

Can an entry be both hard-linked and flow-linked? **No.** These are
mutually exclusive in the current field layout:

- Hard link marker and flow operator occupy the same syntactic slot
  (between name and C4 ID).
- Semantically, a hard link says "this entry shares an inode with other
  entries in this c4m." A flow link says "this entry's content propagates
  to/from a remote location." These are orthogonal concerns, but
  combining them in a single entry creates ambiguity about what exactly
  propagates.

If a hard-linked file needs to flow somewhere, the flow declaration
should be on the directory containing it, or as a separate policy
declaration. This keeps the entry format clean.

### 7b. Symlinks + Flow Links

Can a symlink have a flow declaration? **No**, for the same structural
reason -- the symlink target and flow operator occupy the same position.

More importantly, it would be semantically confusing. A symlink says
"follow this path." A flow link says "propagate this content." A symlink
to a flow-linked path inherits the flow behavior transitively -- the
symlink target resolves to a flow-linked entry. No need for double
annotation.

### 7c. Directories + Flow Links

Directories are the primary use case for flow links. A flow-linked
directory declares that its entire subtree participates in the flow:

```
drwxr-xr-x 2024-01-01T00:00:00Z 0 plates/ <- footage:shoot_001/plates/ -
  -rw-r--r-- 2024-01-01T00:00:00Z 1048576 plate_001.exr c41j3C6J...
  -rw-r--r-- 2024-01-01T00:00:00Z 1048576 plate_002.exr c41j3C6J...
```

The `plates/` directory flows inbound from `footage:shoot_001/plates/`.
The individual files inherit this -- c4d knows to pull them from the
`footage` location.

### 7d. Sequences + Flow Links

A sequence entry can have a flow link:

```
-rw-r--r-- 2024-01-01T00:00:00Z 10485760 render.[0001-0100].exr -> review:finals/ c41j3C6J...
```

The entire sequence flows outbound to `review:finals/`. This works
naturally because the sequence is a single entry in the c4m.

### 7e. Null Metadata + Flow Links

Flow-linked entries can have null metadata fields. A directory that
exists purely as a flow endpoint might have null size and timestamp:

```
drwxr-xr-x - - inbox/ <- dailies: -
```

This is a valid entry: "there is a directory called inbox that receives
inbound flow from the dailies location, with unknown size, unknown
timestamp, and no computed C4 ID."

---

## 8. C4 ID Implications

### 8a. Flow Links Change the C4 ID

Adding a flow link to an entry changes the entry's canonical form, which
changes the c4m's C4 ID. This is correct and intentional.

A c4m file *without* flow links describes a static filesystem tree.
A c4m file *with* flow links describes a filesystem tree plus its
relationships to other locations. These are different structures and
should have different identities.

### 8b. Canonical Form

The flow operator and target appear in the entry's `Canonical()` output:

```
-rw-r--r-- 2024-01-01T00:00:00Z 1048576 plate_001.exr -> footage: c41j3C6J...
```

This means the same file with and without a flow declaration produces
different canonical strings and therefore different C4 IDs for the
containing c4m. This is the correct behavior -- the c4m is a *description*
of a structure, and flow links are part of that description.

### 8c. Individual File C4 IDs Are Unchanged

A flow link does not change the C4 ID of the file content itself. The
`c4...` at the end of the line is still the hash of the file's bytes.
Flow is metadata about the entry's relationship to other locations, not
about the content.

---

## 9. Sort Order

### 9a. Flow Links Do Not Affect Sort Order

c4m entries are canonically sorted: files before directories at each
depth level, natural sort within each group. Flow declarations do not
participate in sorting -- entries are sorted by name only.

Two entries that differ only in flow declarations sort to the same
position. In practice this cannot happen because entry names are unique
within a manifest.

### 9b. No Separate Flow Section

Flow declarations are inline with their entries, not gathered into a
separate section. This keeps the c4m format linear and scannable --
you see the flow relationship right where the entry is declared.

---

## 10. Proposed Syntax with Examples

### 10a. Outbound Flow (content here propagates there)

File with outbound flow and known C4 ID:
```
-rw-r--r-- 2024-01-01T00:00:00Z 1048576 final_grade.exr -> review: c41j3C6J...
```

Directory subtree flows outbound:
```
drwxr-xr-x 2024-01-01T00:00:00Z 0 deliverables/ -> client:project_x/ -
  -rw-r--r-- 2024-01-01T00:00:00Z 2097152 master.mov c41j3C6J...
  -rw-r--r-- 2024-01-01T00:00:00Z 524288 poster.jpg c41j3C6J...
```

Write-only drop slot (data enters, drains elsewhere):
```
--w------- 2024-01-01T00:00:00Z 0 upload/ -> ingest:incoming/ -
```

### 10b. Inbound Flow (content there propagates here)

Read-only cache (data arrives, cannot be locally modified):
```
-r--r--r-- 2024-01-01T00:00:00Z 524288 lut.cube <- color:standards/lut.cube c41j3C6J...
```

Directory receiving inbound flow:
```
dr-xr-xr-x 2024-01-01T00:00:00Z 0 plates/ <- footage:shoot_001/plates/ -
  -r--r--r-- 2024-01-01T00:00:00Z 1048576 plate_001.exr c41j3C6J...
```

Inbound without C4 ID (content not yet pulled):
```
dr-xr-xr-x - - reference/ <- library:assets/ref/ -
```

### 10c. Bidirectional Flow (sync)

Full sync between locations:
```
drwxrwxrwx 2024-01-01T00:00:00Z 0 shared/ <> nas:project/shared/ -
```

Sequence with bidirectional flow:
```
-rw-rw-r-- 2024-01-01T00:00:00Z 10485760 comp.[0001-0100].exr <> review:comp/ c41j3C6J...
```

### 10d. Permission Composition Summary

| Permission | Flow Direction | Behavior |
|-----------|---------------|----------|
| `--w-------` | `->` outbound | Drop slot: write locally, drains to remote |
| `-r--r--r--` | `<-` inbound | Read cache: arrives from remote, read-only locally |
| `-rw-rw-rw-` | `<>` bidirectional | Full sync: read/write locally, propagates both ways |
| `-r--r--r--` | `->` outbound | Read-only mirror: content here is published but not writable |
| `-rw-r--r--` | `<-` inbound | Inbound with local edits: arrives from remote, owner can modify |

Permissions and flow direction compose independently. The permission bits
govern local access; the flow direction governs propagation. There is no
invalid combination -- each composition has a meaningful interpretation.

### 10e. Location Target Syntax

A flow target is always `LOCATION:` or `LOCATION:path/`, where:
- `LOCATION` matches `[a-zA-Z][a-zA-Z0-9_-]*` (starts with letter, then
  alphanumeric plus `-` and `_`)
- The `:` is mandatory and serves as the disambiguator
- The optional path after `:` uses `/` separators (Unix convention)
- A bare `LOCATION:` (nothing after colon) means the root of that location

Examples:
- `footage:` -- root of the "footage" location
- `nas:raw/plates/` -- path within the "nas" location
- `backup:` -- root of "backup"
- `studio-a:project/finals/` -- path with hyphenated location name

---

## 11. Validation Considerations

The validator (`validator.go`) should check:

1. Flow target contains exactly one `:` in the location label position
2. Location label matches `[a-zA-Z][a-zA-Z0-9_-]*`
3. Flow links are not combined with symlink targets or hard link markers
4. `<-` and `<>` operators are not used with the symlink mode bit (`l`)
5. Flow target path (after `:`) does not contain `..` or start with `/`

---

## 12. Builder API Extension

```go
// WithFlowOutbound sets outbound flow to a location
func WithFlowOutbound(target string) EntryOption {
    return func(e *Entry) {
        e.FlowDirection = FlowOutbound
        e.FlowTarget = target
    }
}

// WithFlowInbound sets inbound flow from a location
func WithFlowInbound(target string) EntryOption {
    return func(e *Entry) {
        e.FlowDirection = FlowInbound
        e.FlowTarget = target
    }
}

// WithFlowSync sets bidirectional flow with a location
func WithFlowSync(target string) EntryOption {
    return func(e *Entry) {
        e.FlowDirection = FlowBidirectional
        e.FlowTarget = target
    }
}
```

---

## 13. Summary of Changes Required

| File | Change |
|------|--------|
| `entry.go` | Add `FlowDirection`, `FlowTarget` fields; add `FlowDirection` type and constants; add `IsFlowLinked()`, `FlowOperator()` methods; extend `Format()` and `Canonical()` |
| `decoder.go` | Extend `parseEntryFields` to detect `<-` and `<>` operators; extend `parseNameOrTarget` boundary detection; add flow target parsing |
| `encoder.go` | Extend `formatEntryPretty` for flow links; update column width calculation |
| `builder.go` | Add `WithFlowOutbound`, `WithFlowInbound`, `WithFlowSync` options |
| `validator.go` | Add flow target validation rules |
| `errors.go` | Add `ErrInvalidFlowTarget` sentinel |
| `manifest.go` | No changes needed (sort by name only, flow is transparent) |
| `sequence.go` | No changes needed (sequences can carry flow links via Entry fields) |

The changes are additive. No existing behavior is modified. The `->` with
a location target (containing `:`) is a new interpretation of an existing
operator; `<-` and `<>` are entirely new operators.
