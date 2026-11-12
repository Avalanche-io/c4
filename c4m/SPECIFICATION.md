# C4 Manifest Format (C4M) Specification v1.0

> **Note**: This is the formal C4M specification. For user documentation, see [README.md](./README.md).

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

**Null mode**: `-` (single dash). Represents unspecified/unknown permissions — equivalent to NULL in a database. The entry's type (file vs directory) is determined by the trailing `/` in the name, not the mode field. A directory with unknown permissions is `- - - assets/ -`.

**Zero mode**: `----------` (ten dashes). This is a known value — zero permissions on a regular file. It is NOT the same as null. The distinction matters for progressive resolution: `-` means "haven't checked yet," `----------` means "checked and it's zero."

Partial modes like `d---------` are valid and carry type information (directory with zero permissions). They are not null.

### Timestamp

Canonical format: `YYYY-MM-DDTHH:MM:SSZ` (RFC 3339, UTC only, must end with `Z`).

**Null timestamp**: `-` (single dash) or `0`. Internally represented as Unix epoch (1970-01-01T00:00:00Z).

**Parser flexibility**: The decoder also accepts RFC 3339 with timezone offset (`2025-01-01T10:00:00-08:00`), Unix date format, and pretty-print format (`Jan  2 15:04:05 2006 MST`). All are converted to UTC internally.

### Size

File size in bytes as a decimal integer. No leading zeros except for `0` itself.

**Null size**: `-` (single dash). Internally represented as -1.

**Directory sizes**: Sum of all content within the directory (recursive). See [Directory C4 IDs and Metadata](#directory-c4-ids-and-metadata).

### Name

The bare filename (not a path). Entry names never contain `/`: nesting is expressed through indentation and depth. `/` is the only path separator in c4m. A backslash, a colon and every other byte is a literal character in a name, encoded by SafeName where needed (a backslash is written `\\`); a platform that cannot hold such a name maps or refuses it when it materialises the tree, and the c4m keeps the name as it is. Directory names end with a trailing `/`.

**Filename encoding (SafeName)**: Names are first encoded using the Universal Filename Encoding (see `SafeName`/`UnsafeName` in the c4m package), a three-tier system that represents arbitrary byte sequences as printable UTF-8:

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

**Invalid names**: `.`, `..`, `/`, empty string, names containing null bytes or `/`.

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
   - `-` or `c4...` (the C4 ID) as the only field left on the line: ungrouped hard link
   - anything else is a symlink target, followed by the C4 ID field: the C4 ID is always the last field and a target never holds an unescaped space, so a second field means the first is a target, even when it starts with `c4` (a null mode, as views carry links, is not a symlink mode)

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
   - Numeric segments: compare as integers of any length. Drop leading
     zeros; a shorter remainder is the smaller value; equal lengths compare
     digit by digit. Never parse into a fixed-width integer: a 20-digit run
     overflows 64 bits and a 17-digit run loses precision in a double, and
     either makes the order depend on the implementation or on input order.
   - Equal integers: shorter representation first (`1` < `01` < `001`)
   - Text segments: UTF-8 codepoint comparison
   - Mixed types: text sorts before numeric

The conformance vectors are `testdata/natural_sort_vectors.json`: one
list of names in canonical order, which every implementation must
reproduce from any permutation.

### Examples
```
file1.txt
file2.txt
file10.txt      # Not file1, file10, file2
render.1.exr
render.01.exr   # After render.1.exr (equal value, longer)
render.2.exr
render.10.exr
take.99999999999999999999.exr     # 20 digits: by value, not by a wrapped int64
take.100000000000000000000.exr
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
- **C4 ID**: Hash of the bare-concatenated member C4 IDs (90 chars per ID, no separators). This ID list content is a regular file — it can be stored in the content store or inlined as a trailing line in the c4m file (see [Inline Range Data](#inline-range-data)).
- **Size**: Sum of all member file sizes
- **Timestamp**: Most recent modification time among all members
- All members must be the same entry type (all files, all directories, or all symlinks)
- Sequences with any null (uncomputed) member C4 IDs cannot be folded

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

The size field for a directory is the **total size of all contents**: the recursive sum of file sizes, plus the byte length of each subdirectory's canonical one-level c4m record. That record is content too: it has a C4 ID and lives in the store beside the files, so a directory's size is the whole cost of everything inside it (see `design/directory-size-includes-c4m.md`). Filesystem metadata overhead is not included. A folder holding one 5-byte file therefore reads larger than 5, and its size changes when a child's metadata changes, because the child's record does.

#### Nil-Infectious Propagation

If **any** child entry has null size (-1), the parent directory's size is also null. This applies recursively to the root. The same rule applies to timestamps.

#### Empty and Unread Directories

A directory with no children and the empty-content C4 ID (`c459dsjfscH38cYeXXYogktxf4Cd9ibshE3BHUo6a58hBXmRQdZrAkZzsWcbWtDg5oQstpDuni4Hirj75GEmTc1sFT`, the ID of an empty listing) is **empty**: its size is 0. A directory with no listed children and **no** C4 ID is **unread**: nobody listed it (a directory at `d---------` the scanner could not open, or a hand-written line), so its size is null and stays null when metadata is propagated. Its mode and timestamp are still known if they were observed (`d--------- 2026-01-02T03:04:05Z - unread/ -`). Writing `0` for such a directory would assert an emptiness nobody saw. A scan that records one is partial (exit 2).

### Directory Timestamps

Directory timestamps come from the filesystem's modification time for the directory itself.

## Null and Zero Values

C4M supports null values to enable progressive resolution and manual editing:

| Field | Null representation | Zero representation |
|-------|-------------------|---------------------|
| Mode | `-` (single dash — unknown) | `----------` (ten dashes — known zero) |
| Timestamp | `-` or `0` | Unix epoch |
| Size | `-` | -1 |
| C4 ID | `-` or omitted | nil |
| Name | (cannot be null) | — |

`----------` decodes as the known zero mode and encodes back as `----------`, in canonical and pretty form alike; `-` decodes as null and encodes as `-`. A zero mode is never rewritten as null, and a null mode is never rewritten as zero.

**The type character must agree with the name.** The name's trailing `/` says whether an entry is a directory; the mode's first character says what kind of entry the mode describes. A known zero directory is therefore `d---------`, never `----------`. **Ten dashes on a directory line** (`---------- … name/`) is the one disagreement with a defined meaning: encoders through c4 1.0.21 wrote every null mode as ten dashes in pretty form, and no correct encoder writes a regular-file type beside a `/`, so a decoder MUST read it as the **null** mode (legacy). Ten dashes on a file line is the known zero, as ruled. Any other disagreement (a `d` type on a name without `/`; an `l`, `p`, `s`, `b` or `c` type, or a non-dash regular-file mode, on a name with `/`) is malformed: decoders SHOULD reject it as an invalid entry. (The Go decoder accepts such lines unchanged through 1.1, measured 2026-11-09; rejection awaits a ruling.) Encoders MUST NOT write ten dashes for a null mode on any line, in any form.

### Materializing null metadata

When a description is materialized onto a filesystem (`c4 restore`, `c4 patch`, reconcile):

1. **Applied to a path that already exists**, a null never overwrites the value the path has. The file keeps its timestamp and mode.
2. **Creating a file or directory that did not exist**, a null timestamp sets the Unix epoch, which scans back as null. So a tree created from a null-dated description reads back at that description's ID: the materialization round-trips.
3. **Creating from a null mode**, the file or directory takes the platform default (a regular file as the process's umask makes it, a directory likewise). No on-disk mode reads back as null, so a scan of the result legitimately differs from the description at that field, exactly as a scan at a finer level does.
4. **Verifying a materialization** compares at the description's own knowledge, field by field: the as-built tree is projected onto the description's nulls (a field the description leaves null is not compared) and must then recompute to the description's root ID.

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

The base may itself be a block whose first line names its own base: a consumer follows the references back until it reaches a block with none, so a chain of small blocks, each naming the one before, resolves from the newest block's ID alone. *(Wording added 2026-11-11, describing what the reference resolver, `c4m.ResolveBase`, already does; not a format change.)*

#### Subsequent Bare C4 ID (Block Link)

```
-rw-r--r-- 2025-01-01T00:00:00Z 100 a.txt c4...
c4<90-char-id>
-rw-r--r-- 2025-01-01T00:00:00Z 200 b.txt c4...
```

A bare C4 ID appearing **after entries** is a **checkpoint**: it names the accumulated manifest state at that point (all preceding entries and applied patches). A resolving decoder MUST verify it against the accumulated state and reject the stream with `ErrPatchIDMismatch` on mismatch; a non-resolving (shape-level) reader MAY record it unverified. Verification is one comparison per boundary — a resolving decoder is already folding the state, and the manifest ID hashes only the one-level canonical listing.

> **ERRATUM (2026-07-13).** This paragraph previously defined the bare ID as a *block link* — the ID of the block immediately above, recorded unverified. That semantic was never emitted by any shipped tool (producers always wrote accumulated-state IDs) and contradicted the verification language elsewhere in this document and C4M-STANDARD Section 10.3. Checkpoints are accumulated-state, verified. The write-side locality the block link aimed at survives in the store journal, which is its own format (not c4m) with a per-line invariant.

Entries following the checkpoint are interpreted as a patch against the accumulated state.

### Why the First-Line Rule Differs

A first-line bare C4 ID cannot be verified against accumulated content (there is none yet), so it serves purely as a reference. The human reader knows they need to fetch the base manifest. For subsequent bare C4 IDs, verification is mandatory — this prevents a malicious stream from claiming to represent one state while actually describing another.

### Patch Entry Semantics

Patch entries are matched against the current state **by name (and path)**:

| Condition | Interpretation |
|-----------|---------------|
| Name exists only in patch | **Addition** — entry is added |
| Name exists in both, any metadata differs | **Modification** — patch entry replaces the original |
| Name exists in both, all fields identical, patch entry has **no children** | **Removal** — entry is deleted |
| Name exists in both, all fields identical, patch entry **has children** | **Deepening** — entry is unchanged; children are applied recursively |

"All fields identical" means: same name, mode, timestamp, size, C4 ID, and target. This enables removal by restating an entry exactly — there is no separate delete syntax.

For directories, patch entries may contain children. The children are applied recursively using the same rules.

**A patch is meaningful only over its own base.** Because removal is spelled by exact restatement, what an entry means (removed, or now this) depends on the state the section's boundary names. Applying a patch's text directly to any other tree reads those meanings against the wrong state: an addition that meets an identical entry already present reads as a removal. To carry a patch's changes to another tree, interpret each entry against the patch's base first, then do to the other tree what the entry means: remove the path, or write the entry (the Go library's `ApplyPatchFrom`). A directory line with children under it is context for those children, not a write: it creates a directory in the other tree only when a write lands under it, and a directory's size and C4 ID in the result are derived from the children it then lists, never copied from the patch. A name is one thing: a write of a file or symlink `x` removes a directory `x/` and everything under it from the other tree, and a write that lands under a directory `x/`, or writes `x/` itself, removes a file or symlink `x`; the later write wins by kind as it does by bytes. With the other tree equal to the base this is ordinary patch application, and it is idempotent.

> **ERRATUM (2026-07-31).** The removal rule previously had no children
> condition, so restating a directory identically while declaring
> children under it deleted the directory and those children. A section
> that adds children is not removing their parent. This matters most for
> progressive breadth-first scanning, where a directory's children are
> discovered before anything about the directory itself is known, so the
> parent line is necessarily restated unchanged. To remove a subtree,
> restate the directory entry alone with no children.

### No Closing Validator

> **ERRATUM (2026-11-11).** This section previously called a bare C4 ID
> at EOF the chain's "closing validator" and said it was legal, which
> contradicted the rule below and "Multiple Patches". It was ruled on
> 2026-11-08 that "a bare C4 ID at the end of a c4m file is a format error",
> and on 2026-11-03 that no file can contain its own ID
> (design/no-self-id.md). The 2026-07-13 erratum that made a trailing ID
> legal is withdrawn; its ruling on consecutive checkpoints stands.

**A bare C4 ID at the end of a c4m file is a format error.** A c4m that
ended with the ID of its own resolved state would be a different file from
the one that ID names, and a reader cannot tell such a line from data that
happens to be an ID. Bare IDs appear only as boundaries, naming the state
of the sections before them, where a patch follows. Writers MUST NOT end a
c4m with a bare C4 ID. (How a reader treats such a line in files that
earlier tools wrote, refusing it or accepting it during a migration, is
not settled here.)

Consecutive checkpoints are checkpoint repetition: each names the same
accumulated state, and a resolving decoder verifies both. `ErrEmptyPatch`
is retired; the identifier remains reserved.

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

A chain ends on its last section's entries. A bare ID appears only as a boundary before a patch; a c4m file never ends with a bare ID of its own resolved state, because no file can contain its own ID: a file written from a description must be the file its own C4 ID names (design/no-self-id.md, 2026-11-03).

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

## Inline Range Data

When a c4m file contains sequence (range) entries and no external content store is available, the per-member ID lists can be inlined as trailing lines.

### Format

An inline ID list is a single line containing the bare concatenation of all member C4 IDs in range order. Each C4 ID is exactly 90 characters; the line length is always a multiple of 90 and greater than 90 (minimum 270 for 3-member sequences).

### Disambiguation

- **Patch boundary**: exactly 90 characters, starts with `c4`
- **Inline ID list**: >90 characters, multiple of 90, every 90-char chunk is a valid C4 ID
- **Entry line**: starts with mode characters or indentation

These are unambiguous by length alone.

### Position in Stream

Inline ID list lines appear after entries within a patch section, before the closing patch boundary:

```
<entries>
<inline ID list lines>
c4<patch boundary>
```

### Identity

Inline ID list lines are **not part of the manifest identity**. Adding, removing, or reordering them does not change the manifest's C4 ID or patch boundaries. They are transport data — the content backing a range entry's C4 ID, inlined for portability.

The C4 ID of an inline ID list line (computed by hashing the line content) matches the C4 ID of the corresponding sequence entry. This is self-verifying.

### Lifecycle

- **With store**: ID lists are stored as regular objects. No inline lines in the c4m.
- **Without store**: ID lists are appended as inline lines for self-containment.
- **Decompose**: Extract inline lines to files named by the range expression (e.g., `frames.[0001-0100].exr`). The file content is the bare-concatenated ID list.
- **Compose**: Read range expression files and inline them back.

The line content is byte-identical to the store object, so migration between the two forms is trivial.

## Validation Requirements

Parsers MUST:
- Reject lines beginning with `@` (directives are not supported)
- Reject invalid UTF-8 sequences
- Reject CR (carriage return) characters
- Apply canonical transformation consistently
- Verify bare C4 IDs match accumulated content (except first-line base reference)
- Treat a bare C4 ID at the end of a file as a format error, never as a validator (see "No Closing Validator")
- Accept null values as specified
- Reject path traversal attempts (`../`, `./`, a name containing `/` other than a directory's trailing one). A backslash is a literal name character, never a separator *(corrected 2026-11-11: this listed `\` as a separator; only `/` separates paths, ruled 2026-11-10)*
- Reject duplicate paths within the same scope

Parsers MAY:
- Accept non-sorted entries (re-sorting when processing)
- Accept inconsistent indentation (with warning)
- Accept ergonomic format variations

## Security Considerations

- No path traversal: `../`, `./`, names containing `/`
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
| `ErrEmptyPatch` | Retired (2026-07-13 erratum); identifier reserved |
