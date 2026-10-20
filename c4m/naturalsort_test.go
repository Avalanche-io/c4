package c4m

import (
	"encoding/json"
	"math/rand"
	"os"
	"sort"
	"testing"
)

func loadNaturalSortVectors(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("testdata/natural_sort_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Sorted []string `json:"sorted"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Sorted) < 2 {
		t.Fatal("vectors file holds fewer than two names")
	}
	return v.Sorted
}

// The vectors hold digit runs of 17, 20, 21 and 39 digits. Through
// 1.0.18 the Go comparison parsed runs into an int64, so runs of 20 or
// more digits wrapped: two different runs could compare equal (and then
// sort.Slice's order was whatever the input order was, so one tree got
// more than one ID), or compare in the wrong order. The order is now by
// integer value of any length, with no parse.
func TestNaturalLessVectors(t *testing.T) {
	sorted := loadNaturalSortVectors(t)
	for i := range sorted {
		for j := i + 1; j < len(sorted); j++ {
			a, b := sorted[i], sorted[j]
			if !NaturalLess(a, b) {
				t.Errorf("want %q < %q", a, b)
			}
			if NaturalLess(b, a) {
				t.Errorf("want !(%q < %q)", b, a)
			}
		}
		if NaturalLess(sorted[i], sorted[i]) {
			t.Errorf("%q < itself", sorted[i])
		}
	}
}

// Every permutation of the vectors must sort to the same order: a
// canonical order that depends on input order is a tree with more than
// one ID.
func TestNaturalLessDeterministic(t *testing.T) {
	sorted := loadNaturalSortVectors(t)
	rng := rand.New(rand.NewSource(1))
	for round := 0; round < 200; round++ {
		names := append([]string(nil), sorted...)
		rng.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
		sort.Slice(names, func(i, j int) bool { return NaturalLess(names[i], names[j]) })
		for i := range names {
			if names[i] != sorted[i] {
				t.Fatalf("round %d: position %d is %q, want %q", round, i, names[i], sorted[i])
			}
		}
	}
}

func TestCompareDigitRuns(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0", "0", 0},
		{"0", "00", -1},
		{"1", "01", -1},
		{"01", "1", 1},
		{"7", "10", -1},
		{"007", "10", -1},
		{"10", "9", 1},
		{"18446744073709551616", "18446744073709551617", -1},
		{"18446744073709551617", "2", 1},
		{"99999999999999999999", "100000000000000000000", -1},
		{"000000000000000000001", "1", 1},
		{"", "", 0},
	}
	for _, c := range cases {
		if got := compareDigitRuns(c.a, c.b); got != c.want {
			t.Errorf("compareDigitRuns(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
