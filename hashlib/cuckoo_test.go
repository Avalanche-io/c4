package hashlib

import (
	"fmt"
	"testing"

	"github.com/Avalanche-io/c4"
)

func TestCuckooFilterAddHas(t *testing.T) {
	f := NewCuckooFilter(100)
	id := tid("cuckoo")

	if f.Has(id) {
		t.Fatal("empty filter should not contain anything")
	}

	if !f.Add(id) {
		t.Fatal("add should succeed")
	}

	if !f.Has(id) {
		t.Fatal("filter should contain added ID")
	}
	if f.Count() != 1 {
		t.Fatalf("expected count 1, got %d", f.Count())
	}
}

func TestCuckooFilterDelete(t *testing.T) {
	f := NewCuckooFilter(100)
	id := tid("delete")

	f.Add(id)
	if !f.Delete(id) {
		t.Fatal("delete should succeed")
	}
	if f.Has(id) {
		t.Fatal("deleted ID should not be found")
	}
	if f.Count() != 0 {
		t.Fatalf("expected count 0, got %d", f.Count())
	}
}

func TestCuckooFilterDeleteMiss(t *testing.T) {
	f := NewCuckooFilter(100)
	if f.Delete(tid("nope")) {
		t.Fatal("delete of absent ID should return false")
	}
}

func TestCuckooFilterMany(t *testing.T) {
	const n = 500
	f := NewCuckooFilter(n * 2)

	for i := 0; i < n; i++ {
		if !f.Add(tid(fmt.Sprintf("many-%d", i))) {
			t.Fatalf("add failed at %d", i)
		}
	}

	if f.Count() != n {
		t.Fatalf("expected count %d, got %d", n, f.Count())
	}

	for i := 0; i < n; i++ {
		if !f.Has(tid(fmt.Sprintf("many-%d", i))) {
			t.Fatalf("missing %d", i)
		}
	}
}

func TestCuckooFilterFalsePositiveRate(t *testing.T) {
	const n = 10000
	f := NewCuckooFilter(n * 2)

	// Add n items.
	for i := 0; i < n; i++ {
		f.Add(tid(fmt.Sprintf("fp-in-%d", i)))
	}

	// Check n items that were NOT added.
	falsePositives := 0
	for i := 0; i < n; i++ {
		if f.Has(tid(fmt.Sprintf("fp-out-%d", i))) {
			falsePositives++
		}
	}

	rate := float64(falsePositives) / float64(n)
	// With 16-bit fingerprints, expect < 1% false positive rate.
	if rate > 0.01 {
		t.Fatalf("false positive rate too high: %.4f (%d/%d)", rate, falsePositives, n)
	}
}

func TestCuckooFilterReset(t *testing.T) {
	f := NewCuckooFilter(100)
	f.Add(tid("a"))
	f.Add(tid("b"))

	f.Reset()
	if f.Count() != 0 {
		t.Fatal("count should be 0 after reset")
	}
	if f.Has(tid("a")) {
		t.Fatal("should not find anything after reset")
	}
}

func BenchmarkCuckooFilterAdd(b *testing.B) {
	f := NewCuckooFilter(b.N * 2)
	ids := make([]c4.ID, b.N)
	for i := range ids {
		ids[i] = tid(fmt.Sprintf("bench-%d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Add(ids[i])
	}
}

func BenchmarkCuckooFilterHas(b *testing.B) {
	const n = 10000
	f := NewCuckooFilter(n * 2)
	ids := make([]c4.ID, n)
	for i := range ids {
		ids[i] = tid(fmt.Sprintf("bench-%d", i))
		f.Add(ids[i])
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Has(ids[i%n])
	}
}
