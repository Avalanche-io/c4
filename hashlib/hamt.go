package hashlib

import "github.com/Avalanche-io/c4"

// HAMTMap is a persistent hash array mapped trie mapping C4 IDs to values.
// All mutations return new maps; the original is never modified.
// Structural sharing makes this efficient: only the path from root to
// the modified leaf is copied.
type HAMTMap[V any] struct {
	root *hamtNode[V]
	len  int
}

type hamtNode[V any] struct {
	bitmap   uint32
	children []hamtChild[V]
}

// hamtChild is either a leaf (sub == nil, key is set) or a branch (sub != nil).
type hamtChild[V any] struct {
	key   c4.ID
	value V
	sub   *hamtNode[V]
}

func (c *hamtChild[V]) isLeaf() bool { return c.sub == nil }

// NewHAMTMap returns an empty persistent map.
func NewHAMTMap[V any]() *HAMTMap[V] {
	return &HAMTMap[V]{root: &hamtNode[V]{}}
}

// Len returns the number of entries.
func (m *HAMTMap[V]) Len() int { return m.len }

// Get looks up the value for the given ID.
func (m *HAMTMap[V]) Get(id c4.ID) (V, bool) {
	return m.root.get(id, 0)
}

// Put returns a new map with the given key-value pair added or updated.
func (m *HAMTMap[V]) Put(id c4.ID, value V) *HAMTMap[V] {
	newRoot, added := m.root.put(id, value, 0)
	newLen := m.len
	if added {
		newLen++
	}
	return &HAMTMap[V]{root: newRoot, len: newLen}
}

// Delete returns a new map with the given key removed.
// Returns the same map if the key was not present.
func (m *HAMTMap[V]) Delete(id c4.ID) *HAMTMap[V] {
	newRoot, removed := m.root.del(id, 0)
	if !removed {
		return m
	}
	if newRoot == nil {
		newRoot = &hamtNode[V]{}
	}
	return &HAMTMap[V]{root: newRoot, len: m.len - 1}
}

// Range calls fn for each key-value pair. If fn returns false, iteration stops.
func (m *HAMTMap[V]) Range(fn func(c4.ID, V) bool) {
	m.root.iterate(fn)
}

// --- hamtNode methods ---

func (n *hamtNode[V]) get(id c4.ID, depth int) (V, bool) {
	chunk := Chunk(id, depth)
	bit := uint32(1) << chunk
	if n.bitmap&bit == 0 {
		var zero V
		return zero, false
	}
	idx := popcount(n.bitmap & (bit - 1))
	child := &n.children[idx]
	if child.isLeaf() {
		if child.key == id {
			return child.value, true
		}
		var zero V
		return zero, false
	}
	return child.sub.get(id, depth+1)
}

func (n *hamtNode[V]) put(id c4.ID, value V, depth int) (*hamtNode[V], bool) {
	chunk := Chunk(id, depth)
	bit := uint32(1) << chunk
	idx := popcount(n.bitmap & (bit - 1))

	if n.bitmap&bit == 0 {
		// Empty slot: insert leaf.
		nc := make([]hamtChild[V], len(n.children)+1)
		copy(nc[:idx], n.children[:idx])
		nc[idx] = hamtChild[V]{key: id, value: value}
		copy(nc[idx+1:], n.children[idx:])
		return &hamtNode[V]{bitmap: n.bitmap | bit, children: nc}, true
	}

	child := &n.children[idx]
	nc := make([]hamtChild[V], len(n.children))
	copy(nc, n.children)

	if child.isLeaf() {
		if child.key == id {
			// Update existing key.
			nc[idx] = hamtChild[V]{key: id, value: value}
			return &hamtNode[V]{bitmap: n.bitmap, children: nc}, false
		}
		// Hash collision at this depth: push both entries down.
		sub := &hamtNode[V]{}
		sub, _ = sub.put(child.key, child.value, depth+1)
		sub, _ = sub.put(id, value, depth+1)
		nc[idx] = hamtChild[V]{sub: sub}
		return &hamtNode[V]{bitmap: n.bitmap, children: nc}, true
	}

	// Recurse into sub-node.
	newSub, added := child.sub.put(id, value, depth+1)
	nc[idx] = hamtChild[V]{sub: newSub}
	return &hamtNode[V]{bitmap: n.bitmap, children: nc}, added
}

func (n *hamtNode[V]) del(id c4.ID, depth int) (*hamtNode[V], bool) {
	chunk := Chunk(id, depth)
	bit := uint32(1) << chunk
	if n.bitmap&bit == 0 {
		return n, false
	}
	idx := popcount(n.bitmap & (bit - 1))
	child := &n.children[idx]

	if child.isLeaf() {
		if child.key != id {
			return n, false
		}
		// Remove this leaf.
		newBitmap := n.bitmap &^ bit
		if newBitmap == 0 {
			return nil, true
		}
		nc := make([]hamtChild[V], len(n.children)-1)
		copy(nc[:idx], n.children[:idx])
		copy(nc[idx:], n.children[idx+1:])
		return &hamtNode[V]{bitmap: newBitmap, children: nc}, true
	}

	// Recurse into sub-node.
	newSub, removed := child.sub.del(id, depth+1)
	if !removed {
		return n, false
	}

	nc := make([]hamtChild[V], len(n.children))
	copy(nc, n.children)

	if newSub == nil {
		// Sub-node became empty: remove this slot.
		newBitmap := n.bitmap &^ bit
		if newBitmap == 0 {
			return nil, true
		}
		result := make([]hamtChild[V], len(n.children)-1)
		copy(result[:idx], n.children[:idx])
		copy(result[idx:], n.children[idx+1:])
		return &hamtNode[V]{bitmap: newBitmap, children: result}, true
	}

	// Collapse single-leaf sub-nodes upward.
	if len(newSub.children) == 1 && newSub.children[0].isLeaf() {
		nc[idx] = newSub.children[0]
		return &hamtNode[V]{bitmap: n.bitmap, children: nc}, true
	}

	nc[idx] = hamtChild[V]{sub: newSub}
	return &hamtNode[V]{bitmap: n.bitmap, children: nc}, true
}

func (n *hamtNode[V]) iterate(fn func(c4.ID, V) bool) bool {
	for i := range n.children {
		if n.children[i].isLeaf() {
			if !fn(n.children[i].key, n.children[i].value) {
				return false
			}
		} else {
			if !n.children[i].sub.iterate(fn) {
				return false
			}
		}
	}
	return true
}
