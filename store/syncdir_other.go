//go:build !windows
// +build !windows

package store

import "os"

// SyncDir flushes a directory to stable storage. On darwin os.File.Sync
// issues F_FULLFSYNC, which flushes the device write cache — one call
// makes every previously-fsynced write durable (the batch barrier). On
// other Unixes it is a plain directory fsync, which persists the rename
// entries created by atomic writes.
func SyncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
