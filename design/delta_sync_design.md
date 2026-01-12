# Delta Sync: Content-Defined Chunking for Efficient Transfer

## Architectural Decision

**C4 IDs are for identity, not for sync optimization.**

C4 provides:
- Cryptographic strength (SHA-512)
- Long-term stability (content never changes its ID)
- SMPTE standardized (ST 2114:2017)
- 90 characters, computationally expensive

This is **exactly right** for content identity. But it's **wrong** for byte-level sync:
- Too slow for per-chunk computation
- Too large for chunk index storage
- Overkill for ephemeral sync state

**Solution**: Separate paths for different concerns.

```
┌─────────────────────────────────────────────────────────────────┐
│                         IDENTITY LAYER                           │
│                                                                   │
│   C4 ID (SHA-512 based, 90 chars)                               │
│   - "Is this the same content?"                                 │
│   - Permanent, archival, verifiable                             │
│   - Used for: manifests, deduplication, verification            │
└───────────────────────────────┬───────────────────────────────────┘
                                │
                                │ When C4 IDs differ but
                                │ we suspect partial overlap
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                          SYNC LAYER                              │
│                                                                   │
│   Fast hashes (xxHash3, BLAKE3, gear hash)                      │
│   - "What bytes changed?"                                       │
│   - Ephemeral, session-scoped, speed-optimized                  │
│   - Used for: delta transfer, chunk dedup, rolling updates      │
└─────────────────────────────────────────────────────────────────┘
```

## Package Location

### Option A: `c4/store/sync`
```
c4/
└── store/
    └── sync/
        ├── chunker.go      # CDC implementation
        ├── delta.go        # Delta computation
        ├── transfer.go     # Chunk transfer protocol
        └── rolling.go      # Rolling hash implementation
```

**Pros**: Part of core c4, no c4d dependency
**Cons**: Store package is currently simple

### Option B: `c4d/sync`
```
c4d/
└── sync/
    ├── chunker.go
    ├── delta.go
    ├── transfer.go
    └── protocol.go         # Wire protocol for chunk exchange
```

**Pros**: Naturally fits with daemon's transfer responsibilities
**Cons**: Can't use from c4 core

### Recommendation: `c4d/sync`

Delta sync is inherently a **transfer** concern, which is c4d's domain. The core c4 library computes identity; c4d handles movement. This follows the existing separation.

However, the **chunking algorithm** could live in `c4/store/chunk` if we want it reusable.

```
c4/
└── store/
    └── chunk/
        ├── cdc.go          # Content-defined chunking
        ├── gear.go         # Gear rolling hash
        └── rabin.go        # Rabin fingerprint (alternative)

c4d/
└── sync/
    ├── delta.go            # Uses c4/store/chunk
    ├── transfer.go
    └── protocol.go
```

## State of the Art: Rolling Hashes

### Gear Hash (Recommended)

Used by: restic, borg
Speed: ~3 GB/s on modern CPU
Principle: Uses lookup table, XOR-based rolling

```go
type GearHash struct {
    table [256]uint64  // Random lookup table
    hash  uint64
    mask  uint64       // Determines average chunk size
}

func (g *GearHash) Roll(b byte) uint64 {
    g.hash = (g.hash << 1) + g.table[b]
    return g.hash
}

func (g *GearHash) IsBoundary() bool {
    return (g.hash & g.mask) == 0
}
```

**Why gear hash**:
- Simpler than Rabin
- Faster (no modulo operations)
- Good chunk size distribution

### FastCDC

Academic paper: "FastCDC: a Fast and Efficient Content-Defined Chunking Approach"
Speed: 10x faster than Rabin
Principle: Gear hash + normalized chunking + skip optimization

```go
// FastCDC skips minimum chunk size before checking boundaries
func (f *FastCDC) NextChunk(data []byte) (size int) {
    // Skip to minimum chunk size (no boundary possible)
    i := f.minSize

    // Use gear hash for boundary detection
    for ; i < f.maxSize && i < len(data); i++ {
        f.gear.Roll(data[i])
        if f.gear.IsBoundary() {
            return i
        }
    }
    return i
}
```

### Rabin Fingerprint

Used by: rsync (conceptually), LBFS
Speed: ~500 MB/s
Principle: Polynomial rolling hash

```go
// Classic but slower than gear
func (r *Rabin) Roll(out, in byte) uint64 {
    r.hash = (r.hash - r.table[out]) * r.prime + uint64(in)
    return r.hash
}
```

### Recommendation: FastCDC with Gear Hash

Best balance of speed and chunk quality.

## Chunk Hash Selection

For chunk identification (NOT content identity), we need:
- Speed (computed per chunk, millions of times)
- Collision resistance for session scope (not archival)
- Small output (index efficiency)

### Options

| Hash | Speed | Output | Collision Resistance |
|------|-------|--------|---------------------|
| xxHash3 | 30 GB/s | 64-bit | Good for sync |
| xxHash128 | 25 GB/s | 128-bit | Very good |
| BLAKE3 | 5 GB/s | 256-bit | Cryptographic |
| SHA-256 | 500 MB/s | 256-bit | Cryptographic |
| C4 (SHA-512) | 400 MB/s | 512-bit | Overkill |

### Recommendation: xxHash3 (64-bit) for chunk index

- 75x faster than SHA-512
- 64-bit is enough for session-scoped dedup
- If paranoid: use xxHash128 or BLAKE3

```go
import "github.com/zeebo/xxh3"

func chunkHash(data []byte) uint64 {
    return xxh3.Hash(data)
}
```

## Chunk Size Strategy

### Variable-Size Chunking (CDC)

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Minimum | 64 KB | Avoid tiny chunks |
| Average | 256 KB | Balance dedup vs overhead |
| Maximum | 1 MB | Bound worst case |

```go
type ChunkConfig struct {
    MinSize int  // 64 KB
    AvgSize int  // 256 KB
    MaxSize int  // 1 MB
}

// Mask determines average: mask = avgSize - 1 (for power of 2)
// Boundary when: hash & mask == 0
```

### Adaptive Sizing

For different content types:
- **Video frames**: Larger chunks (1-4 MB), less boundary overhead
- **Text/code**: Smaller chunks (16-64 KB), better dedup
- **Binary data**: Default (256 KB)

```go
func ChunkConfigFor(contentType string) ChunkConfig {
    switch {
    case isVideo(contentType):
        return ChunkConfig{256*KB, 1*MB, 4*MB}
    case isText(contentType):
        return ChunkConfig{8*KB, 32*KB, 128*KB}
    default:
        return ChunkConfig{64*KB, 256*KB, 1*MB}
    }
}
```

## Delta Sync Protocol

### Phase 1: Signature Exchange

```
Sender                              Receiver
   |                                    |
   |  "I have file X, C4 ID: abc123"   |
   |----------------------------------->|
   |                                    |
   |  "I have old version, C4 ID: xyz" |
   |  "Here are my chunk signatures:"  |
   |  [hash1, hash2, hash3, ...]       |
   |<-----------------------------------|
   |                                    |
```

### Phase 2: Delta Computation

```go
type ChunkSignature struct {
    Offset int64
    Size   int
    Hash   uint64  // xxHash3
}

func ComputeDelta(newFile io.Reader, oldSigs []ChunkSignature) *Delta {
    // Build hash -> signature index
    index := make(map[uint64][]ChunkSignature)
    for _, sig := range oldSigs {
        index[sig.Hash] = append(index[sig.Hash], sig)
    }

    // Chunk new file, check against index
    chunker := NewFastCDC(newFile)
    delta := &Delta{}

    for chunk := range chunker.Chunks() {
        hash := xxh3.Hash(chunk.Data)

        if matches, found := index[hash]; found {
            // Receiver already has this chunk
            delta.AddReference(matches[0].Offset, chunk.Size)
        } else {
            // New data, must transfer
            delta.AddLiteral(chunk.Data)
        }
    }

    return delta
}
```

### Phase 3: Delta Transfer

```
Sender                              Receiver
   |                                    |
   |  Delta: [                         |
   |    REF(offset=0, len=256KB),      |  <- Use your chunk
   |    LITERAL(64KB of new data),     |  <- Here's new data
   |    REF(offset=512KB, len=128KB),  |  <- Use your chunk
   |    LITERAL(32KB of new data),     |
   |  ]                                |
   |----------------------------------->|
   |                                    |
   |  "Reconstructed, C4 ID verified"  |
   |<-----------------------------------|
```

### Delta Format

```go
type Delta struct {
    BaseC4ID   c4.ID         // Original file identity
    TargetC4ID c4.ID         // New file identity (for verification)
    Operations []DeltaOp
}

type DeltaOp struct {
    Type   OpType  // REF or LITERAL
    Offset int64   // For REF: offset in base file
    Size   int     // For REF: size to copy; For LITERAL: data size
    Data   []byte  // For LITERAL only
}

type OpType byte
const (
    OpRef     OpType = 'R'
    OpLiteral OpType = 'L'
)
```

## Integration with Transform Package

```go
// In c4m/transform

func (t *Transformer) Transform(source, target *c4m.Manifest) (*Plan, error) {
    // ... existing detection ...

    for path, targetEntry := range modifiedFiles {
        sourceEntry := sourceByPath[path]

        op := Operation{
            Type:   OpModify,
            Target: path,
            Entry:  targetEntry,
            // NEW: Include source C4 ID for delta sync
            SourceC4ID: sourceEntry.C4ID,
        }
        plan.Operations = append(plan.Operations, op)
    }
}

// In c4d/sync

func (s *Syncer) ExecuteModify(op transform.Operation) error {
    if op.SourceC4ID.IsNil() {
        // No base for delta, full transfer
        return s.fullTransfer(op.Entry.C4ID)
    }

    // Attempt delta sync
    delta, err := s.computeDelta(op.SourceC4ID, op.Entry.C4ID)
    if err != nil {
        return s.fullTransfer(op.Entry.C4ID)
    }

    // Transfer only the delta
    return s.transferDelta(delta)
}
```

## Performance Expectations

### Chunking Overhead

| File Size | Chunks (256KB avg) | Chunking Time | Hash Time |
|-----------|-------------------|---------------|-----------|
| 1 MB | 4 | 0.3 ms | 0.03 ms |
| 100 MB | 400 | 30 ms | 3 ms |
| 10 GB | 40,000 | 3 s | 300 ms |

With FastCDC at ~3 GB/s, chunking a 10 GB file takes ~3 seconds.
With xxHash3 at ~30 GB/s, hashing all chunks takes ~300 ms.

### Delta Transfer Savings

| Scenario | File Size | Changed | rsync | C4 Delta |
|----------|-----------|---------|-------|----------|
| Append to log | 1 GB | 1 MB | 1 MB | 1 MB |
| Edit in middle | 1 GB | 1 KB | ~256 KB | ~256 KB |
| Reorder sections | 1 GB | 0 (same data) | 1 GB | ~few KB |
| Random changes | 1 GB | 50% | 500 MB | 500 MB |

**Key insight**: Delta sync is most valuable when:
- Changes are localized
- Content is rearranged (CDC handles this better than rsync)
- Base file is available locally

## When to Use Each Path

```go
func (s *Syncer) Sync(op transform.Operation) error {
    switch {
    case op.Type == OpMove:
        return s.move(op)  // Free, local rename

    case op.Type == OpCopy:
        return s.copy(op)  // Local I/O

    case op.Type == OpAdd:
        return s.fullTransfer(op.Entry.C4ID)  // No base, must transfer all

    case op.Type == OpModify:
        // Decision point: delta vs full transfer
        if s.hasDeltaBase(op.SourceC4ID) {
            if delta := s.tryDelta(op); delta.Efficiency() > 0.3 {
                return s.transferDelta(delta)
            }
        }
        return s.fullTransfer(op.Entry.C4ID)

    case op.Type == OpDelete:
        return s.delete(op)  // Local delete
    }
}
```

### Efficiency Threshold

Don't use delta if it's not worth it:

```go
func (d *Delta) Efficiency() float64 {
    literalBytes := d.LiteralSize()
    totalBytes := d.TargetSize()

    // Efficiency = 1.0 means 100% savings (all refs)
    // Efficiency = 0.0 means no savings (all literal)
    return 1.0 - float64(literalBytes)/float64(totalBytes)
}

// Skip delta if less than 30% savings
const MinDeltaEfficiency = 0.3
```

## Implementation Plan

### Phase 1: Chunking Library (`c4/store/chunk`)

```go
// chunk/cdc.go
type Chunker interface {
    Chunk(r io.Reader) <-chan Chunk
}

type Chunk struct {
    Offset int64
    Size   int
    Data   []byte
    Hash   uint64  // xxHash3
}

// chunk/fastcdc.go
type FastCDC struct {
    config ChunkConfig
    gear   *GearHash
}

func (f *FastCDC) Chunk(r io.Reader) <-chan Chunk
```

### Phase 2: Delta Computation (`c4d/sync`)

```go
// sync/delta.go
type DeltaComputer struct {
    chunker chunk.Chunker
}

func (d *DeltaComputer) Compute(base, target io.Reader) (*Delta, error)
func (d *DeltaComputer) Apply(base io.Reader, delta *Delta) (io.Reader, error)
```

### Phase 3: Transfer Protocol (`c4d/sync`)

```go
// sync/transfer.go
type DeltaTransfer struct {
    store  store.Store
    peers  []Peer
}

func (t *DeltaTransfer) Send(delta *Delta, target Peer) error
func (t *DeltaTransfer) Receive(baseC4ID c4.ID) (*Delta, error)
```

### Phase 4: Integration

```go
// Transform package uses sync for MODIFY operations
// Full transfer for ADD operations
// Local operations for MOVE/COPY
```

## Summary

| Concern | Solution | Hash | Speed |
|---------|----------|------|-------|
| Content Identity | C4 ID | SHA-512 | 400 MB/s |
| Chunk Boundaries | Gear/FastCDC | N/A | 3 GB/s |
| Chunk Dedup | xxHash3 | 64-bit | 30 GB/s |
| Chunk Verification | Optional BLAKE3 | 256-bit | 5 GB/s |

**Separation of concerns**:
- C4 ID answers: "Is this the same content?" (archival, permanent)
- Chunk hash answers: "Have I seen these bytes before?" (ephemeral, session-scoped)

**The transform package remains focused on**:
- What operations are needed (move, copy, add, modify, delete)
- Which files have the same C4 ID (content identity)

**The sync package handles**:
- How to efficiently transfer bytes when C4 IDs differ
- Chunk-level deduplication during transfer
- Delta computation and application

This gives us rsync-like efficiency for modifications while preserving C4's content identity model for everything else.
