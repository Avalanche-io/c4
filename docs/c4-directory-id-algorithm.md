# C4 Directory ID Generation Algorithm

## Overview

The C4 ID of a directory is computed from its **canonical c4m representation**. This ensures that `c4 -i .` equals `c4 . | c4`.

## Key Principle

A directory's C4 ID is computed from a **one-level C4M manifest** where:
- Files are listed with their C4 IDs
- Subdirectories are listed as single entries with their computed C4 IDs
- Subdirectory contents are NOT expanded inline

## Algorithm Steps

### Step 1: Generate One-Level Manifest

When computing a directory's C4 ID:

1. List all entries in the directory (files and subdirectories)
2. For each regular file:
   - Compute its C4 ID from content
   - Include: mode, timestamp, size, name, C4 ID
3. For each subdirectory:
   - **Recursively compute its C4 ID** using this same algorithm
   - Include: mode, timestamp, size, name + "/", computed C4 ID
   - Do NOT include the subdirectory's contents

### Step 2: Create Canonical Form

The canonical form is generated from the one-level manifest:

1. Take only top-level entries (depth 0)
2. Sort entries:
   - Files come before directories
   - Within each group, use natural sort
3. Format each entry without indentation:
   - `mode timestamp size name [C4ID]`

### Step 3: Compute C4 ID

The directory's C4 ID is computed by:
1. Converting the canonical form to UTF-8 bytes
2. Computing the C4 ID of those bytes

## Example

Given directory structure:
```
mydir/
  file1.txt (C4 ID: c41abc...)
  file2.txt (C4 ID: c41def...)
  subdir/
    file3.txt (C4 ID: c41ghi...)
```

### Computing subdir's C4 ID:
1. Generate one-level manifest for subdir:
   ```
   -rw-r--r-- 2024-01-01T00:00:00Z 100 file3.txt c41ghi...
   ```
2. Canonical form (same, as it's already one level)
3. Compute C4 ID of canonical form → `c42sub...`

### Computing mydir's C4 ID:
1. Generate one-level manifest:
   ```
   -rw-r--r-- 2024-01-01T00:00:00Z 50 file1.txt c41abc...
   -rw-r--r-- 2024-01-01T00:00:00Z 60 file2.txt c41def...
   drwxr-xr-x 2024-01-01T00:00:00Z 4096 subdir/ c42sub...
   ```
2. Canonical form (sorted, files first)
3. Compute C4 ID of canonical form → `c42dir...`

## Why This Works

This algorithm ensures `c4 -i .` equals `c4 . | c4` because:

1. `c4 -i .` computes the directory's C4 ID using the algorithm above
2. `c4 .` outputs the c4m listing used in step 1
3. When piped to `c4`, it recognizes the c4m format and computes the C4 ID of the canonical form
4. Both paths use the exact same canonical form

## Key Implementation Details

### In `cmd/c4/main.go`:
- `processDirectory()` uses `generateOneLevel()` for both ID generation and manifest output
- `generateOneLevel()` recursively computes subdirectory C4 IDs

### In `c4m/manifest.go`:
- `Canonical()` extracts only top-level entries (minimum depth)
- Sorts with files before directories
- Uses natural sort within each group

### Recursion
- Subdirectory C4 IDs are computed recursively
- Each subdirectory's C4 ID is based on its own one-level manifest
- This creates a merkle-tree-like structure

## Critical Insight

The initial implementation incorrectly:
1. Generated full recursive manifests for directories
2. Or used the same C4 ID for all directories with similar structure

The fix was to ensure:
1. Directory C4 IDs are computed from one-level manifests
2. Subdirectory C4 IDs are computed recursively
3. The canonical form only includes top-level entries

This creates unique, deterministic C4 IDs that correctly represent directory contents.