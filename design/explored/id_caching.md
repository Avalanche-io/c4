# Explored: ID Caching for Repeated Scans

SHA-512 hashing is expensive. When scanning the same directory repeatedly
(backup verification, CI, integrity checks), most files haven't changed.
A persistent SQLite cache keyed on (path, size, mtime) could skip
re-computation for unchanged files, making repeated scans near-instant.

**Status:** Not implemented. Progress feedback (the other half of the
original design) was implemented separately in `cmd/c4/internal/scan/`.
