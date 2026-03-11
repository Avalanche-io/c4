package hashlib

import "github.com/Avalanche-io/c4"

// HAMTSet is a persistent set of C4 IDs backed by a HAMT.
// All mutations return new sets; the original is never modified.
type HAMTSet struct {
	m *HAMTMap[struct{}]
}

// NewHAMTSet returns an empty persistent set.
func NewHAMTSet() *HAMTSet {
	return &HAMTSet{m: NewHAMTMap[struct{}]()}
}

// Len returns the number of IDs in the set.
func (s *HAMTSet) Len() int { return s.m.Len() }

// Has returns true if the ID is in the set.
func (s *HAMTSet) Has(id c4.ID) bool {
	_, ok := s.m.Get(id)
	return ok
}

// Add returns a new set with the given ID added.
// Returns the same set if the ID was already present.
func (s *HAMTSet) Add(id c4.ID) *HAMTSet {
	if s.Has(id) {
		return s
	}
	return &HAMTSet{m: s.m.Put(id, struct{}{})}
}

// Delete returns a new set with the given ID removed.
// Returns the same set if the ID was not present.
func (s *HAMTSet) Delete(id c4.ID) *HAMTSet {
	newM := s.m.Delete(id)
	if newM == s.m {
		return s
	}
	return &HAMTSet{m: newM}
}

// Range calls fn for each ID in the set. If fn returns false, iteration stops.
func (s *HAMTSet) Range(fn func(c4.ID) bool) {
	s.m.Range(func(id c4.ID, _ struct{}) bool {
		return fn(id)
	})
}

// Union returns a new set containing all IDs from both sets.
func (s *HAMTSet) Union(other *HAMTSet) *HAMTSet {
	// Iterate the smaller set, add to the larger.
	big, small := s, other
	if s.Len() < other.Len() {
		big, small = other, s
	}
	result := big
	small.Range(func(id c4.ID) bool {
		result = result.Add(id)
		return true
	})
	return result
}

// Intersect returns a new set containing only IDs present in both sets.
func (s *HAMTSet) Intersect(other *HAMTSet) *HAMTSet {
	// Iterate the smaller set, keep those in the larger.
	small, big := s, other
	if s.Len() > other.Len() {
		small, big = other, s
	}
	result := NewHAMTSet()
	small.Range(func(id c4.ID) bool {
		if big.Has(id) {
			result = result.Add(id)
		}
		return true
	})
	return result
}

// Diff returns a new set containing IDs in s but not in other.
func (s *HAMTSet) Diff(other *HAMTSet) *HAMTSet {
	result := NewHAMTSet()
	s.Range(func(id c4.ID) bool {
		if !other.Has(id) {
			result = result.Add(id)
		}
		return true
	})
	return result
}
