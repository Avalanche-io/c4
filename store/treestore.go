package store

import (
	"crypto/sha512"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/Avalanche-io/c4"
)

// DefaultSplitThreshold is the maximum number of content files in a leaf
// directory before it splits into 2-char subdirectories.
const DefaultSplitThreshold = 4096

// TreeStore is a content-addressed store that uses adaptive trie sharding.
// Every C4 ID starts with "c4", so the store always has exactly one
// top-level directory: c4/. Real fanout begins at characters 3-4.
//
// Directories are either leaves (contain content files) or interior nodes
// (contain 2-char subdirectories). When a leaf exceeds SplitThreshold
// files, it splits into subdirectories based on the next 2 characters.
type TreeStore struct {
	root           string
	splitThreshold int
	syncMode       SyncMode
	mu             sync.Mutex
	counts         map[string]int // leaf dir → file count (split accounting)
	dirty          bool           // batch writes awaiting a Sync barrier
}

var _ Store = (*TreeStore)(nil)
var _ Syncer = (*TreeStore)(nil)

// NewTreeStore creates a TreeStore rooted at the given directory.
// The directory is created if it does not exist.
func NewTreeStore(root string) (*TreeStore, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, fmt.Errorf("create store root: %w", err)
	}
	return &TreeStore{root: root, splitThreshold: DefaultSplitThreshold}, nil
}

// Root returns the root directory of the store.
func (s *TreeStore) Root() string {
	return s.root
}

// SetSplitThreshold sets the maximum file count before a leaf directory
// splits. This is primarily useful for testing.
func (s *TreeStore) SetSplitThreshold(n int) {
	s.splitThreshold = n
}

// SetSyncMode selects the write-durability policy. The default is
// SyncEach: every object durable before it lands. Set the mode before
// writing, not concurrently with writes.
func (s *TreeStore) SetSyncMode(mode SyncMode) {
	s.syncMode = mode
}

// Sync makes every object written so far durable. Under SyncEach each
// write is already durable and under SyncNone durability is waived, so
// both are no-ops. Under SyncBatch this is the batch barrier: one
// device-cache flush (F_FULLFSYNC on darwin) covering every object
// written since the last Sync.
func (s *TreeStore) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncMode != SyncBatch || !s.dirty {
		return nil
	}
	f, err := os.Open(s.root)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return err
	}
	s.dirty = false
	return nil
}

// Has reports whether the store contains content for the given ID.
func (s *TreeStore) Has(id c4.ID) bool {
	_, err := os.Stat(s.path(id))
	return err == nil
}

// Open opens the content for reading.
func (s *TreeStore) Open(id c4.ID) (io.ReadCloser, error) {
	return os.Open(s.path(id))
}

// Create creates a new entry for writing. The caller must know the ID
// in advance. Writes go to a temp file; Close flushes per the store's
// sync mode and renames atomically.
func (s *TreeStore) Create(id c4.ID) (io.WriteCloser, error) {
	p := s.path(id)
	if _, err := os.Stat(p); err == nil {
		return nil, &os.PathError{Op: "create", Path: p, Err: os.ErrExist}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	w, err := NewDurableWriter(p)
	if err != nil {
		return nil, err
	}
	w.sync = s.syncMode
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
	return w, nil
}

// Put reads all content from r, computes its C4 ID, stores it, and returns
// the ID. If the content already exists the write is skipped. Put is safe
// for concurrent use: hashing, temp writes, and flushes overlap; only the
// publish step (exists-check, rename, split accounting) is serialized.
func (s *TreeStore) Put(r io.Reader) (c4.ID, error) {
	// Write to a temp file while computing the C4 ID.
	tmp, err := os.CreateTemp(s.root, ".ingest.*")
	if err != nil {
		return c4.ID{}, fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // clean up on any error path

	h := sha512.New()
	w := io.MultiWriter(tmp, h)
	if _, err := io.Copy(w, r); err != nil {
		tmp.Close()
		return c4.ID{}, fmt.Errorf("copy: %w", err)
	}
	if err := flushFile(tmp, s.syncMode); err != nil {
		tmp.Close()
		return c4.ID{}, fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return c4.ID{}, fmt.Errorf("close: %w", err)
	}

	var id c4.ID
	copy(id[:], h.Sum(nil))

	// Publish under the lock: path resolution, rename, and split
	// accounting must not interleave with a concurrent split.
	s.mu.Lock()
	defer s.mu.Unlock()

	// If content already exists, skip the rename.
	p := s.path(id)
	if _, err := os.Stat(p); err == nil {
		return id, nil
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return c4.ID{}, fmt.Errorf("mkdir: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return c4.ID{}, fmt.Errorf("rename: %w", err)
	}
	s.dirty = true

	// Check if the leaf directory needs splitting.
	s.noteAdd(filepath.Dir(p))

	return id, nil
}

// ContentPath returns the local filesystem path for id when present.
func (s *TreeStore) ContentPath(id c4.ID) (string, bool) {
	p := s.path(id)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// Remove deletes the content for the given ID.
func (s *TreeStore) Remove(id c4.ID) error {
	return os.Remove(s.path(id))
}

// path resolves the storage path for an ID by walking the trie.
// It follows 2-char prefix subdirectories until reaching a leaf.
func (s *TreeStore) path(id c4.ID) string {
	str := id.String()
	dir := s.root
	for i := 0; i+2 <= len(str); i += 2 {
		sub := filepath.Join(dir, str[i:i+2])
		info, err := os.Stat(sub)
		if err != nil || !info.IsDir() {
			break
		}
		dir = sub
	}
	return filepath.Join(dir, str)
}

// noteAdd records one new file in a leaf directory and splits the leaf
// when it exceeds the threshold. The count is cached so the split check
// is O(1) per Put instead of a ReadDir; the cache is seeded by one
// ReadDir on first touch and invalidated on split. External writers may
// skew the cache — the only consequence is a slightly early or late
// split. Caller must hold s.mu.
func (s *TreeStore) noteAdd(dir string) {
	n, ok := s.counts[dir]
	if !ok {
		n = countFiles(dir) // includes the file just renamed in
	} else {
		n++
	}
	if s.counts == nil {
		s.counts = make(map[string]int)
	}
	s.counts[dir] = n

	if n <= s.splitThreshold {
		return
	}
	s.split(dir)
	delete(s.counts, dir)
}

// countFiles counts regular (non-temp) files in dir.
func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var n int
	for _, e := range entries {
		if !e.IsDir() && !isTemp(e.Name()) {
			n++
		}
	}
	return n
}

// split redistributes a leaf directory's files into 2-char
// subdirectories based on the next prefix segment. Caller must hold s.mu.
func (s *TreeStore) split(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Determine the prefix depth of this directory relative to root.
	depth := s.prefixDepth(dir)

	for _, e := range entries {
		if e.IsDir() || isTemp(e.Name()) {
			continue
		}
		name := e.Name()
		if len(name) <= depth+2 {
			continue // ID too short for another level (shouldn't happen)
		}
		sub := name[depth : depth+2]
		subDir := filepath.Join(dir, sub)
		os.MkdirAll(subDir, 0755)
		os.Rename(filepath.Join(dir, name), filepath.Join(subDir, name))
	}
}

// prefixDepth returns how many characters of the ID are consumed by the
// directory path from root to dir. Each trie level consumes 2 characters.
func (s *TreeStore) prefixDepth(dir string) int {
	rel, err := filepath.Rel(s.root, dir)
	if err != nil {
		return 0
	}
	if rel == "." {
		return 0
	}
	parts := filepath.SplitList(rel)
	if len(parts) == 1 {
		parts = splitPath(rel)
	}
	return len(parts) * 2
}

// splitPath splits a path into its components.
func splitPath(p string) []string {
	var parts []string
	for {
		dir, file := filepath.Split(p)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		if dir == "" || dir == p {
			break
		}
		p = filepath.Clean(dir)
	}
	return parts
}

// isTemp returns true for temp files created during ingestion.
func isTemp(name string) bool {
	return len(name) > 0 && name[0] == '.'
}
