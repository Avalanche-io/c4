# C4M and Production Metadata: Design Analysis

## The Core Question

Should C4M include production metadata like creator info, location, and verification logs?

**Answer: No, but with nuance.**

Production metadata serves real needs, but embedding it in C4M conflicts with content-addressed identity. This document explains why, and outlines the right approach.

## What MHL Provenance Actually Is

### The Honest Truth About MHL "Provenance"

MHL doesn't **provide** provenance - it provides a **place to write claims about provenance**:

1. Someone voluntarily runs `ascmhl create`
2. They type in their name, location, role
3. The software writes that into XML

Nothing prevents:
- Not running the tool at all (thumbdrive sits in a hot car, no log entry)
- Lying about who you are
- Forging entries after the fact
- Running it at a different time than claimed

The XML records **claims**, not **proof**. The "chain of custody" is really "a log of when people chose to run MHL and what they typed."

### What MHL Actually Provides

| What It Seems To Provide | What It Actually Provides |
|--------------------------|---------------------------|
| Proof of who handled media | A place to record who CLAIMS to have handled it |
| Proof of location | A place to record CLAIMED location |
| Chain of custody | A log of when MHL was voluntarily run |
| Verification history | Records of when someone CHOSE to verify |

### Why Production Uses It Anyway

MHL's value isn't cryptographic proof - it's **standardized documentation**:

- **Industry convention**: Studios expect MHL, contracts specify it
- **Legal recognition**: Lawyers accept XML logs as "documentation"
- **Tool ecosystem**: Cameras, DITs, post houses all speak MHL
- **Process formalization**: Having a standard encourages consistent practice

The actual provenance comes from **human trust and process** - MHL just gives that process a standard format.

### C4's More Honest Approach

C4 accepts that:
- **Content identity** can be proven mathematically (the ID)
- **Process documentation** requires human trust regardless of format
- **No format can make someone run software** they don't want to run
- **No format can prevent lying** about metadata

So C4 separates these clearly:
- The **ID proves content** (mathematical, unforgeable)
- **Everything else is external** (documented however you want)

An email saying "the footage ID is c4xyz, signed John Doe DIT" has exactly as much provenance value as an MHL entry claiming the same thing - both require trusting John Doe.

### What C4 Actually Proves vs What MHL Claims

| Aspect | C4 Proof | MHL Claim |
|--------|----------|-----------|
| Content identity | Mathematical (ID = content) | Hash match (same mechanism) |
| Who created manifest | None (external documentation) | XML field (unverified claim) |
| When it was created | None (external documentation) | XML field (unverified claim) |
| Where it was created | None (external documentation) | XML field (unverified claim) |
| That verification occurred | Comparing IDs proves content match | XML entry claims verification happened |

The key insight: **Both systems require trusting human process for provenance. C4 is honest about this; MHL provides a standard format for claims.**

## Why NOT to Embed Metadata in C4M

### The ID Stability Problem

```
# Manifest v1
@c4m 1.0
@creator 2025-01-10T09:00:00Z set.local john c4 1.0
-rw-r--r-- ... file.txt c4abc...

# Manifest ID: c4xyz...
```

```
# Manifest v2 - same files, different creator
@c4m 1.0
@creator 2025-01-10T10:00:00Z office.local jane c4 1.0
-rw-r--r-- ... file.txt c4abc...

# Manifest ID: c4def... (DIFFERENT!)
```

The same files now have different manifest IDs because metadata differs. This breaks the fundamental property: **identical content should have identical identity**.

### The Conceptual Confusion

C4M answers: "What files exist with what content?"

MHL metadata answers: "Who recorded this, when, where, why?"

These are different questions. Mixing them creates a format that does neither well.

## The Right Approach: External Metadata

### Pattern 1: Sidecar Metadata File

```
project/
├── footage.c4m           # Pure content manifest (stable ID)
├── footage.c4m.meta      # Process documentation (mutable)
```

**footage.c4m** (content identity):
```
@c4m 1.0
-rw-r--r-- 2025-01-10T09:00:00Z 512000000 A001.mov c4abc...
-rw-r--r-- 2025-01-10T09:01:00Z 487000000 A002.mov c4def...
```

**footage.c4m.meta** (process documentation):
```json
{
  "manifest_id": "c4xyz...",
  "creator": {
    "name": "John Doe",
    "email": "john@example.com",
    "role": "DIT",
    "timestamp": "2025-01-10T09:15:00Z"
  },
  "location": "Stage 5, Pinewood Studios",
  "notes": "Camera A, CF cards A001-A002",
  "tool": {"name": "c4", "version": "1.0.0"}
}
```

The manifest ID is stable. Metadata can be added, corrected, or extended without changing the content identity.

### Pattern 2: The Email Proof

Your example is perfect:

```
From: DIT on set
To: Post production
Subject: Footage delivery

The camera footage C4 ID is: c4xyz...

- John Doe, DIT
- Stage 5, Pinewood Studios
- 2025-01-10 9:15 AM
```

Post production receives files, computes ID, gets `c4xyz...` - **mathematically proven** to be identical to what was on set.

The email itself is the provenance record. The C4 ID is the proof. No metadata needs to be embedded.

### Pattern 3: Database/Registry

For organizations:
```sql
CREATE TABLE manifests (
    c4_id TEXT PRIMARY KEY,
    created_at TIMESTAMP,
    created_by TEXT,
    location TEXT,
    notes TEXT,
    project_id TEXT
);

INSERT INTO manifests VALUES (
    'c4xyz...',
    '2025-01-10 09:15:00',
    'John Doe',
    'Stage 5',
    'Camera A footage',
    'proj_12345'
);
```

Now you have full provenance tracking without touching the manifest format.

## What C4M Already Has (And Why It's Enough)

### Implemented Directives

| Directive | Purpose | Why It Belongs |
|-----------|---------|----------------|
| `@base` | References parent manifest | Content relationship (cryptographic) |
| `@layer` | Starts changeset | Content structure |
| `@remove` | Marks deletions | Content structure |
| `@by` | Layer author | Changeset attribution (part of layer) |
| `@time` | Layer timestamp | Changeset timing (part of layer) |
| `@note` | Layer description | Changeset documentation (part of layer) |
| `@data` | Embedded data blocks | Content (ID lists, sidecar data) |
| `@expand` | Sequence expansion | Content structure |

Note: `@by`, `@time`, `@note` are **layer metadata**, not manifest metadata. They document changes within a layered manifest, not who created the overall manifest.

### Why @base Is Different From MHL Generations

MHL generations:
```
ascmhl/
├── 0001_gen1.mhl  (labeled "generation 1")
├── 0002_gen2.mhl  (labeled "generation 2", references gen1 by filename)
```

C4M @base:
```
@base c4abc123...  # Cryptographic reference to EXACT previous state
```

The @base ID **is** the previous state. You can't fake it, mislabel it, or accidentally reference the wrong thing. Generation numbers are human labels; @base is mathematical proof.

## MHL Interoperability: The Right Solution

Instead of making C4M more like MHL, provide tools to bridge the formats.

### Export: C4M → MHL

```bash
c4-mhl export footage.c4m \
    --creator "John Doe" john@example.com DIT \
    --location "Stage 5" \
    --comment "Camera A footage" \
    > footage.mhl
```

This generates MHL with all the metadata production workflows need, from a minimal C4M source.

### Import: MHL → C4M

```bash
c4-mhl import ascmhl/ --extract-metadata > footage.c4m
# Metadata goes to sidecar: footage.c4m.meta
```

### Why This Is Better

1. **C4M stays minimal**: Pure content identity
2. **MHL workflows supported**: Full metadata when exporting
3. **No format compromise**: Each format does what it's designed for
4. **User choice**: Add metadata at export time, not manifest creation time

## What NOT to Implement

The following proposed directives should **NOT** be added to C4M:

| Directive | Why Not |
|-----------|---------|
| `@creator` | Changes manifest ID; use external metadata |
| `@author` | Changes manifest ID; use external metadata |
| `@location` | Changes manifest ID; use external metadata |
| `@comment` | Changes manifest ID; use external metadata |
| `@process` | MHL workflow concept; doesn't apply to content identity |
| `@verify` | "Verification" is just ID comparison in C4 |
| `@ignore` | Scanner config, not content state |
| `@previous` | @base already provides this cryptographically |
| `@chain` | Human labeling; @base chain is the real structure |
| `@hashdate` | ID is determined by content, not when computed |
| `@roothash` | Manifest ID IS the root identity |
| `@hashformat` | Multiple hashes undermines single-identity model |

## Conclusion

### C4M's Role
- **Pure content identity**: What exists, with what content
- **Cryptographic proof**: ID proves authenticity mathematically
- **Minimal format**: Easy to parse, small, stable

### External Metadata's Role
- **Process documentation**: Who, when, where, why
- **Audit trails**: For legal/operational needs
- **Mutable information**: Can be corrected without changing content ID

### MHL's Role
- **Production workflows**: Industry-standard format
- **Human-readable audit trail**: XML with full provenance
- **Contractual compliance**: What studios and insurers expect

### The Right Architecture

```
Content Identity          Process Documentation       Production Workflow
     (C4M)                  (External)                   (MHL Export)
       │                        │                            │
       ▼                        ▼                            ▼
   footage.c4m  ─────────► footage.c4m.meta  ─────────► footage.mhl
   (stable ID)              (mutable)                  (full metadata)
```

C4M proves content. External systems document process. MHL export serves production needs. Each layer does what it's designed for.
