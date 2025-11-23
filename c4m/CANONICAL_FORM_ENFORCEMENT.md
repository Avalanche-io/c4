# Canonical Form Enforcement: Design Document

## Document Status
**Status**: Design
**Created**: 2025-10-31
**Purpose**: Specify required changes to enforce canonical form rules when computing C4 IDs

---

## Section A: Problem Statement

### Critical Flaw Discovered

The c4m package currently allows computing C4 IDs from manifests containing null values (represented as `-` in the text format, or zero/negative/nil values in the Go structs). This creates a critical architectural flaw that violates the fundamental principle of C4: **content-addressable identification must be deterministic**.

### Concrete Example of Non-Determinism

Consider a directory containing a single file `test.txt` with 100 bytes of content. Due to the current implementation, this directory can produce **different C4 IDs** depending on how null values are represented:

**Scenario 1: Explicit timestamp**
```
drwxr-xr-x 2024-01-01T00:00:00Z 100 mydir/
  -rw-r--r-- 2024-01-01T00:00:00Z 100 test.txt c41abc...
```
Computes to C4 ID: `c42xyz123...`

**Scenario 2: Null timestamp (represented as `-`)**
```
drwxr-xr-x - 100 mydir/
  -rw-r--r-- - 100 test.txt c41abc...
```
When this manifest's Canonical() method is called, it outputs:
```
drwxr-xr-x 1970-01-01T00:00:00Z 100 mydir/
  -rw-r--r-- 1970-01-01T00:00:00Z 100 test.txt c41abc...
```
Computes to C4 ID: `c42different...` (DIFFERENT!)

**Scenario 3: Null mode**
```
---------- 2024-01-01T00:00:00Z 100 mydir/
  ---------- 2024-01-01T00:00:00Z 100 test.txt c41abc...
```
Computes to yet another C4 ID: `c42another...` (DIFFERENT AGAIN!)

### Why This Violates C4 Principles

The C4 system is built on a fundamental guarantee:

> **Same content always produces the same C4 ID, everywhere, forever**

This guarantee is what makes C4 useful for:
- **Deduplication**: Identical content detected by matching IDs
- **Verification**: Content integrity proven by recomputing ID
- **Distribution**: Content referenced unambiguously across systems
- **Immutability**: Changes detectable by ID mismatch

When null values affect C4 ID computation:
1. The same filesystem state produces multiple valid IDs
2. Content deduplication fails (same content, different IDs)
3. Verification becomes unreliable (which ID is "correct"?)
4. Distributed workflows break (different tools produce different IDs)

### Root Cause

The issue exists in `/Users/joshua/ws/active/c4/c4/c4m/manifest.go` line 366:

```go
// ComputeC4ID computes the C4 ID for the manifest
func (m *Manifest) ComputeC4ID() c4.ID {
    canonical := m.Canonical()
    return c4.Identify(strings.NewReader(canonical))
}
```

And in `/Users/joshua/ws/active/c4/c4/c4m/entry.go` line 98:

```go
// Canonical returns the canonical form for C4 ID computation
func (e *Entry) Canonical() string {
    // No indentation in canonical form
    modeStr := formatMode(e.Mode)
    // Canonical format MUST be UTC only
    timeStr := e.Timestamp.UTC().Format("2006-01-02T15:04:05Z")
    sizeStr := fmt.Sprintf("%d", e.Size)
    // ...
}
```

**The problem**: These methods happily convert null values (Mode=0, Timestamp=Unix(0), Size=-1) into their string representations without validation. Different null representations produce different canonical forms, thus different C4 IDs.

---

## Section B: Canonical Form Requirements

### Definition

A manifest entry is in **canonical form** when all fields required for C4 ID computation have explicit, meaningful values. Canonical form is the **only** form from which C4 IDs may be computed.

### Required Fields (No Nulls Allowed)

For C4 ID computation, the following fields **MUST** have explicit values:

1. **Mode** (os.FileMode)
   - MUST NOT be 0 (unless legitimately a file with no permissions, which is rare)
   - MUST represent actual filesystem permissions
   - Typical values: 0644 (files), 0755 (executables), 0755 (directories)

2. **Timestamp** (time.Time)
   - MUST NOT be Unix epoch (1970-01-01T00:00:00Z) unless that's the actual modification time
   - MUST represent actual modification time from filesystem
   - MUST be in UTC when serialized

3. **Size** (int64)
   - MUST NOT be -1 (the null indicator)
   - MUST be ≥ 0
   - For files: actual byte count
   - For directories: sum of all contained content
   - For empty files/directories: 0 (which is valid and canonical)

4. **Name** (string)
   - MUST NOT be empty
   - MUST be a valid filesystem name
   - MUST NOT be ".", "..", or "/"

5. **C4ID** (c4.ID) - Special Rules
   - MUST be present for non-empty files
   - MUST be present for directories
   - MAY be nil for empty files (size 0)
   - MAY be nil for symlinks to symlinks or broken symlinks
   - MAY be nil for special files (devices, pipes, sockets)

### Why Each Requirement Exists

**Mode**: Different modes produce different canonical strings (`-rw-r--r--` vs `----------`), thus different IDs. The mode is intrinsic to the filesystem content being identified.

**Timestamp**: Different timestamps produce different canonical strings, different IDs. While C4 focuses on content, C4M includes metadata for complete filesystem snapshots. The timestamp must be explicit and consistent.

**Size**: Different sizes produce different canonical strings and different IDs. Size is fundamental to the content being identified. -1 is not a real size; it's a sentinel value meaning "unknown."

**Name**: The name is part of what's being identified in a directory manifest. An entry without a name is meaningless.

**C4ID**: For content-bearing entries (files, directories), the C4 ID is what links the entry to its content. Without it, the entry can't participate in content-addressable operations.

### Valid Canonical Examples

**Regular file (canonical)**:
```go
Entry{
    Mode:      0644,
    Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
    Size:      1024,
    Name:      "document.txt",
    C4ID:      c4.MustID("c41abc..."),
}
```

**Directory (canonical)**:
```go
Entry{
    Mode:      0755 | os.ModeDir,
    Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
    Size:      2048,  // sum of contents
    Name:      "mydir/",
    C4ID:      c4.MustID("c42xyz..."),  // computed from directory manifest
}
```

**Empty file (canonical)**:
```go
Entry{
    Mode:      0644,
    Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
    Size:      0,  // zero is valid for empty files
    Name:      "empty.txt",
    C4ID:      c4.ID{},  // nil is OK for empty files
}
```

---

## Section C: Ergonomic Form Support

### Definition

An **ergonomic form** is a non-canonical manifest representation designed for human usability or workflow convenience. Ergonomic forms may contain null values, but **MUST NOT** be used to compute C4 IDs.

### When Null Values Are Appropriate

Null values serve legitimate purposes in **working manifests** (not for ID computation):

1. **Template Manifests**
   ```
   # Template for file ingestion workflow
   - - - incoming_file.mov -
   ```
   - Mode, timestamp, size unknown until file arrives
   - C4 ID computed after ingestion
   - Used to specify expected structure

2. **Incremental Scanning**
   ```
   drwxr-xr-x - - project/
     -rw-r--r-- - - bigfile.mp4 -
   ```
   - Scan discovers structure first (names, modes)
   - Size/timestamp filled in second pass
   - C4 ID computed last, after all metadata available

3. **Diff/Patch Operations**
   ```
   # Show only changed timestamp
   -rw-r--r-- 2024-10-31T10:00:00Z - document.txt c41abc...
   ```
   - Size unchanged (null = "no change")
   - Timestamp updated
   - Used in layer operations

4. **Metadata Resolution Workflows**
   ```
   # Manifest from untrusted source, needs validation
   -rw-r--r-- - 1024 data.bin c41abc...
   ```
   - Timestamp missing/untrusted
   - Will be resolved from filesystem or trusted source
   - C4 ID verified after resolution

### Clarifying When Ergonomic Forms Are Appropriate

**DO use ergonomic forms for**:
- In-progress manifest construction
- Template-based workflows
- Incremental metadata collection
- Diff/patch generation
- User input/editing

**DO NOT use ergonomic forms for**:
- Computing C4 IDs
- Storing final snapshots
- Content verification
- Deduplication operations
- Canonical storage

### Examples of Valid Ergonomic Manifests

**Template manifest (all nulls except name)**:
```
@c4m 1.0
---------- - - render_[0001-1000].exr -
```
Purpose: Specify expected sequence structure before files exist

**Partially scanned manifest**:
```
@c4m 1.0
drwxr-xr-x 2024-10-31T09:00:00Z - footage/
  -rw-r--r-- 2024-10-31T09:15:23Z - clip001.mov -
  -rw-r--r-- 2024-10-31T09:16:45Z - clip002.mov -
```
Purpose: First pass collected modes/timestamps, waiting for size/C4ID computation

**Timestamp-only update**:
```
@c4m 1.0
-rw-r--r-- 2024-10-31T10:30:00Z - config.json c41xyz...
```
Purpose: Show only the timestamp changed (null size = unchanged)

---

## Section D: Required API Changes

### D.1 ComputeC4ID() Changes

**Current (unsafe) signature**:
```go
// ComputeC4ID computes the C4 ID for the manifest
func (m *Manifest) ComputeC4ID() c4.ID
```

**New (safe) signature**:
```go
// ComputeC4ID computes the C4 ID for the manifest.
// Returns an error if the manifest contains null values or is not in canonical form.
func (m *Manifest) ComputeC4ID() (c4.ID, error)
```

**Implementation**:
```go
func (m *Manifest) ComputeC4ID() (c4.ID, error) {
    // Validate canonical form first
    if err := m.IsCanonical(); err != nil {
        return c4.ID{}, fmt.Errorf("cannot compute C4 ID: %w", err)
    }

    canonical := m.Canonical()
    return c4.Identify(strings.NewReader(canonical)), nil
}
```

**Migration Path**:

All existing callers must be updated:
```go
// Before:
id := manifest.ComputeC4ID()

// After:
id, err := manifest.ComputeC4ID()
if err != nil {
    return fmt.Errorf("compute C4 ID: %w", err)
}
```

Similarly, Entry.ComputeC4ID() should be added if it doesn't exist, or updated to return error.

### D.2 Validation Methods

Add comprehensive validation methods to both Manifest and Entry types:

**IsCanonical() - Check if ready for C4 ID computation**:
```go
// IsCanonical returns an error if the manifest contains any null values
// or is otherwise not suitable for C4 ID computation.
func (m *Manifest) IsCanonical() error {
    if len(m.Entries) == 0 {
        return fmt.Errorf("empty manifest")
    }

    for i, entry := range m.Entries {
        if err := entry.IsCanonical(); err != nil {
            return fmt.Errorf("entry %d (%s): %w", i, entry.Name, err)
        }
    }

    return nil
}

// IsCanonical returns an error if the entry contains any null values.
func (e *Entry) IsCanonical() error {
    var errors []string

    // Check mode
    if e.Mode == 0 && !e.IsDir() && !e.IsSymlink() {
        errors = append(errors, "mode is null (zero)")
    }

    // Check timestamp
    if e.Timestamp.IsZero() || e.Timestamp.Unix() == 0 {
        errors = append(errors, "timestamp is null (Unix epoch)")
    }

    // Check size
    if e.Size < 0 {
        errors = append(errors, fmt.Sprintf("size is null (%d)", e.Size))
    }

    // Check name
    if e.Name == "" {
        errors = append(errors, "name is empty")
    }

    // Check C4 ID for content-bearing entries
    if e.Size > 0 && !e.IsDir() && !e.IsSymlink() {
        if e.C4ID.IsNil() {
            errors = append(errors, "C4 ID is null for non-empty file")
        }
    }

    if len(errors) > 0 {
        return fmt.Errorf("non-canonical entry: %s", strings.Join(errors, "; "))
    }

    return nil
}
```

**ValidateStructure() - Check format (allows nulls)**:
```go
// ValidateStructure checks that the entry has valid structure
// (e.g., name not empty, size not absurdly negative), but allows null values.
func (e *Entry) ValidateStructure() error {
    if e.Name == "" {
        return fmt.Errorf("name is empty")
    }

    if e.Name == "." || e.Name == ".." || e.Name == "/" {
        return fmt.Errorf("invalid name: %s", e.Name)
    }

    if e.Size < -1 {
        return fmt.Errorf("invalid size: %d (must be >= -1)", e.Size)
    }

    // Null values (-1, epoch, nil) are OK for structure validation

    return nil
}

// ValidateStructure checks that the manifest has valid structure.
func (m *Manifest) ValidateStructure() error {
    if m.Version == "" {
        return fmt.Errorf("missing version")
    }

    seen := make(map[string]bool)
    for i, entry := range m.Entries {
        if err := entry.ValidateStructure(); err != nil {
            return fmt.Errorf("entry %d: %w", i, err)
        }

        if seen[entry.Name] {
            return fmt.Errorf("duplicate name: %s", entry.Name)
        }
        seen[entry.Name] = true
    }

    return nil
}
```

**Usage Examples**:

```go
// Check if manifest is ready for C4 ID computation
if err := manifest.IsCanonical(); err != nil {
    log.Printf("Manifest not canonical: %v", err)
    // Resolve nulls before computing ID
}

// Validate structure (allows nulls)
if err := manifest.ValidateStructure(); err != nil {
    return fmt.Errorf("invalid manifest structure: %w", err)
}

// Validate for storage/snapshot (requires canonical)
if err := manifest.IsCanonical(); err != nil {
    return fmt.Errorf("cannot store non-canonical manifest: %w", err)
}
```

### D.3 Canonicalization API

**Canonicalize() - Resolve all null values**:

```go
// MetadataResolver provides metadata for entries with null values.
type MetadataResolver interface {
    // ResolveMetadata fills in missing metadata for an entry.
    // It should set Mode, Timestamp, and Size to their actual values.
    // Returns error if metadata cannot be determined.
    ResolveMetadata(entry *Entry) error
}

// Canonicalize resolves all null values in the manifest using the provided resolver.
// After successful canonicalization, the manifest is ready for C4 ID computation.
func (m *Manifest) Canonicalize(resolver MetadataResolver) error {
    for i, entry := range m.Entries {
        if err := entry.Canonicalize(resolver); err != nil {
            return fmt.Errorf("entry %d (%s): %w", i, entry.Name, err)
        }
    }
    return nil
}

// Canonicalize resolves all null values in the entry using the provided resolver.
func (e *Entry) Canonicalize(resolver MetadataResolver) error {
    // Check if already canonical
    if err := e.IsCanonical(); err == nil {
        return nil // already canonical
    }

    // Use resolver to fill in metadata
    if err := resolver.ResolveMetadata(e); err != nil {
        return fmt.Errorf("resolve metadata: %w", err)
    }

    // Verify now canonical
    if err := e.IsCanonical(); err != nil {
        return fmt.Errorf("still not canonical after resolution: %w", err)
    }

    return nil
}
```

**Implementing Resolvers**:

```go
// FilesystemResolver resolves metadata from the actual filesystem
type FilesystemResolver struct {
    BasePath string
}

func (r *FilesystemResolver) ResolveMetadata(entry *Entry) error {
    path := filepath.Join(r.BasePath, entry.Name)

    info, err := os.Lstat(path)
    if err != nil {
        return fmt.Errorf("stat %s: %w", path, err)
    }

    // Fill in null values
    if entry.Mode == 0 {
        entry.Mode = info.Mode()
    }

    if entry.Timestamp.IsZero() || entry.Timestamp.Unix() == 0 {
        entry.Timestamp = info.ModTime().UTC()
    }

    if entry.Size < 0 {
        entry.Size = info.Size()
    }

    // Compute C4 ID if needed
    if entry.C4ID.IsNil() && !entry.IsDir() && entry.Size > 0 {
        f, err := os.Open(path)
        if err != nil {
            return fmt.Errorf("open %s: %w", path, err)
        }
        defer f.Close()

        entry.C4ID = c4.Identify(f)
    }

    return nil
}

// DefaultResolver provides sensible defaults for null values
type DefaultResolver struct {
    DefaultMode      os.FileMode
    DefaultTimestamp time.Time
}

func (r *DefaultResolver) ResolveMetadata(entry *Entry) error {
    if entry.Mode == 0 {
        if r.DefaultMode != 0 {
            entry.Mode = r.DefaultMode
        } else {
            entry.Mode = 0644 // sensible default
        }
    }

    if entry.Timestamp.IsZero() || entry.Timestamp.Unix() == 0 {
        if !r.DefaultTimestamp.IsZero() {
            entry.Timestamp = r.DefaultTimestamp
        } else {
            entry.Timestamp = time.Now().UTC()
        }
    }

    if entry.Size < 0 {
        // Cannot default size without filesystem access
        return fmt.Errorf("cannot resolve size without filesystem access")
    }

    return nil
}
```

**Common Scenarios**:

```go
// Scenario 1: Resolve from filesystem
resolver := &FilesystemResolver{BasePath: "/data/archive"}
if err := manifest.Canonicalize(resolver); err != nil {
    return fmt.Errorf("canonicalize: %w", err)
}

// Scenario 2: Use defaults
resolver := &DefaultResolver{
    DefaultMode:      0644,
    DefaultTimestamp: scanTime,
}
if err := manifest.Canonicalize(resolver); err != nil {
    return fmt.Errorf("canonicalize: %w", err)
}

// Scenario 3: Chain resolvers (try filesystem first, fall back to defaults)
chain := &ChainResolver{
    Resolvers: []MetadataResolver{
        &FilesystemResolver{BasePath: "/data"},
        &DefaultResolver{DefaultMode: 0644},
    },
}
if err := manifest.Canonicalize(chain); err != nil {
    return fmt.Errorf("canonicalize: %w", err)
}
```

### D.4 Helper Methods

**HasNullValues() - Quick check**:
```go
// HasNullValues returns true if the manifest contains any null values.
func (m *Manifest) HasNullValues() bool {
    for _, entry := range m.Entries {
        if entry.HasNullValues() {
            return true
        }
    }
    return false
}

// HasNullValues returns true if the entry contains any null values.
func (e *Entry) HasNullValues() bool {
    if e.Mode == 0 && !e.IsDir() && !e.IsSymlink() {
        return true
    }
    if e.Timestamp.IsZero() || e.Timestamp.Unix() == 0 {
        return true
    }
    if e.Size < 0 {
        return true
    }
    // Note: Nil C4 ID is allowed for empty files, so don't check it here
    return false
}
```

**GetNullFields() - Descriptive diagnostics**:
```go
// GetNullFields returns a list of field names that have null values.
func (e *Entry) GetNullFields() []string {
    var nulls []string

    if e.Mode == 0 && !e.IsDir() && !e.IsSymlink() {
        nulls = append(nulls, "mode")
    }
    if e.Timestamp.IsZero() || e.Timestamp.Unix() == 0 {
        nulls = append(nulls, "timestamp")
    }
    if e.Size < 0 {
        nulls = append(nulls, "size")
    }
    if e.C4ID.IsNil() && e.Size > 0 && !e.IsDir() {
        nulls = append(nulls, "c4id")
    }

    return nulls
}
```

**IsReadyForSnapshot() - Comprehensive check**:
```go
// IsReadyForSnapshot returns an error if the manifest is not ready to be
// stored as a permanent snapshot. This includes canonical form checks plus
// additional consistency checks.
func (m *Manifest) IsReadyForSnapshot() error {
    // Must be canonical
    if err := m.IsCanonical(); err != nil {
        return err
    }

    // Additional snapshot-specific checks
    if m.Version == "" {
        return fmt.Errorf("missing version")
    }

    // Check sort order
    if !m.IsSorted() {
        return fmt.Errorf("entries not properly sorted")
    }

    // Verify directory size calculations if needed
    // ... additional checks ...

    return nil
}
```

---

## Section E: Implementation Plan

### Phase 1: Prevent the Problem (Critical - Do First)

**Goal**: Stop generating incorrect C4 IDs immediately

**Tasks**:
1. Add `IsCanonical() error` method to Entry
2. Add `IsCanonical() error` method to Manifest
3. Update `ComputeC4ID()` to return `(c4.ID, error)` and validate before computing
4. Add comprehensive tests for rejection of null values
5. Update all callers in c4m package to handle errors
6. Update all callers in c4d to handle errors
7. Update all callers in other tools (c4v, c4 CLI)

**Files to modify**:
- `/Users/joshua/ws/active/c4/c4/c4m/entry.go` - Add IsCanonical()
- `/Users/joshua/ws/active/c4/c4/c4m/manifest.go` - Add IsCanonical(), update ComputeC4ID()
- `/Users/joshua/ws/active/c4/c4/c4m/generator.go` - Update C4 ID computation calls
- `/Users/joshua/ws/active/c4/c4/c4m/bundle.go` - Update C4 ID computation calls
- All test files - Add tests for null rejection
- `/Users/joshua/ws/active/c4/c4d/` - Update all ComputeC4ID() callers
- `/Users/joshua/ws/active/c4/c4v/` - Update all ComputeC4ID() callers (if exists)

**Success Criteria**:
- All tests pass
- ComputeC4ID() returns error when manifest has nulls
- No existing functionality breaks (all callers handle errors)

### Phase 2: Canonicalization Support

**Goal**: Provide tools to resolve null values properly

**Tasks**:
1. Define `MetadataResolver` interface
2. Implement `Canonicalize()` method for Entry and Manifest
3. Implement `FilesystemResolver`
4. Implement `DefaultResolver`
5. Implement helper `ChainResolver`
6. Add comprehensive tests for canonicalization
7. Update scanner to use canonicalization
8. Document resolver patterns

**Files to create/modify**:
- `/Users/joshua/ws/active/c4/c4/c4m/canonicalize.go` - New file for canonicalization logic
- `/Users/joshua/ws/active/c4/c4/c4m/resolver.go` - Resolver implementations
- `/Users/joshua/ws/active/c4/c4/c4m/scanner_generic.go` - Use canonicalization
- Test files - Add canonicalization tests

**Success Criteria**:
- Resolvers correctly fill in null values
- Canonicalization produces valid canonical manifests
- Scanners use canonicalization workflow
- Clear error messages when resolution fails

### Phase 3: Enhanced Validation

**Goal**: Provide comprehensive validation at different levels

**Tasks**:
1. Implement `ValidateStructure()` (allows nulls)
2. Implement `HasNullValues()` helper
3. Implement `GetNullFields()` helper
4. Implement `IsReadyForSnapshot()` comprehensive check
5. Add validation to Manifest.Validate() with options
6. Update error messages to be descriptive and actionable
7. Add examples to documentation

**Files to modify**:
- `/Users/joshua/ws/active/c4/c4/c4m/validator.go` - Split into levels
- `/Users/joshua/ws/active/c4/c4/c4m/entry.go` - Add helper methods
- `/Users/joshua/ws/active/c4/c4/c4m/manifest.go` - Add helper methods
- Test files - Comprehensive validation tests

**Success Criteria**:
- Three validation levels work correctly
- Error messages clearly identify problems
- Helpers provide useful diagnostics
- Documentation shows all usage patterns

### Phase 4: Specification Updates

**Goal**: Formalize the canonical form requirements

**Tasks**:
1. Update SPECIFICATION.md to clarify canonical form
2. Add section on null values and when they're allowed
3. Document workflow: ergonomic → canonical → C4 ID
4. Create workflow diagrams
5. Add examples to specification
6. Update README.md with quick start guide
7. Add migration guide for existing code

**Files to modify**:
- `/Users/joshua/ws/active/c4/c4/c4m/SPECIFICATION.md` - Canonical form section
- `/Users/joshua/ws/active/c4/c4/c4m/README.md` - Quick start updates
- `/Users/joshua/ws/active/c4/c4/c4m/IMPLEMENTATION_NOTES.md` - Add reference

**Success Criteria**:
- Specification clearly defines canonical form
- Examples show correct usage
- Workflows documented with diagrams
- Migration path clear for developers

---

## Section F: Migration Guide

### Overview

The change from `ComputeC4ID() c4.ID` to `ComputeC4ID() (c4.ID, error)` is a breaking change. All existing code must be updated.

### Pattern 1: Simple Computation

**Before (unsafe)**:
```go
manifest := buildManifest()
id := manifest.ComputeC4ID()
storeManifest(id, manifest)
```

**After (safe)**:
```go
manifest := buildManifest()

// Verify canonical before computing
if err := manifest.IsCanonical(); err != nil {
    return fmt.Errorf("manifest not canonical: %w", err)
}

id, err := manifest.ComputeC4ID()
if err != nil {
    return fmt.Errorf("compute C4 ID: %w", err)
}

storeManifest(id, manifest)
```

### Pattern 2: With Canonicalization

**Before (unsafe)**:
```go
manifest := scanFilesystem("/data")
id := manifest.ComputeC4ID()
```

**After (safe)**:
```go
manifest := scanFilesystem("/data")

// Check if canonicalization needed
if manifest.HasNullValues() {
    resolver := &FilesystemResolver{BasePath: "/data"}
    if err := manifest.Canonicalize(resolver); err != nil {
        return fmt.Errorf("canonicalize: %w", err)
    }
}

// Now safe to compute
id, err := manifest.ComputeC4ID()
if err != nil {
    return fmt.Errorf("compute C4 ID: %w", err)
}
```

### Pattern 3: Graceful Degradation

**Before**:
```go
id := manifest.ComputeC4ID()
// Always succeeds
```

**After**:
```go
id, err := manifest.ComputeC4ID()
if err != nil {
    // Try to canonicalize
    if err := manifest.Canonicalize(defaultResolver); err != nil {
        return fmt.Errorf("cannot compute C4 ID: %w", err)
    }

    // Try again
    id, err = manifest.ComputeC4ID()
    if err != nil {
        return fmt.Errorf("compute C4 ID after canonicalization: %w", err)
    }
}
```

### Pattern 4: Validation Before Storage

**Before**:
```go
manifest := loadManifest()
id := manifest.ComputeC4ID()
verifyIntegrity(id, manifest)
```

**After**:
```go
manifest := loadManifest()

// Validate structure first
if err := manifest.ValidateStructure(); err != nil {
    return fmt.Errorf("invalid manifest: %w", err)
}

// Check canonical form
if err := manifest.IsCanonical(); err != nil {
    return fmt.Errorf("manifest not canonical (cannot verify): %w", err)
}

// Compute and verify
id, err := manifest.ComputeC4ID()
if err != nil {
    return fmt.Errorf("compute C4 ID: %w", err)
}

verifyIntegrity(id, manifest)
```

### Pattern 5: Diagnostic Information

**Before**:
```go
id := manifest.ComputeC4ID()
log.Printf("Computed ID: %s", id)
```

**After**:
```go
// Check for issues
if manifest.HasNullValues() {
    for i, entry := range manifest.Entries {
        if entry.HasNullValues() {
            nullFields := entry.GetNullFields()
            log.Printf("Entry %d (%s) has null fields: %v",
                i, entry.Name, nullFields)
        }
    }
    return fmt.Errorf("cannot compute ID: manifest has null values")
}

id, err := manifest.ComputeC4ID()
if err != nil {
    return fmt.Errorf("compute C4 ID: %w", err)
}

log.Printf("Computed ID: %s", id)
```

### Common Pitfalls

**Pitfall 1: Assuming zero/epoch/nil are valid**
```go
// WRONG: Treating Unix epoch as a valid timestamp
entry := &Entry{
    Mode:      0644,
    Timestamp: time.Unix(0, 0).UTC(), // This is NULL!
    Size:      100,
    Name:      "file.txt",
}
id, _ := entry.ComputeC4ID() // Will error!
```

**Pitfall 2: Not checking HasNullValues()**
```go
// WRONG: Attempting to compute without checking
id, err := manifest.ComputeC4ID()
// err will be non-nil if any nulls present

// RIGHT: Check first
if manifest.HasNullValues() {
    // Resolve nulls first
}
id, err := manifest.ComputeC4ID()
```

**Pitfall 3: Using nil C4 ID as null for all cases**
```go
// WRONG: Nil C4 ID is invalid for all files
entry := &Entry{
    Size: 100, // non-empty file
    C4ID: c4.ID{}, // nil is NOT OK here
}

// RIGHT: Empty files can have nil C4 ID
emptyEntry := &Entry{
    Size: 0, // empty file
    C4ID: c4.ID{}, // nil is OK here
}
```

### Update Checklist

For each file that calls `ComputeC4ID()`:

- [ ] Add error return value to function signature
- [ ] Add error handling at call site
- [ ] Consider if canonicalization is needed
- [ ] Add validation if storing/transmitting manifest
- [ ] Update tests to verify error cases
- [ ] Update documentation/comments

---

## Section G: Test Requirements

### Critical Test Cases

**Test 1: Reject null mode**
```go
func TestComputeC4ID_RejectsNullMode(t *testing.T) {
    entry := &Entry{
        Mode:      0, // null mode
        Timestamp: time.Now().UTC(),
        Size:      100,
        Name:      "test.txt",
        C4ID:      c4.MustID("c41abc..."),
    }

    _, err := entry.ComputeC4ID()
    if err == nil {
        t.Fatal("expected error for null mode, got nil")
    }

    if !strings.Contains(err.Error(), "mode") {
        t.Errorf("error should mention mode: %v", err)
    }
}
```

**Test 2: Reject null timestamp**
```go
func TestComputeC4ID_RejectsNullTimestamp(t *testing.T) {
    entry := &Entry{
        Mode:      0644,
        Timestamp: time.Unix(0, 0).UTC(), // null (epoch)
        Size:      100,
        Name:      "test.txt",
        C4ID:      c4.MustID("c41abc..."),
    }

    _, err := entry.ComputeC4ID()
    if err == nil {
        t.Fatal("expected error for null timestamp, got nil")
    }
}
```

**Test 3: Reject null size**
```go
func TestComputeC4ID_RejectsNullSize(t *testing.T) {
    entry := &Entry{
        Mode:      0644,
        Timestamp: time.Now().UTC(),
        Size:      -1, // null size
        Name:      "test.txt",
        C4ID:      c4.MustID("c41abc..."),
    }

    _, err := entry.ComputeC4ID()
    if err == nil {
        t.Fatal("expected error for null size, got nil")
    }
}
```

**Test 4: Same content always produces same ID**
```go
func TestComputeC4ID_Deterministic(t *testing.T) {
    // Create same manifest two different ways
    manifest1 := &Manifest{
        Version: "1.0",
        Entries: []*Entry{
            {
                Mode:      0644,
                Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
                Size:      100,
                Name:      "test.txt",
                C4ID:      c4.MustID("c41abc..."),
            },
        },
    }

    manifest2 := &Manifest{
        Version: "1.0",
        Entries: []*Entry{
            {
                Mode:      0644,
                Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
                Size:      100,
                Name:      "test.txt",
                C4ID:      c4.MustID("c41abc..."),
            },
        },
    }

    id1, err := manifest1.ComputeC4ID()
    if err != nil {
        t.Fatalf("manifest1 ComputeC4ID: %v", err)
    }

    id2, err := manifest2.ComputeC4ID()
    if err != nil {
        t.Fatalf("manifest2 ComputeC4ID: %v", err)
    }

    if id1 != id2 {
        t.Errorf("same content produced different IDs: %s != %s", id1, id2)
    }
}
```

**Test 5: IsCanonical detects all null scenarios**
```go
func TestIsCanonical_DetectsAllNulls(t *testing.T) {
    tests := []struct {
        name  string
        entry Entry
    }{
        {
            name: "null mode",
            entry: Entry{
                Mode:      0,
                Timestamp: time.Now().UTC(),
                Size:      100,
                Name:      "test.txt",
            },
        },
        {
            name: "null timestamp",
            entry: Entry{
                Mode:      0644,
                Timestamp: time.Unix(0, 0).UTC(),
                Size:      100,
                Name:      "test.txt",
            },
        },
        {
            name: "null size",
            entry: Entry{
                Mode:      0644,
                Timestamp: time.Now().UTC(),
                Size:      -1,
                Name:      "test.txt",
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.entry.IsCanonical()
            if err == nil {
                t.Errorf("IsCanonical() should detect %s", tt.name)
            }
        })
    }
}
```

**Test 6: Canonicalize resolves values correctly**
```go
func TestCanonicalize_ResolvesValues(t *testing.T) {
    entry := &Entry{
        Mode:      0,          // null
        Timestamp: time.Unix(0, 0).UTC(), // null
        Size:      -1,         // null
        Name:      "test.txt",
    }

    resolver := &DefaultResolver{
        DefaultMode:      0644,
        DefaultTimestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
    }

    // Should fail due to size
    err := entry.Canonicalize(resolver)
    if err == nil {
        t.Fatal("expected error for unresolvable size")
    }

    // Provide size
    entry.Size = 100

    // Should succeed now
    err = entry.Canonicalize(resolver)
    if err != nil {
        t.Fatalf("Canonicalize failed: %v", err)
    }

    // Verify values filled in
    if entry.Mode != 0644 {
        t.Errorf("Mode = %v, want 0644", entry.Mode)
    }
    if entry.Timestamp.Unix() == 0 {
        t.Error("Timestamp still null after canonicalization")
    }
}
```

**Test 7: Error messages are clear and actionable**
```go
func TestErrorMessages_AreActionable(t *testing.T) {
    entry := &Entry{
        Mode:      0,
        Timestamp: time.Unix(0, 0).UTC(),
        Size:      -1,
        Name:      "test.txt",
    }

    err := entry.IsCanonical()
    if err == nil {
        t.Fatal("expected error")
    }

    errMsg := err.Error()

    // Should mention all the problems
    if !strings.Contains(errMsg, "mode") {
        t.Error("error should mention mode")
    }
    if !strings.Contains(errMsg, "timestamp") {
        t.Error("error should mention timestamp")
    }
    if !strings.Contains(errMsg, "size") {
        t.Error("error should mention size")
    }
}
```

### Integration Tests

**Test 8: Full workflow from parsing to C4 ID**
```go
func TestFullWorkflow_ParseCanonicalizeSCompute(t *testing.T) {
    // Parse manifest with nulls
    input := `@c4m 1.0
---------- - - test.txt -`

    manifest, err := GenerateFromReader(strings.NewReader(input))
    if err != nil {
        t.Fatalf("parse: %v", err)
    }

    // Should have nulls
    if !manifest.HasNullValues() {
        t.Fatal("expected null values")
    }

    // Should fail to compute C4 ID
    _, err = manifest.ComputeC4ID()
    if err == nil {
        t.Fatal("should fail to compute C4 ID with nulls")
    }

    // Canonicalize
    resolver := &DefaultResolver{
        DefaultMode:      0644,
        DefaultTimestamp: time.Now().UTC(),
    }

    // Set size manually (can't be defaulted)
    for _, entry := range manifest.Entries {
        entry.Size = 100
        entry.C4ID = c4.MustID("c41abc...")
    }

    err = manifest.Canonicalize(resolver)
    if err != nil {
        t.Fatalf("canonicalize: %v", err)
    }

    // Now should succeed
    id, err := manifest.ComputeC4ID()
    if err != nil {
        t.Fatalf("compute after canonicalize: %v", err)
    }

    if id.IsNil() {
        t.Error("computed ID is nil")
    }
}
```

---

## Section H: Code Examples

### Example 1: Creating Working Manifests with Nulls

```go
package main

import (
    "fmt"
    "time"

    "github.com/Avalanche-io/c4/c4/c4m"
)

func createWorkingManifest() (*c4m.Manifest, error) {
    // Create a template manifest for a render job
    manifest := c4m.NewManifest()

    // Add entries with null values (not ready for C4 ID yet)
    for i := 1; i <= 100; i++ {
        entry := &c4m.Entry{
            Mode:      0, // Will be filled in when files exist
            Timestamp: time.Unix(0, 0).UTC(), // Null - not created yet
            Size:      -1, // Null - size unknown
            Name:      fmt.Sprintf("render.%04d.exr", i),
            C4ID:      c4.ID{}, // Null - will compute after render
        }
        manifest.AddEntry(entry)
    }

    // This is a valid working manifest
    if err := manifest.ValidateStructure(); err != nil {
        return nil, fmt.Errorf("invalid structure: %w", err)
    }

    // But NOT ready for C4 ID computation
    if err := manifest.IsCanonical(); err != nil {
        fmt.Printf("Expected: not canonical: %v\n", err)
    }

    return manifest, nil
}
```

### Example 2: Resolving Nulls Through Canonicalization

```go
func canonicalizeManifest(manifest *c4m.Manifest, basePath string) error {
    // Check if canonicalization needed
    if !manifest.HasNullValues() {
        return nil // already canonical
    }

    // Create filesystem resolver
    resolver := &c4m.FilesystemResolver{
        BasePath: basePath,
    }

    // Resolve all null values
    if err := manifest.Canonicalize(resolver); err != nil {
        return fmt.Errorf("canonicalize: %w", err)
    }

    // Verify now canonical
    if err := manifest.IsCanonical(); err != nil {
        return fmt.Errorf("still not canonical after resolution: %w", err)
    }

    fmt.Println("Manifest successfully canonicalized")
    return nil
}
```

### Example 3: Computing C4 IDs Safely

```go
func computeManifestID(manifest *c4m.Manifest) (c4.ID, error) {
    // First, validate structure
    if err := manifest.ValidateStructure(); err != nil {
        return c4.ID{}, fmt.Errorf("invalid structure: %w", err)
    }

    // Check if canonical
    if err := manifest.IsCanonical(); err != nil {
        // Provide helpful diagnostic
        if manifest.HasNullValues() {
            fmt.Println("Manifest has null values:")
            for i, entry := range manifest.Entries {
                if entry.HasNullValues() {
                    nullFields := entry.GetNullFields()
                    fmt.Printf("  Entry %d (%s): null fields: %v\n",
                        i, entry.Name, nullFields)
                }
            }
        }
        return c4.ID{}, fmt.Errorf("not canonical: %w", err)
    }

    // Safe to compute
    id, err := manifest.ComputeC4ID()
    if err != nil {
        return c4.ID{}, fmt.Errorf("compute C4 ID: %w", err)
    }

    return id, nil
}
```

### Example 4: Handling Errors Appropriately

```go
func processManifest(manifest *c4m.Manifest) error {
    // Attempt to compute C4 ID
    id, err := manifest.ComputeC4ID()
    if err != nil {
        // Check if it's a canonical form issue
        if manifest.HasNullValues() {
            fmt.Println("Manifest has null values, attempting canonicalization...")

            // Try to resolve with defaults
            resolver := &c4m.DefaultResolver{
                DefaultMode:      0644,
                DefaultTimestamp: time.Now().UTC(),
            }

            if err := manifest.Canonicalize(resolver); err != nil {
                return fmt.Errorf("cannot canonicalize: %w", err)
            }

            // Try again
            id, err = manifest.ComputeC4ID()
            if err != nil {
                return fmt.Errorf("compute after canonicalize: %w", err)
            }
        } else {
            // Some other error
            return fmt.Errorf("compute C4 ID: %w", err)
        }
    }

    fmt.Printf("Computed C4 ID: %s\n", id)
    return nil
}
```

### Example 5: Implementing Custom Resolvers

```go
// TrustedSourceResolver resolves metadata from a trusted external source
type TrustedSourceResolver struct {
    MetadataDB *sql.DB
}

func (r *TrustedSourceResolver) ResolveMetadata(entry *c4m.Entry) error {
    // Query trusted database for metadata
    var mode uint32
    var timestamp time.Time
    var size int64

    err := r.MetadataDB.QueryRow(
        "SELECT mode, timestamp, size FROM files WHERE name = ?",
        entry.Name,
    ).Scan(&mode, &timestamp, &size)

    if err != nil {
        return fmt.Errorf("query metadata for %s: %w", entry.Name, err)
    }

    // Fill in null values
    if entry.Mode == 0 {
        entry.Mode = os.FileMode(mode)
    }

    if entry.Timestamp.IsZero() || entry.Timestamp.Unix() == 0 {
        entry.Timestamp = timestamp.UTC()
    }

    if entry.Size < 0 {
        entry.Size = size
    }

    return nil
}

// Usage
func resolveFromTrustedSource(manifest *c4m.Manifest, db *sql.DB) error {
    resolver := &TrustedSourceResolver{MetadataDB: db}
    return manifest.Canonicalize(resolver)
}
```

### Example 6: Complete Workflow Example

```go
package main

import (
    "fmt"
    "log"
    "os"
    "time"

    "github.com/Avalanche-io/c4"
    "github.com/Avalanche-io/c4/c4/c4m"
)

func main() {
    // Step 1: Parse a manifest (possibly with nulls)
    manifest, err := c4m.ParseFile("input.c4m")
    if err != nil {
        log.Fatalf("parse: %v", err)
    }

    // Step 2: Validate structure (allows nulls)
    if err := manifest.ValidateStructure(); err != nil {
        log.Fatalf("invalid structure: %v", err)
    }

    // Step 3: Check if canonical
    if manifest.HasNullValues() {
        fmt.Println("Manifest has null values, canonicalizing...")

        // Step 4: Canonicalize
        resolver := &c4m.FilesystemResolver{
            BasePath: "/data/archive",
        }

        if err := manifest.Canonicalize(resolver); err != nil {
            log.Fatalf("canonicalize: %v", err)
        }

        fmt.Println("Canonicalization complete")
    }

    // Step 5: Verify canonical
    if err := manifest.IsCanonical(); err != nil {
        log.Fatalf("not canonical: %v", err)
    }

    // Step 6: Compute C4 ID (now safe)
    id, err := manifest.ComputeC4ID()
    if err != nil {
        log.Fatalf("compute C4 ID: %v", err)
    }

    fmt.Printf("Manifest C4 ID: %s\n", id)

    // Step 7: Store canonical manifest
    f, err := os.Create("output.c4m")
    if err != nil {
        log.Fatalf("create output: %v", err)
    }
    defer f.Close()

    if _, err := manifest.WriteTo(f); err != nil {
        log.Fatalf("write manifest: %v", err)
    }

    fmt.Println("Canonical manifest saved to output.c4m")
}
```

---

## Appendix A: Backward Compatibility Considerations

### Breaking Changes

1. **ComputeC4ID() signature change** - All callers must be updated
2. **Validation strictness** - Code relying on computing IDs from non-canonical manifests will break

### Migration Timeline

**Recommended approach**: Feature branch with all changes, thorough testing, single merged update

**Alternative**: Phased rollout
- Phase 1: Add new methods (IsCanonical, Canonicalize) without breaking existing API
- Phase 2: Update internal code to use new methods
- Phase 3: Change ComputeC4ID() signature (breaking change)
- Phase 4: Remove deprecated code

### Version Compatibility

- Manifests written by old code will parse correctly in new code
- Manifests written by new code will parse correctly in old code (if canonical)
- C4 IDs computed by new code will be consistent and deterministic
- C4 IDs computed by old code from non-canonical manifests are **invalid** and should be recomputed

---

## Appendix B: Related Issues and Future Work

### Related Issues

1. **Directory size calculation** - Ensure null sizes in directories are properly handled
2. **Incremental scanning** - Support building manifests progressively with nulls, then canonicalizing
3. **Manifest merging** - Handle null values when merging manifests
4. **Layer operations** - Clarify null value semantics in @layer sections

### Future Enhancements

1. **Automatic canonicalization** - Add option to ComputeC4ID to auto-canonicalize if possible
2. **Partial canonicalization** - Resolve only specific fields
3. **Validation profiles** - Different validation levels for different use cases
4. **Resolver chains** - More sophisticated resolver composition
5. **Metadata propagation** - Automatically propagate metadata from parent directories

---

## Appendix C: Glossary

**Canonical Form**: A manifest representation with all fields explicitly set to their actual values, suitable for C4 ID computation.

**Ergonomic Form**: A manifest representation optimized for human readability or workflow convenience, may contain null values.

**Null Value**: A sentinel value indicating missing/unknown metadata (Mode=0, Timestamp=Unix(0), Size=-1, C4ID=nil).

**Canonicalization**: The process of resolving all null values to produce a canonical form manifest.

**Metadata Resolver**: An interface that provides missing metadata for entries during canonicalization.

**Deterministic Identification**: The guarantee that the same content always produces the same C4 ID.

**Content-Addressable**: A system where content is referenced by its cryptographic hash (C4 ID), not by location.
