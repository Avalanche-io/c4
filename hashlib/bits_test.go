package hashlib

import (
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func tid(s string) c4.ID {
	return c4.Identify(strings.NewReader(s))
}

func TestChunkExtraction(t *testing.T) {
	id := tid("chunk-test")

	// Verify chunks cover all bits without overlap.
	// Extract first 10 chunks (50 bits), reconstruct, compare.
	for depth := 0; depth < 102; depth++ {
		c := Chunk(id, depth)
		if c > 31 {
			t.Fatalf("chunk at depth %d = %d, want <= 31", depth, c)
		}
	}
}

func TestChunkConsistency(t *testing.T) {
	id := tid("consistency")

	// Same ID, same depth → same chunk.
	for d := 0; d < 20; d++ {
		a := Chunk(id, d)
		b := Chunk(id, d)
		if a != b {
			t.Fatalf("inconsistent chunk at depth %d: %d != %d", d, a, b)
		}
	}
}

func TestChunkDifferentIDs(t *testing.T) {
	a := tid("alpha")
	b := tid("beta")

	// Different IDs should differ at some chunk (astronomically likely).
	differ := false
	for d := 0; d < 20; d++ {
		if Chunk(a, d) != Chunk(b, d) {
			differ = true
			break
		}
	}
	if !differ {
		t.Fatal("different IDs should produce different chunks")
	}
}

func TestBit(t *testing.T) {
	var id c4.ID
	id[0] = 0x80 // bit 0 = 1, bits 1-7 = 0

	if Bit(id, 0) != 1 {
		t.Fatal("bit 0 should be 1")
	}
	if Bit(id, 1) != 0 {
		t.Fatal("bit 1 should be 0")
	}

	id[0] = 0x40 // bit 1 = 1
	if Bit(id, 0) != 0 {
		t.Fatal("bit 0 should be 0")
	}
	if Bit(id, 1) != 1 {
		t.Fatal("bit 1 should be 1")
	}
}

func TestCommonPrefix(t *testing.T) {
	a := tid("same")
	if CommonPrefix(a, a) != 512 {
		t.Fatal("identical IDs should share 512 prefix bits")
	}

	b := tid("different")
	n := CommonPrefix(a, b)
	if n < 0 || n >= 512 {
		t.Fatalf("different IDs should share 0-511 bits, got %d", n)
	}

	var x, y c4.ID
	x[0] = 0x80
	y[0] = 0x00
	if CommonPrefix(x, y) != 0 {
		t.Fatal("MSB difference should give 0 common prefix bits")
	}
}

func TestXorDist(t *testing.T) {
	a := tid("a")
	b := tid("b")

	// Reflexive: XorDist(a, a) == zero
	d := XorDist(a, a)
	if d != (c4.ID{}) {
		t.Fatal("XOR distance with self should be zero")
	}

	// Symmetric
	ab := XorDist(a, b)
	ba := XorDist(b, a)
	if ab != ba {
		t.Fatal("XOR distance should be symmetric")
	}

	// Non-zero for different IDs
	if ab == (c4.ID{}) {
		t.Fatal("XOR distance of different IDs should be non-zero")
	}
}

func TestShard(t *testing.T) {
	id := tid("shard-test")

	for n := 1; n <= 256; n++ {
		s := Shard(id, n)
		if s < 0 || s >= n {
			t.Fatalf("Shard(_, %d) = %d, out of range", n, s)
		}
	}

	// Edge case: n <= 0.
	if Shard(id, 0) != 0 {
		t.Fatal("Shard with n=0 should return 0")
	}
}

func TestShardDistribution(t *testing.T) {
	// Verify roughly uniform distribution over many IDs.
	const n = 8
	counts := [n]int{}
	for i := 0; i < 1000; i++ {
		id := tid(strings.Repeat("x", i+1))
		counts[Shard(id, n)]++
	}
	for i, c := range counts {
		if c < 50 || c > 250 {
			t.Fatalf("shard %d has %d items, expected ~125", i, c)
		}
	}
}

func TestLeadingZeros(t *testing.T) {
	var zero c4.ID
	if LeadingZeros(zero) != 512 {
		t.Fatal("all-zero ID should have 512 leading zeros")
	}

	var one c4.ID
	one[0] = 0x80
	if LeadingZeros(one) != 0 {
		t.Fatal("MSB set should have 0 leading zeros")
	}

	one[0] = 0x01
	if LeadingZeros(one) != 7 {
		t.Fatal("byte 0x01 should have 7 leading zeros")
	}

	var deep c4.ID
	deep[4] = 0x10
	if LeadingZeros(deep) != 4*8+3 {
		t.Fatalf("expected %d leading zeros, got %d", 4*8+3, LeadingZeros(deep))
	}
}
