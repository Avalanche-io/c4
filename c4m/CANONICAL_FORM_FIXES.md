# Canonical Form Enforcement Implementation Checklist

This document tracks the implementation of canonical form enforcement for C4 ID computation in the c4m package.

**Reference**: See [CANONICAL_FORM_ENFORCEMENT.md](./CANONICAL_FORM_ENFORCEMENT.md) for complete design specification.

**Status**: Not Started
**Created**: 2025-10-31

---

## Phase 1: Critical Fixes (MUST DO FIRST)

**Goal**: Stop generating incorrect C4 IDs immediately

### Core Implementation

- [ ] **Add Entry.IsCanonical() method**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry.go`
  - Check Mode != 0 (or is legitimately zero for special files)
  - Check Timestamp != Unix(0)
  - Check Size >= 0 (not -1)
  - Check C4ID not nil for non-empty files
  - Return descriptive error listing all null fields

- [ ] **Add Manifest.IsCanonical() method**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Iterate all entries and call Entry.IsCanonical()
  - Return error identifying which entry failed
  - Handle empty manifests appropriately

- [ ] **Update Manifest.ComputeC4ID() signature**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Change from `func (m *Manifest) ComputeC4ID() c4.ID`
  - To: `func (m *Manifest) ComputeC4ID() (c4.ID, error)`
  - Add validation call to IsCanonical() before computing
  - Return error if not canonical

- [ ] **Add Entry.ComputeC4ID() method if needed**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry.go`
  - Signature: `func (e *Entry) ComputeC4ID() (c4.ID, error)`
  - Validate canonical before computing
  - Return computed C4 ID or error

### Testing

- [ ] **Test: ComputeC4ID rejects null mode**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Create entry with Mode=0
  - Verify ComputeC4ID() returns error
  - Verify error message mentions "mode"

- [ ] **Test: ComputeC4ID rejects null timestamp**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Create entry with Timestamp=Unix(0)
  - Verify ComputeC4ID() returns error
  - Verify error message mentions "timestamp"

- [ ] **Test: ComputeC4ID rejects null size**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Create entry with Size=-1
  - Verify ComputeC4ID() returns error
  - Verify error message mentions "size"

- [ ] **Test: ComputeC4ID rejects missing C4ID for non-empty files**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Create file entry with Size>0, C4ID=nil
  - Verify ComputeC4ID() returns error

- [ ] **Test: ComputeC4ID accepts canonical manifest**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Create fully populated manifest
  - Verify ComputeC4ID() succeeds
  - Verify deterministic (same content = same ID)

- [ ] **Test: IsCanonical detects all null scenarios**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry_test.go`
  - Test null mode detection
  - Test null timestamp detection
  - Test null size detection
  - Test missing C4ID detection

- [ ] **Test: Same content produces same ID**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Create identical manifest two different ways
  - Verify both produce identical C4 IDs
  - Test with different entry orders (should sort first)

### Update Callers in c4m Package

- [ ] **Update generator.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/generator.go`
  - Search for ComputeC4ID() calls
  - Add error handling
  - Consider canonicalization if needed

- [ ] **Update bundle.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/bundle.go`
  - Search for ComputeC4ID() calls
  - Add error handling
  - Ensure bundles only contain canonical manifests

- [ ] **Update bundle_scanner.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/bundle_scanner.go`
  - Search for ComputeC4ID() calls
  - Add error handling
  - May need canonicalization workflow

- [ ] **Update scanner_generic.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/scanner_generic.go`
  - Ensure scanner produces canonical entries
  - Add error handling for C4 ID computation

- [ ] **Update progressive_scanner.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/progressive_scanner.go`
  - Review scanning workflow
  - Ensure final manifests are canonical
  - Add error handling

- [ ] **Update operations.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/operations.go`
  - Search for ComputeC4ID() calls
  - Add error handling
  - Consider implications for manifest operations

- [ ] **Review and update all test files**
  - Files: All `*_test.go` in `/Users/joshua/ws/active/c4/c4/c4m/`
  - Update tests to handle new error return
  - Add canonicalization where needed
  - Ensure tests verify canonical behavior

### Update Callers in c4d Package

- [ ] **Search for ComputeC4ID() in c4d**
  - Command: `cd /Users/joshua/ws/active/c4/c4d && grep -r "ComputeC4ID" --include="*.go"`
  - Create list of all files needing updates

- [ ] **Update c4d callers (list after search)**
  - Add file-by-file checklist items after search
  - Each caller must handle error return
  - Consider canonicalization needs per caller

### Update Callers in Other Packages

- [ ] **Search for ComputeC4ID() in c4v (if exists)**
  - Command: `cd /Users/joshua/ws/active/c4/c4v && grep -r "ComputeC4ID" --include="*.go"`
  - Create list if package exists

- [ ] **Search for ComputeC4ID() in c4 CLI (if exists)**
  - Search in other relevant directories
  - Update any command-line tool usage

### Verification

- [ ] **All c4m tests pass**
  - Command: `cd /Users/joshua/ws/active/c4/c4/c4m && go test ./...`

- [ ] **All c4d tests pass**
  - Command: `cd /Users/joshua/ws/active/c4/c4d && go test ./...`

- [ ] **All c4v tests pass (if exists)**
  - Command: `cd /Users/joshua/ws/active/c4/c4v && go test ./...`

- [ ] **Integration tests pass**
  - Run full test suite across all packages
  - Verify no regressions

---

## Phase 2: Canonicalization Support

**Goal**: Provide tools to resolve null values properly

### Interface and Core Methods

- [ ] **Define MetadataResolver interface**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize.go` (new)
  - Define `ResolveMetadata(entry *Entry) error` method
  - Document interface contract
  - Add examples in comments

- [ ] **Implement Entry.Canonicalize()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry.go`
  - Check if already canonical (early return)
  - Call resolver.ResolveMetadata()
  - Verify canonical after resolution
  - Return descriptive errors

- [ ] **Implement Manifest.Canonicalize()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Iterate all entries
  - Call Entry.Canonicalize() for each
  - Collect errors with entry context
  - Support partial success option?

### Resolver Implementations

- [ ] **Implement FilesystemResolver**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/resolver.go` (new)
  - BasePath field for root directory
  - ResolveMetadata() implementation:
    - Use os.Lstat() to get file info
    - Fill Mode if null
    - Fill Timestamp if null
    - Fill Size if null
    - Compute C4 ID if null and file has content
  - Handle errors (file not found, permission denied, etc.)

- [ ] **Implement DefaultResolver**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/resolver.go`
  - DefaultMode field
  - DefaultTimestamp field
  - ResolveMetadata() implementation:
    - Use defaults for Mode and Timestamp
    - Cannot default Size (error)
    - Document when appropriate to use

- [ ] **Implement ChainResolver**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/resolver.go`
  - Resolvers []MetadataResolver field
  - Try resolvers in order
  - Stop at first success
  - Return error only if all fail

- [ ] **Implement TrustedDatabaseResolver (example)**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/resolver.go`
  - Example implementation for documentation
  - Shows how to implement custom resolver
  - May use simple map instead of actual DB

### Testing

- [ ] **Test: Entry.Canonicalize() with FilesystemResolver**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize_test.go` (new)
  - Create test files with known metadata
  - Create entry with nulls
  - Resolve from filesystem
  - Verify all fields filled correctly

- [ ] **Test: Entry.Canonicalize() with DefaultResolver**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize_test.go`
  - Create entry with null mode/timestamp
  - Apply defaults
  - Verify fields filled
  - Verify error when size cannot be defaulted

- [ ] **Test: Manifest.Canonicalize() full workflow**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize_test.go`
  - Create manifest with multiple entries having nulls
  - Canonicalize
  - Verify all entries now canonical
  - Verify can compute C4 ID

- [ ] **Test: ChainResolver**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize_test.go`
  - Create chain with filesystem fallback to defaults
  - Test successful resolution
  - Test partial resolution (some succeed, some fail)

- [ ] **Test: Canonicalize() error cases**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize_test.go`
  - File not found
  - Permission denied
  - Invalid metadata
  - Verify descriptive error messages

### Integration with Scanners

- [ ] **Update scanner to use canonicalization**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/scanner_generic.go`
  - Consider if scanner should produce canonical entries directly
  - Or if it should produce with nulls then canonicalize
  - Document the chosen approach

- [ ] **Update progressive scanner workflow**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/progressive_scanner.go`
  - First pass: structure (may have nulls)
  - Second pass: canonicalize
  - Final pass: compute C4 IDs
  - Document phases

### Documentation

- [ ] **Add canonicalization examples to README**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/README.md`
  - Show basic canonicalization workflow
  - Show resolver usage
  - Link to full design doc

- [ ] **Document resolver interface**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/canonicalize.go`
  - Comprehensive godoc comments
  - Usage examples
  - Common patterns

---

## Phase 3: Enhanced Validation

**Goal**: Provide comprehensive validation at different levels

### Validation Methods

- [ ] **Implement Entry.ValidateStructure()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry.go`
  - Check name not empty
  - Check name not ".", "..", or "/"
  - Check size >= -1 (allow null -1)
  - Allow null values (not an error)
  - Return error for structural problems only

- [ ] **Implement Manifest.ValidateStructure()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Check version present
  - Check no duplicate names
  - Call Entry.ValidateStructure() for each
  - Allow null values

- [ ] **Implement Entry.HasNullValues()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry.go`
  - Quick boolean check
  - Check Mode == 0 (for non-dir/symlink)
  - Check Timestamp == Unix(0)
  - Check Size == -1
  - Don't check C4ID (nil is valid for empty files)

- [ ] **Implement Manifest.HasNullValues()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Check any entry has null values
  - Early return on first null found

- [ ] **Implement Entry.GetNullFields()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry.go`
  - Return []string of field names that are null
  - Useful for diagnostic messages
  - Example: ["mode", "timestamp"]

- [ ] **Implement Manifest.GetNullFieldsSummary()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Return map[string][]string (entry name -> null fields)
  - Or return structured summary
  - For debugging/reporting

- [ ] **Implement Manifest.IsReadyForSnapshot()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Call IsCanonical()
  - Check sort order
  - Check version
  - Any other snapshot-specific requirements
  - Comprehensive pre-storage check

### Update Existing Validation

- [ ] **Review Manifest.Validate()**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest.go`
  - Consider renaming to ValidateStrict() or similar
  - Or add ValidationMode parameter
  - Ensure clear distinction from ValidateStructure()

- [ ] **Review validator.go**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/validator.go`
  - Split into structure vs canonical validation
  - Update to use new methods
  - Maintain backward compatibility if possible

### Error Messages

- [ ] **Ensure all validation errors are descriptive**
  - Each error should identify:
    - Which entry (index or name)
    - Which field
    - What's wrong
    - How to fix it
  - Examples:
    - "entry 5 (test.txt): mode is null (zero)"
    - "manifest not canonical: 3 entries have null values"

- [ ] **Add error wrapping consistently**
  - Use fmt.Errorf with %w
  - Maintain error chain
  - Allow caller to inspect specific error types

### Testing

- [ ] **Test: ValidateStructure() allows nulls**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/validator_test.go`
  - Create entry with all nulls
  - Verify ValidateStructure() succeeds
  - Verify it catches structural problems

- [ ] **Test: HasNullValues() detects correctly**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry_test.go`
  - Test all null scenarios
  - Test canonical entry returns false
  - Test mixed scenarios

- [ ] **Test: GetNullFields() lists correctly**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/entry_test.go`
  - Test entry with mode null: returns ["mode"]
  - Test entry with multiple nulls
  - Test canonical entry: returns []

- [ ] **Test: IsReadyForSnapshot() comprehensive**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/manifest_test.go`
  - Test rejects non-canonical
  - Test rejects unsorted
  - Test accepts valid snapshot-ready manifest

- [ ] **Test: Error messages are actionable**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/validator_test.go`
  - Verify error messages contain all required info
  - Test that errors can be parsed/inspected
  - Verify helpful suggestions where appropriate

### Documentation

- [ ] **Document validation levels in godoc**
  - Clearly explain three levels:
    1. Structure (allows nulls)
    2. Canonical (requires all values)
    3. Snapshot-ready (comprehensive)
  - Show when to use each

- [ ] **Add validation examples**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/validator.go`
  - Example usage in comments
  - Show common validation patterns

---

## Phase 4: Specification and Documentation Updates

**Goal**: Formalize canonical form requirements in specification

### Specification Updates

- [ ] **Update SPECIFICATION.md: Canonical Form section**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/SPECIFICATION.md`
  - Add explicit "Canonical Form Requirements" section
  - List all required fields
  - Explain why each is required
  - Show canonical vs non-canonical examples

- [ ] **Update SPECIFICATION.md: Null Values section**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/SPECIFICATION.md`
  - Define null value representation
  - Explain when null values are allowed
  - Clarify ergonomic vs canonical forms
  - Document null value semantics in different contexts

- [ ] **Update SPECIFICATION.md: C4 ID Computation section**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/SPECIFICATION.md`
  - Explicitly state: "MUST be canonical"
  - Reference canonical form requirements
  - Show workflow diagram (ergonomic → canonical → ID)

- [ ] **Add workflow diagrams**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/SPECIFICATION.md`
  - Create ASCII or markdown diagrams showing:
    - Parse → Validate → Canonicalize → Compute
    - Template → Fill → Canonicalize → Snapshot
    - Scan → Metadata → Canonicalize → Store

### README Updates

- [ ] **Update README.md: Quick Start section**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/README.md`
  - Show canonical form requirement upfront
  - Add canonicalization example
  - Link to full documentation

- [ ] **Update README.md: API Examples**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/README.md`
  - Update examples to handle errors
  - Show canonicalization usage
  - Show validation usage

- [ ] **Add "Canonical Form" section to README**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/README.md`
  - Brief explanation
  - Why it matters
  - How to ensure canonical
  - Link to design doc

### Implementation Notes

- [ ] **Verify IMPLEMENTATION_NOTES.md reference**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/IMPLEMENTATION_NOTES.md`
  - Already added in this PR
  - Verify link works
  - Verify quick reference is accurate

- [ ] **Add practical examples to IMPLEMENTATION_NOTES**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/IMPLEMENTATION_NOTES.md`
  - Common canonicalization patterns
  - Error handling examples
  - Migration patterns

### Migration Guide

- [ ] **Create or update MIGRATION.md**
  - File: `/Users/joshua/ws/active/c4/c4/c4m/MIGRATION.md` (if needed)
  - Or add section to CANONICAL_FORM_ENFORCEMENT.md
  - Show before/after code examples
  - List all breaking changes
  - Provide step-by-step migration instructions

- [ ] **Document breaking changes**
  - Clearly list all API changes
  - ComputeC4ID() signature change
  - Validation behavior changes
  - Any removed/deprecated methods

- [ ] **Provide timeline guidance**
  - Recommended upgrade path
  - Testing recommendations
  - Rollback considerations

### Examples and Tutorials

- [ ] **Create examples directory (if not exists)**
  - Directory: `/Users/joshua/ws/active/c4/c4/c4m/examples/`
  - Or use example tests

- [ ] **Add example: Basic canonicalization**
  - Show parsing with nulls
  - Show canonicalization
  - Show C4 ID computation

- [ ] **Add example: Custom resolver**
  - Show implementing MetadataResolver
  - Show real-world use case
  - Show error handling

- [ ] **Add example: Validation workflow**
  - Show three validation levels
  - Show when to use each
  - Show error handling

- [ ] **Add example: Full scanner workflow**
  - Show progressive scanning
  - Show incremental canonicalization
  - Show final C4 ID computation

### Documentation Review

- [ ] **Review all godoc comments**
  - Ensure API documentation is complete
  - Ensure examples are included
  - Ensure links work

- [ ] **Review all markdown docs**
  - Check formatting
  - Check links
  - Check code examples compile

- [ ] **Spell check and grammar**
  - Run through all documentation
  - Fix typos
  - Ensure consistent terminology

---

## Final Verification

### Testing

- [ ] **All unit tests pass**
  - Command: `go test ./...` in c4m package

- [ ] **All integration tests pass**
  - Run full test suite

- [ ] **Code coverage check**
  - Ensure new code is well-tested
  - Aim for >80% coverage on new code

- [ ] **Benchmark tests (if any)**
  - Verify no performance regressions
  - Document any performance changes

### Code Quality

- [ ] **Run go vet**
  - Command: `go vet ./...`
  - Fix all issues

- [ ] **Run golint (if used)**
  - Check for style issues
  - Fix critical issues

- [ ] **Run staticcheck (if used)**
  - Check for bugs
  - Fix all issues

- [ ] **Code review**
  - Self-review all changes
  - Check for TODOs left in code
  - Verify error handling is consistent

### Documentation Quality

- [ ] **All documentation links work**
  - Check internal links
  - Check external links

- [ ] **Code examples compile**
  - Verify all code snippets are valid
  - Consider extracting to actual test files

- [ ] **Terminology is consistent**
  - "Canonical form" used consistently
  - "Null value" vs "missing value" clarified
  - No conflicting definitions

### Breaking Change Review

- [ ] **List all breaking changes**
  - ComputeC4ID() signature
  - Validation behavior
  - Any others?

- [ ] **Verify migration path exists**
  - Each breaking change has migration guide
  - Examples show old vs new

- [ ] **Consider deprecation warnings**
  - Could add deprecated versions temporarily?
  - Or is clean break preferred?

### Final Steps

- [ ] **Update CHANGELOG** (if exists)
  - List all changes
  - Categorize: Added, Changed, Fixed, Breaking
  - Reference issue numbers

- [ ] **Update version number** (if applicable)
  - Bump major version for breaking changes
  - Or follow project versioning scheme

- [ ] **Create pull request** (when ready)
  - Write comprehensive PR description
  - Reference design document
  - List all changes
  - Request reviews

---

## Notes and Issues

### Questions to Resolve

- Should ComputeC4ID() auto-canonicalize with a default resolver?
  - Pro: Easier API, fewer errors
  - Con: Hidden behavior, unexpected metadata sources
  - Decision: ?

- Should there be a ComputeC4IDUnsafe() that allows nulls?
  - For backward compatibility
  - Clearly marked as dangerous
  - Decision: ?

- Should scanners always produce canonical entries?
  - Or produce with nulls then require explicit canonicalization?
  - Decision: ?

### Issues Encountered

(Add issues as they arise during implementation)

### Design Decisions

(Document key decisions made during implementation)

---

## Progress Tracking

**Phase 1**: ☐ Not Started | ☐ In Progress | ☐ Completed
**Phase 2**: ☐ Not Started | ☐ In Progress | ☐ Completed
**Phase 3**: ☐ Not Started | ☐ In Progress | ☐ Completed
**Phase 4**: ☐ Not Started | ☐ In Progress | ☐ Completed

**Overall Progress**: 0% (0 of 147 tasks completed)

---

## References

- [CANONICAL_FORM_ENFORCEMENT.md](./CANONICAL_FORM_ENFORCEMENT.md) - Complete design specification
- [IMPLEMENTATION_NOTES.md](./IMPLEMENTATION_NOTES.md) - Implementation notes with canonical form section
- [SPECIFICATION.md](./SPECIFICATION.md) - Formal C4M specification (to be updated)
- [README.md](./README.md) - User documentation (to be updated)
