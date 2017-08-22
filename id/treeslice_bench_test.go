package id

import (
	"bytes"
	"container/heap"
	"math/rand"
	"sort"
	"testing"
)

// These benchmarks show the cost of different sorting
// strategies for creating Digest slices.  They show
// that sorting after creation is best.
//
// ns/op times reported by the benchmarks should
// be divided by the batch_size (100000)

const batch_size = 100000

// Sortable digest slice

type sortDS []Digest

func (s sortDS) Len() int {
	return len(s)
}

func (s sortDS) Less(i, j int) bool {
	return bytes.Compare(s[i], s[j]) == -1
}

func (s sortDS) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Heap digest slice
type heapDS []Digest

func (s *heapDS) Len() int {
	return len(*s)
}

func (s *heapDS) Less(i, j int) bool {
	return bytes.Compare((*s)[i], (*s)[j]) == -1
}

func (s *heapDS) Swap(i, j int) {
	(*s)[i], (*s)[j] = (*s)[j], (*s)[i]
}

func (s *heapDS) Push(x interface{}) {
	*s = append(*s, x.(Digest))
}

func (s *heapDS) Pop() interface{} {
	d := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return d
}

// 56741010 ns/op == 567 ns/n
func BenchmarkSortSlice(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		// stop the clock and create a set of random digests
		b.StopTimer()
		s := make(sortDS, batch_size)
		for i := 0; i < batch_size; i++ {
			s[i] = randomDigest()
		}
		// restart the clock
		b.StartTimer()
		sort.Sort(s)
	}
}

// Accidentally sorting an already sorted slice, is idempotent but wasteful.
// How wasteful?
func BenchmarkSortingASortedSlice(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		// stop the clock and create a set of random digests
		b.StopTimer()
		s := make(sortDS, batch_size)
		for i := 0; i < batch_size; i++ {
			s[i] = randomDigest()
		}
		// restart the clock
		b.StartTimer()
		sort.Sort(s)
		sort.Sort(s)
	}
}

// What is the added cost of detecting a digest slice that is already sorted.
func BenchmarkDetectingASortedSlice(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		// stop the clock and create a set of random digests
		b.StopTimer()
		s := make(sortDS, batch_size)
		for i := 0; i < batch_size; i++ {
			s[i] = randomDigest()
		}
		// restart the clock
		b.StartTimer()
		sort.Sort(s)
		if !sort.IsSorted(s) {
			b.Errorf("incorrectly evaluated to not sorted")
			sort.Sort(s)
		}
	}
}

// Insert sort is slow.  100x !
func BenchmarkInsertSlice(b *testing.B) {
	b.ReportAllocs()
	s := make(DigestSlice, batch_size)

	// stop the clock and create a set of random digests
	b.StopTimer()
	for i := 0; i < batch_size; i++ {
		s[i] = randomDigest()
	}
	// restart the clock
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		var target DigestSlice
		for i := range s {
			target.Insert(s[i])
		}
	}
}

// Heap sort is rather slow, 3x, plus look at all the allocs 2n!
func BenchmarkHeapSlice(b *testing.B) {
	b.ReportAllocs()
	s := make(heapDS, batch_size)
	ss := make([]Digest, batch_size)

	// stop the clock and create a set of random digests
	b.StopTimer()
	for i := 0; i < batch_size; i++ {
		s[i] = randomDigest()
	}
	// restart the clock
	b.StartTimer()
	for n := 0; n < b.N; n++ {
		target := new(heapDS)
		heap.Init(target)
		for i := 0; i < batch_size; i++ {
			heap.Push(target, s[i])
		}
		i := 0
		for target.Len() > 0 {
			ss[i] = heap.Pop(target).(Digest)
			i++
		}
	}
}

// The cost of the first half of building a heap
func BenchmarkHalfHeapSlice(b *testing.B) {
	b.ReportAllocs()
	s := make(heapDS, batch_size)
	// ss := make([]Digest, batch_size)

	// stop the clock and create a set of random digests
	b.StopTimer()
	for i := 0; i < batch_size; i++ {
		s[i] = randomDigest()
	}
	// restart the clock
	b.StartTimer()
	for n := 0; n < b.N; n++ {
		target := new(heapDS)
		heap.Init(target)
		for i := 0; i < batch_size; i++ {
			heap.Push(target, s[i])
		}
		// i := 0
		// for target.Len() > 0 {
		// 	ss[i] = heap.Pop(target).(Digest)
		// 	i++
		// }
	}
}

var e *Encoder = NewEncoder()
var data [16]byte

// utility to create a random c4.Digest
func randomDigest() Digest {
	// Create some random bytes.
	rand.Read(data[:])
	e.Reset()
	e.Write(data[:])
	return e.ID().Digest()
}
