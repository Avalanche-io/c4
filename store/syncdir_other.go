//go:build !windows
// +build !windows

package store

import (
	"os"
	"syscall"
)

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

// fsyncDir persists a directory's entries with a plain fsync(2): the
// renames that landed in it reach the filesystem's journal or log,
// without the device-cache flush that SyncDir issues on darwin. The
// batch barrier calls it once per directory a rename touched, then
// SyncDir once for the whole batch, so the device flush is paid once,
// not per directory.
func fsyncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return syscall.Fsync(int(f.Fd()))
}
