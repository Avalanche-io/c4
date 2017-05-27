package id

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
)

// A DigestSlice represents a sorted list of unique Digests, and can be used to
// compute a C4 Digest for any set of non-contiguous data such as files in a
// folder.
type DigestSlice []Digest

// Insert adds a Digest to the slice in sorted order. Insert has no effect if the argument
// is nil, or is already a member of the slice.
func (s *DigestSlice) Insert(d Digest) {
	if d == nil {
		return
	}
	i := s.Index(d)

	// d is already in the slice.
	if i < len(*s) && bytes.Compare((*s)[i], d) == 0 {
		return
	}
	(*s) = append(*s, nil)

	copy((*s)[i+1:], (*s)[i:])
	(*s)[i] = d
}

// Digest returns the Digest of the slice, or nil if the slice is empty.
// The Digest is computed by identifying successive pairs of Digests from the slice
// and iterating across each new list of digest repeating the process until only a
// single ID remains which is the ID returned as the C4 ID of the items in the slice.
func (s DigestSlice) Digest() Digest {
	if len(s) == 0 {
		return nil
	}
	// s is implicitly sorted, during inserts. We cast it to a regular slice
	// here since all subsequent rounds must not be sorted.
	list := []Digest(s)
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
func (s DigestSlice) Index(x Digest) int {
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
func (s DigestSlice) Read(p []byte) (int, error) {
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
func (s DigestSlice) Write(p []byte) (int, error) {
	if len(p)%64 != 0 {
		return 0, errors.New("input must be divisible by 64")
	}
	for i := 0; i < len(p); i += 64 {
		j := i + 8
		s.Insert(Digest(p[i:j]))
	}
	return len(p), nil
}
