# OpenAssetIO and C4: Integration Analysis

## Executive Summary

OpenAssetIO and C4 operate at different layers of the asset management stack:

- **OpenAssetIO**: API abstraction between DCCs and Asset Management Systems - handles workflow, versioning, and metadata through opaque entity references resolved at runtime
- **C4**: Content-addressed identity system - provides mathematical certainty about content identity

These systems are complementary. OpenAssetIO answers "where is asset X right now?" while C4 answers "what content IS this?" Together, they enable robust, portable, and verifiable asset workflows.

## OpenAssetIO Overview

### What It Is

OpenAssetIO is an ASWF (Academy Software Foundation) project providing a standardized API between:

- **Hosts**: Digital Content Creation tools (Maya, Nuke, Houdini, pipeline scripts)
- **Managers**: Asset Management Systems (ShotGrid, ftrack, AYON, custom solutions)

Rather than tools directly accessing file paths or proprietary AMS APIs, they communicate through OpenAssetIO's common interface.

### The Problem It Solves

Traditional pipelines reference assets by file path:
```
/projects/show/shot001/comp/v003/shot001_comp_v003.exr
```

This creates problems:
- **Non-portable**: Path depends on mount points, OS, render farm configuration
- **Static**: Can't represent "latest version" or "approved version"
- **Metadata-blind**: Path doesn't convey color space, frame range, or relationships
- **Tight coupling**: DCCs must understand each AMS's conventions

### Entity References

OpenAssetIO replaces paths with **Entity References** - opaque URIs owned by the Manager:

```
shotgrid://Project/123/Shot/456/PublishedFile/789
ftrack://show.shot001.comp.v003
bal:///project/shot001/comp?v=latest
```

The Host doesn't interpret these - it passes them to the Manager for **resolution**.

### Resolution

Resolution converts an Entity Reference into usable data:

```python
# Host asks: "Where is this asset?"
reference = "mymanager://show/shot001/comp"
traits = {LocatableContentTrait.kId}
result = manager.resolve(reference, traits, context)

# Manager returns: current path for this context
location = LocatableContentTrait(result).getLocation()
# → "/render/cache/shot001_comp_v003.exr" (on render farm)
# → "s3://bucket/shot001_comp_v003.exr" (in cloud)
# → "/mnt/projects/show/shot001/comp/v003/shot001_comp_v003.exr" (local)
```

The same Entity Reference resolves to different locations based on context.

### Traits

Traits define what data can be requested about an entity:

| Trait | Purpose | Properties |
|-------|---------|------------|
| `LocatableContent` | Where to access data | `location`, `mimeType` |
| `Version` | Revision information | `specifiedTag`, `stableTag` |
| `FrameRanged` | Temporal data | `startFrame`, `endFrame`, `fps` |
| `Image` | 2D visual data | (family marker) |
| `PixelBased` | Raster image | (family marker) |
| `OCIOColorManaged` | Color management | `colorspace` |
| `DisplayName` | Human-readable name | `name`, `qualifiedName` |
| `Proxy` | Alternate representation | `scaleRatio`, `qualityRatio` |

Hosts request specific traits; Managers return applicable data.

### Publishing

The reverse workflow - registering new assets:

```python
# Host creates new data, populates traits
data = TraitsData()
LocatableContentTrait(data).setLocation("/tmp/render_001.exr")
ImageTrait.imbueTo(data)
PixelBasedTrait.imbueTo(data)
OCIOColorManagedTrait(data).setColorspace("ACES - ACEScg")

# Manager handles storage, versioning, database entry
new_ref = manager.register(data, target_ref, context)
```

## C4 Overview

### What It Is

C4 (SMPTE ST 2114:2017) provides content-addressed identity - a 512-bit identifier derived from content using SHA-512.

### The Fundamental Difference

```
OpenAssetIO: Entity Reference → (resolution) → Location
C4:          Content → (hash) → ID
```

OpenAssetIO's Entity Reference is assigned by the Manager - it's an arbitrary handle. The relationship between reference and content is maintained by the database.

C4's ID is computed FROM the content - it's a mathematical fact. The relationship between ID and content is guaranteed by cryptography.

### Properties of C4 IDs

| Property | Meaning |
|----------|---------|
| **Deterministic** | Same content always produces same ID |
| **Collision-resistant** | Different content cannot have same ID |
| **Source-agnostic** | Any source with matching ID has identical content |
| **Immutable** | Content cannot change without changing ID |
| **Unforgeable** | Cannot create different content with target ID |

### C4M Manifests

C4M extends C4 to file collections:

```
drwxr-xr-x 2024-01-15T10:00:00Z 0 shot001/
-rw-r--r-- 2024-01-15T10:00:00Z 1048576 shot001/comp_v003.exr c4abc123...
-rw-r--r-- 2024-01-15T10:00:00Z 2097152 shot001/plate.exr c4def456...
```

The manifest describes structure; C4 IDs identify content. Structure and content can travel separately.

## The Indirection Models: Where They Overlap and Differ

Both systems use indirection - separating "what you want" from "getting the data." This is the source of apparent overlap.

### C4's "Push Intent, Pull Data"

```
┌─────────────────┐                    ┌─────────────────┐
│   C4M Manifest  │  ───(email)───►   │    Recipient    │
│  (structure +   │                    │                 │
│    C4 IDs)      │                    │  Has manifest,  │
└─────────────────┘                    │  no content yet │
        │                              └────────┬────────┘
        │                                       │
        ▼                                       ▼
   "I want files                         Fetch content
    organized                            by C4 ID from
    like THIS"                           ANY source
```

**Intent (pushed)**: The manifest describes structure and identifies content by C4 ID
**Data (pulled)**: Content fetched by ID from wherever it's available

The key property: **C4 ID IS the content.** The manifest carries identity itself, not a handle that must be resolved by an authority.

### OpenAssetIO's "Reference → Resolution → Data"

```
┌─────────────────┐                    ┌─────────────────┐
│  Nuke Script    │                    │    ShotGrid     │
│  (contains      │ ───(resolve)───►   │    (Manager)    │
│   entity refs)  │                    │                 │
└─────────────────┘                    └────────┬────────┘
        │                                       │
        │                               "That entity is
        ▼                                currently at
   "I want asset                         /path/to/v003"
    shot001/comp/                               │
    approved"                                   ▼
                                         Fetch from
                                         resolved path
```

**Reference**: Opaque handle to a workflow entity (shot, version, approval)
**Resolution**: Manager answers "where is that right now?"
**Data**: Fetched from the resolved location

The key property: **Manager controls the mapping.** The document carries a handle; the Manager must be consulted to know what it means.

### The Fundamental Difference

| Aspect | C4 | OpenAssetIO |
|--------|-----|-------------|
| **What does the reference identify?** | Content itself | Workflow entity (concept) |
| **Who controls the mapping?** | Mathematics (immutable) | Manager database (mutable) |
| **Can the mapping change?** | No - same content = same ID forever | Yes - "approved" version can change |
| **Can you work offline?** | Yes, if content is cached | No, need Manager to resolve |
| **Verification** | Compute ID, compare | Trust Manager's answer |

### A Concrete Example

**OpenAssetIO alone:**
```python
# Script contains reference
ref = "shotgrid://shot001/comp/approved"

# Must ask Manager what this means RIGHT NOW
location = manager.resolve(ref)  # → /path/to/v003.exr

# Load from that path
content = load(location)

# But wait... is this actually the right content?
# We're trusting the Manager's database AND the filesystem
```

If the Manager is offline, the script breaks.
If someone replaced v003.exr with garbage, we won't know.

**C4 alone:**
```python
# Manifest contains content identity
entry = manifest.get("shot001/comp.exr")  # C4 ID: c4abc123...

# Find content with that ID - could be anywhere
content = find_by_id(entry.c4_id)  # Local cache? Cloud? USB drive?

# Verify it's correct (just compute ID)
assert c4.identify(content) == entry.c4_id

# But wait... is this the APPROVED version?
# C4 doesn't know about workflow - just content
```

C4 guarantees you have the right content, but doesn't help with "which version is approved."

**Combined:**
```python
# Script contains reference
ref = "shotgrid://shot001/comp/approved"

# Ask Manager for location AND content identity
location, c4_id = manager.resolve(ref)  # path + C4 ID

# Check cache first - skip download if we have it
if cache.has(c4_id):
    content = cache.get(c4_id)
else:
    content = fetch(location)
    assert c4.identify(content) == c4_id  # Verify!
    cache.put(c4_id, content)

# Now we have:
# - The APPROVED version (OpenAssetIO workflow)
# - VERIFIED content (C4 identity)
```

### What Each System Provides

**OpenAssetIO provides:**
- Workflow concepts: versions, approvals, relationships
- Context-aware resolution: different paths for different environments
- Metadata: traits like color space, frame range
- Abstraction: same API works with any AMS

**OpenAssetIO requires:**
- Manager access to resolve references
- Trust in Manager's database accuracy
- Trust in filesystem integrity

**C4 provides:**
- Content identity: mathematical certainty about what content IS
- Verification: prove content matches expectations
- Source agnosticism: get content from anywhere
- Offline operation: work without central authority

**C4 requires:**
- The content (or a cache of it)
- Does NOT provide workflow concepts (versions, approvals)

### They Answer Different Questions

```
                    ┌─────────────────────────────────────────┐
                    │                                         │
   OpenAssetIO:     │  "What is the approved comp for shot 1? │
                    │   Where is it on this render farm?"     │
                    │                                         │
                    └─────────────────────────────────────────┘
                                        │
                                        ▼
                    ┌─────────────────────────────────────────┐
                    │                                         │
   Answer:          │  Entity: shotgrid://shot001/comp/v003   │
                    │  Location: /render/cache/shot001.exr    │
                    │                                         │
                    └─────────────────────────────────────────┘
                                        │
                                        ▼
                    ┌─────────────────────────────────────────┐
                    │                                         │
   C4:              │  "Is the file at that location actually │
                    │   the content I expect?"                │
                    │                                         │
                    └─────────────────────────────────────────┘
                                        │
                                        ▼
                    ┌─────────────────────────────────────────┐
                    │                                         │
   Answer:          │  Compute ID → c4abc123...               │
                    │  Expected:  → c4abc123...  ✓ Match      │
                    │                                         │
                    └─────────────────────────────────────────┘
```

OpenAssetIO: "Which content do we want?" (workflow decision)
C4: "Is this the content we want?" (verification)

## Comparison: Different Questions, Different Answers

| Aspect | OpenAssetIO | C4 |
|--------|-------------|-----|
| **Core question** | "Where is asset X?" | "What IS this content?" |
| **Identity model** | Opaque handle (Manager-assigned) | Content-derived (mathematical) |
| **Resolution** | Dynamic (context-dependent) | Fixed (same content = same ID) |
| **Versioning** | First-class concept | Orthogonal (different version = different ID) |
| **Metadata** | Traits with properties | External (just another file) |
| **Scope** | Workflow integration | Content identity |

### What OpenAssetIO Does That C4 Doesn't

1. **Version management**: "Get latest approved version" - concepts like `v003`, `vLatest`, approval status
2. **Context-aware resolution**: Same reference → different path on render farm vs. local
3. **Relationship navigation**: "Get all assets related to this shot"
4. **Metadata propagation**: Color space, frame range, proxy relationships
5. **UI delegation**: Manager provides custom browsers and pickers
6. **Publishing workflow**: Preflight, register, post-registration hooks

### What C4 Does That OpenAssetIO Doesn't

1. **Content verification**: Prove you have exactly the expected content
2. **Source-agnostic retrieval**: Get content from any source with matching ID
3. **Deduplication**: Same content stored once regardless of logical versions
4. **Offline description**: Work with file structure without having files
5. **Transfer integrity**: Detect any modification during transfer
6. **Universal identity**: Same ID across all systems, forever

## Integration Design: OpenAssetIO + C4

### The Gap OpenAssetIO Has

When OpenAssetIO resolves an Entity Reference to a location:

```python
location = LocatableContentTrait(result).getLocation()
# → "/render/cache/shot001_comp.exr"
```

**How do you know the file at that location is what you expect?**

Options without C4:
- Trust the Manager's database is correct
- Trust the filesystem hasn't been modified
- Compute a hash and... compare to what?

The Manager knows "entity 789 should be at this path" but has no content-independent way to verify.

### C4 Fills the Gap

Add a C4 trait to OpenAssetIO:

```python
# Resolution returns location AND content identity
location = LocatableContentTrait(result).getLocation()
c4_id = C4IdentityTrait(result).getId()

# Now we can verify
actual_id = c4.identify(location)
if actual_id != c4_id:
    raise ContentMismatchError(f"Expected {c4_id}, got {actual_id}")
```

The Manager's resolution is now verifiable.

### Proposed C4 Trait

```yaml
# Addition to OpenAssetIO-MediaCreation traits.yml
package: openassetio-mediacreation
description: C4 content identity trait

traits:
  identity:
    members:
      C4Identity:
        description: >
          Content identity per SMPTE ST 2114:2017. The ID is computed
          from content and provides mathematical certainty that content
          matches expectations.
        usage:
          - entity
        properties:
          id:
            type: string
            description: >
              The C4 ID of the content (90-character base58 string
              starting with "c4").
```

### Integration Scenarios

#### Scenario 1: Verified Resolution

```python
def load_asset_verified(reference, manager, context):
    """Load asset with C4 verification."""
    traits = {LocatableContentTrait.kId, C4IdentityTrait.kId}
    result = manager.resolve(reference, traits, context)

    location = LocatableContentTrait(result).getLocation()
    expected_id = C4IdentityTrait(result).getId()

    # Verify before loading
    actual_id = c4.identify(location)
    if actual_id != expected_id:
        raise VerificationError(f"Content mismatch at {location}")

    return load_file(location)
```

#### Scenario 2: Content-Addressed Caching

```python
def load_with_cache(reference, manager, context, cache):
    """Load from cache if content already available."""
    traits = {C4IdentityTrait.kId}
    result = manager.resolve(reference, traits, context)
    c4_id = C4IdentityTrait(result).getId()

    # Check cache by content identity, not by reference
    if cache.has(c4_id):
        return cache.get(c4_id)  # Skip download entirely

    # Fallback to location-based fetch
    traits = {LocatableContentTrait.kId}
    result = manager.resolve(reference, traits, context)
    location = LocatableContentTrait(result).getLocation()

    content = fetch(location)
    cache.put(c4_id, content)
    return content
```

#### Scenario 3: Multi-Source Resolution

```python
def resolve_from_anywhere(reference, manager, context, sources):
    """Get content from any available source."""
    traits = {C4IdentityTrait.kId}
    result = manager.resolve(reference, traits, context)
    c4_id = C4IdentityTrait(result).getId()

    # Try each source until content found
    for source in sources:
        content = source.get_by_id(c4_id)
        if content is not None:
            return content

    # Fallback to Manager's location
    traits = {LocatableContentTrait.kId}
    result = manager.resolve(reference, traits, context)
    return fetch(LocatableContentTrait(result).getLocation())
```

#### Scenario 4: C4M Manifest as Entity

A C4M manifest can be an entity in OpenAssetIO:

```python
# Entity Reference points to manifest
manifest_ref = "mymanager://project/shot001/delivery_manifest"

# Resolution returns manifest location and its C4 ID
result = manager.resolve(manifest_ref, traits, context)
manifest_path = LocatableContentTrait(result).getLocation()
manifest_id = C4IdentityTrait(result).getId()

# Load and verify manifest
manifest = c4m.load(manifest_path)
assert manifest.id == manifest_id  # Manifest verified

# Now we have verified identity for ALL files in the manifest
for entry in manifest.entries:
    # entry.c4_id is the content identity
    # entry.path is the logical path
    content = get_content_by_id(entry.c4_id)
```

### Publishing with C4

```python
def publish_with_c4(data_path, target_ref, manager, context):
    """Publish with C4 identity recorded."""
    # Compute C4 ID of content being published
    c4_id = c4.identify(data_path)

    # Populate traits including C4 identity
    data = TraitsData()
    LocatableContentTrait(data).setLocation(data_path)
    C4IdentityTrait(data).setId(c4_id)
    ImageTrait.imbueTo(data)

    # Manager stores both location and C4 ID
    return manager.register(data, target_ref, context)
```

The Manager's database now contains:
- Entity Reference (its handle)
- Current location (can change)
- C4 ID (immutable content identity)

### Version Relationships

OpenAssetIO tracks versions as relationships between entities. C4 treats each version as distinct content (different ID).

These perspectives are compatible:

```
Entity Reference: mymanager://shot001/comp/v003
↓ resolve
Location: /path/to/shot001_comp_v003.exr
C4 ID: c4abc123...  ← THIS specific content

Entity Reference: mymanager://shot001/comp/v004
↓ resolve
Location: /path/to/shot001_comp_v004.exr
C4 ID: c4def456...  ← DIFFERENT content
```

OpenAssetIO says "v004 is a newer version of v003" (workflow relationship).
C4 says "these are different content" (identity fact).

Both are true. The relationship is maintained by the Manager; the identity is mathematical.

## Deduplication Across Versions

Sometimes versions have identical content (re-approval, metadata-only changes):

```
v002: c4abc123...  ← Approved
v003: c4def456...  ← Changes
v004: c4abc123...  ← Reverted to v002 content (SAME ID)
```

With C4:
- Storage can deduplicate (v002 and v004 share content)
- Verification works (v004 resolves to same content as v002)
- OpenAssetIO versioning still works (different entity references)

## Manifest Integration

### C4M Manifest Trait

```yaml
traits:
  content:
    members:
      C4Manifest:
        description: >
          Entity represents a C4M manifest describing file structure
          with content identity.
        usage:
          - entity
        properties:
          entryCount:
            type: integer
            description: Number of entries in manifest
          totalSize:
            type: integer
            description: Total size of all content in bytes
```

### Specification

```yaml
specifications:
  - id: C4ManifestSpecification
    description: A C4M manifest file
    traits:
      - openassetio-mediacreation:content.LocatableContent
      - openassetio-mediacreation:identity.C4Identity
      - openassetio-mediacreation:content.C4Manifest
```

### Workflow: Delivery via C4M

```python
# 1. Create manifest of files to deliver
manifest = c4m.scan("/project/shot001/delivery/")

# 2. Publish manifest as entity
data = TraitsData()
LocatableContentTrait(data).setLocation("/tmp/delivery.c4m")
C4IdentityTrait(data).setId(manifest.id)
C4ManifestTrait(data).setEntryCount(len(manifest.entries))
C4ManifestTrait(data).setTotalSize(manifest.total_size)

delivery_ref = manager.register(data, shot_ref, context)

# 3. Recipient resolves manifest (can be sent via email, small file)
result = manager.resolve(delivery_ref, traits, context)
manifest = c4m.load(LocatableContentTrait(result).getLocation())

# 4. Recipient fetches content by C4 ID from ANY source
for entry in manifest.entries:
    content = content_store.get(entry.c4_id)
    write_to(entry.path, content)
```

## Implementation Recommendations

### For OpenAssetIO Managers

1. **Store C4 IDs**: When content is published, compute and store its C4 ID
2. **Return C4 IDs**: Include C4IdentityTrait in resolution responses
3. **Enable verification**: Clients can verify content matches expectation
4. **Support ID-based lookup**: Allow querying by C4 ID, not just Entity Reference

### For Hosts (DCCs)

1. **Request C4 trait**: Include C4IdentityTrait.kId in resolve trait sets
2. **Verify on load**: Check content matches expected C4 ID before use
3. **Cache by ID**: Use C4 ID as cache key, not path or Entity Reference
4. **Report mismatches**: Alert user when content verification fails

### For Pipeline Tools

1. **Use C4M for transfers**: Send manifest separately from content
2. **Source-agnostic fetch**: Get content from any source by C4 ID
3. **Verify at boundaries**: Check C4 IDs when content enters/leaves systems

## Summary

| Layer | System | Purpose |
|-------|--------|---------|
| **Workflow** | OpenAssetIO | Version management, metadata, relationships, UI |
| **Identity** | C4 | Content verification, deduplication, source-agnostic retrieval |

OpenAssetIO provides the "where" and "what version" - workflow concepts managed by the AMS.

C4 provides the "what exactly" - mathematical certainty about content.

Together:
- OpenAssetIO handles the dynamic, context-dependent parts (which path for this render farm, which version is approved)
- C4 handles the static, immutable part (this content has this identity, forever)

**They don't compete. They complement.**

A pipeline using both gets:
- Portable documents (Entity References)
- Dynamic resolution (context-aware paths)
- Version management (relationships)
- Rich metadata (traits)
- Content verification (C4 IDs)
- Deduplication (same content = same ID)
- Source-agnostic retrieval (fetch from anywhere)
- Offline workflows (C4M manifests describe content without having it)

## References

- [OpenAssetIO GitHub](https://github.com/OpenAssetIO/OpenAssetIO)
- [OpenAssetIO Documentation](http://docs.openassetio.org/OpenAssetIO/)
- [OpenAssetIO-MediaCreation](https://github.com/OpenAssetIO/OpenAssetIO-MediaCreation)
- [ASWF OpenAssetIO Page](https://www.aswf.io/blog/openassetio-beta-release-now-available/)
- [SMPTE ST 2114:2017 (C4 ID)](https://ieeexplore.ieee.org/document/7983807)
