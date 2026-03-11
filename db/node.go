// Package db provides an MVCC content-addressed database.
//
// The core data structure is an immutable persistent tree with structural
// sharing. Mutations produce new roots via copy-on-write. Readers hold
// immutable snapshots. Writers CAS the root pointer.
//
// Persistence uses a Merkle tree of per-directory c4m blobs. Each directory
// is its own c4m file listing immediate children. When a leaf changes, only
// the directories along the path to root are re-serialized — unchanged
// subtrees retain their existing blob IDs.
package db

import (
	"strings"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/hashlib"
)

// nodeChild is a named child in a sorted children slice.
type nodeChild struct {
	name string
	node *node
}

// node is an immutable tree node. Nodes are never mutated after creation;
// modifications produce new nodes via copy-on-write. A directory node has
// a non-nil children slice. A leaf node has a non-nil C4 ID.
//
// blobID tracks the persisted c4m ID for directory nodes. When COW creates
// a new directory node, blobID is zero (dirty). After persist, blobID is
// set to the stored c4m ID. Unchanged subtrees keep their blobID, so
// persistNode skips them — O(depth) instead of O(n).
type node struct {
	children []nodeChild // sorted by name; non-nil for directories
	id       c4.ID       // for leaves: the user's c4m ID at this path
	blobID   c4.ID       // for directories: persisted c4m ID (zero = dirty)
}

func emptyDir() *node {
	return &node{children: []nodeChild{}}
}

func newLeaf(id c4.ID) *node {
	return &node{id: id}
}

func (n *node) isDir() bool  { return n.children != nil }
func (n *node) isLeaf() bool { return !n.id.IsNil() }

// findChild returns the child node and its index via binary search.
// If not found, returns (nil, insertion point).
func (n *node) findChild(name string) (*node, int) {
	lo, hi := 0, len(n.children)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if n.children[mid].name < name {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(n.children) && n.children[lo].name == name {
		return n.children[lo].node, lo
	}
	return nil, lo
}

// resolve walks the tree and returns the leaf c4m ID at the given path.
func (n *node) resolve(parts []string) (c4.ID, bool) {
	cur := n
	for _, part := range parts {
		if cur.children == nil {
			return c4.ID{}, false
		}
		child, _ := cur.findChild(part)
		if child == nil {
			return c4.ID{}, false
		}
		cur = child
	}
	if cur.id.IsNil() {
		return c4.ID{}, false
	}
	return cur.id, true
}

// get returns the node at the given path, or nil if not found.
func (n *node) get(parts []string) *node {
	cur := n
	for _, part := range parts {
		if cur.children == nil {
			return nil
		}
		child, _ := cur.findChild(part)
		if child == nil {
			return nil
		}
		cur = child
	}
	return cur
}

// put returns a new tree with the given path set to id.
// Creates intermediate directories as needed.
func (n *node) put(parts []string, id c4.ID) *node {
	if len(parts) == 0 {
		return newLeaf(id)
	}
	name := parts[0]
	child, _ := n.findChild(name)
	if child == nil {
		child = emptyDir()
	}
	return n.withChild(name, child.put(parts[1:], id))
}

// del returns a new tree with the node at the given path removed.
// Empty parent directories are NOT pruned automatically.
func (n *node) del(parts []string) *node {
	if len(parts) == 0 {
		return nil
	}
	name := parts[0]
	child, _ := n.findChild(name)
	if child == nil {
		return n
	}
	newChild := child.del(parts[1:])
	if newChild == nil {
		return n.withoutChild(name)
	}
	if newChild == child {
		return n // nothing changed
	}
	return n.withChild(name, newChild)
}

// size returns the total number of nodes in the tree (dirs + leaves).
func (n *node) size() int {
	if n == nil {
		return 0
	}
	count := 1
	for i := range n.children {
		count += n.children[i].node.size()
	}
	return count
}

// leafCount returns the number of leaf nodes (files) in the tree.
func (n *node) leafCount() int {
	if n == nil {
		return 0
	}
	if n.isLeaf() {
		return 1
	}
	count := 0
	for i := range n.children {
		count += n.children[i].node.leafCount()
	}
	return count
}

// walkLeaves calls fn for every leaf in the tree with its full path.
func (n *node) walkLeaves(fn func(path string, id c4.ID)) {
	n.walkLeavesHelper(make([]byte, 0, 256), fn)
}

func (n *node) walkLeavesHelper(buf []byte, fn func(path string, id c4.ID)) {
	if n.isLeaf() {
		if len(buf) > 0 {
			fn(string(buf[:len(buf)-1]), n.id) // trim trailing /
		}
		return
	}
	baseLen := len(buf)
	for i := range n.children {
		buf = append(buf[:baseLen], n.children[i].name...)
		buf = append(buf, '/')
		n.children[i].node.walkLeavesHelper(buf, fn)
	}
}

// collectDirBlobIDs adds all non-nil directory blobIDs to the filter.
// Returns false if the filter overflows.
// Used by GC to mark Merkle tree structure blobs as live.
func (n *node) collectDirBlobIDs(live *hashlib.CuckooFilter) bool {
	if !n.blobID.IsNil() {
		if !live.Add(n.blobID) {
			return false
		}
	}
	for i := range n.children {
		if n.children[i].node.isDir() {
			if !n.children[i].node.collectDirBlobIDs(live) {
				return false
			}
		}
	}
	return true
}

// collectDirBlobIDsMap adds all non-nil directory blobIDs to a map.
// Used by GC map fallback. Cannot fail.
func (n *node) collectDirBlobIDsMap(live *mapLiveSet) {
	if !n.blobID.IsNil() {
		live.Add(n.blobID)
	}
	for i := range n.children {
		if n.children[i].node.isDir() {
			n.children[i].node.collectDirBlobIDsMap(live)
		}
	}
}

// --- COW helpers ---

// withChild returns a new directory node with the named child replaced or
// inserted. The new node has a zero blobID (dirty) because its content changed.
func (n *node) withChild(name string, child *node) *node {
	old, idx := n.findChild(name)
	if old != nil {
		// Replace existing at idx
		nc := make([]nodeChild, len(n.children))
		copy(nc, n.children)
		nc[idx] = nodeChild{name, child}
		return &node{children: nc}
	}
	// Insert new at idx
	nc := make([]nodeChild, len(n.children)+1)
	copy(nc[:idx], n.children[:idx])
	nc[idx] = nodeChild{name, child}
	copy(nc[idx+1:], n.children[idx:])
	return &node{children: nc}
}

// withoutChild returns a new directory node with the named child removed.
// The new node has a zero blobID (dirty).
func (n *node) withoutChild(name string) *node {
	_, idx := n.findChild(name)
	if idx >= len(n.children) || n.children[idx].name != name {
		return n
	}
	nc := make([]nodeChild, len(n.children)-1)
	copy(nc[:idx], n.children[:idx])
	copy(nc[idx:], n.children[idx+1:])
	return &node{children: nc}
}

// --- path utilities ---

func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// defaultRoot creates the standard namespace root with system directories.
func defaultRoot() *node {
	names := []string{"bin", "etc", "home", "mnt", "tmp"}
	children := make([]nodeChild, len(names))
	for i, name := range names {
		children[i] = nodeChild{name, emptyDir()}
	}
	return &node{children: children}
}
