package hashlib

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestHAMTMapPutGet(t *testing.T) {
	m := NewHAMTMap[string]()
	id := tid("hello")

	m = m.Put(id, "world")
	got, ok := m.Get(id)
	if !ok {
		t.Fatal("expected to find key")
	}
	if got != "world" {
		t.Fatalf("got %q, want %q", got, "world")
	}
}

func TestHAMTMapGetMiss(t *testing.T) {
	m := NewHAMTMap[int]()
	_, ok := m.Get(tid("missing"))
	if ok {
		t.Fatal("empty map should return not found")
	}
}

func TestHAMTMapOverwrite(t *testing.T) {
	id := tid("key")
	m := NewHAMTMap[string]()
	m = m.Put(id, "v1")
	m = m.Put(id, "v2")

	got, _ := m.Get(id)
	if got != "v2" {
		t.Fatalf("expected v2, got %q", got)
	}
	if m.Len() != 1 {
		t.Fatalf("expected len 1, got %d", m.Len())
	}
}

func TestHAMTMapDelete(t *testing.T) {
	id := tid("delete-me")
	m := NewHAMTMap[int]()
	m = m.Put(id, 42)
	m = m.Delete(id)

	if _, ok := m.Get(id); ok {
		t.Fatal("deleted key should not be found")
	}
	if m.Len() != 0 {
		t.Fatalf("expected len 0, got %d", m.Len())
	}
}

func TestHAMTMapDeleteMiss(t *testing.T) {
	m := NewHAMTMap[int]()
	m = m.Put(tid("a"), 1)
	m2 := m.Delete(tid("b"))

	if m2 != m {
		t.Fatal("deleting nonexistent key should return same map")
	}
}

func TestHAMTMapCOW(t *testing.T) {
	id := tid("cow")
	v1 := NewHAMTMap[string]().Put(id, "original")
	v2 := v1.Put(id, "modified")

	got1, _ := v1.Get(id)
	got2, _ := v2.Get(id)

	if got1 != "original" {
		t.Fatal("v1 should still see original")
	}
	if got2 != "modified" {
		t.Fatal("v2 should see modified")
	}
}

func TestHAMTMapRange(t *testing.T) {
	m := NewHAMTMap[int]()
	ids := make(map[c4.ID]int)

	for i := 0; i < 50; i++ {
		id := tid(fmt.Sprintf("range-%d", i))
		m = m.Put(id, i)
		ids[id] = i
	}

	seen := make(map[c4.ID]int)
	m.Range(func(id c4.ID, v int) bool {
		seen[id] = v
		return true
	})

	if len(seen) != len(ids) {
		t.Fatalf("expected %d entries, got %d", len(ids), len(seen))
	}
	for id, want := range ids {
		if got, ok := seen[id]; !ok || got != want {
			t.Fatalf("mismatch for %s: got %d, want %d", id, got, want)
		}
	}
}

func TestHAMTMapRangeEarlyStop(t *testing.T) {
	m := NewHAMTMap[int]()
	for i := 0; i < 100; i++ {
		m = m.Put(tid(fmt.Sprintf("stop-%d", i)), i)
	}

	count := 0
	m.Range(func(_ c4.ID, _ int) bool {
		count++
		return count < 10
	})
	if count != 10 {
		t.Fatalf("expected 10 iterations, got %d", count)
	}
}

func TestHAMTMapMany(t *testing.T) {
	m := NewHAMTMap[int]()
	ids := make([]c4.ID, 1000)

	for i := range ids {
		ids[i] = c4.Identify(strings.NewReader(fmt.Sprintf("item-%d", i)))
		m = m.Put(ids[i], i)
	}

	if m.Len() != 1000 {
		t.Fatalf("expected 1000 entries, got %d", m.Len())
	}

	for i, id := range ids {
		got, ok := m.Get(id)
		if !ok {
			t.Fatalf("missing key %d", i)
		}
		if got != i {
			t.Fatalf("key %d: got %d, want %d", i, got, i)
		}
	}
}

func TestHAMTMapDeleteMany(t *testing.T) {
	m := NewHAMTMap[int]()
	ids := make([]c4.ID, 200)

	for i := range ids {
		ids[i] = tid(fmt.Sprintf("del-%d", i))
		m = m.Put(ids[i], i)
	}

	// Delete even-indexed entries.
	for i := 0; i < len(ids); i += 2 {
		m = m.Delete(ids[i])
	}

	if m.Len() != 100 {
		t.Fatalf("expected 100 entries after deletion, got %d", m.Len())
	}

	for i, id := range ids {
		_, ok := m.Get(id)
		if i%2 == 0 && ok {
			t.Fatalf("entry %d should be deleted", i)
		}
		if i%2 == 1 && !ok {
			t.Fatalf("entry %d should exist", i)
		}
	}
}

func TestHAMTMapStress(t *testing.T) {
	// Insert, overwrite, delete in patterns.
	m := NewHAMTMap[int]()
	live := make(map[c4.ID]int)

	for i := 0; i < 500; i++ {
		id := tid(fmt.Sprintf("stress-%d", i%100))
		if i%7 == 0 {
			m = m.Delete(id)
			delete(live, id)
		} else {
			m = m.Put(id, i)
			live[id] = i
		}
	}

	if m.Len() != len(live) {
		t.Fatalf("len mismatch: got %d, want %d", m.Len(), len(live))
	}

	for id, want := range live {
		got, ok := m.Get(id)
		if !ok {
			t.Fatal("missing expected key")
		}
		if got != want {
			t.Fatalf("value mismatch: got %d, want %d", got, want)
		}
	}
}

func BenchmarkHAMTMapPut(b *testing.B) {
	ids := make([]c4.ID, b.N)
	for i := range ids {
		ids[i] = c4.Identify(strings.NewReader(fmt.Sprintf("bench-%d", i)))
	}
	m := NewHAMTMap[int]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m = m.Put(ids[i], i)
	}
}

func BenchmarkHAMTMapGet(b *testing.B) {
	m := NewHAMTMap[int]()
	ids := make([]c4.ID, 10000)
	for i := range ids {
		ids[i] = c4.Identify(strings.NewReader(fmt.Sprintf("bench-%d", i)))
		m = m.Put(ids[i], i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Get(ids[i%len(ids)])
	}
}
