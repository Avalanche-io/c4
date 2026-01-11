# ASC MHL and C4M: Complementary Tools

## Executive Summary

MHL and C4M both create manifests with hashes, but they serve different purposes:

- **MHL**: Couples metadata to content - attach arbitrary information to files
- **C4M**: Decouples structure from content - organize files before they arrive, reunite automatically

The key architectural difference: MHL's weaker hash support means content must stay attached to the manifest. C4's strong IDs mean content can safely be detached and stored separately.

## The C4M Advantage: Work Without the Files

The most immediate practical benefit of C4M is **communicating about files without having them**.

### The Problem Today

Nearly all file workflows require a full physical copy of the files to discuss them in detail:
- Post-production can't organize footage until it physically arrives
- Editors wait for terabytes to transfer before they can plan
- Any discussion of "which files?" requires having those files on hand
- Remote collaboration means shipping drives or waiting for uploads

### The C4M Solution

A C4M manifest is tiny - just text describing files with their C4 IDs. Send it from set via cell phone:

```
# DIT on set texts this to post-production
Camera A, Day 1 footage ID: c4xyz...
[attaches: day1_camera_a.c4m - 50KB]
```

Post-production receives the manifest in seconds and can immediately:
- See all file names, sizes, timestamps
- Reorganize the structure (rename shots, create folders)
- Identify what's needed vs. what's not
- Plan the edit structure
- Discuss specific files by name (the IDs are for machines, not conversations)

**All before a single byte of actual footage arrives.**

### Re-association on Arrival

When the physical media arrives (drive, upload, whatever):

```go
// The organized manifest from post-production
organized := LoadManifest("edit_structure.c4m")

// The actual files, from ANY source
for _, entry := range organized.Entries {
    content := FindContentByID(entry.C4ID)  // Source doesn't matter
    PlaceAt(content, entry.Path)            // Put it where the editor wants it
}
```

The files can arrive from:
- Original camera cards from set
- Backup drive from the vault
- Cloud upload from another facility
- Any source at all

**If the C4 ID matches, it's the right content. The data IS the data.**

### Why MHL Can't Do This

MHL is designed around files being present:
- Verification requires re-hashing files you have
- Metadata describes files at their current location
- Weak hashes mean you can't trust content from arbitrary sources

MHL couples metadata to content. C4M decouples structure from content - organize files before they arrive, reunite automatically.

## What Each System Does

### MHL: Metadata + Integrity

MHL is fundamentally a **metadata container**:

```
Files exist → Record metadata about files → Include hashes for integrity checking
```

The manifest holds:
- File paths and hashes
- Creator information, timestamps, locations
- Process notes, verification history
- Any arbitrary metadata the workflow needs

MHL uses hashing for integrity verification - "has this file changed since we recorded it?"

### C4M: Structure + Identity

C4M is fundamentally a **structure definition with content identity**:

```
Files exist → Record structure → Each file has its C4 ID
```

The manifest holds:
- File paths with filesystem metadata (mode, timestamp, size)
- C4 IDs for each file's content
- Hierarchical structure

C4M uses C4 IDs for identity - "this IS the content, identified forever by this ID."

## The Hash Quality Difference

This is the crucial architectural distinction:

### MHL: Variable Hash Strength

MHL supports MD5, SHA1, XXH64, C4, and others. This flexibility has consequences:

- **MD5/SHA1/XXH64 are not collision resistant** - MD5 and SHA1 have known attacks; XXH64 is a speed-optimized hash not designed for security
- **Content must stay attached** - you can't safely store files by hash since collisions could be forged
- **Which hash is authoritative?** - ambiguity when multiple algorithms are present

MHL trades security for compatibility with legacy systems.

### C4M: Strong Identity Only

C4M uses only C4 IDs (SMPTE ST 2114:2017, 512-bit SHA-512):

- **No known collision attacks** - finding two files with the same C4 ID is computationally infeasible
- **Content can be detached** - safely store files by C4 ID, reference them from any manifest
- **ID IS identity** - no ambiguity, no "which hash should I trust?"

This enables content-addressed storage: files stored once by ID, referenced from anywhere.

## Content Detachment: The Key Capability

Because C4 IDs are cryptographically strong, C4M enables something MHL cannot safely do:

```
# C4M workflow
project/
├── manifest.c4m           # Structure definition
└── .c4_store/             # Content stored by ID
    ├── c4abc.../          # File content
    └── c4def.../          # File content

# Or even more separated:
manifest.c4m               # Just the structure (can email this)
content-server.example.com # Files stored by C4 ID elsewhere
```

The manifest says "file.txt has ID c4abc..." - you can find that content anywhere it's stored by ID. The structure is fully detached from the content.

With MHL's weaker hashes, this would be dangerous - someone could forge a collision and substitute malicious content.

## Metadata: Different Philosophies

### MHL: Embedded Metadata

MHL embeds metadata directly:
```xml
<hash>
  <creatorinfo>John Doe, DIT</creatorinfo>
  <location>Stage 5</location>
  <process>original</process>
</hash>
```

This is MHL's purpose - it's a metadata container.

### C4M: Metadata is Just Another File

C4M doesn't embed arbitrary metadata because it doesn't need to. In the C4 worldview:

```
project/
├── footage/
│   └── A001.mov          c4abc...
├── footage.c4m           # Structure manifest
└── metadata.json         # Any metadata you want
```

Want to associate metadata with files? Add a file to the bundle. The C4 ID of that metadata file proves it hasn't changed either.

This isn't a limitation - it's a simpler model. Metadata is content too.

## Re-association: Finding Content by ID

In C4M, if you have:
- A manifest with IDs
- Files somewhere (maybe stored by ID, maybe not)

You can always re-associate them:

```go
// Trivial if files are stored by ID
content := store.Get(entry.C4ID)

// Possible even without ID-based storage (compute IDs to find matches)
for _, file := range allFiles {
    if c4.Identify(file) == entry.C4ID {
        // Found it
    }
}
```

MHL can't do this as reliably because weak hashes might have collisions.

## Verification vs Identity

### MHL: "Has it changed?"

MHL verifies by re-computing hash and comparing:
```
original_hash == computed_hash  →  file unchanged
original_hash != computed_hash  →  file changed (or hash collision)
```

### C4M: "What is it?"

C4M identifies by computing ID:
```
computed_id == expected_id  →  this IS that content
computed_id != expected_id  →  this IS different content
```

The distinction is subtle but important. MHL asks "is this still the same?" C4M says "this IS content X" (mathematical fact, not verification).

## In Production: Using Both Together

A typical production will use both formats - they don't intersect much:

**MHL handles**:
- Chain-of-custody documentation
- Embedded production metadata (who, when, where)
- Contractual compliance
- Integration with cameras and post-production tools

**C4M handles**:
- Working with files before they arrive
- Content-addressed storage and deduplication
- Distributed file synchronization
- Re-association from any source

**Together in a project**:

```
project/
├── ascmhl/               # MHL for chain of custody, production metadata
├── project.c4m           # C4M for content identity, remote collaboration
└── files/                # Actual content
```

Tools can bridge when needed:
- `c4 export --format mhl` - Generate MHL from C4M
- `c4 import-mhl` - Convert MHL to C4M

## Summary

| Aspect | MHL | C4M |
|--------|-----|-----|
| **Core model** | Couples metadata to content | Decouples structure from content |
| **Hash strength** | Variable (MD5, SHA1, etc.) | Strong only (C4/SHA-512) |
| **Work without files** | No - files must be present | Yes - organize before arrival |
| **Arbitrary metadata** | Embedded in format | Just another file in bundle |
| **Re-association** | By path/hash (collision risk) | By ID (mathematically certain) |

The formats complement each other. MHL couples metadata to content for production documentation. C4M decouples structure from content so you can work with files before they arrive. Use both.
