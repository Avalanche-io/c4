package db

import (
	"math/rand"
	"strconv"
	"testing"
)

// updated, delete me

func TestShuffle(t *testing.T) {
	rand.Seed(42)
	for j := 1; j < 1000; j++ {
		var list []string
		for i := 0; i < j; i++ {
			list = append(list, strconv.Itoa(i))
		}
		shuffle(list)
		count := 0
		for i := 0; i < j; i++ {
			if list[i] == strconv.Itoa(i) {
				count++
			}
		}
		if j > 12 && (float32(count)/float32(j)) > 0.31 {
			t.Errorf("shuffle ratio for %d: %.2f", j, (float32(count) / float32(j)))
		}
		if j == 10 {
			t.Logf("shuffle: %v", list)
		}
	}

}
