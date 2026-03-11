package db

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Avalanche-io/c4"
)

// blobStore is a filesystem-backed content store using sharded directories.
// Blobs are stored under a two-character shard prefix derived from chars 3-4
// of the C4 ID string. This keeps directory sizes manageable.
type blobStore struct {
	root string // absolute path to the store directory
}

func newBlobStore(dir string) (*blobStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &blobStore{root: dir}, nil
}

// shardKey returns the two-character shard prefix (chars 3-4 of the C4 ID).
func shardKey(id c4.ID) string {
	s := id.String()
	return s[3:5]
}

func (s *blobStore) shardPath(id c4.ID) string {
	return filepath.Join(s.root, shardKey(id), id.String())
}

func (s *blobStore) flatPath(id c4.ID) string {
	return filepath.Join(s.root, id.String())
}

// Put stores content from r, computes and returns its C4 ID.
// Idempotent: storing the same content twice is a no-op.
func (s *blobStore) Put(r io.Reader) (c4.ID, error) {
	tmp, err := os.CreateTemp(s.root, ".c4-tmp-*")
	if err != nil {
		return c4.ID{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	id := c4.Identify(io.TeeReader(r, tmp))
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return c4.ID{}, err
	}
	tmp.Close()

	finalPath := s.shardPath(id)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0755); err != nil {
		return c4.ID{}, err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		// Rename can fail on Windows if target is locked.
		if s.Has(id) {
			return id, nil
		}
		return c4.ID{}, err
	}
	return id, nil
}

// Get retrieves content by C4 ID. Caller must close the reader.
func (s *blobStore) Get(id c4.ID) (io.ReadCloser, error) {
	f, err := os.Open(s.shardPath(id))
	if err == nil {
		return f, nil
	}
	// Fall back to flat layout for backward compatibility.
	return os.Open(s.flatPath(id))
}

// Has returns true if the store contains the given C4 ID.
func (s *blobStore) Has(id c4.ID) bool {
	if _, err := os.Stat(s.shardPath(id)); err == nil {
		return true
	}
	_, err := os.Stat(s.flatPath(id))
	return err == nil
}

// Delete removes content by C4 ID.
func (s *blobStore) Delete(id c4.ID) error {
	if err := os.Remove(s.shardPath(id)); err == nil {
		return nil
	}
	err := os.Remove(s.flatPath(id))
	if os.IsNotExist(err) {
		return os.ErrNotExist
	}
	return err
}

// Walk iterates over all stored C4 IDs, calling fn for each one.
// If fn returns a non-nil error, iteration stops and that error is returned.
func (s *blobStore) Walk(fn func(c4.ID) error) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, shardDir := range entries {
		if !shardDir.IsDir() || strings.HasPrefix(shardDir.Name(), ".") {
			continue
		}
		shardPath := filepath.Join(s.root, shardDir.Name())
		files, err := os.ReadDir(shardPath)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
				continue
			}
			id, err := c4.Parse(f.Name())
			if err != nil {
				continue
			}
			if err := fn(id); err != nil {
				return err
			}
		}
	}
	// Also walk flat (unsharded) files for backward compatibility.
	for _, f := range entries {
		if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		id, err := c4.Parse(f.Name())
		if err != nil {
			continue
		}
		if err := fn(id); err != nil {
			return err
		}
	}
	return nil
}
