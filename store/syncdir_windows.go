//go:build windows
// +build windows

package store

// SyncDir is a no-op on Windows: FlushFileBuffers cannot be issued on a
// directory handle opened without backup semantics (os.Open returns
// handles that fail with ERROR_ACCESS_DENIED), and it is not needed —
// os.File.Sync on each file already reaches stable storage, and NTFS
// journals the rename metadata.
func SyncDir(path string) error {
	return nil
}
