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
	mu             sync.Mutex
}

var _ Store = (*TreeStore)(nil)

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
// in advance. Writes go to a temp file; Close syncs and renames atomically.
func (s *TreeStore) Create(id c4.ID) (io.WriteCloser, error) {
	p := s.path(id)
	if _, err := os.Stat(p); err == nil {
		return nil, &os.PathError{Op: "create", Path: p, Err: os.ErrExist}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	return NewDurableWriter(p)
}

// Put reads all content from r, computes its C4 ID, stores it, and returns
// the ID. If the content already exists the write is skipped.
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
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return c4.ID{}, fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return c4.ID{}, fmt.Errorf("close: %w", err)
	}

	var id c4.ID
	copy(id[:], h.Sum(nil))

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

	// Check if the leaf directory needs splitting.
	s.maybeSplit(filepath.Dir(p), id.String())

	return id, nil
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

// maybeSplit checks if the leaf directory containing a newly added file
// exceeds the split threshold, and if so redistributes files into 2-char
// subdirectories based on the next prefix segment.
func (s *TreeStore) maybeSplit(dir, idStr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Count only regular files (not subdirectories or temps).
	var fileCount int
	for _, e := range entries {
		if !e.IsDir() && !isTemp(e.Name()) {
			fileCount++
		}
	}

	if fileCount <= s.splitThreshold {
		return
	}

	// Determine the prefix depth of this directory relative to root.
	depth := s.prefixDepth(dir)

	// Redistribute files into 2-char subdirectories.
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
