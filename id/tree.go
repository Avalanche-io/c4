package id

import (
	"bytes"
	"crypto/sha512"
)

// Type tree implements an ID tree as used for calculating IDs of non-contiguous
// sets of data. The tree represents a type of merkle tree, the only differences
// being that the starting list of IDs is in sorted order, and each node in the
// tree is always hashed with lesser child ID first. This is done so that
// a given set of IDs always has the same root ID, and so that any pair of
// ID is always IDed the same way.

// To avoid storing a given ID more than once an entire tree is stored as
// slice of IDs in the same was as is typical for a balanced binary tree.

type Tree struct {
	data []byte
	rows [][]byte
}

// NewTree creates a new Tree from a DigestSlice, and copies the digests into
// the tree.  However, it does not compute the tree.
func NewTree(s DigestSlice) *Tree {
	rows, data := allocateTree(len(s))
	last_row := len(rows) - 1
	for i, digest := range s {
		copy(rows[last_row][i*64:], []byte(digest))
	}
	return &Tree{
		rows: rows,
		data: data,
	}
}

func (t *Tree) IDcount() int {
	return len(t.rows[len(t.rows)-1]) / 64
}

func (t *Tree) NodeCount() int {
	return len(t.data) / 64
}

func (t *Tree) RowCount() int {
	return len(t.rows)
}

func (t *Tree) Row(i int) []Digest {
	row := t.rows[i]
	digests := make([]Digest, len(row)/64)
	for j := range digests {
		k := j * 64
		digests[j] = Digest(row[k : k+64])
	}
	return digests
}

// At returns the Digest located at the given row, and index
func (t *Tree) At(row, index int) Digest {
	i := index * 64
	return Digest(t.rows[row][i : i+64])
}

// Compute resolves all Digests in the tree, and returns the root Digest
func (t *Tree) Compute() Digest {
	h := sha512.New()
	for i := len(t.rows) - 2; i >= 0; i-- {
		l := len(t.rows[i+1])
		for j := 0; j < l; j += 64 * 2 {
			jj := j / 2
			if j+64 >= l {
				copy(t.rows[i][jj:], t.rows[i+1][j:j+64])
			} else {
				h.Reset()
				// left := []byte(t.rows[i+1][j : j+64])
				// right := []byte(t.rows[i+1][j+64 : j+64*2])
				if bytes.Compare(t.rows[i+1][j:j+64], t.rows[i+1][j+64:j+64*2]) == 1 {
					h.Write(t.rows[i+1][j+64 : j+64*2])
					h.Write(t.rows[i+1][j : j+64])
				} else {
					h.Write(t.rows[i+1][j : j+64*2])

				}
				copy(t.rows[i][jj:], h.Sum(nil)[0:64])
			}
		}
	}
	return Digest(t.rows[0])
}

func (t *Tree) String() string {
	var out string
	for _, row := range t.rows {
		for j := 0; j < len(row); j += 64 {
			out += Digest(row[j : j+64]).ID().String()
		}
	}
	return out
}

// Length returns the number of IDs in the entire tree
func (t *Tree) Length() int {
	return len(t.data) / 64
}

// Size returns the number of bytes required to serialize the tree
// (in binary format).
func (t *Tree) Size() int {
	return len(t.data)
}

// Count returns the number of items in the list this tree represents.
func (t *Tree) Count() int {
	last_row := len(t.rows) - 1
	return len(t.rows[last_row]) / 64
}

func (t *Tree) ID() *ID {
	return Digest(t.rows[0]).ID()
}

func (t *Tree) Digest() Digest {
	return Digest(t.rows[0])
}

// A node represents a specific ID triplet within a tree. Nodes have three
// IDs: the Label, Left, and Right.
type Node struct {
	// pointer to the tree that this node references
	t *Tree
	// the row and index of this node
	row, i uint64
}

// // A tree index maps Digests to the node within a tree for which the Digest
// // is the label, or a leaf node.
// type TreeIndex struct {
// 	t   *Tree
// 	idx map[string]uint64
// }

// func (t *TreeIndex) Add(n Node) {
// 	t.idx[string(n.Label())] = n.i
// }

// func (t *TreeIndex) Find(d Digest) Node {
// 	return Node{
// 		t: t.t,
// 		i: t.idx[string(d)],
// 	}
// }

// func (t Tree) Index() *TreeIndex {
// 	idx := TreeIndex{
// 		t:   t,
// 		idx: make(map[string]uint64),
// 	}
// 	for i := 0; i < len(t); i++ {

// 	}
// 	return &idx
// }

func (t *Tree) Node(i uint64) Node {
	return Node{
		t: t,
		i: i,
	}
}

func (n Node) Parent() Node {
	if n.i < 3 {
		return Node{
			t: n.t,
			i: 0,
		}
	}
	return Node{
		t: n.t,
		i: (n.i - 1) / 2,
	}
}

func (n Node) Label() Digest {
	i := n.i * 64
	return Digest(n.t.rows[n.row][i : i+64])
}

func (n Node) Left() Digest {
	row := n.row + 1
	if row >= uint64(len(n.t.rows)) {
		return nil
	}
	i := n.i * 2 * 64
	return Digest(n.t.rows[row][i : i+64])
}

func (n Node) Right() Digest {
	row := n.row + 1
	if row >= uint64(len(n.t.rows)) {
		return nil
	}
	i := (n.i + 1) * 2 * 64
	return Digest(n.t.rows[row][i : i+64])
}
