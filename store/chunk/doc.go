// Package chunk provides content-defined chunking (CDC) algorithms for
// efficient delta synchronization.
//
// This package implements FastCDC with Gear rolling hash for chunk boundary
// detection, and xxHash3 for fast chunk identification. It is designed for
// sync optimization, not content identity (use C4 IDs for that).
//
// Key design principles:
//   - C4 IDs are for identity (archival, permanent, cryptographic)
//   - Chunk hashes are for sync (ephemeral, session-scoped, speed-optimized)
//
// # Algorithms
//
// Gear Hash: XOR-based rolling hash using lookup table. ~3 GB/s on modern CPUs.
// Used by restic and borg backup tools.
//
// FastCDC: Content-defined chunking with normalized chunk sizes. 10x faster
// than Rabin fingerprinting. Skips minimum chunk size before checking boundaries.
//
// xxHash3: 64-bit fast hash for chunk identification. ~30 GB/s, 75x faster
// than SHA-512.
//
// # Usage
//
//	chunker := chunk.NewFastCDC(chunk.DefaultConfig())
//	chunks := chunker.Chunk(reader)
//	for chunk := range chunks {
//	    fmt.Printf("Offset: %d, Size: %d, Hash: %x\n",
//	        chunk.Offset, chunk.Size, chunk.Hash)
//	}
//
// # Configuration
//
// Chunk sizes can be tuned for different content types:
//
//	// Video/large files: larger chunks, less overhead
//	config := chunk.Config{MinSize: 256*KB, AvgSize: 1*MB, MaxSize: 4*MB}
//
//	// Text/code: smaller chunks, better dedup
//	config := chunk.Config{MinSize: 8*KB, AvgSize: 32*KB, MaxSize: 128*KB}
//
//	// Default (general binary data)
//	config := chunk.DefaultConfig() // 64KB min, 256KB avg, 1MB max
package chunk
