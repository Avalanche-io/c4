package id

import "sort"

type Slice []*ID

// Append id to slice.
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

//String returns the slice of c4ids concatenated together without spaces or newlines.
func (s *Slice) String() string {
	result := ""
	for _, id := range *s {
		result += id.String()
	}
	return result
}

// SearchIDs searches for x in a sorted slice of *ID and returns the index
// as specified by sort.Search. The slice must be sorted in ascending order.
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

// ID of a sorted slice of IDs
func (s Slice) ID() *ID {
	// s is implicitly sorted, during inserts. We cast it to a regular slice
	// here since all subsequent rounds must not be sorted.
	digests := make(DigestSlice, 0, len(s))
	for _, id := range s {
		if id == nil {
			panic("how did that happen?")
		}
		digests = append(digests, id.Digest())
	}
	if len(s) > 0 && len(s) != len(digests) {
		panic("bad length")
	}
	d := digests.Digest()

	return d.ID()
}
