# C4 Manifest Format (C4M) Specification v1.0

> **Note**: This is the formal C4M specification. For implementation notes and edge cases, see [IMPLEMENTATION_NOTES.md](./IMPLEMENTATION_NOTES.md). For user documentation, see [README.md](./README.md).

## Overview

The C4 Manifest Format (.c4m) is a text-based (UTF-8) format for describing filesystem contents with content-addressed identification using C4 IDs (SMPTE ST 2114:2017). It preserves filesystem metadata while enabling content verification, deduplication, and distributed workflows.

A c4m file is an encapsulation of a filesystem: a small, self-contained description that behaves exactly like the full filesystem without needing the file content it describes.

## Core Format

### File Structure

1. **Entry-only format**: A c4m file is a sequence of entry lines. There is no header line and no directives. Lines beginning with `@` are rejected.
2. **Character Encoding**: UTF-8 only, no BOM
3. **Line Endings**: LF (0x0A) only. CR (0x0D) is forbidden.
4. **Blank lines**: Ignored by the parser.

### Entry Line Format

Each entry occupies one line:

```
<indentation><mode> <timestamp> <size> <name> [link-operator target] <c4id>
```

All four metadata fields (mode, timestamp, size, name) are required. The C4 ID or `-` (null) is always the last field. Between the name and C4 ID, an optional link operator may appear.

### Indentation

- Indentation is expressed as leading spaces.
- The indent width is detected from the first indented line and must remain consistent.
- Depth = indent / width. Depth 0 = top-level entries.
- Children of a directory appear at depth+1 immediately following their parent.

## Field Specifications

### Mode (10 characters)

Standard Unix file mode string:

| Position | Meaning |
|----------|---------|
| 0 | File type: `-` regular, `d` directory, `l` symlink, `p` pipe, `s` socket, `b` block device, `c` char device |
| 1-3 | Owner permissions: `rwx` / `-` |
| 4-6 | Group permissions: `rwx` / `-` |
| 7-9 | Other permissions: `rwx` / `-` |

Special bits: setuid (`s`/`S` at position 3), setgid (`s`/`S` at position 6), sticky (`t`/`T` at position 9).

**Null mode**: `-` (single dash) or `----------` (ten dashes). Represents unspecified permissions.

### Timestamp

Canonical format: `YYYY-MM-DDTHH:MM:SSZ` (RFC 3339, UTC only, must end with `Z`).

**Null timestamp**: `-` (single dash) or `0`. Internally represented as Unix epoch (1970-01-01T00:00:00Z).

**Parser flexibility**: The decoder also accepts RFC 3339 with timezone offset (`2025-01-01T10:00:00-08:00`), Unix date format, and pretty-print format (`Jan  2 15:04:05 2006 MST`). All are converted to UTC internally.

### Size

File size in bytes as a decimal integer. No leading zeros except for `0` itself.

**Null size**: `-` (single dash). Internally represented as -1.

**Directory sizes**: Sum of all content within the directory (recursive). See [Directory C4 IDs and Metadata](#directory-c4-ids-and-metadata).

### Name

The bare filename (not a path). Entry names never contain `/` or `\` as separators — nesting is expressed through indentation and depth. Directory names end with a trailing `/`.

**Filename encoding (SafeName)**: Names are first encoded using the Universal Filename Encoding ([design/filename-encoding.md](../design/filename-encoding.md)), a three-tier system that represents arbitrary byte sequences as printable UTF-8:

- **Tier 1**: Printable UTF-8 passes through unchanged (except `¤` and `\`)
- **Tier 2**: Backslash escapes for `\0` (null), `\t` (tab), `\n` (newline), `\r` (CR), `\\` (backslash)
- **Tier 3**: Non-printable bytes encoded as braille codepoints (U+2800–U+28FF) between `¤` delimiters

**c4m field-boundary escaping**: After SafeName encoding, the following characters are backslash-escaped to prevent ambiguity with c4m field delimiters:

| Character | Escape | Reason |
|-----------|--------|--------|
| Space (U+0020) | `\ ` | Field delimiter |
| Double-quote (U+0022) | `\"` | Reserved |
| `[` | `\[` | Sequence notation (non-sequence names only) |
| `]` | `\]` | Sequence notation (non-sequence names only) |

No quoting mechanism exists. All field-boundary characters are backslash-escaped.

**Invalid names**: `.`, `..`, `/`, empty string, names containing null bytes or path separators.

### C4 ID

A C4 ID (SMPTE ST 2114:2017) is a 90-character base58 string starting with `c4`. It always appears as the last field on the line.

**Null C4 ID**: `-` (single dash) or omitted. Represents uncomputed or unavailable content hash.

## Link Operators

Between the name and C4 ID, an optional link operator specifies a relationship:

### Symlinks

```
lrwxrwxrwx 2025-01-01T00:00:00Z 0 link.txt -> target.txt c4...
```

- `->` followed by the target path
- Target uses backslash escaping (`\ ` for spaces, `\"` for quotes)
- C4 ID is computed from the **target file's content** (not the link itself)
- Symlinks to symlinks or broken symlinks: nil C4 ID
- Symlinks to directories: C4 ID of the target directory's manifest

### Hard Links

```
-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt -> c4...
-rw-r--r-- 2025-01-01T00:00:00Z 100 link.txt ->2 c4...
```

- `->` (no target path, no space after) = ungrouped hard link
- `->N` (digit immediately after `->`) = hard link group N

### Flow Links

Flow links declare cross-location data relationships:

```
drwxr-xr-x 2025-01-01T00:00:00Z 4096 outbox/ -> studio:inbox/ c4...
drwxr-xr-x 2025-01-01T00:00:00Z 4096 inbox/  <- nas:renders/  c4...
drwxr-xr-x 2025-01-01T00:00:00Z 4096 shared/ <> peer:shared/  c4...
```

| Operator | Direction | Meaning |
|----------|-----------|---------|
| `->` | Outbound | Content here propagates there |
| `<-` | Inbound | Content there propagates here |
| `<>` | Bidirectional | Two-way sync |

The target is a location reference: `<location-name>:<path>`. Location names match `[a-zA-Z][a-zA-Z0-9_-]*`.

**Disambiguation**: The `->` operator is overloaded but unambiguous. The parser uses the entry's **mode** (already parsed) as the primary discriminator:
1. **Symlink mode** (`l`): `->` is always a symlink target — no further checks needed
2. **Non-symlink mode**: examine what follows `->`:
   - A digit 1-9 immediately (no space): hard link group
   - A location reference (`name:...`): outbound flow link
   - `-` or `c4...` (the C4 ID): ungrouped hard link

## Canonical Form

For computing C4 IDs, content must be in canonical form:

1. No leading indentation
2. Single space between fields
3. No padding or alignment
4. No comma separators in sizes
5. No leading zeros in sizes (except `0`)
6. UTC timestamps with `Z` suffix
7. Null mode: `-` (single dash, not `----------`)
8. Null timestamp: `-`
9. Null size: `-`
10. Null C4 ID: `-`
11. Natural sort ordering (files before directories, then natural sort within each group)

## Ergonomic Forms

Parsers accept and tools may output ergonomic variations for human readability:

### Padded Size Fields

Right-align sizes with spaces; use comma separators for thousands:
```
-rw-r--r-- 2025-01-01T00:00:00Z     100 small.txt
-rw-r--r-- 2025-01-01T00:00:00Z   1,234 medium.txt
-rw-r--r-- 2025-01-01T00:00:00Z 100,000 large.txt
```

### Local Timestamps

Display timestamps with timezone for readability:
```
-rw-r--r-- 2025-01-01T10:00:00-08:00 1024 file.txt
-rw-r--r-- Jan  1 10:00:00 2025 PST  1024 file.txt
```

### Column-Aligned C4 IDs

```
-rw-r--r-- 2025-01-01T00:00:00Z 100 a.txt          c41abc...
-rw-r--r-- 2025-01-01T00:00:00Z 200 longer.txt     c41def...
```

**Column alignment rules**:
- Default C4 ID column: 80
- If longest line + 10 exceeds 80, shift to next 10-column boundary (90, 100, ...)
- Minimum 10 spaces between content and C4 ID
- Sizes right-aligned and padded to widest value

### Parser Requirements

Parsers MUST accept all ergonomic forms and convert to canonical form before computing C4 IDs.

## Natural Sort Algorithm

Entries are sorted using natural ordering:

1. Files sort before directories at the same depth level
2. Within each group, split names into alternating text/numeric segments
3. Compare segments left-to-right:
   - Numeric segments: compare as integers
   - Equal integers: shorter representation first (`1` < `01` < `001`)
   - Text segments: UTF-8 codepoint comparison
   - Mixed types: text sorts before numeric

### Examples
```
file1.txt
file2.txt
file10.txt      # Not file1, file10, file2
render.1.exr
render.01.exr   # After render.1.exr (equal value, longer)
render.2.exr
render.10.exr
```

## Media File Sequences

Sequences provide compact representation of numbered files common in media workflows.

### Notation
- Contiguous: `frame.[0001-0100].exr`
- Stepped: `frame.[0001-0100:2].exr` (every other frame)
- Discontinuous: `frame.[0001-0050,0075-0100].exr`
- Individual: `frame.[0001,0005,0010].exr`
- Directory sequences: `shot_[001-100]/`

### Rules

- Names with escaped brackets (`\[`, `\]`) are never interpreted as sequences
- The C4 ID of a sequence is computed like a directory: from the canonical form of its expanded entries
- **Size**: Sum of all member file sizes
- **Timestamp**: Most recent modification time among all members
- All members must be the same entry type (all files, all directories, or all symlinks)

## Directory C4 IDs and Metadata

### Computing Directory C4 IDs

Directory C4 IDs use a **one-level canonical representation**:

1. List direct children only (files and subdirectories)
2. For files: use their content C4 ID
3. For subdirectories: recursively compute their C4 ID using this same algorithm
4. Sort: files before directories, then natural sort within each group
5. Format as canonical entries (no indentation, single space between fields)
6. Compute C4 ID from the UTF-8 bytes of this canonical text

This creates a Merkle tree where each directory's identity depends on its contents.

### Directory Sizes

The size field for a directory is the **total size of all contents** (recursive sum of file sizes). Does not include filesystem metadata overhead.

#### Nil-Infectious Propagation

If **any** child entry has null size (-1), the parent directory's size is also null. This applies recursively to the root. The same rule applies to timestamps.

### Directory Timestamps

Directory timestamps come from the filesystem's modification time for the directory itself.

## Null and Zero Values

C4M supports null values to enable progressive resolution and manual editing:

| Field | Null representation | Internal value |
|-------|-------------------|----------------|
| Mode | `-` or `----------` | 0 |
| Timestamp | `-` or `0` | Unix epoch |
| Size | `-` | -1 |
| C4 ID | `-` or omitted | nil |
| Name | (cannot be null) | — |

### Use Cases

- **Progressive resolution**: Start with just names, fill in metadata as it becomes available
- **Boolean set operations**: Create manifests with only names to define sets
- **Manual editing**: Leave fields unspecified when exact values are unknown

## Patch Format

A c4m stream can contain inline patches, enabling incremental updates without duplicating unchanged content.

### Bare C4 ID Lines

A line containing only a C4 ID (exactly 90 characters, starting with `c4`) acts as a **patch boundary**. There are two cases:

#### First-Line Bare C4 ID (External Base Reference)

```
c4<90-char-id>
-rw-r--r-- 2025-01-01T00:00:00Z 200 new.txt c4...
```

A bare C4 ID on the **first non-blank line** of the stream (before any entries) references an **external base manifest**. The consumer must fetch this manifest by its C4 ID to know the starting state. The entries that follow are a patch applied against it.

This is stored on `Manifest.Base`.

#### Subsequent Bare C4 ID (Inline Checkpoint)

```
-rw-r--r-- 2025-01-01T00:00:00Z 100 a.txt c4...
c4<90-char-id>
-rw-r--r-- 2025-01-01T00:00:00Z 200 b.txt c4...
```

A bare C4 ID appearing **after entries** is an inline checkpoint. It **must match** the canonical C4 ID of all accumulated content above it. If it doesn't match, the stream is malformed (`ErrPatchIDMismatch`).

Entries following the checkpoint are interpreted as a patch against the accumulated state.

### Why the First-Line Rule Differs

A first-line bare C4 ID cannot be verified against accumulated content (there is none yet), so it serves purely as a reference. The human reader knows they need to fetch the base manifest. For subsequent bare C4 IDs, verification is mandatory — this prevents a malicious stream from claiming to represent one state while actually describing another.

### Patch Entry Semantics

Patch entries are matched against the current state **by name (and path)**:

| Condition | Interpretation |
|-----------|---------------|
| Name exists only in patch | **Addition** — entry is added |
| Name exists in both, any metadata differs | **Modification** — patch entry replaces the original |
| Name exists in both, all fields identical | **Removal** — entry is deleted |

"All fields identical" means: same name, mode, timestamp, size, C4 ID, and target. This enables removal by restating an entry exactly — there is no separate delete syntax.

For directories, patch entries may contain children. The children are applied recursively using the same rules.

### Empty Patch Rejection

Every patch section must contain at least one entry. A bare C4 ID followed by nothing (EOF) or by another bare C4 ID (consecutive checkpoints) is rejected as `ErrEmptyPatch`.

### Multiple Patches

A stream may contain multiple successive patches:

```
<base entries>
c4<id-of-base>
<patch-1 entries>
c4<id-after-patch-1>
<patch-2 entries>
```

Each checkpoint verifies the state after all preceding patches have been applied.

### Encoding Patches

`EncodePatch` writes a patch section: the old manifest's C4 ID as a bare line, followed by the patch entries. `PatchDiff` computes the patch between two manifests.

### Example: Streaming Updates

```
-rw-r--r-- 2025-03-06T12:00:00Z 100 a.txt c4abc...
c449ByTh8Hkx...  # C4 ID of the manifest containing just a.txt
-rw-r--r-- 2025-03-06T12:00:00Z 200 b.txt c4def...
c4Rq7Jm2Pnk...  # C4 ID of the manifest containing a.txt + b.txt
-rw-r--r-- 2025-03-06T12:00:00Z 100 a.txt c4abc...
```

This stream:
1. Starts with `a.txt`
2. First checkpoint verifies the base state
3. Adds `b.txt` (patch 1)
4. Second checkpoint verifies state after adding `b.txt`
5. Restates `a.txt` identically → removes it (patch 2)
6. Final state: only `b.txt`

## Validation Requirements

Parsers MUST:
- Reject lines beginning with `@` (directives are not supported)
- Reject invalid UTF-8 sequences
- Reject CR (carriage return) characters
- Apply canonical transformation consistently
- Verify bare C4 IDs match accumulated content (except first-line base reference)
- Reject empty patch sections
- Accept null values as specified
- Reject path traversal attempts (`../`, `./`, names containing `/` or `\`)
- Reject duplicate paths within the same scope

Parsers MAY:
- Accept non-sorted entries (re-sorting when processing)
- Accept inconsistent indentation (with warning)
- Accept ergonomic format variations

## Security Considerations

- No path traversal: `../`, `./`, names containing path separators
- No null bytes in names
- Control characters forbidden (0x00-0x1F) — encoded via Universal Filename Encoding
- Maximum line length: implementation-defined (suggested 1MB)
- Patch C4 ID verification prevents substitution attacks (a stream cannot claim to represent one manifest while containing another)
- First-line external base reference is explicitly visible to human readers

## Error Types

| Error | Meaning |
|-------|---------|
| `ErrInvalidEntry` | Malformed entry line |
| `ErrDuplicatePath` | Duplicate path in manifest |
| `ErrPathTraversal` | Path traversal attempt |
| `ErrInvalidFlowTarget` | Malformed flow link target |
| `ErrPatchIDMismatch` | Bare C4 ID does not match accumulated content |
| `ErrEmptyPatch` | Patch section contains no entries |
