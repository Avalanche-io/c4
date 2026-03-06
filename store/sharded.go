package store

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4"
)

var _ Store = ShardedFolder("")

// ShardedFolder stores content-addressed files in a two-level directory
// structure using characters 3-4 of the C4 ID as the shard key. This
// distributes files across up to 3,364 subdirectories, avoiding the
// performance degradation that occurs when a single directory accumulates
// more than ~10,000 entries.
type ShardedFolder string

func shardKey(id c4.ID) string {
	s := id.String()
	return s[3:5]
}

func (f ShardedFolder) path(id c4.ID) string {
	return filepath.Join(string(f), shardKey(id), id.String())
}

// Open opens a sharded file for reading. Falls back to flat layout for
// backward compatibility with unsharded stores.
func (f ShardedFolder) Open(id c4.ID) (io.ReadCloser, error) {
	rc, err := os.Open(f.path(id))
	if err == nil {
		return rc, nil
	}
	// Fall back to flat layout
	return os.Open(filepath.Join(string(f), id.String()))
}

// Create creates a file in the sharded layout.
func (f ShardedFolder) Create(id c4.ID) (io.WriteCloser, error) {
	p := f.path(id)
	if _, err := os.Stat(p); err == nil {
		return nil, &os.PathError{Op: "create", Path: p, Err: os.ErrExist}
	}
	// Also check flat layout for existing content
	flat := filepath.Join(string(f), id.String())
	if _, err := os.Stat(flat); err == nil {
		return nil, &os.PathError{Op: "create", Path: flat, Err: os.ErrExist}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return nil, err
	}
	return os.Create(p)
}

// Remove removes a file from sharded or flat layout.
func (f ShardedFolder) Remove(id c4.ID) error {
	err := os.Remove(f.path(id))
	if err == nil {
		return nil
	}
	// Try flat layout
	return os.Remove(filepath.Join(string(f), id.String()))
}

// Path returns the sharded path for a C4 ID.
func (f ShardedFolder) Path(id c4.ID) string {
	return f.path(id)
}

// Has returns true if the content exists in either sharded or flat layout.
func (f ShardedFolder) Has(id c4.ID) bool {
	if _, err := os.Stat(f.path(id)); err == nil {
		return true
	}
	_, err := os.Stat(filepath.Join(string(f), id.String()))
	return err == nil
}
