package ar

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"

	c4 "github.com/Avalanche-io/c4/id"
)

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

	// For the time being we extract up to 16 32 bit hashes from the c4 id
	// hash_period := math.Pow(2, 32)
	hash_period := float64(4294967296)
	if m > hash_period {
		// We constraint the size of the bitfiled to the largest number the hash can represent
		m = hash_period
	} else {
		// We insure the bitfiled is a power of 2 so the modulo of the hash is uniformly distributed
		// between 0 and m
		bitsize := math.Log2(m)
		if (bitsize - math.Floor(bitsize)) > 0 {
			bitsize = math.Ceil(bitsize)
		}
		m = math.Pow(2, bitsize)
	}
	b.m = int(m)
	// k: Number of hashes
	b.k = int(math.Ceil(float64(b.m) / float64(b.n) * math.Ln2))
	// Limited (for now) to the 16 32bit hashes included in a c4 id
	if b.k > 16 {
		b.k = 16
	}
	f := new(big.Int)
	b.bits = (*Bitfield)(f)
}

func (b *Bloom) Capacity(n int) *Bloom {
	b.n = n
	b.bits = nil
	return b
}

func (b *Bloom) Rate(p float64) *Bloom {
	b.p = p
	b.bits = nil
	return b
}

func NewBloom() *Bloom {
	// create bloom filter with 100k file capacity at 0.0015 false positive rate.
	b := new(Bloom).Capacity(100000).Rate(0.0015)
	// fmt.Printf("n: %d\tm: %d\tk: %d, bitmask: %f\n", n, b.Size, b.Hashes, math.Floor(math.Log2(float64(b.Size)))+1)
	// b.Initialize()
	return b
}

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
