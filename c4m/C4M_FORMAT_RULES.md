# C4M Format Rules and Chunking Specification

## Core Format Rules

### 1. Root Directory Exclusion
- The root directory being scanned MUST NEVER appear in any chunk
- Only its contents should be included
- Example: When scanning `/Users/joshua/ws/active`, the directory `active/` itself should not appear

### 2. Files Before Directories
- At any given depth level, ALL files MUST appear before ANY directories
- This rule applies within each parent directory context
- Violation example:
  ```
  drwxr-xr-x dir1/     # WRONG: directory before file
  -rw-r--r-- file1.txt
  ```
- Correct example:
  ```
  -rw-r--r-- file1.txt  # Correct: files first
  drwxr-xr-x dir1/      # Then directories
  ```

### 3. Depth Ordering
- Depth can only INCREASE by 1 at a time
- Depth can DECREASE by any amount
- Each indentation level is 2 spaces per depth

### 4. Directory Naming
- Directory names MUST end with `/`
- Directory names should NOT be quoted even if they contain spaces
- Example: `Alex /` is valid (note the space before the slash)

### 5. Sorting and Ordering Rules
- **Primary sort**: By type (files first, then directories)
- **Secondary sort**: Within each type, sort by name using natural sort order
- **Important**: Whether a directory is collapsed or not does NOT affect sort order
- Processing order at any depth level:
  1. All files (sorted naturally by name)
  2. All directories (sorted naturally by name, regardless of collapsed status)
- Natural sort examples:
  - Correct: file1.txt, file2.txt, file10.txt (not file1.txt, file10.txt, file2.txt)
  - Numbers within names are compared numerically
  - "2" comes before "10" when compared as numbers

## Chunking Scenarios

### Scenario 1: Collapsed Directory (Isolated Chunk Series)

**When:** A directory contains >70% of max chunk capacity

**Important**: Collapsing is an implementation optimization, NOT a different type of entry. Collapsed directories appear in the exact same sorted position as they would if not collapsed.

**Characteristics:**
- Directory gets its own isolated chunk series
- The directory entry itself NEVER appears in its own chunks
- Chunks start at depth 0 (as if scanning in isolation)
- Parent manifest references the directory with C4 ID of the LAST chunk

**Chunk Structure:**
```
# Parent chunk:
-rw-r--r-- file1.txt
-rw-r--r-- file2.txt
drwxr-xr-x another_dir/      # Regular dir (alphabetically before large_dir)
  -rw-r--r-- nested.txt
drwxr-xr-x large_dir/ c4xxx...  # Collapsed dir in alphabetical order
drwxr-xr-x zebra_dir/        # Another regular dir (alphabetically after)

# Collapsed directory chunk 1:
@c4m 1.0
-rw-r--r-- fileA.txt    # At depth 0
-rw-r--r-- fileB.txt
drwxr-xr-x subdir1/     # At depth 0
  -rw-r--r-- fileC.txt  # At depth 1

# Collapsed directory chunk 2 (if needed):
@c4m 1.0
@base c4xxx...  # References chunk 1
drwxr-xr-x subdir2/     # Continue at depth 0
  -rw-r--r-- fileD.txt
```

### Scenario 2: Regular Continuation (Mid-Scan Chunking)

**When:** Hit chunk limit while scanning normally

**Characteristics:**
- Must preserve full parent path context
- Continuation chunks use @base directive
- Must restate parent directories at start of continuation

**Example:**
```
# Chunk 1:
@c4m 1.0
-rw-r--r-- root_file.txt
drwxr-xr-x foo/
  drwxr-xr-x bar/
    -rw-r--r-- file1.txt
    -rw-r--r-- file2.txt

# Chunk 2 (continuation):
@c4m 1.0
@base c4xxx...
drwxr-xr-x foo/
  drwxr-xr-x bar/
    -rw-r--r-- file3.txt  # Continue where we left off
```

## Critical Implementation Notes

1. **Never mix scenarios** - A chunk is either:
   - Part of normal scanning (may need continuation)
   - Part of a collapsed directory series
   - But never both

2. **Collapsed directories are atomic** - Once you start a collapsed directory:
   - Complete ALL its chunks before returning to parent
   - Use consistent depth (starting at 0) throughout

3. **Continuation state tracking** - When in continuation mode:
   - Track the exact path where continuation started
   - Preserve parent chain for restating
   - Clear state when starting fresh chunk

4. **Processing order is critical**:
   - Read directory contents
   - Separate files and dirs
   - Sort each group naturally by name
   - Identify which dirs will be collapsed (>70% capacity)
   - Process all files first
   - Process all directories in sorted order (collapsed or not)
   - Collapsed dirs: spawn separate scan, use resulting C4 ID
   - Regular dirs: process inline with nested content

## Symlink Handling (Implementation Detail)

### Current Implementation
- Symlinks currently do NOT have trailing slashes, even when pointing to directories
- This is technically incorrect per spec but not the current priority

### Correct Specification (To Be Implemented)
- Symlinks to directories SHOULD be treated as directories with format:
  `lrwxrwxrwx timestamp 0 dirname/ -> target_path/ c4xxx...`
- The trailing slash on the name indicates it's a directory symlink
- The C4 ID would be computed from the target directory's manifest

### Note
This is a known deviation from the specification. The large filesystem scan should pass without this fix first, then this can be addressed in a future update.