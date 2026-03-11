package hashlib

import "github.com/Avalanche-io/c4"

const (
	cfBucketSize = 4   // entries per bucket
	cfMaxKicks   = 500 // max displacements before declaring full
)

// CuckooFilter is a probabilistic set supporting Add, Has, and Delete.
// It uses non-overlapping byte windows of the C4 ID as independent
// hash functions — no runtime hashing required.
//
// False positive rate is approximately 0.012% with 16-bit fingerprints.
type CuckooFilter struct {
	buckets []cfBucket
	count   int
	mask    uint // len(buckets) - 1
}

type cfBucket [cfBucketSize]uint16

// NewCuckooFilter creates a filter sized for the given capacity.
// The actual capacity may be slightly larger (rounded to a power of 2).
func NewCuckooFilter(capacity int) *CuckooFilter {
	n := nextPow2(capacity / cfBucketSize)
	if n < 1 {
		n = 1
	}
	return &CuckooFilter{
		buckets: make([]cfBucket, n),
		mask:    uint(n - 1),
	}
}

// Count returns the number of items in the filter.
func (f *CuckooFilter) Count() int { return f.count }

// Add inserts an ID into the filter. Returns false if the filter is full.
func (f *CuckooFilter) Add(id c4.ID) bool {
	fp := cfFingerprint(id)
	i1, i2 := f.bucketIndices(id)

	if f.buckets[i1].insert(fp) {
		f.count++
		return true
	}
	if f.buckets[i2].insert(fp) {
		f.count++
		return true
	}

	// Both buckets full — start cuckoo eviction.
	i := i1
	if fp&1 == 0 {
		i = i2
	}
	for kick := 0; kick < cfMaxKicks; kick++ {
		// Evict an entry (deterministic: use fp to choose slot).
		j := int(fp>>1) % cfBucketSize
		fp, f.buckets[i][j] = f.buckets[i][j], fp
		i = f.altIndex(i, fp)
		if f.buckets[i].insert(fp) {
			f.count++
			return true
		}
	}

	return false // filter is full
}

// Has returns true if the ID is probably in the filter.
// False positives are possible; false negatives are not.
func (f *CuckooFilter) Has(id c4.ID) bool {
	fp := cfFingerprint(id)
	i1, i2 := f.bucketIndices(id)
	return f.buckets[i1].has(fp) || f.buckets[i2].has(fp)
}

// Delete removes an ID from the filter. Returns false if not found.
func (f *CuckooFilter) Delete(id c4.ID) bool {
	fp := cfFingerprint(id)
	i1, i2 := f.bucketIndices(id)
	if f.buckets[i1].remove(fp) {
		f.count--
		return true
	}
	if f.buckets[i2].remove(fp) {
		f.count--
		return true
	}
	return false
}

// Reset clears the filter.
func (f *CuckooFilter) Reset() {
	for i := range f.buckets {
		f.buckets[i] = cfBucket{}
	}
	f.count = 0
}

// --- internals ---

// cfFingerprint extracts a 16-bit fingerprint from the ID.
// Uses bytes 0-1; ensures non-zero (0 means empty slot).
func cfFingerprint(id c4.ID) uint16 {
	fp := uint16(id[0])<<8 | uint16(id[1])
	if fp == 0 {
		return 1
	}
	return fp
}

// bucketIndices returns the two candidate bucket indices for the given ID.
func (f *CuckooFilter) bucketIndices(id c4.ID) (uint, uint) {
	// Primary: bytes 2-5 of the ID.
	i1 := uint(uint32(id[2])<<24|uint32(id[3])<<16|uint32(id[4])<<8|uint32(id[5])) & f.mask
	// Alternate: i1 XOR hash(fingerprint).
	fp := cfFingerprint(id)
	i2 := (i1 ^ uint(fpMix(fp))) & f.mask
	return i1, i2
}

// altIndex computes the alternate bucket for a fingerprint at bucket i.
func (f *CuckooFilter) altIndex(i uint, fp uint16) uint {
	return (i ^ uint(fpMix(fp))) & f.mask
}

// fpMix is a simple multiplicative hash for fingerprints.
func fpMix(fp uint16) uint32 {
	h := uint32(fp) * 0x5bd1e995
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return h
}

func (b *cfBucket) insert(fp uint16) bool {
	for i := range b {
		if b[i] == 0 {
			b[i] = fp
			return true
		}
	}
	return false
}

func (b *cfBucket) has(fp uint16) bool {
	for _, f := range b {
		if f == fp {
			return true
		}
	}
	return false
}

func (b *cfBucket) remove(fp uint16) bool {
	for i := range b {
		if b[i] == fp {
			b[i] = 0
			return true
		}
	}
	return false
}

func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}
