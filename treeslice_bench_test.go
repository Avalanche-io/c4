package c4

import (
	"bytes"
	"container/heap"
	"math/rand"
	"sort"
	"testing"
)

func BenchmarkSortSlice(b *testing.B) {
	b.ReportAllocs()
	s := make(IDs, b.N)
	for i := range s {
		id := randomDigest()
		if id.IsNil() {
			b.Fatal("id is unexpectedly nil")
		}
		s[i] = id
	}
	b.ResetTimer()
	sort.Sort(s)
}

func BenchmarkTree(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(64)
	s := make(IDs, b.N)
	for i := range s {
		id := randomDigest()
		if id.IsNil() {
			b.Fatal("id is unexpectedly nil")
		}
		s[i] = id
	}
	b.ResetTimer()
	tree := s.Tree()
	id := tree.ID()
	_ = id
}

// Heap digest slice
type heapID []ID

func (s heapID) Len() int           { return len(s) }
func (s heapID) Less(i, j int) bool { return s[i].Cmp(s[j]) == -1 }
func (s heapID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (s *heapID) Push(x interface{}) { *s = append(*s, x.(ID)) }
func (s *heapID) Pop() interface{} {
	d := (*s)[len(*s)-1]
	*s = (*s)[:len(*s)-1]
	return d
}

// Heap sort is rather slow, 3x, plus look at all the allocs 2n!
func BenchmarkHeapSlice(b *testing.B) {
	b.ReportAllocs()
	s := make(heapID, b.N)
	ss := make([]ID, b.N)

	// stop the clock and create a set of random digests
	for i := range s {
		s[i] = randomDigest()
	}
	b.ResetTimer()
	target := new(heapID)
	heap.Init(target)
	for i := range s {
		heap.Push(target, s[i])
	}
	for i := range s {
		ss[i] = heap.Pop(target).(ID)
	}

}

// The cost of the first half of building a heap
func BenchmarkHalfHeapSlice(b *testing.B) {
	b.ReportAllocs()
	s := make(heapID, b.N)

	// stop the clock and create a set of random digests
	for i := range s {
		s[i] = randomDigest()
	}
	b.ResetTimer()
	target := new(heapID)
	heap.Init(target)
	for i := range s {
		heap.Push(target, s[i])
	}
}

// utility to create a random c4.ID
func randomDigest() ID {
	var data [64]byte
	// Create some random bytes.
	rand.Read(data[:])

	return Identify(bytes.NewReader(data[:]))
}
