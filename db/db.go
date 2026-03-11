package db

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// ErrConflict is returned when a compare-and-put detects a mismatch.
var ErrConflict = errors.New("db: conflict")

// DB is the MVCC content-addressed database. Readers are lock-free and
// concurrent. Writers are serialized. Snapshots are immutable.
type DB struct {
	dir   string
	store *blobStore

	writeMu sync.Mutex           // serializes write transactions
	root    atomic.Pointer[node] // current immutable root

	snapMu sync.Mutex
	snaps  map[*Snapshot]c4.ID // live snapshot -> root c4m ID (for GC)

	holdMu sync.RWMutex
	holds  map[c4.ID]int // blob ID -> hold count (protects from GC)

	watchMu  sync.Mutex
	watchers []chan struct{} // closed on every commit

	maxFilterMemory int // max bytes for GC CuckooFilter (0 = default 64MB)
}

// Entry is a namespace entry returned by List.
type Entry struct {
	Name  string
	ID    c4.ID
	IsDir bool
}

// Open opens or creates a database at the given directory.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("db: mkdir: %w", err)
	}

	storeDir := filepath.Join(dir, "blobs")
	bs, err := newBlobStore(storeDir)
	if err != nil {
		return nil, fmt.Errorf("db: store: %w", err)
	}

	db := &DB{
		dir:   dir,
		store: bs,
		snaps: make(map[*Snapshot]c4.ID),
		holds: make(map[c4.ID]int),
	}

	root, err := db.loadRoot()
	if err != nil {
		return nil, fmt.Errorf("db: load root: %w", err)
	}
	db.root.Store(root)

	return db, nil
}

// Close releases database resources.
func (db *DB) Close() error {
	return nil
}

// --- Blob storage (outside transaction hot path) ---

// Store writes a blob. The caller has already computed the C4 ID.
// Idempotent: storing the same content is a no-op.
func (db *DB) Store(id c4.ID, r io.Reader) error {
	got, err := db.store.Put(r)
	if err != nil {
		return err
	}
	if got != id {
		db.store.Delete(got)
		return fmt.Errorf("db: ID mismatch: got %s, want %s", got, id)
	}
	return nil
}

// StoreBytes is a convenience for storing a byte slice.
func (db *DB) StoreBytes(data []byte) (c4.ID, error) {
	return db.store.Put(bytes.NewReader(data))
}

// Has returns true if a blob exists in the store.
func (db *DB) Has(id c4.ID) bool {
	return db.store.Has(id)
}

// Get retrieves a blob by C4 ID. Caller must close the reader.
func (db *DB) Get(id c4.ID) (io.ReadCloser, error) {
	return db.store.Get(id)
}

// --- Snapshots (lock-free, immutable) ---

// Snapshot is an immutable point-in-time view of the namespace.
// Call Release when done to allow GC of unreferenced blobs.
type Snapshot struct {
	db   *DB
	root *node
	id   c4.ID // root c4m ID (for GC tracking)
	once sync.Once
}

// Snapshot returns the current immutable view. Call Release when done.
func (db *DB) Snapshot() *Snapshot {
	root := db.root.Load()
	s := &Snapshot{db: db, root: root, id: root.blobID}
	db.snapMu.Lock()
	db.snaps[s] = s.id
	db.snapMu.Unlock()
	return s
}

// Release marks the snapshot as no longer needed.
func (s *Snapshot) Release() {
	s.once.Do(func() {
		s.db.snapMu.Lock()
		delete(s.db.snaps, s)
		s.db.snapMu.Unlock()
	})
}

// Resolve looks up a path and returns the C4 ID at that location.
func (s *Snapshot) Resolve(path string) (c4.ID, error) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return c4.ID{}, errors.New("empty path")
	}
	id, ok := s.root.resolve(parts)
	if !ok {
		return c4.ID{}, fmt.Errorf("not found: %s", path)
	}
	return id, nil
}

// List returns entries under the given directory path.
func (s *Snapshot) List(path string) ([]Entry, error) {
	parts := splitPath(path)
	target := s.root.get(parts)
	if target == nil {
		return nil, fmt.Errorf("not found: %s", path)
	}
	if !target.isDir() {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	entries := make([]Entry, len(target.children))
	for i := range target.children {
		entries[i] = Entry{
			Name:  target.children[i].name,
			ID:    target.children[i].node.id,
			IsDir: target.children[i].node.isDir(),
		}
	}
	return entries, nil
}

// Has returns true if a blob exists.
func (s *Snapshot) Has(id c4.ID) bool {
	return s.db.Has(id)
}

// Get retrieves a blob by C4 ID.
func (s *Snapshot) Get(id c4.ID) (io.ReadCloser, error) {
	return s.db.Get(id)
}

// Root returns the root c4m ID of this snapshot.
func (s *Snapshot) Root() c4.ID { return s.id }

// WalkLeaves calls fn for every leaf in the snapshot with its path and ID.
func (s *Snapshot) WalkLeaves(fn func(path string, id c4.ID)) {
	s.root.walkLeaves(fn)
}

// View runs fn against a read-only snapshot. The snapshot is
// automatically released when fn returns.
func (db *DB) View(fn func(s *Snapshot) error) error {
	s := db.Snapshot()
	defer s.Release()
	return fn(s)
}

// --- Holds (GC protection) ---

// Hold marks the given IDs as protected from garbage collection.
// Returns a release function that MUST be called when done.
// Safe to call concurrently. Holds are reference-counted.
func (db *DB) Hold(ids []c4.ID) func() {
	db.holdMu.Lock()
	for _, id := range ids {
		db.holds[id]++
	}
	db.holdMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			db.holdMu.Lock()
			for _, id := range ids {
				db.holds[id]--
				if db.holds[id] <= 0 {
					delete(db.holds, id)
				}
			}
			db.holdMu.Unlock()
		})
	}
}

// IsHeld returns true if the given ID is currently held.
func (db *DB) IsHeld(id c4.ID) bool {
	db.holdMu.RLock()
	defer db.holdMu.RUnlock()
	return db.holds[id] > 0
}

// --- Watchers (commit notifications) ---

// Watch returns a channel that will be closed on the next successful commit.
// After receiving the notification, call Watch again for the next one.
func (db *DB) Watch() <-chan struct{} {
	ch := make(chan struct{})
	db.watchMu.Lock()
	db.watchers = append(db.watchers, ch)
	db.watchMu.Unlock()
	return ch
}

func (db *DB) notifyWatchers() {
	db.watchMu.Lock()
	for _, ch := range db.watchers {
		close(ch)
	}
	db.watchers = db.watchers[:0]
	db.watchMu.Unlock()
}

// --- Transactions (serialized writers) ---

// Tx is a read-write transaction. Mutations are applied to a COW copy
// of the tree. On commit, the new root is persisted and swapped atomically.
type Tx struct {
	db   *DB
	base *node // original root (for conflict detection if needed)
	root *node // working copy (COW)
}

// Resolve looks up a path in the transaction's working state.
func (tx *Tx) Resolve(path string) (c4.ID, error) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return c4.ID{}, errors.New("empty path")
	}
	id, ok := tx.root.resolve(parts)
	if !ok {
		return c4.ID{}, fmt.Errorf("not found: %s", path)
	}
	return id, nil
}

// List returns entries under the given directory in the working state.
func (tx *Tx) List(path string) ([]Entry, error) {
	parts := splitPath(path)
	target := tx.root.get(parts)
	if target == nil {
		return nil, fmt.Errorf("not found: %s", path)
	}
	if !target.isDir() {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	entries := make([]Entry, len(target.children))
	for i := range target.children {
		entries[i] = Entry{
			Name:  target.children[i].name,
			ID:    target.children[i].node.id,
			IsDir: target.children[i].node.isDir(),
		}
	}
	return entries, nil
}

// Has returns true if a blob exists in the store.
func (tx *Tx) Has(id c4.ID) bool {
	return tx.db.Has(id)
}

// Put places a C4 ID at the given path.
func (tx *Tx) Put(path string, id c4.ID) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return errors.New("empty path")
	}
	if id.IsNil() {
		return errors.New("nil ID")
	}
	tx.root = tx.root.put(parts, id)
	return nil
}

// CompareAndPut atomically updates path to newID only if the current
// value equals expected. Returns ErrConflict on mismatch.
func (tx *Tx) CompareAndPut(path string, expected, newID c4.ID) error {
	current, err := tx.Resolve(path)
	if err != nil && !expected.IsNil() {
		return ErrConflict
	}
	if err == nil && current != expected {
		return ErrConflict
	}
	return tx.Put(path, newID)
}

// Delete removes the entry at the given path.
func (tx *Tx) Delete(path string) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return errors.New("empty path")
	}
	if _, ok := tx.root.resolve(parts); !ok {
		return fmt.Errorf("not found: %s", path)
	}
	tx.root = tx.root.del(parts)
	return nil
}

// Update runs fn in a serialized write transaction. If fn returns nil,
// the transaction commits: the tree is persisted and the root pointer
// is swapped atomically.
func (db *DB) Update(fn func(tx *Tx) error) error {
	db.writeMu.Lock()
	defer db.writeMu.Unlock()

	tx := &Tx{
		db:   db,
		base: db.root.Load(),
		root: db.root.Load(),
	}

	if err := fn(tx); err != nil {
		return err
	}

	// Nothing changed
	if tx.root == tx.base {
		return nil
	}

	// Persist dirty directory nodes (O(depth), not O(n))
	rootID, err := db.persistNode(tx.root)
	if err != nil {
		return fmt.Errorf("db: persist: %w", err)
	}

	// Update root pointer file
	if err := db.writeRootPointer(rootID); err != nil {
		return fmt.Errorf("db: root pointer: %w", err)
	}

	// Swap in-memory root (visible to new readers immediately)
	db.root.Store(tx.root)

	// Notify watchers
	db.notifyWatchers()
	return nil
}

// --- Persistence ---

func (db *DB) rootPointerPath() string {
	return filepath.Join(db.dir, "ROOT")
}

// loadRoot loads the tree from the persisted root pointer, or creates
// a default root for a new database.
func (db *DB) loadRoot() (*node, error) {
	data, err := os.ReadFile(db.rootPointerPath())
	if errors.Is(err, os.ErrNotExist) {
		root := defaultRoot()
		rootID, err := db.persistNode(root)
		if err != nil {
			return nil, err
		}
		if err := db.writeRootPointer(rootID); err != nil {
			return nil, err
		}
		return root, nil
	}
	if err != nil {
		return nil, err
	}

	idStr := strings.TrimSpace(string(data))
	rootID, err := c4.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("parse root ID: %w", err)
	}

	return db.loadNode(rootID)
}

// persistNode recursively persists dirty directory nodes as per-directory
// c4m blobs. Clean subtrees (non-zero blobID) are skipped — only the
// path from root to the changed leaf is re-serialized.
func (db *DB) persistNode(n *node) (c4.ID, error) {
	if n.isLeaf() {
		return n.id, nil
	}
	if !n.blobID.IsNil() {
		return n.blobID, nil // already persisted, unchanged
	}

	// Dirty directory — persist children first, then serialize this level.
	m := c4m.NewManifest()
	for _, ch := range n.children {
		childID, err := db.persistNode(ch.node)
		if err != nil {
			return c4.ID{}, fmt.Errorf("persist child %q: %w", ch.name, err)
		}
		e := &c4m.Entry{C4ID: childID, Name: ch.name}
		if ch.node.isDir() {
			e.Mode = os.ModeDir
			e.Name += "/" // c4m convention for directories
		}
		m.AddEntry(e)
	}

	data, err := c4m.Marshal(m)
	if err != nil {
		return c4.ID{}, fmt.Errorf("marshal dir: %w", err)
	}
	blobID, err := db.store.Put(bytes.NewReader(data))
	if err != nil {
		return c4.ID{}, fmt.Errorf("store dir blob: %w", err)
	}

	// Safe to set: this node was just created by COW, not yet published.
	n.blobID = blobID
	return blobID, nil
}

// loadNode recursively loads a directory node from a per-directory c4m blob.
func (db *DB) loadNode(id c4.ID) (*node, error) {
	rc, err := db.store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("get dir blob %s: %w", id, err)
	}
	data, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}

	m, err := c4m.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal dir blob: %w", err)
	}

	children := make([]nodeChild, 0, len(m.Entries))
	for _, e := range m.Entries {
		name := strings.TrimSuffix(e.Name, "/")
		if e.IsDir() {
			child, err := db.loadNode(e.C4ID)
			if err != nil {
				return nil, fmt.Errorf("load child dir %q: %w", name, err)
			}
			children = append(children, nodeChild{name, child})
		} else {
			children = append(children, nodeChild{name, newLeaf(e.C4ID)})
		}
	}
	return &node{children: children, blobID: id}, nil
}

// writeRootPointer atomically updates the root pointer file.
func (db *DB) writeRootPointer(id c4.ID) error {
	p := db.rootPointerPath()
	tmp, err := os.CreateTemp(db.dir, ".root-tmp-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(id.String() + "\n"); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// liveRootIDs returns the set of root c4m IDs that are currently in use
// (all held snapshots). Used by GC.
func (db *DB) liveRootIDs() []c4.ID {
	db.snapMu.Lock()
	defer db.snapMu.Unlock()

	seen := make(map[c4.ID]struct{})
	var ids []c4.ID

	for _, id := range db.snaps {
		if id.IsNil() {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}
