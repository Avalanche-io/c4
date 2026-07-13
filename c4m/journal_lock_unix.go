//go:build !windows
// +build !windows

package c4m

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive advisory lock on f, blocking until it is
// available. The lock is tied to the file descriptor: closing f — or
// the process dying — releases it, so a crashed writer never wedges
// the journal.
func lockFile(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			return err
		}
	}
}
