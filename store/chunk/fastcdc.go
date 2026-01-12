package chunk

import (
	"io"
	"sync"

	"github.com/zeebo/xxh3"
)

// FastCDC implements the FastCDC content-defined chunking algorithm.
// It uses Gear hash for boundary detection and xxHash3 for chunk identification.
//
// FastCDC is ~10x faster than Rabin fingerprinting because:
//   - Uses Gear hash (simpler, no modulo)
//   - Skips minimum chunk size (no boundary check until minSize)
//   - Normalized chunking (consistent chunk sizes)
//
// Reference: "FastCDC: a Fast and Efficient Content-Defined Chunking Approach"
type FastCDC struct {
	config Config
	mask   uint64
	maskS  uint64 // Small mask for normalized chunking near boundaries
	maskL  uint64 // Large mask for normalized chunking near boundaries
	err    error
	mu     sync.Mutex
}

// NewFastCDC creates a new FastCDC chunker with the given configuration.
func NewFastCDC(config Config) *FastCDC {
	// Ensure config has valid values
	if config.MinSize <= 0 {
		config.MinSize = 64 * KB
	}
	if config.AvgSize <= 0 {
		config.AvgSize = 256 * KB
	}
	if config.MaxSize <= 0 {
		config.MaxSize = 1 * MB
	}

	// Calculate masks for normalized chunking
	// Main mask for average size
	mask := nextPowerOf2(uint64(config.AvgSize)) - 1

	// Normalized chunking: use different masks near min/max boundaries
	// maskS (small): lower bits set = easier to hit boundary (used near minSize)
	// maskL (large): more bits set = harder to hit boundary (used near maxSize)
	maskS := mask >> 1 // Easier to match = smaller chunks
	maskL := mask << 1 // Harder to match = larger chunks

	return &FastCDC{
		config: config,
		mask:   mask,
		maskS:  maskS,
		maskL:  maskL,
	}
}

// nextPowerOf2 returns the next power of 2 >= n
func nextPowerOf2(n uint64) uint64 {
	if n == 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
}

// Chunk reads from r and returns a channel of chunks.
// The channel is closed when EOF is reached or an error occurs.
func (f *FastCDC) Chunk(r io.Reader) <-chan Chunk {
	out := make(chan Chunk, 16)

	go func() {
		defer close(out)

		// Read buffer - sized for efficiency
		bufSize := f.config.MaxSize * 2
		if bufSize > 16*MB {
			bufSize = 16 * MB
		}
		buf := make([]byte, bufSize)
		data := buf[:0]
		offset := int64(0)
		eof := false

		for {
			// Fill buffer until we have enough data or hit EOF
			for len(data) < f.config.MaxSize && !eof {
				n, err := r.Read(buf[len(data):])
				data = buf[:len(data)+n]
				if err == io.EOF {
					eof = true
				} else if err != nil {
					f.setErr(err)
					return
				}
			}

			// No more data to process
			if len(data) == 0 {
				break
			}

			// Find chunk boundary
			size := f.findBoundary(data)

			// Create chunk
			chunkData := make([]byte, size)
			copy(chunkData, data[:size])

			out <- Chunk{
				Offset: offset,
				Size:   size,
				Hash:   xxh3.Hash(chunkData),
				Data:   chunkData,
			}

			// Advance
			offset += int64(size)
			copy(buf, data[size:])
			data = buf[:len(data)-size]
		}
	}()

	return out
}

// ChunkBytes chunks in-memory data directly.
// More efficient than Chunk for small data.
func (f *FastCDC) ChunkBytes(data []byte) []Chunk {
	if len(data) == 0 {
		return nil
	}

	var chunks []Chunk
	offset := int64(0)

	for len(data) > 0 {
		size := f.findBoundary(data)

		chunks = append(chunks, Chunk{
			Offset: offset,
			Size:   size,
			Hash:   xxh3.Hash(data[:size]),
			Data:   data[:size],
		})

		offset += int64(size)
		data = data[size:]
	}

	return chunks
}

// findBoundary finds the next chunk boundary in data.
// Returns the size of the next chunk.
func (f *FastCDC) findBoundary(data []byte) int {
	n := len(data)
	if n <= f.config.MinSize {
		return n
	}

	// Create local gear hash (avoids race conditions)
	gear := NewGearHash(f.mask)

	// Skip to minimum size (no boundary possible before this)
	i := f.config.MinSize

	// Cap at max size or data length
	maxPos := f.config.MaxSize
	if maxPos > n {
		maxPos = n
	}

	// Normalized chunking: use different masks in different regions
	// Region 1: minSize to centerSize - use maskS (easier boundaries)
	// Region 2: centerSize to maxSize - use maskL (harder boundaries)
	centerSize := f.config.MinSize + (f.config.MaxSize-f.config.MinSize)/2

	// Region 1: easier to find boundary (encourages size near average)
	for ; i < centerSize && i < maxPos; i++ {
		gear.Roll(data[i])
		if (gear.Hash() & f.maskS) == 0 {
			return i
		}
	}

	// Region 2: harder to find boundary (discourages very large chunks)
	for ; i < maxPos; i++ {
		gear.Roll(data[i])
		if (gear.Hash() & f.maskL) == 0 {
			return i
		}
	}

	// Force boundary at max size or end of data
	return maxPos
}

// Err returns any error that occurred during chunking.
func (f *FastCDC) Err() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *FastCDC) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err == nil {
		f.err = err
	}
}

// ChunkReader wraps a reader and provides chunk-by-chunk reading.
// Useful when you want to process chunks sequentially without channels.
type ChunkReader struct {
	r      io.Reader
	config Config
	gear   *GearHash
	maskS  uint64
	maskL  uint64
	buf    []byte
	data   []byte
	offset int64
	eof    bool
	err    error
}

// NewChunkReader creates a ChunkReader for sequential chunk reading.
func NewChunkReader(r io.Reader, config Config) *ChunkReader {
	if config.MinSize <= 0 {
		config = DefaultConfig()
	}

	mask := nextPowerOf2(uint64(config.AvgSize)) - 1

	bufSize := config.MaxSize * 2
	if bufSize > 16*MB {
		bufSize = 16 * MB
	}

	return &ChunkReader{
		r:      r,
		config: config,
		gear:   NewGearHash(mask),
		maskS:  mask >> 1,
		maskL:  mask << 1,
		buf:    make([]byte, bufSize),
	}
}

// Next reads and returns the next chunk, or nil if done.
// Returns nil when EOF is reached. Check Err() for errors.
func (cr *ChunkReader) Next() *Chunk {
	// Fill buffer if needed
	for len(cr.data) < cr.config.MaxSize && !cr.eof {
		n, err := cr.r.Read(cr.buf[len(cr.data):])
		cr.data = cr.buf[:len(cr.data)+n]
		if err == io.EOF {
			cr.eof = true
		} else if err != nil {
			cr.err = err
			return nil
		}
	}

	if len(cr.data) == 0 {
		return nil
	}

	// Find boundary using normalized chunking
	size := cr.findBoundary()

	// Create chunk
	chunkData := make([]byte, size)
	copy(chunkData, cr.data[:size])

	chunk := &Chunk{
		Offset: cr.offset,
		Size:   size,
		Hash:   xxh3.Hash(chunkData),
		Data:   chunkData,
	}

	// Advance
	cr.offset += int64(size)
	copy(cr.buf, cr.data[size:])
	cr.data = cr.buf[:len(cr.data)-size]

	return chunk
}

func (cr *ChunkReader) findBoundary() int {
	n := len(cr.data)
	if n <= cr.config.MinSize {
		return n
	}

	cr.gear.Reset()
	i := cr.config.MinSize

	maxPos := cr.config.MaxSize
	if maxPos > n {
		maxPos = n
	}

	centerSize := cr.config.MinSize + (cr.config.MaxSize-cr.config.MinSize)/2

	for ; i < centerSize && i < maxPos; i++ {
		cr.gear.Roll(cr.data[i])
		if (cr.gear.Hash() & cr.maskS) == 0 {
			return i
		}
	}

	for ; i < maxPos; i++ {
		cr.gear.Roll(cr.data[i])
		if (cr.gear.Hash() & cr.maskL) == 0 {
			return i
		}
	}

	return maxPos
}

// Err returns any error that occurred during reading.
func (cr *ChunkReader) Err() error {
	return cr.err
}
