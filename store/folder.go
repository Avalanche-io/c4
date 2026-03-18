package store

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Avalanche-io/c4"
)

var _ Store = Folder("")

// Folder is an implementation of the Store interface that uses c4 id nameed
// files in a filsystem folder.
type Folder string

// Open opens a file named the given c4.ID in read-only mode from the folder. If
// the file does not exist an error is returned.
func (f Folder) Open(id c4.ID) (io.ReadCloser, error) {
	return os.Open(filepath.Join(string(f), id.String()))
}

// Create creates and opens for writing a file with the given c4 id as its
// name if the file does not already exist. Writes go to a temp file; Close
// syncs to disk and atomically renames to the final path.
func (f Folder) Create(id c4.ID) (io.WriteCloser, error) {
	path := filepath.Join(string(f), id.String())
	if _, err := os.Stat(path); err == nil {
		return nil, &os.PathError{Op: "create", Path: path, Err: os.ErrExist}
	}
	return NewDurableWriter(path)
}

func (f Folder) Remove(id c4.ID) error {
	path := filepath.Join(string(f), id.String())
	return os.Remove(path)
}
