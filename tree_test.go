package c4_test

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"
	// "fmt"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/xtgo/set"
)

func i2b(i int) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, int32(i))
	return buf.Bytes()
}

func TestTreeString(t *testing.T) {
	var list c4.IDs
	for _, input := range test_vectors {
		list = append(list, c4.Identify(strings.NewReader(input)))
	}
	tree := list.Tree()
	str := tree.String()
	for i, row := range test_vector_ids {
		if i == 0 {
			sort.Strings(row)
		}
		s := len(str) - 90*len(row)

		if str[s:] != strings.Join(row, "") {
			t.Fatalf("strings not equal:\n%s\n%s", str[s:], strings.Join(row, ""))
		}
		str = str[:s]
	}
	// test_vector_ids
}

func TestTreeEncoding(t *testing.T) {
	nilID := c4.Identify(strings.NewReader(""))
	for length := 3; length < 1024; length++ {

		// build a list of IDs
		list := make(c4.IDs, length)

		// Create `length` IDs
		for i := range list {
			id := c4.Identify(bytes.NewReader(i2b(i)))
			if id.IsNil() || id.Cmp(nilID) == 0 {
				t.Fatalf("bad int conversion")
			}
			list[i] = id
		}
		sort.Sort(list)
		n := set.Uniq(list)
		list = list[:n]
		// Create a tree from the list.
		tree := c4.NewTree(list)
		if tree == nil {
			t.Fatalf("NewTree failed")
		}
		data := tree.Bytes()

		// Create a new tree and unmarshal the tree from the binary format.
		tree2, err := c4.ReadTree(bytes.NewReader(data))
		// err := tree2.UnmarshalBinary(data)
		if err != nil {
			t.Fatalf("Failed to unmarshal tree %q", err)
		}

		// Root IDs should match
		if tree.ID().Cmp(tree2.ID()) != 0 {
			t.Fatalf("tree os size %d failed to unmarshal with correct ID", length)
		}

		if len(list) != tree2.Len() {
			t.Fatalf("incorrect list construction after unmarshal")
		}

		// Root IDs should be idempotent
		if tree.ID().Cmp(tree2.ID()) != 0 {
			t.Fatalf("tree os size %d failed to recompute ID correctly", length)
		}
	}
}
