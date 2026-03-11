package hashlib

import "github.com/Avalanche-io/c4"

// PatriciaTrie is a persistent binary trie (crit-bit tree) over the C4 ID
// hash space. Perfectly balanced by construction on uniformly distributed
// keys. Supports XOR-nearest-neighbor queries, range iteration, and O(1)
// snapshots via structural sharing.
//
// All mutations return new tries; the original is never modified.
type PatriciaTrie struct {
	root *pNode
	len  int
}

type pNode struct {
	crit  int        // critical bit position (internal nodes)
	child [2]*pNode  // child[0] = bit is 0, child[1] = bit is 1
	id    c4.ID      // stored ID (leaf nodes)
	leaf  bool
}

// NewPatriciaTrie returns an empty trie.
func NewPatriciaTrie() *PatriciaTrie {
	return &PatriciaTrie{}
}

// Len returns the number of IDs in the trie.
func (t *PatriciaTrie) Len() int { return t.len }

// Has returns true if the ID is in the trie.
func (t *PatriciaTrie) Has(id c4.ID) bool {
	if t.root == nil {
		return false
	}
	leaf := findLeaf(t.root, id)
	return leaf.id == id
}

// Insert returns a new trie with the given ID added.
// Returns the same trie if the ID was already present.
func (t *PatriciaTrie) Insert(id c4.ID) *PatriciaTrie {
	if t.root == nil {
		return &PatriciaTrie{
			root: &pNode{id: id, leaf: true},
			len:  1,
		}
	}

	nearest := findLeaf(t.root, id)
	crit := firstDifferingBit(nearest.id, id)
	if crit < 0 {
		return t // duplicate
	}

	dir := Bit(id, crit)
	newLeaf := &pNode{id: id, leaf: true}
	newRoot := insertAtBit(t.root, newLeaf, crit, dir)

	return &PatriciaTrie{root: newRoot, len: t.len + 1}
}

// Delete returns a new trie with the given ID removed.
// Returns the same trie if the ID was not present.
func (t *PatriciaTrie) Delete(id c4.ID) *PatriciaTrie {
	if t.root == nil {
		return t
	}
	newRoot, removed := deleteFromNode(t.root, id)
	if !removed {
		return t
	}
	return &PatriciaTrie{root: newRoot, len: t.len - 1}
}

// Nearest returns the k IDs closest to the query by XOR distance.
// The result is ordered nearest-first. If the trie contains fewer than
// k IDs, all IDs are returned.
func (t *PatriciaTrie) Nearest(id c4.ID, k int) []c4.ID {
	if t.root == nil || k <= 0 {
		return nil
	}
	results := make([]c4.ID, 0, k)
	nearestDFS(t.root, id, k, &results)
	return results
}

// Range calls fn for each ID in the trie (ordered by hash value,
// left-to-right DFS). If fn returns false, iteration stops.
func (t *PatriciaTrie) Range(fn func(c4.ID) bool) {
	if t.root == nil {
		return
	}
	t.root.rangeAll(fn)
}

// --- internal ---

func findLeaf(n *pNode, id c4.ID) *pNode {
	for !n.leaf {
		n = n.child[Bit(id, n.crit)]
	}
	return n
}

func insertAtBit(n *pNode, newLeaf *pNode, crit int, dir uint) *pNode {
	// Insert before nodes with less significant (higher numbered) critical bits.
	if n.leaf || crit < n.crit {
		newNode := &pNode{crit: crit}
		newNode.child[dir] = newLeaf
		newNode.child[1-dir] = n
		return newNode
	}

	// Continue down (COW).
	childDir := Bit(newLeaf.id, n.crit)
	newN := &pNode{crit: n.crit}
	newN.child[1-childDir] = n.child[1-childDir] // shared
	newN.child[childDir] = insertAtBit(n.child[childDir], newLeaf, crit, dir)
	return newN
}

func deleteFromNode(n *pNode, id c4.ID) (*pNode, bool) {
	if n.leaf {
		if n.id == id {
			return nil, true
		}
		return n, false
	}

	dir := Bit(id, n.crit)
	newChild, removed := deleteFromNode(n.child[dir], id)
	if !removed {
		return n, false
	}

	if newChild == nil {
		// Child was the deleted leaf; return sibling.
		return n.child[1-dir], true
	}

	// COW: new internal node with updated child.
	newN := &pNode{crit: n.crit}
	newN.child[dir] = newChild
	newN.child[1-dir] = n.child[1-dir]
	return newN, true
}

func nearestDFS(n *pNode, id c4.ID, k int, results *[]c4.ID) {
	if len(*results) >= k {
		return
	}
	if n.leaf {
		*results = append(*results, n.id)
		return
	}
	// Explore the preferred branch first (matching bit = closer by XOR).
	preferred := Bit(id, n.crit)
	nearestDFS(n.child[preferred], id, k, results)
	if len(*results) < k {
		nearestDFS(n.child[1-preferred], id, k, results)
	}
}

func (n *pNode) rangeAll(fn func(c4.ID) bool) bool {
	if n.leaf {
		return fn(n.id)
	}
	if !n.child[0].rangeAll(fn) {
		return false
	}
	return n.child[1].rangeAll(fn)
}
