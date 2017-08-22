package id_test

import (
	"bytes"
	"encoding/binary"
	// "fmt"
	"testing"

	c4 "github.com/Avalanche-io/c4/id"
)

func TestTree(t *testing.T) {
	var digests c4.DigestSlice
	e := c4.NewEncoder()
	for _, s := range test_vectors {
		e.Write([]byte(s))
		digests.Insert(e.Digest())
		e.Reset()
	}
	tree := c4.NewTree(digests)
	if tree == nil {
		t.Fatalf("NewTree failed")
	}
	tree.Compute()
	// fmt.Printf("tree id: %s\n", tree.Compute().ID())

	for i, k := 0, len(test_vector_ids)-1; i < tree.RowCount(); i, k = i+1, k-1 {

		digests := tree.Row(i)
		expected_digests := make([]c4.Digest, len(test_vector_ids[k]))
		for j := range test_vector_ids[k] {
			id, err := c4.Parse(test_vector_ids[k][j])
			if err != nil {
				t.Fatal(err)
			}
			expected_digests[j] = id.Digest()
		}
		if k == 0 {
			var srt c4.DigestSlice
			for i := range expected_digests {
				srt.Insert(expected_digests[i])
			}
			for i, d := range []c4.Digest(srt) {
				expected_digests[i] = d
			}
		}
		for j, d := range digests {
			if d.ID().Cmp(expected_digests[j].ID()) != 0 {
				t.Fatalf("tree ids do not match %q != %q\n", d.ID(), expected_digests[k].ID())
			}
		}
	}

	for z := 3; z < 30; z++ {
		count := z
		digests = make([]c4.Digest, count)
		for i := range digests {
			id := c4.Identify(bytes.NewReader([]byte{uint8(i)}))
			digests[i] = id.Digest()
		}
		tree = c4.NewTree(digests)
	}

}

func TestTreeEncoding(t *testing.T) {
	for length := 3; length < 1024; length++ {

		// build a list of IDs
		var list c4.DigestSlice

		// Create `length` IDs
		for i := 0; i < length; i++ {
			var buf [8]byte
			binary.PutVarint(buf[:], int64(i))
			id := c4.Identify(bytes.NewReader(buf[:]))
			if id == nil || id == c4.NIL_ID {
				t.Fatalf("bad int conversion")
			}
			list.Insert(id.Digest())
		}

		// Create a tree from the list.
		tree := c4.NewTree(list)
		if tree == nil {
			t.Fatalf("NewTree failed")
		}
		tree.Compute()

		data, err := tree.MarshalBinary()
		if err != nil {
			t.Fatalf("Failed to marshal tree %q", err)
		}

		tree2 := new(c4.Tree)
		// Create a new tree and unmarshal the tree from the binary format.
		err = tree2.UnmarshalBinary(data)
		if err != nil {
			t.Fatalf("Failed to unmarshal tree %q", err)
		}

		// Root IDs should match
		if tree.ID().Cmp(tree2.ID()) != 0 {
			t.Fatalf("tree os size %d failed to unmarshal with correct ID", length)
		}

		if len(list) != tree2.Count() {
			t.Fatalf("incorrect list construction after unmarshal")
		}

		// Root IDs should be idempotent
		tree2.Compute()
		if tree.ID().Cmp(tree2.ID()) != 0 {
			t.Fatalf("tree os size %d failed to recompute ID correctly", length)
		}
	}
}
