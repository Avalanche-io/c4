# Explored: Delta Sync via Content-Defined Chunking

When two versions of a large file differ (same name, different C4 IDs),
transferring the entire file is wasteful. FastCDC content-defined
chunking can identify changed regions and transfer only deltas.

Key design decision: C4 IDs (SHA-512, SMPTE standard) are for identity.
Chunk hashes (xxHash3, 8 bytes) are for sync optimization. These are
separate concerns with separate storage. Chunk indexes are ephemeral
and local — they don't affect the c4m format or content identity.

**Status:** Not implemented. The `internal/encryption` package has
XChaCha20-Poly1305 primitives. FastCDC chunking would live in a new
`c4d/sync` package.
