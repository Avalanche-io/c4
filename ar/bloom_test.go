package ar_test

import (
	"strings"
	"testing"

	"github.com/cheekybits/is"

	c4ar "github.com/avalanche-io/c4/ar"
	c4id "github.com/avalanche-io/c4/id"
)

func TestBasicBloom(t *testing.T) {
	is := is.New(t)
	b := c4ar.NewBloom().Capacity(1000)
	is.OK(b)
	pfx := "http://foo.bar.bat/"
	keys := []string{"one", "two", pfx + "one", pfx + "two"}
	ids := make([]*c4id.ID, len(keys))
	for i, k := range keys {
		id := c4id.Identify(strings.NewReader(k))
		ids[i] = id
	}
	err := b.Add(ids...)
	is.NoErr(err)
	for _, id := range ids {
		is.True(b.Test(id))
	}
	badID := c4id.Identify(strings.NewReader("not in filter"))
	is.False(b.Test(badID))
}

// int to bytes
func writeInt32(i uint32, b *[]byte) {
	// equivalnt of return int32(binary.LittleEndian.Uint32(b))

	(*b)[0] = byte(i)
	(*b)[1] = byte(i >> 8)
	(*b)[2] = byte(i >> 16)
	(*b)[3] = byte(i >> 24)
	return
}

func readInt32(b []byte) uint32 {
	// equivalnt of return int32(binary.LittleEndian.Uint32(b))
	return uint32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}

type intbytes uint32

func (i intbytes) Read(p []byte) (n int, err error) {
	p[0] = byte(i)
	p[1] = byte(i >> 8)
	p[2] = byte(i >> 16)
	p[3] = byte(i >> 24)
	return 4, nil
}

func (i intbytes) Bytes() []byte {
	p := make([]byte, 4)
	p[0] = byte(i)
	p[1] = byte(i >> 8)
	p[2] = byte(i >> 16)
	p[3] = byte(i >> 24)
	return p
}

func TestPeformanceBloom(t *testing.T) {
	is := is.New(t)
	fp_rate := float64(0.015) // Maximum false positive rate
	positives := 10000        // Number of test values for positive cases
	negatives := 10000        // Number of test values for negative or false positive cases
	b := c4ar.NewBloom().Capacity(positives).Rate(fp_rate)
	is.NotNil(b)
	data := make([]byte, 4)
	writeInt32(328172, &data)
	is.Equal(readInt32(data), 328172)
	var id *c4id.ID
	e := c4id.NewEncoder()
	for i := 0; i < positives; i++ {
		e.Write(intbytes(i).Bytes())
		id = e.ID()
		e.Reset()
		is.NoErr(b.Add(id))
		// We expect 100% positive rate for the IDs added.
		is.True(b.Test(id))
	}
	// does not re-initialize
	b.Initialize()
	is.Equal(b.Count, positives)
	t.Logf("Successfully matched %d ids", positives)
	false_positive := 0

	for i := 0; i < negatives; i++ {
		// we create ids that will not match the filter set by offsetting
		e.Write(intbytes(i + positives).Bytes())
		id = e.ID()
		e.Reset()
		if b.Test(id) == true {
			false_positive++
		}
	}
	rate := float64(false_positive) / float64(negatives)
	t.Logf("\nfilter size: %d\nfalse positives: %d in %d\nrate: %0.8f", b.Size(), false_positive, negatives, rate)
	is.True(fp_rate >= rate)
}

func TestErrors(t *testing.T) {
	is := is.New(t)
	b := c4ar.NewBloom().Capacity(10).Rate(0.15) //float64(1.0)
	id := c4id.Identify(strings.NewReader("foo"))
	is.False(b.Test(id))
	b.Add(id)
	is.Equal(b.String(), "*c4.Bloom=&{False Positives:0.150000, Cap:10, Bits:64, Hashes:5, Ids:1, Bitfiled:2322168557933568}")
	id = c4id.Identify(strings.NewReader("bar"))
	b.Add(id)
	is.Equal(b.String(), "*c4.Bloom=&{False Positives:0.150000, Cap:10, Bits:64, Hashes:5, Ids:2, Bitfiled:11364552186991872}")
}
