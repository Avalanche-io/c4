package asset

import (
	"bytes"
	"io"
	"sort"
)

type IDSlice []*ID

func (s IDSlice) Len() int           { return len(s) }
func (s IDSlice) Less(i, j int) bool { return s[i].Cmp(s[j]) < 0 }
func (s IDSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Sort is a convenience method.
func (s IDSlice) Sort() {
	sort.Sort(s)
}

// Append id to slice.
func (s *IDSlice) Push(id *ID) {
	*s = append(*s, id)
}

//String returns the slice of c4ids concatenated together without spaces or newlines.
func (s *IDSlice) String() string {
	result := ""
	for _, bigID := range *s {
		result += ((*ID)(bigID)).String()
	}
	return result
}

// SearchIDs searches for x in a sorted slice of *ID and returns the index
// as specified by sort.Search. The slice must be sorted in ascending order.
func SearchIDs(a IDSlice, x *ID) int {
	return sort.Search(len(a), func(i int) bool { return a[i].Cmp(x) >= 0 })
}

// ID of a sorted slice of IDs
func (s IDSlice) ID() (*ID, error) {
	s.Sort()
	encoder := NewIDEncoder()
	for _, bigID := range s {
		_, err := io.Copy(encoder, bytes.NewReader(bigID.RawBytes()))
		if err != nil {
			return nil, err
		}
	}
	return encoder.ID(), nil
}
