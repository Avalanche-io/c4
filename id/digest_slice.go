package id

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/xtgo/set"
)

// A DigestSlice represents a sorted list of unique Digests, and can be used to
// compute a C4 Digest for any set of non-contiguous data such as files in a
// folder.
type DigestSlice []Digest

// Insert adds a Digest to the slice in sorted order. Insert has no effect if the argument
// is nil, or is already a member of the slice. When successful Insert returns
// the insertion index. If d is nil then Insert will return -1. If d is already
// in the slice then Insert will return the index as a negative minus 1.
//
// For example if d is already item 5 of the slice Insert will return -6. A
// return value of -1 means d was inserted at position 0 if d is not nil.
func (ds *DigestSlice) Insert(d Digest) int {
	s := *ds
	if d == nil {
		return -1
	}
	i := s.Index(d)

	// d is already in the slice.
	if i < len(s) && bytes.Compare(s[i], d) == 0 {
		return -(i + 1)
	}
	s = append(s, nil)

	copy(s[i+1:], s[i:])
	s[i] = d
	*ds = s
	return i
}

// Digest returns the Digest of the slice, or nil if the slice is empty.
// The Digest is computed by identifying successive pairs of Digests from the slice
// and iterating across each new list of digest repeating the process until only a
// single ID remains which is the ID returned as the C4 ID of the items in the slice.
func (ds *DigestSlice) Digest() Digest {
	s := *ds
	if len(s) == 0 {
		return nil
	}
	// s is implicitly sorted, during inserts. However, subsequent rounds below
	// must not be sorted.

	// copy to avoid modifying the DigestSlice itself.
	list := make([]Digest, len(s))
	copy(list, s)

	for len(list) > 1 {
		odd := oddIndex(len(list))
		prev := list
		list = list[:0]
		// If the list has an odd number of items, we set aside the last item.
		// Create Digests for each pair of Digests in the previous round.
		for i := 0; i < len(prev)-1; i += 2 {
			list = append(list, prev[i].Sum(prev[i+1]))
		}
		// Append the odd Digest if necessary.
		if odd >= 0 {
			list = append(list, prev[odd])
		}
	}

	return list[0]
}

// Index returns the location of x in the DigestSlice, or the index at which
// x would be inserted into the slice if it is not in the set.
func (ds *DigestSlice) Index(x Digest) int {
	s := *ds
	return sort.Search(len(s), func(i int) bool { return bytes.Compare(s[i], x) >= 0 })
}

func print(s []Digest) {
	for i, dig := range s {
		fmt.Printf("%d: %s\n", i, Digest(dig).ID())
	}
}

// Read implements the io.Reader interface to output the bytes of the
// DigestSlice. Read returns an error if p is not large enough to hold the
// entire DigestSlice (64 * it's length).
// The output of Read is the most compact form of the DigestSlice and
// cannot be compressed further.
func (ds *DigestSlice) Read(p []byte) (int, error) {
	s := *ds
	if len(p) < len(s)*64 {
		return 0, errors.New("buffer too small")
	}
	for i, digest := range s {
		copy(p[i*64:], []byte(digest))
	}
	return len(s) * 64, nil
}

// Write implements the io.Writer interface to input the bytes of
// a serialized DigestSlice. Write returns an error and does not write any
// data if p is not a multiple of 64 in length.
func (s *DigestSlice) Write(p []byte) (int, error) {
	if len(p)%64 != 0 {
		return 0, errors.New("input must be divisible by 64")
	}
	for i := 0; i < len(p); i += 64 {
		j := i + 64
		d := Digest(p[i:j])
		s.Insert(d)
	}
	return len(p), nil
}

func (s *DigestSlice) Append(d Digest) {
	(*s) = append((*s), d)
}

func (a *DigestSlice) Copy(b *DigestSlice) {
	for _, digest := range *b {
		a.Append(digest)
	}
}

// Sort interface

func (s *DigestSlice) Len() int           { return len(*s) }
func (s *DigestSlice) Swap(i, j int)      { (*s)[i], (*s)[j] = (*s)[j], (*s)[i] }
func (s *DigestSlice) Less(i, j int) bool { return s.Comp(i, j) < 0 }

func (s *DigestSlice) Comp(i, j int) int {
	return bytes.Compare((*s)[i], (*s)[j])
}

func (s *DigestSlice) Sort() {
	sort.Sort(s)
	n := set.Uniq(s)
	(*s) = (*s)[:n]
}
