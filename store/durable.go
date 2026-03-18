package store

import (
	"os"
	"path/filepath"
)

// DurableWriter writes to a temp file, then on Close syncs to disk and
// atomically renames to the final path. This guarantees that the final
// file is either fully written or absent — never partially written.
type DurableWriter struct {
	tmp   *os.File
	final string
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

func (w *DurableWriter) Write(b []byte) (int, error) {
	return w.tmp.Write(b)
}

func (w *DurableWriter) Close() error {
	if err := w.tmp.Sync(); err != nil {
		w.tmp.Close()
		os.Remove(w.tmp.Name())
		return err
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
