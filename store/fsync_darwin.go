//go:build darwin
// +build darwin

package store

import (
	"os"
	"syscall"
)

// fsyncData flushes file data to the device with plain fsync(2) — not
// the F_FULLFSYNC that os.File.Sync issues on darwin. Data reaches the
// device but may sit in its volatile cache; pair with a later
// F_FULLFSYNC barrier for durability.
func fsyncData(f *os.File) error {
	for {
		err := syscall.Fsync(int(f.Fd()))
		if err != syscall.EINTR {
			return err
		}
	}
}
