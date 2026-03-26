# Directory Size Must Include C4M Content

## Problem

Currently, a directory's size field is the sum of its children's file content sizes. But a directory's own c4m description is also content — it has a C4 ID and lives in the store. Its byte size is not counted in the parent directory's size.

This is inconsistent. If you store a directory, its c4m occupies real bytes in the store. The parent directory's size should reflect the total cost of everything inside it, including the c4m entries that describe subdirectories.

## Example

```
drwxr-xr-x ... 862208 scenes/    c41ZGZv3...
  -rw-r--r-- ... 862208 monkey.blend  c439ryWc...
```

The scenes/ directory shows size 862208 — the size of monkey.blend. But scenes/ also has a c4m stored in the store (the one-level listing of its children). That c4m might be 100 bytes. The true size of scenes/ should be 862308 (file content + c4m content).

## Fix

When computing directory sizes during canonicalization / metadata propagation:
1. Sum file content sizes (existing behavior)
2. Also add the byte size of each subdirectory's canonical c4m

The subdirectory c4m size can be computed by marshaling the one-level canonical form and measuring the byte length.

## Impact

- Changes directory sizes across all implementations
- Changes directory C4 IDs (because size is part of the canonical entry)
- This is a breaking change for existing c4m files — directory IDs will differ

## Scope

All implementations: Go, Python, TypeScript, Swift, C

## Status

Pending. This is a correctness fix but also a breaking change. Needs careful consideration of migration path.
