# C4 Manifest Format (C4M) — Formal Standard

**Version**: 1.0
**Status**: Working Draft
**Date**: 2026-03-10
**Authors**: Joshua Kolden
**References**: SMPTE ST 2114:2017 (C4 Content Identification)

---

## 1. Introduction

### 1.1 Purpose

This document defines the C4 Manifest Format (C4M) for describing filesystem
contents with content-addressed identification. It uses normative language per
RFC 2119: the key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT",
"SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" have precise
meaning as defined therein.

### 1.2 Scope

This standard specifies:

- The text encoding and line grammar of a C4M stream
- The canonical form used for C4 ID computation
- The patch format for incremental updates
- Filename encoding for non-printable characters
- Error conditions and their classification

### 1.3 Normative References

- **SMPTE ST 2114:2017** — Content Identification using Cryptographic Hashing
  (defines C4 IDs: SHA-512 encoded as base58, yielding 90-character identifiers
  prefixed with `c4`)
- **RFC 3629** — UTF-8, a transformation format of ISO 10646
- **RFC 3339** — Date and Time on the Internet: Timestamps
- **RFC 2119** — Key words for use in RFCs to Indicate Requirement Levels

---

## 2. Notation and Conventions

### 2.1 ABNF Grammar

Where formal grammar is given, it follows Augmented Backus-Naur Form (ABNF)
per RFC 5234, with the following terminals:

```abnf
LF          = %x0A
SP          = %x20
DIGIT       = %x30-39
ALPHA       = %x41-5A / %x61-7A
BASE58CHAR  = %x31-39 / %x41-48 / %x4A-4E / %x50-5A
            / %x61-6B / %x6D-7A
            ; 1-9, A-H, J-N, P-Z, a-k, m-z (no 0, O, I, l)
UTF8-PRINT  = <any codepoint where unicode.IsPrint() is true>
```

### 2.2 Terminology

- **c4m stream**: A byte sequence conforming to this specification
- **c4m file**: A file whose content is a c4m stream
- **entry**: A single line describing a filesystem object
- **manifest**: The parsed in-memory representation of a c4m stream
- **canonical form**: The normalization used for C4 ID computation
- **ergonomic form**: Human-optimized variations accepted by parsers

---

## 3. Stream Structure

### 3.1 Character Encoding

A c4m stream MUST be encoded as UTF-8 (RFC 3629).

A c4m stream MUST NOT contain a byte order mark (BOM, U+FEFF).

### 3.2 Line Endings

Lines MUST be terminated by LF (0x0A).

Lines MUST NOT contain CR (0x0D). A conforming decoder SHALL reject any stream
containing CR with a fatal error.

### 3.3 Stream Grammar

```abnf
c4m-stream  = *( line LF )
line        = entry-line / bare-c4id / inline-idlist / blank-line
blank-line  = *SP
bare-c4id   = c4-id               ; exactly 90 characters
inline-idlist = 2*c4-id           ; >90 characters, multiple of 90
entry-line  = [indent] mode SP timestamp SP size SP name
              [SP link-part] SP c4id-or-null
indent      = 1*SP
```

A c4m stream is entry-only. There is no header, no version declaration, and no
directives. Lines beginning with `@` (U+0040) after whitespace trimming MUST
be rejected as invalid.

Inline ID list lines (Section 10.9) are distinguished from bare C4 ID lines
by length: bare C4 IDs are exactly 90 characters, inline ID lists are always
longer (a multiple of 90, minimum 180).

### 3.4 Blank Lines

Blank lines (lines containing only whitespace or empty) MUST be ignored by
the parser.

---

## 4. Entry Fields

### 4.1 Mode

```abnf
mode        = type-char perm-chars
            / null-mode
type-char   = "-" / "d" / "l" / "p" / "s" / "b" / "c"
perm-chars  = 9( "r" / "w" / "x" / "s" / "S" / "t" / "T" / "-" )
null-mode   = "-"                ; single dash, canonical null
            / "----------"       ; ten dashes, ergonomic null
```

**Type characters**:

| Character | File Type |
|-----------|-----------|
| `-` | Regular file |
| `d` | Directory |
| `l` | Symbolic link |
| `p` | Named pipe (FIFO) |
| `s` | Socket |
| `b` | Block device |
| `c` | Character device |

**Permission bits** (positions 1-9):

| Position | Bit | Characters |
|----------|-----|------------|
| 1 | Owner read | `r` / `-` |
| 2 | Owner write | `w` / `-` |
| 3 | Owner execute | `x` / `s` (setuid+exec) / `S` (setuid, no exec) / `-` |
| 4 | Group read | `r` / `-` |
| 5 | Group write | `w` / `-` |
| 6 | Group execute | `x` / `s` (setgid+exec) / `S` (setgid, no exec) / `-` |
| 7 | Other read | `r` / `-` |
| 8 | Other write | `w` / `-` |
| 9 | Other execute | `x` / `t` (sticky+exec) / `T` (sticky, no exec) / `-` |

**Null mode**: When mode is unspecified, the canonical representation is a
single `-` character. The decoder MUST also accept `----------` (ten dashes).

**Implementation note**: The decoder distinguishes single-dash null mode from
the regular-file type indicator by checking for `- ` (dash followed by space
at position 1). A 10-character mode always begins at position 0 and extends
to position 9 inclusive.

> **AMBIGUITY**: In canonical output, null mode is `-` (1 character). In
> standard (non-canonical) output, null mode is `----------` (10 characters).
> Both are accepted on input. Implementations MUST use `-` in canonical form
> and SHOULD use `----------` in non-canonical output. The difference is
> cosmetic; the single-dash form is the normative representation for C4 ID
> computation.

### 4.2 Timestamp

```abnf
timestamp       = canonical-ts / null-ts
canonical-ts    = date "T" time "Z"
date            = 4DIGIT "-" 2DIGIT "-" 2DIGIT
time            = 2DIGIT ":" 2DIGIT ":" 2DIGIT
null-ts         = "-" / "0"
```

Canonical timestamps MUST be in UTC and MUST end with `Z`. The format is a
strict subset of RFC 3339.

**Null timestamp**: `-` or `0`. Internally represented as the Unix epoch
(1970-01-01T00:00:00Z).

**Ergonomic forms** (accepted by decoder, converted to UTC):

| Form | Example |
|------|---------|
| RFC 3339 with offset | `2025-01-01T10:00:00-08:00` |
| Unix date | `Mon Jan  2 15:04:05 MST 2006` |
| Pretty with TZ | `Jan  2 15:04:05 2006 MST` |

Decoders MUST accept all ergonomic forms and convert to UTC.

Encoders MUST output canonical form (`...Z`) in canonical mode. Encoders
MAY output local time with timezone in ergonomic/pretty mode.

### 4.3 Size

```abnf
size        = "0" / ( non-zero-digit *DIGIT ) / null-size
non-zero-digit = %x31-39
null-size   = "-"
```

Size is a non-negative decimal integer representing file content in bytes.
Leading zeros are forbidden (except `0` itself).

**Null size**: `-`. Internally represented as -1.

**Ergonomic form**: Comma-separated thousands (e.g., `1,234,567`). Decoders
MUST accept commas in size fields and strip them before parsing.

**Directory sizes**: The size of a directory MUST be the recursive sum of all
file content sizes within it. This excludes filesystem metadata overhead.

#### 4.3.1 Nil-Infectious Propagation

If any child entry has null size (-1), the parent directory's size MUST be
null. This rule applies recursively: a null anywhere in a subtree propagates
to the root.

The same rule applies to timestamps: if any child has a null timestamp, the
parent directory's timestamp MUST be null.

### 4.4 Name

```abnf
name          = 1*( name-char ) [ "/" ]
name-char     = escaped-char / safe-char
escaped-char  = "\" ( SP / DQUOTE / "[" / "]" )
safe-char     = <SafeName-encoded printable UTF-8, see 4.4.2>
```

Names are bare filenames (not paths). They MUST NOT contain path separators
(`/` as a separator — trailing `/` for directories is a type marker, not a
separator), or null bytes (0x00).

Directory names MUST end with `/`.

**Invalid names** (MUST be rejected):
- Empty string
- `.` (current directory)
- `..` (parent directory)
- `/` (root)
- Names containing `/` other than as a trailing directory marker
- Names containing 0x00 (null byte)

#### 4.4.1 Two-Layer Name Encoding

Names in c4m undergo two encoding layers, applied in order:

1. **SafeName encoding** (Section 4.4.2): Transforms arbitrary filesystem
   byte sequences into printable UTF-8 text. This handles control characters,
   non-printable bytes, and invalid UTF-8.

2. **c4m field-boundary escaping** (Section 4.4.3): Backslash-escapes
   characters that would be ambiguous as c4m field delimiters.

On decode, the layers are reversed: the decoder first consumes c4m
field-boundary escapes, then applies `UnsafeName` to recover the raw
byte sequence.

#### 4.4.2 Universal Filename Encoding (SafeName)

Filesystem names may contain arbitrary bytes. The Universal Filename Encoding
(see `SafeName`/`UnsafeName` in the c4m package)
provides a three-tier system for representing all possible filenames in
printable UTF-8:

**Tier 1 — Printable passthrough**: Printable UTF-8 codepoints (where
`unicode.IsPrint()` is true) pass through unchanged, except for `¤` (U+00A4)
which is the Tier 3 escape delimiter and `\` (U+005C) which is the Tier 2
escape sigil.

**Tier 2 — Conventional control escapes**: Common control characters use
standard backslash escapes:

| Byte | Escape | Description |
|------|--------|-------------|
| 0x00 (NUL) | `\0` | Null / path delimiter |
| 0x09 (TAB) | `\t` | Horizontal tab |
| 0x0A (LF)  | `\n` | Line feed |
| 0x0D (CR)  | `\r` | Carriage return |
| 0x5C (`\`) | `\\` | Literal backslash |

**Tier 3 — Byte-level braille encoding**: All other non-printable bytes are
encoded as Braille Pattern codepoints (U+2800–U+28FF) between `¤` (generic
currency sign, U+00A4) delimiters:

```
¤⠁⠂⣿¤    →  bytes 0x01, 0x02, 0xFF
```

Each braille codepoint encodes one byte: the value is the codepoint minus
U+2800. Consecutive non-printable bytes share a single pair of `¤`
delimiters. Invalid UTF-8 sequences are encoded byte-by-byte in Tier 3.

**Canonical encoding priority**: Tier 1 > Tier 2 > Tier 3. A byte eligible
for a higher-priority tier MUST use that tier, ensuring a unique canonical
representation for every input.

**Round-trip identity**: For every byte sequence B not containing 0x00,
`UnsafeName(SafeName(B)) = B`.

The functions `SafeName` (encode) and `UnsafeName` (decode) implement this
transformation.

#### 4.4.3 c4m Field-Boundary Escaping

After SafeName encoding, the following characters are backslash-escaped to
prevent ambiguity with c4m field delimiters. No quoting mechanism exists.

| Character | Escape | Reason |
|-----------|--------|--------|
| Space (U+0020) | `\ ` | Field delimiter |
| Double-quote (U+0022) | `\"` | Reserved character |
| `[` | `\[` | Sequence notation (non-sequence names only) |
| `]` | `\]` | Sequence notation (non-sequence names only) |

The encoder applies these escapes after SafeName encoding and before output.
The decoder consumes them before applying UnsafeName.

**Bracket escaping rule**: In non-sequence names, literal `[` and `]` MUST
be escaped as `\[` and `\]` to prevent misinterpretation as sequence
notation. In sequence names, brackets forming the range notation
(`[0001-0100]`) are NOT escaped; only brackets in the prefix and suffix
portions are escaped.

#### 4.4.4 Name Boundary Detection

The parser uses `/` as a termination signal for directory names. Upon
encountering `/`, the parser stops and returns the name including the
trailing `/`. Directory names cannot contain embedded `/` characters —
which is correct, since names are bare filenames.

For non-directory names, the parser scans until it encounters a boundary:
a space followed by a link operator (`->`, `<-`, `<>`), a C4 ID prefix
(`c4`), a null C4 ID (`-`), or end-of-line. Escaped spaces (`\ `) are
consumed as literal characters and do not trigger boundary detection.

### 4.5 C4 ID

```abnf
c4id-or-null = c4-id / null-c4id
c4-id        = "c4" 88BASE58CHAR  ; exactly 90 characters total
null-c4id    = "-"
```

A C4 ID is a SMPTE ST 2114:2017 identifier: 90 characters, always starting
with `c4`, encoded in base58 (Bitcoin alphabet: no 0, O, I, l).

The C4 ID or null placeholder `-` is always the **last field** on the line.

**Null C4 ID**: `-` (single dash). Represents an uncomputed or unavailable
content identifier. Internally represented as the zero-value `c4.ID{}`.

> **IMPLEMENTATION NOTE**: When encoding, the C4 ID or `-` is always emitted
> as the final field. When decoding, if the remaining text after the name
> (and any link operator) does not begin with `c4` or `-`, no C4 ID is set
> (the field defaults to nil). This means the C4 ID is effectively optional
> on input but always present on output.

---

## 5. Link Operators

### 5.1 Grammar

```abnf
link-part      = symlink-op / hardlink-op / flow-op
symlink-op     = "->" SP target
hardlink-op    = "->" [group-num]
                 ; no space before group-num
group-num      = %x31-39 *DIGIT  ; starts with 1-9
flow-op        = flow-dir SP flow-target
flow-dir       = "->" / "<-" / "<>"
flow-target    = location-name ":" [path-part]
location-name  = ALPHA *( ALPHA / DIGIT / "_" / "-" )
```

### 5.2 Symlinks

```
lrwxrwxrwx 2025-01-01T00:00:00Z 0 link.txt -> target.txt c4abc...
```

The symlink target is the path stored by the filesystem. Target paths MAY
be absolute, relative, or contain path separators. Targets use the same
backslash escaping as names (`\ ` for spaces, `\"` for quotes) but do NOT
escape brackets (brackets in target paths are always literal).

**C4 ID rules for symlinks**:

| Target resolves to | C4 ID |
|--------------------|-------|
| Regular file | C4 ID of file content |
| Directory | C4 ID of directory manifest |
| Another symlink | Nil (prevents infinite recursion) |
| Nonexistent (broken) | Nil |

### 5.3 Hard Links

```
-rw-r--r-- 2025-01-01T00:00:00Z 100 file.txt -> c4abc...
-rw-r--r-- 2025-01-01T00:00:00Z 100 link.txt ->2 c4abc...
```

- `->` with no target text and followed directly by C4 ID or `-`: ungrouped
  hard link (internal value: -1)
- `->N` where N starts with digit 1-9: hard link group number (internal
  value: N)

### 5.4 Flow Links

```
drwxr-xr-x 2025-01-01T00:00:00Z 4096 outbox/ -> studio:inbox/ c4...
drwxr-xr-x 2025-01-01T00:00:00Z 4096 inbox/  <- nas:renders/  c4...
drwxr-xr-x 2025-01-01T00:00:00Z 4096 shared/ <> peer:shared/  c4...
```

| Operator | Direction | Semantics |
|----------|-----------|-----------|
| `->` | Outbound | Content at this path propagates to the remote location |
| `<-` | Inbound | Content at the remote location propagates here |
| `<>` | Bidirectional | Two-way synchronization |

The flow target MUST match the `location-name ":" [path]` pattern.

### 5.5 Disambiguation of `->`

The `->` token is overloaded across symlinks, hard links, and outbound flow
links. The decoder disambiguates using the entry's **mode** (already parsed
before the link operator):

1. **Symlink mode** (`l` in position 0): `->` is ALWAYS a symlink target.
   No further disambiguation needed. This is unambiguous because a symlink
   cannot have a null target.

2. **Non-symlink mode**: examine what follows `->`:
   a. If `->` is immediately followed by a digit 1-9 (no space):
      **hard link group**
   b. Otherwise, skip whitespace and examine the next token:
      - If the next token matches a flow target pattern (`name:...`):
        **outbound flow link**
      - If remaining text is `-` or starts with `c4`: **ungrouped hard link**
      - Otherwise: **symlink target** (fallback, for non-standard entries)

> **NOTE**: The flow target check MUST run before the `c4` prefix check.
> Location names starting with `c4` (e.g., `c4studio:inbox/`) are legal
> and would be misclassified as ungrouped hard links if the `c4` prefix
> were checked first.

This design is unambiguous: the mode character determines the entry type,
and the `->` operator is interpreted in context. Hard links only appear
on regular files. Flow links only appear on directories. Symlinks always
have `l` mode.

---

## 6. Indentation and Hierarchy

### 6.1 Indentation

```abnf
indent = 1*SP
```

Indentation is expressed as leading space characters. The indentation width
is detected from the first indented line encountered and MUST remain
consistent throughout the stream.

**Depth**: `depth = indent_chars / indent_width`. Depth 0 entries have no
leading spaces.

### 6.2 Parent-Child Relationship

Children of a directory appear at `depth + 1` immediately following the
directory entry. The parent of an entry at depth N is the nearest preceding
directory entry at depth N-1.

```
drwxr-xr-x 2025-01-01T00:00:00Z 300 project/             ; depth 0
  -rw-r--r-- 2025-01-01T00:00:00Z 100 readme.txt c4...    ; depth 1
  drwxr-xr-x 2025-01-01T00:00:00Z 200 src/                ; depth 1
    -rw-r--r-- 2025-01-01T00:00:00Z 200 main.go c4...     ; depth 2
```

---

## 7. Canonical Form

### 7.1 Definition

Canonical form is the normative representation used for C4 ID computation.
A manifest in canonical form conforms to ALL of the following:

1. No leading indentation (all entries at depth 0)
2. Single SP (0x20) between every field
3. No trailing whitespace
4. No padding or alignment
5. Sizes as bare decimal integers, no commas, no leading zeros (except `0`)
6. Timestamps in UTC with `Z` suffix
7. Null mode: `-` (single dash)
8. Null timestamp: `-`
9. Null size: `-`
10. Null C4 ID: `-`
11. Entries sorted: files before directories, then natural sort within groups
12. Each line terminated by LF

### 7.2 Canonicalization Procedure

To produce canonical form from an arbitrary manifest:

1. Copy the manifest (to avoid mutating the original)
2. Propagate metadata: resolve null directory sizes and timestamps from
   children (nil-infectious propagation)
3. Collect only top-level entries (minimum depth)
4. Sort: files before directories, then natural sort by name
5. Format each entry using canonical field representations
6. Concatenate with LF terminators

### 7.3 C4 ID Computation

The C4 ID of a manifest is computed by:

1. Producing canonical form as specified in 7.2
2. Computing SMPTE ST 2114:2017 identifier from the UTF-8 bytes

**Directory C4 IDs** use one-level canonical form: only direct children
(at depth 0 of the sub-manifest) are included. Subdirectories are represented
by their recursively-computed C4 IDs. This creates a Merkle tree.

> **IMPLEMENTATION NOTE**: `Manifest.ComputeC4ID()` copies the manifest,
> calls `Canonicalize()`, then computes the ID. `Canonicalize()` propagates
> metadata but does NOT substitute defaults for null mode or null timestamps
> — null values render as `-` in canonical form. This means two manifests
> with different null patterns produce different C4 IDs, which is correct
> (they represent different states of knowledge about the filesystem).

---

## 8. Natural Sort

### 8.1 Algorithm

Names are compared using natural ordering:

1. Files sort before directories at the same nesting level
2. Split each name into alternating text and numeric segments
3. Compare segments left-to-right:
   - **Both numeric**: compare as integers
   - **Equal integers**: shorter string representation sorts first
     (`"1"` < `"01"` < `"001"`)
   - **Both text**: compare by UTF-8 codepoint value
   - **Mixed** (one text, one numeric): text sorts before numeric
4. If all segments match, the shorter name sorts first

### 8.2 Definition of Numeric Segment

A numeric segment is a maximal contiguous run of ASCII digits (0x30-0x39).
Only ASCII digits participate in natural sort; Unicode digit characters are
treated as text.

---

## 9. Media File Sequences

### 9.1 Notation

```abnf
sequence-name  = prefix range-spec suffix
range-spec     = "[" range-body "]"
range-body     = range-item *("," range-item)
range-item     = number [ "-" number [ ":" number ] ]
number         = 1*DIGIT
```

Examples:
- `frame.[0001-0100].exr` — contiguous range
- `frame.[0001-0100:2].exr` — stepped (every 2nd frame)
- `frame.[0001-0050,0075-0100].exr` — discontinuous
- `frame.[0001,0005,0010].exr` — individual members

The sequence pattern regex used by the implementation:
```
\[([0-9,\-:]+)\]
```

### 9.2 Sequence vs Literal Brackets

- Names with escaped brackets (`\[`, `\]`) are NEVER interpreted as sequences
- Names containing unescaped `[digits,hyphens,colons]` that match the
  sequence pattern are treated as sequences
- Literal brackets in non-sequence names MUST be escaped: `\[`, `\]`

### 9.3 Sequence Properties

- **Size**: Sum of all member file sizes
- **Timestamp**: Most recent modification time among all members
- **C4 ID**: Hash of the bare-concatenated member C4 IDs (90 chars per
  member, no separators). This content is a regular file in the store
  or may be inlined (see Section 10.9).
- All members MUST be the same entry type
- Sequences with any null member C4 IDs MUST NOT be folded

---

## 10. Patch Format

### 10.1 Bare C4 ID Lines

```abnf
bare-c4id = c4-id
          ; exactly 90 characters, starts with "c4"
          ; appears on a line with no other content (after trimming)
```

A line containing only a C4 ID (after whitespace trimming) acts as a patch
boundary. The decoder detects this by checking: length == 90 AND first two
characters are `c4`.

### 10.2 First-Line Bare C4 ID (External Base Reference)

When the first non-blank line of a stream is a bare C4 ID, it is treated as
an **external base reference**. The C4 ID identifies a manifest that the
consumer must obtain independently.

- The ID is stored on `Manifest.Base`
- No verification is performed (there is no accumulated content to verify
  against)
- Entries following this line are a patch applied against the external base

```
c4<base-manifest-id>
-rw-r--r-- 2025-01-01T00:00:00Z 200 added.txt c4...
```

### 10.3 Subsequent Bare C4 ID (Inline Checkpoint)

When a bare C4 ID appears after entries have been accumulated, it is an
inline checkpoint.

**Verification requirement**: The bare C4 ID MUST match the C4 ID of the
manifest as currently accumulated (all preceding entries and applied patches).
If it does not match, the decoder MUST reject the stream with
`ErrPatchIDMismatch`.

**State transition**: Upon successful verification, the decoder enters patch
mode. All subsequent entries (until the next bare C4 ID or EOF) are
interpreted as a patch against the verified state.

### 10.4 Patch Entry Semantics

When applying a patch, entries are matched against the current manifest
state by **name** (within the same parent context for nested entries):

| Condition | Interpretation |
|-----------|---------------|
| Name does not exist in current state | **Addition** |
| Name exists and entry differs in any compared field | **Modification** (patch entry replaces original) |
| Name exists and all compared fields are identical | **Removal** (entry is deleted) |

#### 10.4.1 Identity Comparison

Two entries are considered identical when ALL of the following match:

- `Name`
- `Mode`
- `Timestamp` (using time equality, not string comparison)
- `Size`
- `C4ID`
- `Target` (symlink target)
- `HardLink` (hard link group)
- `FlowDirection`
- `FlowTarget`

> **NOTE**: `IsSequence` and `Pattern` are derived from the name during
> parsing and are not compared independently. If a name contains sequence
> notation, it will be detected on re-parse.

#### 10.4.2 Directory Patch Application

For directories, patch application is recursive. When a directory entry
appears in the patch:

1. The directory entry itself is matched by name
2. If it's a modification (different metadata), the directory entry is replaced
3. Children of the patch directory entry are recursively applied against
   children of the base directory

This means a patch can modify files deep in a hierarchy by including only
the changed path: the parent directory entries provide context, and the
unchanged siblings are preserved from the base.

### 10.5 Empty Patch Rejection

Every patch section MUST contain at least one entry.

- A bare C4 ID at EOF with no following entries: `ErrEmptyPatch`
- Two consecutive bare C4 IDs (empty section between them): `ErrEmptyPatch`

### 10.6 Multiple Patches

A stream MAY contain multiple successive patches:

```
<initial entries>
c4<checkpoint-1>        ; verifies initial entries
<patch-1 entries>
c4<checkpoint-2>        ; verifies state after patch-1
<patch-2 entries>
```

Each checkpoint verifies the complete manifest state at that point (all
preceding content and patches applied).

### 10.7 Unclosed Patch at EOF

A patch section that reaches EOF without a closing bare C4 ID is valid.
The entries are applied as a patch, but no final verification occurs.

> **NOTE**: This means the final patch in a stream is not verified by a
> checkpoint. If stream integrity verification is required at the end, the
> producer SHOULD append a closing bare C4 ID after the final patch entries.

### 10.8 Inline Range Data

When a c4m stream contains sequence entries (Section 9) and no external
content store is available, the per-member ID lists MAY be inlined as
trailing lines within a patch section.

#### 10.8.1 Format

An inline ID list is a single line containing the bare concatenation of
all member C4 IDs in range order. Each C4 ID is exactly 90 characters.
The total line length MUST be a positive multiple of 90 and MUST be
greater than 90 (i.e., at least two concatenated IDs).

#### 10.8.2 Disambiguation

Inline ID list lines are distinguished from other line types by length:

| Line type | Length | Starts with |
|-----------|--------|-------------|
| Patch boundary | exactly 90 | `c4` |
| Inline ID list | >90, multiple of 90 | `c4` |
| Entry | varies | mode char or indent |

#### 10.8.3 Position

Inline ID list lines appear after entries and before the closing patch
boundary within a section:

```
<entries>
<inline ID list lines>
c4<checkpoint>
```

#### 10.8.4 Identity Exclusion

Inline ID list lines MUST NOT be included in manifest C4 ID computation.
They are supplementary transport data. Adding or removing inline ID list
lines MUST NOT change the manifest's C4 ID or any patch boundary values.

The C4 ID of an inline ID list (computed by hashing the line content as
bytes) MUST match the C4 ID of the corresponding sequence entry. This
provides self-verification.

#### 10.8.5 Decomposition

Inline ID list lines MAY be extracted to separate files named by the
range expression they correspond to (e.g., the line for sequence
`frames.[0001-0100].exr` is written to a file named
`frames.[0001-0100].exr`). The file content is the bare-concatenated
ID list, byte-identical to the inline line content.

### 10.9 Encoding Patches

The `EncodePatch` method writes a patch section:

1. Write the base manifest's C4 ID as a bare line
2. Write the patch entries using standard `Encode`

The `PatchDiff` function computes the minimal patch between two manifests:

- Traverses both manifests as trees
- Skips subtrees where the directory C4 IDs match (efficiency: for a
  million-file tree with one changed file, only entries along the path to
  that file are examined)
- Emits entries using patch semantics: additions, modifications (different
  metadata), and removals (exact duplicates)

---

## 11. Directory C4 IDs

### 11.1 Computation

The C4 ID of a directory is computed from a **one-level canonical manifest**:

1. Collect direct children only
2. For each child file: use its content C4 ID
3. For each child directory: recursively compute its C4 ID using this
   algorithm
4. Sort children: files before directories, then natural sort within each
   group
5. Format as canonical entries at depth 0, each terminated by LF
6. Compute SMPTE ST 2114:2017 identifier from the resulting UTF-8 bytes

This produces a Merkle tree: each directory's identity depends on the
identities of its children.

### 11.2 Identity Property

`c4 id <dir> | c4` produces the directory's C4 ID

That is: the C4 ID of a directory equals the C4 ID of its canonical c4m
listing piped through the C4 hash function.

---

## 12. Null Values

### 12.1 Null Value Summary

| Field | Null text | Internal value | Notes |
|-------|-----------|----------------|-------|
| Mode | `-` or `----------` | 0 (zero) | Canonical: `-` |
| Timestamp | `-` or `0` | Unix epoch (1970-01-01T00:00:00Z) | |
| Size | `-` | -1 | |
| C4 ID | `-` or absent | nil / zero-value ID | |
| Name | (no null) | — | Required field |

### 12.2 Progressive Resolution

Null values enable progressive resolution: a c4m file can be created
immediately with just filenames, then enriched with metadata as it becomes
available. Each resolution pass fills in additional fields.

| Phase | Mode | Timestamp | Size | C4 ID |
|-------|------|-----------|------|-------|
| 0 (names only) | `-` | `-` | `-` | `-` |
| 1 (stat) | resolved | resolved | resolved | `-` |
| 2 (full) | resolved | resolved | resolved | resolved |

---

## 13. Sorting

### 13.1 Entry Sort Order

Within a group of siblings (entries at the same depth under the same parent):

1. Files sort before directories
2. Within files: natural sort by name
3. Within directories: natural sort by name

### 13.2 Hierarchical Ordering

The flat entry list maintains depth-first traversal order:

```
file_a.txt            ; depth 0, file
file_b.txt            ; depth 0, file
dir_a/                ; depth 0, directory
  child_1.txt         ; depth 1, file
  subdir/             ; depth 1, directory
    deep.txt          ; depth 2, file
dir_b/                ; depth 0, directory
```

### 13.3 Deduplication

When sorting, if multiple entries at the same depth have the same name, the
**last occurrence wins** and earlier occurrences are discarded.

> **IMPLEMENTATION NOTE**: `Encoder.Encode()` sorts a copy of the manifest
> for output. The caller's manifest is not modified.

---

## 14. Validation

### 14.1 Fatal Errors

Decoders MUST reject streams containing:

| Condition | Error |
|-----------|-------|
| Line starting with `@` | `ErrInvalidEntry` |
| CR (0x0D) anywhere | Fatal parse error |
| Invalid UTF-8 | Fatal parse error |
| Missing required field (mode, timestamp, size, name) | `ErrInvalidEntry` |
| Name is `.`, `..`, `/`, or empty | `ErrPathTraversal` |
| Name contains path separator (`/` as non-trailing, `\`) | `ErrPathTraversal` |
| Duplicate path within same scope | `ErrDuplicatePath` |
| Bare C4 ID mismatch (non-first-line) | `ErrPatchIDMismatch` |
| Empty patch section | `ErrEmptyPatch` |
| Malformed flow target | `ErrInvalidFlowTarget` |

### 14.2 Accepted Variations

Decoders SHOULD accept without error:

- Ergonomic timestamp formats (converting to UTC)
- Comma-separated size values (stripping commas)
- Ten-dash null mode (`----------`)
- Inconsistent indentation width (with warning)
- Non-sorted entries (re-sorting when processing)

---

## 15. Security Considerations

### 15.1 Path Traversal Prevention

Names MUST NOT contain `..` (parent directory reference), `.` (current
directory reference as a complete name), or embedded path separators.
Decoders MUST reject such names with `ErrPathTraversal`.

### 15.2 Patch ID Verification

Bare C4 ID lines (after the first line) MUST be verified against the
accumulated manifest state. This prevents a class of attacks where a stream
claims to represent one manifest while actually containing different content.

The first-line exception for external base references is acceptable because:
- The human reader can see the base reference is external
- The base must be independently fetched and verified
- There is no accumulated content to misrepresent

### 15.3 Character Restrictions

- Null bytes (0x00): Forbidden in names. Encoded via SafeName Tier 2 (`\0`)
  if they appear in raw filesystem names.
- Control characters (0x00-0x1F): Forbidden in raw form. Encoded via
  SafeName (Tier 2 or Tier 3).
- Maximum line length: Implementation-defined (RECOMMENDED minimum: 1 MB).

---

## Appendix A: Collected ABNF Grammar

```abnf
; Stream
c4m-stream     = *( line LF )
line           = entry-line / bare-c4id / inline-idlist / blank-line
blank-line     = *SP
bare-c4id      = c4-id               ; exactly 90 chars
inline-idlist  = 2*c4-id             ; >90 chars, multiple of 90

; Entry
entry-line     = [indent] mode SP timestamp SP size SP name
                 [SP link-part] SP c4id-or-null
indent         = 1*SP

; Mode
mode           = type-char perm-chars / "-" / "----------"
type-char      = "-" / "d" / "l" / "p" / "s" / "b" / "c"
perm-chars     = 9perm-char
perm-char      = "r" / "w" / "x" / "s" / "S" / "t" / "T" / "-"

; Timestamp
timestamp      = date "T" time "Z" / ts-offset / "-" / "0"
date           = 4DIGIT "-" 2DIGIT "-" 2DIGIT
time           = 2DIGIT ":" 2DIGIT ":" 2DIGIT
ts-offset      = date "T" time ("+" / "-") 2DIGIT ":" 2DIGIT

; Size
size           = "0" / non-zero *DIGIT / "-"
non-zero       = %x31-39

; Name (backslash-escaped, no quoting)
name           = 1*name-char [ "/" ]
name-char      = escaped-char / safe-char
escaped-char   = "\" ( SP / DQUOTE / "[" / "]" )
safe-char      = <SafeName-encoded printable UTF-8>

; Link operators
link-part      = symlink-part / hardlink-part / flow-part
symlink-part   = "->" SP target
hardlink-part  = "->" [ group-num ]
group-num      = %x31-39 *DIGIT
flow-part      = flow-dir SP flow-target
flow-dir       = "->" / "<-" / "<>"
flow-target    = location ":" *VCHAR
location       = ALPHA *( ALPHA / DIGIT / "_" / "-" )
target         = 1*( target-char )
target-char    = "\" ( SP / DQUOTE ) / <printable UTF-8>
               ; ends at c4-id boundary or "-" or EOL

; C4 ID
c4id-or-null   = c4-id / "-"
c4-id          = "c4" 88BASE58CHAR
BASE58CHAR     = %x31-39 / %x41-48 / %x4A-4E / %x50-5A
               / %x61-6B / %x6D-7A
```

---

## Appendix B: Error Codes

| Error | Sentinel | Description |
|-------|----------|-------------|
| Invalid entry | `ErrInvalidEntry` | Malformed entry line (missing fields, bad mode, directive line) |
| Duplicate path | `ErrDuplicatePath` | Same path appears twice in manifest scope |
| Path traversal | `ErrPathTraversal` | Name contains `.`, `..`, or path separators |
| Invalid flow target | `ErrInvalidFlowTarget` | Flow target does not match `location:path` pattern |
| Patch ID mismatch | `ErrPatchIDMismatch` | Bare C4 ID does not match accumulated manifest state |
| Empty patch | `ErrEmptyPatch` | Patch section contains zero entries |

---

## Appendix C: Design Notes

### C.1 Unclosed Final Patch

The final patch section in a stream is not verified by a closing checkpoint
(see Section 10.7). This is by design for streaming use cases where the
final C4 ID may not be known at write time. Stream consumers who require
end-to-end verification should compute the manifest's C4 ID after decoding
and compare against an expected value.

### C.2 Null Mode Dual Representation

Null mode has two text representations: `-` (canonical) and `----------`
(ergonomic). The encoder uses `----------` in standard mode and `-` in
canonical mode. Both are accepted on input. The single-dash form is
normative for C4 ID computation.
