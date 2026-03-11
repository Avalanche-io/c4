package hashlib

import (
	"fmt"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestHAMTSetAddHas(t *testing.T) {
	s := NewHAMTSet()
	id := tid("set-test")

	if s.Has(id) {
		t.Fatal("empty set should not contain anything")
	}

	s = s.Add(id)
	if !s.Has(id) {
		t.Fatal("set should contain added ID")
	}
	if s.Len() != 1 {
		t.Fatalf("expected len 1, got %d", s.Len())
	}
}

func TestHAMTSetAddDuplicate(t *testing.T) {
	id := tid("dup")
	s := NewHAMTSet().Add(id)
	s2 := s.Add(id)

	if s2 != s {
		t.Fatal("adding duplicate should return same set")
	}
}

func TestHAMTSetDelete(t *testing.T) {
	id := tid("delete")
	s := NewHAMTSet().Add(id)
	s = s.Delete(id)

	if s.Has(id) {
		t.Fatal("deleted ID should not be present")
	}
	if s.Len() != 0 {
		t.Fatalf("expected len 0, got %d", s.Len())
	}
}

func TestHAMTSetDeleteMiss(t *testing.T) {
	s := NewHAMTSet().Add(tid("a"))
	s2 := s.Delete(tid("b"))

	if s2 != s {
		t.Fatal("deleting nonexistent should return same set")
	}
}

func TestHAMTSetCOW(t *testing.T) {
	id := tid("cow")
	v1 := NewHAMTSet().Add(id)
	v2 := v1.Delete(id)

	if !v1.Has(id) {
		t.Fatal("v1 should still contain the ID")
	}
	if v2.Has(id) {
		t.Fatal("v2 should not contain the ID")
	}
}

func TestHAMTSetRange(t *testing.T) {
	s := NewHAMTSet()
	ids := make(map[c4.ID]bool)
	for i := 0; i < 20; i++ {
		id := tid(fmt.Sprintf("range-%d", i))
		s = s.Add(id)
		ids[id] = true
	}

	seen := 0
	s.Range(func(id c4.ID) bool {
		if !ids[id] {
			t.Fatal("unexpected ID in range")
		}
		seen++
		return true
	})
	if seen != 20 {
		t.Fatalf("expected 20 entries, got %d", seen)
	}
}

func TestHAMTSetUnion(t *testing.T) {
	a := NewHAMTSet()
	b := NewHAMTSet()

	for i := 0; i < 10; i++ {
		a = a.Add(tid(fmt.Sprintf("a-%d", i)))
		b = b.Add(tid(fmt.Sprintf("b-%d", i)))
	}
	// Shared element.
	shared := tid("shared")
	a = a.Add(shared)
	b = b.Add(shared)

	u := a.Union(b)
	if u.Len() != 21 {
		t.Fatalf("expected 21 in union, got %d", u.Len())
	}

	for i := 0; i < 10; i++ {
		if !u.Has(tid(fmt.Sprintf("a-%d", i))) {
			t.Fatal("union missing a element")
		}
		if !u.Has(tid(fmt.Sprintf("b-%d", i))) {
			t.Fatal("union missing b element")
		}
	}
	if !u.Has(shared) {
		t.Fatal("union missing shared element")
	}
}

func TestHAMTSetIntersect(t *testing.T) {
	a := NewHAMTSet()
	b := NewHAMTSet()

	shared := []c4.ID{tid("s1"), tid("s2"), tid("s3")}
	for _, id := range shared {
		a = a.Add(id)
		b = b.Add(id)
	}
	a = a.Add(tid("only-a"))
	b = b.Add(tid("only-b"))

	inter := a.Intersect(b)
	if inter.Len() != 3 {
		t.Fatalf("expected 3 in intersection, got %d", inter.Len())
	}
	for _, id := range shared {
		if !inter.Has(id) {
			t.Fatal("intersection missing shared element")
		}
	}
}

func TestHAMTSetDiff(t *testing.T) {
	a := NewHAMTSet()
	b := NewHAMTSet()

	shared := tid("shared")
	a = a.Add(shared)
	b = b.Add(shared)

	onlyA := []c4.ID{tid("a1"), tid("a2")}
	for _, id := range onlyA {
		a = a.Add(id)
	}
	b = b.Add(tid("b1"))

	diff := a.Diff(b)
	if diff.Len() != 2 {
		t.Fatalf("expected 2 in diff, got %d", diff.Len())
	}
	for _, id := range onlyA {
		if !diff.Has(id) {
			t.Fatal("diff missing a-only element")
		}
	}
	if diff.Has(shared) {
		t.Fatal("diff should not contain shared element")
	}
}

func TestHAMTSetEmpty(t *testing.T) {
	a := NewHAMTSet()
	b := NewHAMTSet()

	if a.Union(b).Len() != 0 {
		t.Fatal("union of empties should be empty")
	}
	if a.Intersect(b).Len() != 0 {
		t.Fatal("intersection of empties should be empty")
	}
	if a.Diff(b).Len() != 0 {
		t.Fatal("diff of empties should be empty")
	}
}

func BenchmarkHAMTSetAdd(b *testing.B) {
	ids := make([]c4.ID, b.N)
	for i := range ids {
		ids[i] = tid(fmt.Sprintf("bench-%d", i))
	}
	s := NewHAMTSet()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s = s.Add(ids[i])
	}
}
