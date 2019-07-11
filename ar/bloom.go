package ar

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	c4 "github.com/Avalanche-io/c4/id"
)

// updated, delete me

type Bitfield big.Int

func (b *Bitfield) SetBit(n uint32) {
	bigBits := (*big.Int)(b)
	bigBits.SetBit(bigBits, int(n), 1)
}

func (b *Bitfield) Bit(n uint32) bool {
	bigBits := (*big.Int)(b)
	return bigBits.Bit(int(n)) > 0
}

func (b *Bitfield) String() string {
	bigBits := (*big.Int)(b)
	return bigBits.String()
}

type Bloom struct {
	bits  *Bitfield
	p     float64 // Target false positive rate.
	n     int     // capacity of IDs
	m     int     // number of bits in bitfield
	k     int     // number of 'hash functions' (maximum of 16 at the moment.)
	Count int     // number IDs actually added to the filter
}

func ln(x float64) float64 {
	return math.Log(x)
}

func (b *Bloom) String() string {
	return fmt.Sprintf("*c4.Bloom=&{False Positives:%06f, Cap:%d, Bits:%d, Hashes:%d, Ids:%d, Bitfiled:%s}",
		b.p, b.n, b.m, b.k, b.Count, b.bits)
}

func (b *Bloom) Initialize() {
	if b.bits != nil {
		return
	}
	m := -((float64(b.n) * ln(b.p)) / (math.Ln2 * math.Ln2))
	// b.k = int(math.Ceil(m / float64(b.n) * math.Ln2))

	hash_period := float64(1 << 32)
	if m > hash_period {
		// We constrain the size of the bitfiled to the largest number the hash can
		// represent
		m = hash_period
	} else {
		// We insure the bitfiled is a power of 2 so the modulo of the hash is
		// uniformly distributed between 0 and m
		bitsize := math.Log2(m)
		if (bitsize - math.Floor(bitsize)) > 0 {
			bitsize = math.Ceil(bitsize)
		}
		m = math.Pow(2, bitsize)
	}
	b.m = int(m)
	// k: Number of hashes
	b.k = int(math.Ceil(float64(b.m) / float64(b.n) * math.Ln2))
	// Limited (for now) to the 16 no-overlapping 32bit hashes included in a c4 id
	if b.k > 16 {
		b.k = 16
	}
	f := new(big.Int)
	b.bits = (*Bitfield)(f)
}

// Capacity set the target capacity, and returns a pointer to
// the same bloom filter for appending other options.
func (b *Bloom) Capacity(n int) *Bloom {
	b.n = n
	b.bits = nil
	return b
}

// Rate sets the target false positive rate, and returns a pointer to
// the same bloom filter for appending other options.
func (b *Bloom) Rate(p float64) *Bloom {
	b.p = p
	b.bits = nil
	return b
}

// NewBloom creates a new BloomFilter, with default capacity and rate.
// The defaults currently are:
// capacity: 100000
// rate: 0.0015
func NewBloom() *Bloom {
	// create bloom filter with 100k capacity at 0.0015 false positive rate.
	b := new(Bloom).Capacity(100000).Rate(0.0015)
	return b
}

// Add adds any number of ids to the bloom filter.
func (b *Bloom) Add(ids ...*c4.ID) error {
	if b.bits == nil {
		b.Initialize()
	}
	for _, id := range ids {
		raw := id.Digest()
		i := 0
		for i < b.k {
			num := binary.BigEndian.Uint32(raw[i*4:i*4+4]) % uint32(b.m)
			b.bits.SetBit(num)
			i++
		}
		b.Count++
	}
	return nil
}

// Test returns true if id appears to be in the set. If test returns false
//  the ID is definitely not in the set.
func (b *Bloom) Test(id *c4.ID) bool {
	if b.bits == nil || id == nil {
		return false
	}

	raw := id.Digest()
	i := 0
	for i < b.k {
		num := binary.BigEndian.Uint32(raw[i*4:i*4+4]) % uint32(b.m)
		if !b.bits.Bit(num) {
			return false
		}
		i++
	}
	return true
}

func (b *Bloom) Size() int {
	return len((*big.Int)(b.bits).Bytes())
}
