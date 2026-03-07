package store

import (
	"os"
	"path/filepath"
)

// durableWriter writes to a temp file, then on Close syncs to disk and
// atomically renames to the final path. This guarantees that the final
// file is either fully written or absent — never partially written.
type durableWriter struct {
	tmp   *os.File
	final string
}

// newDurableWriter creates a temp file in the same directory as final,
// ensuring the rename will be atomic (same filesystem).
func newDurableWriter(final string) (*durableWriter, error) {
	f, err := os.CreateTemp(filepath.Dir(final), ".tmp.*")
	if err != nil {
		return nil, err
	}
	return &durableWriter{tmp: f, final: final}, nil
}

func (w *durableWriter) Write(b []byte) (int, error) {
	return w.tmp.Write(b)
}

func (w *durableWriter) Close() error {
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
