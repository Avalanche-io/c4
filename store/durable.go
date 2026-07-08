package store

import (
	"io"
	"os"
	"path/filepath"
)

// DurableWriter writes to a temp file, then on Close syncs to disk and
// atomically renames to the final path. This guarantees that the final
// file is either fully written or absent — never partially written.
type DurableWriter struct {
	tmp    *os.File
	final  string
	nosync bool
}

// NewDurableWriter creates a temp file in the same directory as final,
// ensuring the rename will be atomic (same filesystem).
func NewDurableWriter(final string) (*DurableWriter, error) {
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(dir, ".tmp.*")
	if err != nil {
		return nil, err
	}
	return &DurableWriter{tmp: f, final: final}, nil
}

// NewAtomicWriter is like NewDurableWriter but Close skips the sync:
// the rename is still atomic — readers never observe a partial file —
// but a power failure may lose the content. Suitable for scratch writes
// whose content can be re-materialized from a store.
func NewAtomicWriter(final string) (*DurableWriter, error) {
	w, err := NewDurableWriter(final)
	if err != nil {
		return nil, err
	}
	w.nosync = true
	return w, nil
}

func (w *DurableWriter) Write(b []byte) (int, error) {
	return w.tmp.Write(b)
}

// ReadFrom delegates to the underlying temp file so io.Copy can use OS
// copy acceleration (copy_file_range on Linux — a CoW reflink on
// filesystems that support it) when the source is also a file.
func (w *DurableWriter) ReadFrom(r io.Reader) (int64, error) {
	return w.tmp.ReadFrom(r)
}

func (w *DurableWriter) Close() error {
	if !w.nosync {
		if err := w.tmp.Sync(); err != nil {
			w.tmp.Close()
			os.Remove(w.tmp.Name())
			return err
		}
	}
	if err := w.tmp.Close(); err != nil {
		os.Remove(w.tmp.Name())
		return err
	}
	if err := os.Rename(w.tmp.Name(), w.final); err != nil {
		os.Remove(w.tmp.Name())
		return err
	}
	return nil
}
