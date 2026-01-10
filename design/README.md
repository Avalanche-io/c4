# C4 Design Documentation

This directory contains design specifications and requirements documents for C4 features.

## Structure

- **Active**: Feature specifications currently being designed or implemented
- **archived/**: Superseded designs and historical documents
- **archived/transform/**: Archived transform package (removed for complexity reduction)
- **archived/superseded/**: Earlier design iterations now superseded by implementation

## Implemented Features

### [C4M Bundle System](c4m_bundle_system.md)
A system for handling unbounded filesystems by chunking output into multiple C4M files stored in a structured bundle format.

**Status**: ✅ Implemented
**Location**: `cmd/c4/internal/bundle/`
**Features**:
- Chunked progressive output with @base chains
- Resumable scans with last chunk tracking
- Directory compaction for large subdirectories
- Bundle extraction and materialization

### [Progressive UI Status](progressive_ui_status.md)
Real-time feedback system for long-running filesystem scans.

**Status**: ✅ Implemented
**Location**: `cmd/c4/internal/scan/`
**Features**:
- Three-stage scanning (structure, metadata, C4 IDs)
- Signal handling (SIGINT/SIGTERM graceful shutdown)
- Progress display with stage tracking

### [Progress Feedback](progress_feedback_and_id_caching.md)
Visual progress indicators with live statistics during scans.

**Status**: ⚠️ Partially Implemented
**Notes**:
- Progress display: ✅ Implemented
- ID caching (SQLite): ❌ Not implemented (future work)

## Reference Documents

### [C4M Metadata Extension](c4m_metadata_extension.md)
Proposed metadata directives for production workflows (@creator, @author, @location, etc.).

**Status**: 📋 Future Extension Proposal
**Notes**: Core layer directives (@base, @layer, @remove, @by, @time, @note, @data) are implemented. Production metadata directives are proposed for future implementation.

### [C4M Range Format](c4m_range_format.md)
Sequence notation and @data block specification for media file sequences.

**Status**: ✅ Implemented
**Location**: `c4m/sequence.go`

### [MHL vs C4M Comparison](mhl_vs_c4m_comparison.md)
Feature comparison between C4M and ASC Media Hash List format.

**Status**: 📚 Reference Document

### [C4M Superiority Analysis](c4m_superiority_analysis.md)
Strategic positioning of C4M's content-addressed approach.

**Status**: 📚 Reference Document

## Archived Documents

Documents in `archived/superseded/` represent earlier design iterations:

- `c4m_testing_strategy.md` - Testing approach now reflected in actual test files
- `bundle_chunking_notes.md` - Chunking details now in implementation
- `bundle_compartmentalized_chunking.md` - Compartmentalization now implemented

## Design Process

1. **Requirements Document**: Create detailed specification in this directory
2. **Review**: Team review and refinement of requirements
3. **Implementation**: Build feature according to specification
4. **Validation**: Ensure implementation meets all requirements
5. **Archive**: Move to `archived/` when superseded; update status when implemented

## Document Template

Each design document should include:

1. **Overview**: Brief description of the feature
2. **Motivation**: Why this feature is needed
3. **Requirements**: Detailed functional and non-functional requirements
4. **Architecture**: Technical design and structure
5. **Configuration**: User-configurable options
6. **Testing Strategy**: How to validate the implementation
7. **Success Criteria**: Measurable goals
8. **Future Extensions**: Potential enhancements

## Authoritative Sources

- **Implementation**: Source code in `c4m/`, `cmd/c4/internal/` is the source of truth
- **Specification**: `c4m/SPECIFICATION.md` documents the implemented format
- **Design docs**: Reference for intent and future direction
