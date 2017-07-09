package id

import (
	"testing"
)

func TestTreeSizes(t *testing.T) {
	for N := 3; N < 1<<22; N += 1 {
		total := treeSize(N)
		if listSize(total) != N {
			t.Fatalf("N: %d, total: %d, NN: %d\n", N, total, listSize(total))
		}
	}
}
