package hashlib

import (
	"fmt"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestPatriciaTrieInsertHas(t *testing.T) {
	tr := NewPatriciaTrie()
	id := tid("patricia")

	if tr.Has(id) {
		t.Fatal("empty trie should not contain anything")
	}

	tr = tr.Insert(id)
	if !tr.Has(id) {
		t.Fatal("trie should contain inserted ID")
	}
	if tr.Len() != 1 {
		t.Fatalf("expected len 1, got %d", tr.Len())
	}
}

func TestPatriciaTrieInsertDuplicate(t *testing.T) {
	id := tid("dup")
	tr := NewPatriciaTrie().Insert(id)
	tr2 := tr.Insert(id)

	if tr2 != tr {
		t.Fatal("inserting duplicate should return same trie")
	}
}

func TestPatriciaTrieDelete(t *testing.T) {
	id := tid("delete")
	tr := NewPatriciaTrie().Insert(id)
	tr = tr.Delete(id)

	if tr.Has(id) {
		t.Fatal("deleted ID should not be present")
	}
	if tr.Len() != 0 {
		t.Fatalf("expected len 0, got %d", tr.Len())
	}
}

func TestPatriciaTrieDeleteMiss(t *testing.T) {
	tr := NewPatriciaTrie().Insert(tid("a"))
	tr2 := tr.Delete(tid("b"))

	if tr2 != tr {
		t.Fatal("deleting nonexistent should return same trie")
	}
}

func TestPatriciaTrieCOW(t *testing.T) {
	id := tid("cow")
	v1 := NewPatriciaTrie().Insert(id)
	v2 := v1.Delete(id)

	if !v1.Has(id) {
		t.Fatal("v1 should still contain the ID")
	}
	if v2.Has(id) {
		t.Fatal("v2 should not contain the ID")
	}
}

func TestPatriciaTrieMany(t *testing.T) {
	tr := NewPatriciaTrie()
	ids := make([]c4.ID, 500)

	for i := range ids {
		ids[i] = tid(fmt.Sprintf("many-%d", i))
		tr = tr.Insert(ids[i])
	}

	if tr.Len() != 500 {
		t.Fatalf("expected 500 entries, got %d", tr.Len())
	}

	for i, id := range ids {
		if !tr.Has(id) {
			t.Fatalf("missing entry %d", i)
		}
	}
}

func TestPatriciaTrieDeleteMany(t *testing.T) {
	tr := NewPatriciaTrie()
	ids := make([]c4.ID, 100)

	for i := range ids {
		ids[i] = tid(fmt.Sprintf("del-%d", i))
		tr = tr.Insert(ids[i])
	}

	// Delete even-indexed entries.
	for i := 0; i < len(ids); i += 2 {
		tr = tr.Delete(ids[i])
	}

	if tr.Len() != 50 {
		t.Fatalf("expected 50 entries, got %d", tr.Len())
	}

	for i, id := range ids {
		has := tr.Has(id)
		if i%2 == 0 && has {
			t.Fatalf("entry %d should be deleted", i)
		}
		if i%2 == 1 && !has {
			t.Fatalf("entry %d should exist", i)
		}
	}
}

func TestPatriciaTrieNearest(t *testing.T) {
	tr := NewPatriciaTrie()
	ids := make([]c4.ID, 50)
	for i := range ids {
		ids[i] = tid(fmt.Sprintf("near-%d", i))
		tr = tr.Insert(ids[i])
	}

	query := tid("query")
	results := tr.Nearest(query, 5)
	if len(results) != 5 {
		t.Fatalf("expected 5 nearest, got %d", len(results))
	}

	// All results should be in the trie.
	for _, id := range results {
		if !tr.Has(id) {
			t.Fatal("nearest result not in trie")
		}
	}

	// First result should be the absolute nearest.
	bestDist := XorDist(query, results[0])
	for _, id := range ids {
		d := XorDist(query, id)
		if d.Cmp(bestDist) < 0 {
			t.Fatal("found a closer ID not returned as nearest")
		}
	}
}

func TestPatriciaTrieNearestAll(t *testing.T) {
	tr := NewPatriciaTrie()
	for i := 0; i < 10; i++ {
		tr = tr.Insert(tid(fmt.Sprintf("all-%d", i)))
	}

	// Request more than available.
	results := tr.Nearest(tid("q"), 100)
	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
}

func TestPatriciaTrieNearestEmpty(t *testing.T) {
	tr := NewPatriciaTrie()
	results := tr.Nearest(tid("q"), 5)
	if len(results) != 0 {
		t.Fatal("empty trie should return no results")
	}
}

func TestPatriciaTrieRange(t *testing.T) {
	tr := NewPatriciaTrie()
	expect := make(map[c4.ID]bool)

	for i := 0; i < 30; i++ {
		id := tid(fmt.Sprintf("range-%d", i))
		tr = tr.Insert(id)
		expect[id] = true
	}

	seen := 0
	tr.Range(func(id c4.ID) bool {
		if !expect[id] {
			t.Fatal("unexpected ID in range")
		}
		seen++
		return true
	})

	if seen != 30 {
		t.Fatalf("expected 30 entries in range, got %d", seen)
	}
}

func TestPatriciaTrieRangeOrder(t *testing.T) {
	tr := NewPatriciaTrie()
	for i := 0; i < 100; i++ {
		tr = tr.Insert(tid(fmt.Sprintf("order-%d", i)))
	}

	// Range should produce IDs in binary order (left-to-right DFS).
	var prev c4.ID
	first := true
	tr.Range(func(id c4.ID) bool {
		if !first && id.Cmp(prev) <= 0 {
			t.Fatal("range should produce IDs in ascending order")
		}
		prev = id
		first = false
		return true
	})
}

func BenchmarkPatriciaTrieInsert(b *testing.B) {
	ids := make([]c4.ID, b.N)
	for i := range ids {
		ids[i] = tid(fmt.Sprintf("bench-%d", i))
	}
	tr := NewPatriciaTrie()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr = tr.Insert(ids[i])
	}
}

func BenchmarkPatriciaTrieNearest(b *testing.B) {
	tr := NewPatriciaTrie()
	for i := 0; i < 10000; i++ {
		tr = tr.Insert(tid(fmt.Sprintf("bench-%d", i)))
	}
	query := tid("query")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Nearest(query, 10)
	}
}
