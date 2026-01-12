package chunk

import "io"

// Size constants for convenience
const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

// Chunk represents a content-defined chunk of data
type Chunk struct {
	Offset int64  // Byte offset in the source
	Size   int    // Size in bytes
	Hash   uint64 // xxHash3 of the chunk data
	Data   []byte // Chunk data (may be nil if not retained)
}

// Config defines the chunking parameters
type Config struct {
	MinSize int // Minimum chunk size (skip boundary check below this)
	AvgSize int // Target average chunk size
	MaxSize int // Maximum chunk size (force boundary at this point)
}

// DefaultConfig returns the default chunking configuration
// suitable for general binary data.
//
// MinSize: 64 KB  - Avoid tiny chunks
// AvgSize: 256 KB - Balance dedup vs overhead
// MaxSize: 1 MB   - Bound worst case
func DefaultConfig() Config {
	return Config{
		MinSize: 64 * KB,
		AvgSize: 256 * KB,
		MaxSize: 1 * MB,
	}
}

// VideoConfig returns chunking configuration optimized for
// video and large media files.
//
// Larger chunks reduce overhead and improve streaming performance.
func VideoConfig() Config {
	return Config{
		MinSize: 256 * KB,
		AvgSize: 1 * MB,
		MaxSize: 4 * MB,
	}
}

// TextConfig returns chunking configuration optimized for
// text and source code files.
//
// Smaller chunks improve deduplication for fine-grained edits.
func TextConfig() Config {
	return Config{
		MinSize: 8 * KB,
		AvgSize: 32 * KB,
		MaxSize: 128 * KB,
	}
}

// Chunker is the interface for content-defined chunking algorithms
type Chunker interface {
	// Chunk reads from r and returns a channel of chunks.
	// The channel is closed when EOF is reached or an error occurs.
	// Call Err() after the channel closes to check for errors.
	Chunk(r io.Reader) <-chan Chunk

	// ChunkBytes chunks in-memory data directly.
	// More efficient than Chunk for small data.
	ChunkBytes(data []byte) []Chunk

	// Err returns any error that occurred during chunking.
	// Should be called after the Chunk channel closes.
	Err() error
}

// Signature represents the chunk signature of a file,
// used for delta computation.
type Signature struct {
	Chunks []ChunkSig
}

// ChunkSig is a lightweight chunk signature for delta sync
type ChunkSig struct {
	Offset int64
	Size   int
	Hash   uint64
}

// NewSignature creates a signature from chunks
func NewSignature(chunks []Chunk) *Signature {
	sigs := make([]ChunkSig, len(chunks))
	for i, c := range chunks {
		sigs[i] = ChunkSig{
			Offset: c.Offset,
			Size:   c.Size,
			Hash:   c.Hash,
		}
	}
	return &Signature{Chunks: sigs}
}

// Index builds a hash-to-chunk index for fast lookup
func (s *Signature) Index() map[uint64][]ChunkSig {
	idx := make(map[uint64][]ChunkSig)
	for _, c := range s.Chunks {
		idx[c.Hash] = append(idx[c.Hash], c)
	}
	return idx
}
