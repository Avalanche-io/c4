//go:build !windows

package managed

import (
	"os"
	"path/filepath"
	"syscall"
)

// lock acquires an exclusive advisory lock on .c4/lock. The returned
// function releases the lock. Callers must defer the release.
func (d *Dir) lock() (unlock func(), err error) {
	lockPath := filepath.Join(d.meta, "lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
