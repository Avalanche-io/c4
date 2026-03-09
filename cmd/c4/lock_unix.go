//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// lockC4mFile acquires an exclusive advisory lock on a c4m file's sidecar
// lock file. Returns an unlock function that must be deferred. This prevents
// concurrent c4 commands from racing on the same c4m file.
func lockC4mFile(path string) (unlock func(), err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	lockPath := abs + ".lock"
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
