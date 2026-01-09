# C4M Range Format Specification

## Overview

Ranges (also called sequences) provide a compact representation for ordered collections of files that share metadata but have unique content. This is particularly useful for VFX/animation frame sequences, numbered backups, and similar patterns.

## Motivation

A 10,000-frame animation sequence would traditionally require 10,000 manifest entries:

```
-rw-r--r-- 2025-01-01T10:00:00Z 3000 frame_0001.exr c4...A
-rw-r--r-- 2025-01-01T10:00:01Z 3000 frame_0002.exr c4...B
-rw-r--r-- 2025-01-01T10:00:02Z 3000 frame_0003.exr c4...C
... (9,997 more lines)
```

With ranges, this becomes a single line plus an ID list:

```
-rw-r--r-- 2025-01-01T10:00:00Z 30000000 frame_[0001-10000].exr c4...X
```

## Range Entry Format

A range entry follows the standard c4m entry format:

```
<mode> <timestamp> <size> <pattern> <c4id>
```

Where:
- **mode**: Most restrictive permissions among all files in the range
- **timestamp**: Latest modification time among all files in the range
- **size**: Total size of all files in the range (sum)
- **pattern**: Filename with range notation (e.g., `file_[001-100].txt`)
- **c4id**: C4 ID of the ID list file

### Pattern Syntax

Range patterns use bracket notation with zero-padded numbers:

```
frame_[0001-1000].exr     # frames 0001 through 1000
shot_a_[001-050]_v2.dpx   # shot_a_001_v2.dpx through shot_a_050_v2.dpx
backup_[1-99].tar         # backup_1.tar through backup_99.tar (no padding)
```

The padding width is determined by the start value notation.

## ID List File

The C4 ID on a range entry references an "ID list file" - a file containing only the C4 IDs of each file in the range, in order.

### Format

One C4 ID per line, trailing newline on each line:

```
c41abc...xyz
c41def...uvw
c41ghi...rst
```

### Canonical Form

For C4 ID computation, the ID list must be in canonical form:
- One C4 ID per line
- No leading or trailing whitespace on lines
- No blank lines
- Trailing newline after each ID (including the last)
- IDs in sequence order

### Whitespace Tolerance

Parsers should gracefully accept:
- Leading/trailing whitespace on lines
- Blank lines between IDs
- Missing final newline

Before any operation (validation, ID computation), normalize to canonical form.

## Metadata Aggregation

When creating a range from individual files:

| Field | Aggregation | Rationale |
|-------|-------------|-----------|
| Size | Sum | Total storage footprint |
| Timestamp | Latest | Most recent modification |
| Mode | Most restrictive | Security - never grant excess permissions |

### Example

Input files:
```
frame_001.exr  -rw-r--r-- (0644)  2025-01-01T10:00:00Z  3000 bytes
frame_002.exr  -rw-r----- (0640)  2025-01-01T10:05:00Z  3200 bytes
frame_003.exr  -rw-r--r-- (0644)  2025-01-01T10:03:00Z  2800 bytes
```

Resulting range entry:
```
-rw-r----- 2025-01-01T10:05:00Z 9000 frame_[001-003].exr c4...X
```

Where `c4...X` is the C4 ID of:
```
c4<id-of-frame_001.exr>
c4<id-of-frame_002.exr>
c4<id-of-frame_003.exr>
```

## Expansion

To expand a range entry into individual file entries:

1. Parse the filename pattern to get the numeric range
2. Fetch the ID list file by its C4 ID
3. For each number in the range (in order):
   - Generate the filename from the pattern
   - Take the corresponding C4 ID from the list
   - Apply the shared metadata (mode, timestamp)
   - Size becomes unknown (null) or can be fetched from content storage

### Expanded Metadata

On expansion:
- **mode**: All files get the range's mode
- **timestamp**: All files get the range's timestamp
- **size**: Individual file sizes are not preserved in the range format; use null (-1) or fetch from storage
- **c4id**: Each file gets its individual C4 ID from the list

## Self-Contained Manifests: @data Directive

Manifests can optionally embed ID list files inline using the `@data` directive, making them self-contained without external fetches.

### Syntax

```
@data <c4id>
<content>
```

Content runs until the next `@` directive or EOF.

### Content Encoding

**C4 ID lists** (detected automatically): Plain text, one ID per line
```
@data c4...X
c4...A
c4...B
c4...C
```

**Arbitrary binary/text content**: Base64 encoded, 76-character line limit
```
@data c4...Y
SGVsbG8gV29ybGQhIFRoaXMgaXMgYSB0ZXN0IGZpbGUuTm90
IGp1c3QgQzQgSURzLCBzbyBpdCBuZWVkcyBhcm1vcmluZy4K
```

### Detection Rules

Content is treated as a plain C4 ID list if and only if every non-empty line (after whitespace trimming) matches the C4 ID pattern: `c4[1-9A-Za-z]{87}`

Otherwise, content is interpreted as base64-encoded binary data.

### Validation

The decoded content must hash to the declared C4 ID. A mismatch indicates a corrupt or tampered manifest.

## Complete Example

```
@c4m 1.0
-rw-r--r-- 2025-01-01T09:00:00Z 1024 readme.txt c4...R
-rw-r----- 2025-01-01T10:05:00Z 30000000 frame_[0001-10000].exr c4...X
drwxr-xr-x 2025-01-01T10:05:00Z 0 renders/

@data c4...X
c4...frame0001
c4...frame0002
c4...frame0003
... (9,997 more IDs)
c4...frame10000
```

This manifest is fully self-contained. The range line can also be shared independently - anyone with access to content-addressed storage can fetch `c4...X` to expand the range.

## Atomicity

A key design principle: **a single range line contains all information needed to materialize the files**.

```
-rw-r----- 2025-01-01T10:05:00Z 9000 frame_[001-003].exr c4...X
```

This line alone tells you:
- The filename pattern and count (3 files)
- The shared metadata (permissions, timestamp)
- Total size (9000 bytes)
- Where to find the individual C4 IDs (`c4...X`)

Users can copy/paste a single range line to share a sequence.

## Implementation Notes

### Creating Ranges

1. Detect sequence patterns in filenames
2. Verify all files match the pattern with sequential numbers
3. Collect individual C4 IDs in order
4. Compute aggregate metadata (sum size, latest timestamp, most restrictive mode)
5. Create ID list file, compute its C4 ID
6. Generate range entry

### Parsing Ranges

1. Detect range pattern in filename (bracket notation)
2. Store as a special entry type with pattern and ID list reference
3. On expansion request, fetch ID list and generate individual entries

### Storage

ID list files are stored in the same content-addressed storage as any other content. They're just text files with a specific format.
