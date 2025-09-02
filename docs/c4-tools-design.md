# C4 Filesystem Tools Design Concept

## Overview
Unix-like filesystem tools that integrate C4 content addressing for integrity tracking and verification.

## Proposed Tools

### c4-cp (Copy with verification)
```bash
# Basic copy with C4 verification
c4-cp source.txt dest.txt

# Recursive copy with manifest generation
c4-cp -r sourcedir/ destdir/
# Creates destdir.c4m manifest automatically

# Copy and verify against existing manifest
c4-cp --verify manifest.c4m sourcedir/ destdir/

# Options:
  -v, --verify     Verify against manifest
  -m, --manifest   Generate manifest after copy
  -c, --check      Check C4 IDs match after copy
```

**Implementation ideas:**
- Compute C4 ID while reading source
- Write and verify destination matches
- Generate C4M manifest for copied directories
- Fail if verification doesn't match

### c4-mv (Move with tracking)
```bash
# Move with C4 verification
c4-mv oldname.txt newname.txt

# Move and update manifest
c4-mv --update-manifest dir.c4m file.txt newdir/file.txt

# Batch move with manifest
c4-mv --from-manifest moves.c4m
```

**Implementation ideas:**
- Verify source C4 ID before move
- Update manifests to reflect new paths
- Support manifest-based batch operations
- Atomic operations where possible

### c4-rm (Remove with tracking)
```bash
# Remove and update manifest
c4-rm --update-manifest dir.c4m file.txt

# Archive before removal (move to .c4-trash/)
c4-rm --archive file.txt

# Remove only if C4 ID matches
c4-rm --if-id c41j3C6Jq... file.txt
```

**Implementation ideas:**
- Optional archival to trash directory
- Manifest updates on removal
- Conditional removal based on C4 ID
- Generate removal manifest for audit

### c4-sync (Synchronize with C4M)
```bash
# Sync directories using C4M
c4-sync source/ dest/

# Sync based on manifest
c4-sync --from-manifest source.c4m dest/

# Dry run to show what would change
c4-sync --dry-run source/ dest/
```

**Implementation ideas:**
- Use C4M diff to determine changes
- Only copy files with different C4 IDs
- Support bidirectional sync
- Manifest-based sync for reproducibility

### c4-verify (Verify filesystem against manifest)
```bash
# Verify directory matches manifest
c4-verify manifest.c4m directory/

# Verify and fix permissions/timestamps
c4-verify --fix-metadata manifest.c4m directory/

# Continuous monitoring
c4-verify --watch manifest.c4m directory/
```

## Alternative Approach: Git-Style External Commands

Following git's extensibility model, we could use a naming convention where any executable named `c4-*` becomes available as a c4 subcommand:

```bash
# These executables in PATH:
c4-cp
c4-mv
c4-rm
c4-sync
c4-verify

# Can be called as:
c4 cp source dest
c4 mv old new  
c4 rm file
c4 sync dir1 dir2
c4 verify manifest dir
```

This provides several advantages:
- **Extensibility**: Users can add their own c4-* tools
- **Independent development**: Each tool can be its own project
- **Language flexibility**: Tools can be written in different languages
- **Gradual adoption**: Add tools as they're ready
- **Backwards compatibility**: Direct execution still works (`c4-cp` or `c4 cp`)

The main `c4` command would check if unknown subcommands match a `c4-*` executable and exec it with the remaining arguments.

## Integration Patterns

### Shell Aliases
Users could alias standard commands:
```bash
alias cp='c4-cp'
alias mv='c4-mv'
alias rm='c4-rm'
```

### Manifest Sidecar Files
Automatically create `.c4m` files alongside operations:
```
project/
  file.txt
  .file.txt.c4m  # Auto-generated sidecar
```

### Hook System
Support pre/post hooks for custom workflows:
```bash
export C4_POST_CP_HOOK="git add $DEST.c4m && git commit -m 'Added $DEST'"
```

## Benefits

1. **Transparent Integration** - Works like standard Unix tools
2. **Automatic Verification** - Every operation verified
3. **Audit Trail** - Manifest history of changes
4. **Data Integrity** - Detect corruption immediately
5. **Reproducibility** - Manifest-based operations

## Challenges

1. **Performance** - C4 computation adds overhead
2. **Atomicity** - Ensuring operations are atomic
3. **Compatibility** - Matching Unix tool semantics exactly
4. **Error Handling** - What to do on verification failure

## Implementation Priority

Suggested order:
1. `c4-verify` - Most immediately useful
2. `c4-cp` - Natural extension of verification
3. `c4-sync` - Builds on diff functionality
4. `c4-mv` / `c4-rm` - Less critical initially

## Open Questions

1. Should tools be separate binaries or subcommands?
2. How closely should we match Unix tool flags?
3. Should manifests be automatic or opt-in?
4. How to handle partial failures in batch operations?
5. Integration with version control systems?

## Next Steps

1. Prototype `c4-verify` as proof of concept
2. Get user feedback on interface design
3. Consider integration with existing tools (rsync, rclone, etc.)
4. Define manifest update semantics precisely