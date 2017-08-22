package id

import "sort"

type Slice []*ID

// Insert adds an ID to the slice in sorted order.
func (s *Slice) Insert(id *ID) {
	if id == nil {
		return
	}
	i := s.Index(id)

	// id is nil or already in the slice.
	if i < len(*s) && (*s)[i].Cmp(id) == 0 {
		return
	}
	(*s) = append(*s, nil)

	copy((*s)[i+1:], (*s)[i:])
	(*s)[i] = id
}

// String returns the slice of IDs concatenated together without spaces or newlines.
func (s *Slice) String() string {
	result := ""
	for _, id := range *s {
		result += id.String()
	}
	return result
}

// Index returns the array index where the ID is, or would be inserted if
// not in the slice already. The slice must be sorted in ascending order.
func (s Slice) Index(id *ID) int {
	if id == nil {
		return -1
	}
	return sort.Search(len(s), func(i int) bool { return s[i] != nil && s[i].Cmp(id) >= 0 })
}

func oddIndex(l int) int {
	if l%2 == 1 {
		return l - 1
	}
	return -1
}

// The ID method returns the ID of a sorted slice of IDs.
// For performance the ID() method assumes the slice is already sorted, and
// will return an incorrect ID() if that is not the case. If an error
// is encountered nil is returned.
// Possible errors are, the slice is empty, or the slice has nil entries
func (s Slice) ID() *ID {
	// s is implicitly sorted, during inserts, subsequent rounds must not be
	// sorted.

	digests := make(DigestSlice, 0, len(s))
	for _, id := range s {
		if id == nil {
			return nil
		}
		digests = append(digests, id.Digest())
	}
	d := digests.Digest()

	return d.ID()
}
