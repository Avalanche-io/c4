# Explored: Incremental Scanner Patterns

Progressive resolution strategy for scanning large filesystems:
1. Names first (readdir — fast, gives structure)
2. Metadata second (stat — permissions, size, mtime)
3. Content identity last (SHA-512 — expensive, parallelizable)

Each stage produces a valid c4m representation at its resolution level.
A partially-scanned filesystem is not "in progress" — it is a version.
Priority-driven scheduling can hash files the user is looking at first.

**Status:** The scan package (`cmd/c4/internal/scan/`) implements basic
progressive scanning. The priority-driven and paged patterns described
in the original doc are not yet implemented.
