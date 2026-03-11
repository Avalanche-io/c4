package hashlib

import (
	"encoding/binary"
	"math/bits"

	"github.com/Avalanche-io/c4"
)

const (
	chunkBits = 5
	chunkMask = (1 << chunkBits) - 1 // 0x1f
)

// Chunk extracts a 5-bit chunk from the ID at the given trie depth.
// Depth 0 reads bits 0-4, depth 1 reads bits 5-9, etc.
func Chunk(id c4.ID, depth int) uint {
	bitPos := uint(depth) * chunkBits
	byteIdx := bitPos / 8
	bitOff := bitPos % 8

	if byteIdx >= 64 {
		return 0
	}

	// Read 2 bytes (big-endian) to handle chunks spanning a byte boundary.
	var raw uint16
	raw = uint16(id[byteIdx]) << 8
	if byteIdx+1 < 64 {
		raw |= uint16(id[byteIdx+1])
	}

	shift := 16 - bitOff - chunkBits
	return uint((raw >> shift) & chunkMask)
}

// Bit returns the value of bit n (0 = MSB) as 0 or 1.
func Bit(id c4.ID, n int) uint {
	return uint(id[n/8]>>(7-uint(n%8))) & 1
}

// CommonPrefix returns the number of leading bits shared by a and b.
// Returns 512 if the IDs are identical.
func CommonPrefix(a, b c4.ID) int {
	for i := 0; i < 64; i++ {
		x := a[i] ^ b[i]
		if x != 0 {
			return i*8 + bits.LeadingZeros8(x)
		}
	}
	return 512
}

// XorDist returns the bitwise XOR of two IDs (Kademlia distance metric).
func XorDist(a, b c4.ID) c4.ID {
	var out c4.ID
	for i := 0; i < 64; i++ {
		out[i] = a[i] ^ b[i]
	}
	return out
}

// Shard assigns an ID to one of n shards (0 to n-1).
// Uses the first 8 bytes of the ID for maximum entropy.
func Shard(id c4.ID, n int) int {
	if n <= 0 {
		return 0
	}
	v := binary.BigEndian.Uint64(id[:8])
	return int(v % uint64(n))
}

// LeadingZeros returns the number of leading zero bits in the ID.
func LeadingZeros(id c4.ID) int {
	for i := 0; i < 64; i++ {
		if id[i] != 0 {
			return i*8 + bits.LeadingZeros8(id[i])
		}
	}
	return 512
}

// firstDifferingBit returns the position of the first bit where a and b
// differ, or -1 if they are identical.
func firstDifferingBit(a, b c4.ID) int {
	n := CommonPrefix(a, b)
	if n == 512 {
		return -1
	}
	return n
}

func popcount(x uint32) int {
	return bits.OnesCount32(x)
}
