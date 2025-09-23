# C4 Manifest Format (C4M) Specification v1.0

> **Note**: This is the formal C4M specification. For implementation notes and edge cases, see [IMPLEMENTATION_NOTES.md](./IMPLEMENTATION_NOTES.md). For user documentation, see [README.md](./README.md).

## Overview

The C4 Manifest Format (.c4m) is a text-based (UTF-8) format for describing filesystem contents with content-addressed identification using C4 IDs (SMPTE ST 2114:2017). It preserves filesystem metadata while enabling content verification, deduplication, and distributed workflows.

## Core Requirements

### File Structure

1. **Mandatory Version Header**: First line must be `@c4m 1.0`
2. **Character Encoding**: UTF-8 only, no BOM
3. **Line Endings**: LF (0x0A) only, no CR
4. **Line Format**: `<indentation><mode> <timestamp> <size> <name> [-> target] [c4id]`

### Field Specifications

- **Indentation**: Spaces indicating nesting level (consistent width throughout file)
- **Mode**: Unix-style permissions (10 characters, e.g., `-rw-r--r--`, `drwxr-xr-x`)
- **Timestamp**: ISO 8601 format ending in 'Z' (UTC only): `YYYY-MM-DDTHH:MM:SSZ`
- **Size**: File size in bytes (decimal digits only in canonical form)
- **Name**: File/directory name (directories end with `/`)
- **Target**: For symlinks, preceded by ` -> `
- **C4 ID**: Optional but typical, linking to content (see Symlinks section for special rules)

## Canonical Form

For computing directory C4 IDs, content must be transformed to canonical form:

1. Remove all leading indentation
2. Single space between fields
3. No padding or alignment spaces
4. No comma separators in sizes
5. No leading zeros in sizes (except "0" itself)
6. Natural sort ordering (see below)

## Ergonomic Forms

While the canonical form is required for C4 ID computation, parsers SHOULD accept and tools MAY output ergonomic forms for improved human readability:

### Allowed Ergonomic Variations

1. **Padded Size Fields**: Right-align size values with spaces to match the width of the largest size in the manifest
   ```
   -rw-r--r-- 2024-01-01T00:00:00Z     100 small.txt
   -rw-r--r-- 2024-01-01T00:00:00Z   1,234 medium.txt
   -rw-r--r-- 2024-01-01T00:00:00Z 100,000 large.txt
   ```

2. **Local Timestamps**: Display in human-readable format with timezone (must still parse to UTC internally)
   ```
   # ISO 8601 format with timezone offset
   -rw-r--r-- 2024-01-01T10:00:00-08:00 1024 file.txt
   
   # Unix ls-style format (preferred for ergonomic display)
   -rw-r--r-- Jan  1 10:00:00 2024 PST 1024 file.txt
   -rw-r--r-- Sep 15 14:30:45 2024 CDT 1024 file2.txt
   ```
   
   The Unix ls-style format provides better readability while preserving second precision for accurate conversion.

3. **Column-Aligned C4 IDs**: Align all C4 IDs at a consistent column position
   ```
   -rw-r--r-- 2024-01-01T00:00:00Z 100 a.txt          c41abc...
   -rw-r--r-- 2024-01-01T00:00:00Z 200 longer.txt     c41def...
   -rw-r--r-- 2024-01-01T00:00:00Z 300 very_long.txt  c41ghi...
   ```

### Column Alignment Rules

For column-aligned C4 IDs:
- Start at column 80 by default
- If the longest line (excluding C4 ID) exceeds column 70, shift to the next 10-column boundary
- Maintain at least 2 spaces between the rightmost content and the C4 ID column
- Example column positions: 80, 90, 100, 110, etc.

### Parser Requirements

Parsers MUST:
- Accept all ergonomic forms as valid input
- Convert to canonical form before computing C4 IDs
- Preserve ergonomic formatting in pass-through operations when possible

### Pretty-Print Mode

Tools SHOULD offer a pretty-print mode that outputs ergonomic form with:
- Padded size fields with comma separators for thousands
- Column-aligned C4 IDs based on content width
- Consistent indentation for nested entries
- Optional local timezone display

## Natural Sort Algorithm

Files are sorted using natural ordering that handles numeric sequences intelligently:

1. Split filenames into alternating text/numeric segments
2. Compare segments left-to-right:
   - Numeric segments: compare as integers
   - Equal integers: shorter representation first (e.g., "1" < "01" < "001")
   - Text segments: UTF-8 codepoint comparison
   - Mixed types: text sorts before numeric
3. Files sort before directories at same level

### Examples
```
file1.txt
file2.txt
file10.txt      # Not file1.txt, file10.txt, file2.txt
render.1.exr
render.01.exr   # Sorted after render.1.exr (equal value, longer)
render.2.exr
render.10.exr
```

## Quoting and Escaping

### Quoted Names
Names must be quoted when containing:
- Spaces
- Quotes (escaped as `\"`)
- Backslashes (escaped as `\\`)
- Newlines (escaped as `\n`)
- Leading/trailing whitespace

### Unquoted Names
- No spaces allowed (except escaped in sequence notation, see below)
- No special character escaping outside of sequence notation

### Escaping in Sequence Notation
In unquoted sequence patterns, backslash escapes are required for characters that would be ambiguous:

**Must be escaped**:
- Space: `\ ` 
- Square brackets: `\[`, `\]` (when literal, not part of range)
- Backslash itself: `\\`
- Quote marks: `\"`
- Comma: `\,` (when literal, not a range separator)
- Hyphen: `\-` (when literal, not a range indicator)

**Examples**:
```
# Spaces in base filename
my\ animation.[001-100].exr         # Expands to "my animation.001.exr"

# Literal brackets in filename
file\[test\].[001-010].dat         # Expands to "file[test].001.dat"

# Literal comma in base name
data\,backup.[01-05].csv           # Expands to "data,backup.01.csv"

# Multiple escapes
my\ file\[v2\].[001-100].exr       # Expands to "my file[v2].001.exr"

# Backslash itself
path\\to\\file.[001-010].txt       # Expands to "path\to\file.001.txt"
```

**Note**: Quoted filenames are never interpreted as sequences - they are always literal. Use quoting when the entire filename should be treated as-is, use unquoted with escapes when you need sequence expansion.

## Symlinks (Symbolic Links)

Symlinks are preserved as distinct filesystem entities with special C4 ID handling:

### C4 ID Rules for Symlinks

1. **Symlink to regular file**: The C4 ID is computed from the **target file's content**
   ```
   lrwxrwxrwx 2024-01-01T00:00:00Z 0 link.txt -> file.txt c41abc...
   # c41abc... is the C4 ID of file.txt's content
   ```

2. **Symlink to symlink**: Empty/nil C4 ID (prevents infinite recursion)
   ```
   lrwxrwxrwx 2024-01-01T00:00:00Z 0 link1 -> link2
   # No C4 ID or empty C4 ID shown
   ```

3. **Broken symlink**: Empty/nil C4 ID (target doesn't exist)
   ```
   lrwxrwxrwx 2024-01-01T00:00:00Z 0 broken -> nonexistent
   # No C4 ID or empty C4 ID shown
   ```

4. **Symlink to directory**: C4 ID computed from target directory's manifest
   ```
   lrwxrwxrwx 2024-01-01T00:00:00Z 0 linkdir -> targetdir/ c42xyz...
   # c42xyz... is the C4 ID of targetdir's manifest
   ```

### Symlink Preservation

- Symlinks are preserved as-is, including circular references, out-of-scope targets, and broken links
- The manifest documents the filesystem state without judgment or correction
- Target paths are stored exactly as they appear in the filesystem (relative or absolute)

### Symlink Ranges

When multiple symlinks follow a numeric pattern, they can be represented as ranges:

**Uniform target mapping** (all follow the same pattern):
```
lrwxrwxrwx 2024-01-01T00:00:00Z 0 render.[0001-0100].exr -> /cache/source.[0001-0100].exr
```

**Non-uniform targets** (mixed or irregular patterns):
```
lrwxrwxrwx 2024-01-01T00:00:00Z 0 render.[0001-0100].exr -> ...
```

The `...` notation indicates that individual symlinks have different target patterns that cannot be expressed as a single range. The `@expand` directive or referenced C4 ID provides the complete mapping.

### Generator Options

Implementations should provide options for symlink handling:
- **Preserve mode** (default): Keep symlinks as symlinks with target C4 IDs
- **Follow mode**: Resolve symlinks to their targets (collapse symlinks)

## Media File Sequences

Sequences provide compact representation of numbered files:

### Notation Patterns
- Contiguous: `frame.[0001-0100].exr`
- Stepped: `frame.[0001-0100:2].exr` (every other frame)
- Discontinuous: `frame.[0001-0050,0075-0100].exr`
- Individual: `frame.[0001,0005,0010].exr`

### Sequence C4 IDs
The C4 ID of a sequence is computed exactly like a directory C4 ID - from the canonical form of its expanded entries:
1. Expand the sequence to individual entries
2. Create canonical form (sorted, one entry per line)
3. Compute C4 ID from the canonical form

This treats the sequence as a virtual container of its members.

### Size and Timestamp Rules
**Size**: The size of a sequence is the sum of all member file sizes
```
frame.[001-003].exr 300  # If frame.001.exr=100, frame.002.exr=100, frame.003.exr=100
```

**Timestamp**: The timestamp of a sequence is the most recent modification time among all members
```
# If frame.001.exr modified at 10:00, frame.002.exr at 11:00, frame.003.exr at 10:30
frame.[001-003].exr  # Uses 11:00 (most recent)
```

### Rules
- Quoted names are never sequences (always literal)
- Sequence C4 ID references expansion (inline or external)
- Files in sequence need not share modification times or C4 IDs
- Directory sequences supported: `shot_[001-100]/`
- All members must be the same entry type (all files, all directories, or all symlinks)

## Layer System

Layers enable changeset representation without duplicating unchanged content:

### Layer Types

#### @base
References a base manifest:
```
@base c4<ID of base manifest>
```

#### @remove
Lists entries to remove:
```
@remove
@by "Joshua Kolden"
@note "Removing deprecated files"
drwxr-xr-x 2023-01-01T12:00:00Z 1024 old-lib/
```

#### @layer
Adds/modifies entries:
```
@layer
@by "Jane Smith"
@time 2025-02-27T15:00:00Z
@note "Security update"
drwxr-xr-x 2023-01-01T12:00:00Z 4096 lib/
  -rw-r--r-- 2023-01-01T12:00:00Z 2048 secure.so c4ABC...
```

#### @expand
Provides inline expansion of sequences:
```
-rw-r--r-- 2023-01-01T12:00:00Z 10000 frames.[01-03].exr c4XYZ...

@expand c4XYZ
-rw-r--r-- 2023-01-01T12:00:00Z 3000 frames.01.exr c4AAA...
-rw-r--r-- 2023-01-01T12:00:00Z 3500 frames.02.exr c4BBB...
-rw-r--r-- 2023-01-01T12:00:00Z 3500 frames.03.exr c4CCC...
```

## Metadata Keywords

Can follow any @ directive that starts a section:

- `@by`: Who made the change
- `@time`: When the change occurred (ISO 8601/RFC 3339)
- `@note`: Human-readable comment
- `@data`: C4 ID reference to application-specific metadata

## Directory C4 IDs and Metadata

### Directory Sizes
The size field for a directory should represent the **total size of all contents** (recursive):
- Sum of all file sizes within the directory and all subdirectories
- Does not include filesystem metadata overhead
- Example: A directory containing two 100-byte files shows size 200

### Directory Timestamps
Directory timestamps come directly from the filesystem's modification time for the directory itself (not derived from contents).

### Computing Directory C4 IDs

Directory C4 IDs are computed from a **one-level canonical C4M representation**:

1. **Generate one-level manifest**:
   - List direct children only (files and subdirectories)
   - For files: compute C4 ID from content
   - For subdirectories: recursively compute their C4 ID using this same algorithm
   - Do NOT expand subdirectory contents inline

2. **Create canonical form**:
   - Use only depth-0 entries (top level)
   - Sort: files before directories, then natural sort within each group
   - Format without indentation: `<mode> <timestamp> <size> <name> [c4id]`
   - Single space between fields, no alignment

3. **Compute C4 ID**:
   - Generate C4 ID from the UTF-8 bytes of the canonical form

### Example
```
# Directory structure:
mydir/
  file1.txt    # C4 ID: c41abc...
  subdir/      # Contains files
    file2.txt  # C4 ID: c41def...

# One-level manifest for mydir:
-rw-r--r-- 2024-01-01T00:00:00Z 100 file1.txt c41abc...
drwxr-xr-x 2024-01-01T00:00:00Z 4096 subdir/ c42xyz...

# The C4 ID c42xyz... for subdir is computed from its own one-level manifest
```

This approach ensures:
- Each directory has a unique C4 ID based on its contents
- Subdirectories are represented by their computed C4 IDs (merkle-tree structure)
- The command `c4 <dir>` equals `c4 -m <dir> | c4`

## Null and Zero Values

C4M supports null/zero values for fields to enable boolean set operations and manual manifest creation:

### Null Value Representations

1. **Mode**: 
   - `-` (single dash) or `----------` (ten dashes)
   - Represents unspecified permissions (zero value)
   - Used when only file existence matters, not permissions

2. **Timestamp**:
   - `-` (single dash) or `0` (zero)
   - Parsed as Unix epoch (1970-01-01T00:00:00Z)
   - Used for comparison operations where oldest-first ordering is desired

3. **Size**:
   - `-` (single dash)
   - Internally represented as -1
   - Used when file size is unknown or irrelevant

4. **C4 ID**:
   - `-` (single dash) or omitted
   - Represents zero/nil C4 ID
   - Used for incomplete manifests in set operations

5. **Name**:
   - Cannot be null (required field)

### Use Cases

- **Boolean Operations**: Create manifests with only names to define sets
- **Comparison Operations**: Use zero timestamps to ensure "oldest" ordering
- **Manual Editing**: Leave fields unspecified when exact values are unknown
- **Template Manifests**: Create patterns for matching operations

### Example

```
@c4m 1.0
---------- - - important.txt -
-rw-r--r-- 0 100 oldest.txt
- 2024-01-01T00:00:00Z - template.txt
```

## Validation Requirements

Parsers MUST:
- Verify first line is `@c4m X.Y`
- Reject invalid UTF-8 sequences
- Apply canonical transformation consistently
- Verify C4 IDs match content (when available)
- Accept null values as specified above

Parsers MAY:
- Accept non-sorted entries (with warning)
- Accept inconsistent indentation (with warning)
- Resort entries when processing

## Security Considerations

- No path traversal (`../`, `./`)
- No null bytes in names
- Control characters forbidden (0x00-0x1F except tab)
- Maximum line length: implementation-defined (suggested 1MB)
