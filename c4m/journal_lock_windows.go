//go:build windows
// +build windows

package c4m

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = kernel32.NewProc("LockFileEx")
)

const (
	lockfileExclusiveLock = 0x00000002
	errorLockViolation    = syscall.Errno(33) // ERROR_LOCK_VIOLATION
)

// lockFile takes an exclusive lock on f via LockFileEx, retrying until
// available. Windows releases file locks when the handle closes or the
// process dies, matching the dies-with-holder pin.
func lockFile(f *os.File) error {
	var ol syscall.Overlapped
	for {
		r, _, err := procLockFileEx.Call(
			f.Fd(),
			uintptr(lockfileExclusiveLock),
			0,
			1, 0, // lock one byte at offset 0 — serializes appenders
			uintptr(unsafe.Pointer(&ol)),
		)
		if r != 0 {
			return nil
		}
		if err == errorLockViolation {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		return err
	}
}
