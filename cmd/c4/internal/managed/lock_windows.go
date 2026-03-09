//go:build windows

package managed

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	modkernel32    = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx = modkernel32.NewProc("LockFileEx")
	procUnlockFile = modkernel32.NewProc("UnlockFile")
)

const lockfileExclusiveLock = 0x00000002

// lock acquires an exclusive lock on .c4/lock using LockFileEx on Windows.
// The returned function releases the lock. Callers must defer the release.
func (d *Dir) lock() (unlock func(), err error) {
	lockPath := filepath.Join(d.meta, "lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	h := syscall.Handle(f.Fd())
	ol := new(syscall.Overlapped)
	r1, _, e1 := procLockFileEx.Call(
		uintptr(h),
		uintptr(lockfileExclusiveLock),
		0,
		1, 0,
		uintptr(unsafe.Pointer(ol)),
	)
	if r1 == 0 {
		f.Close()
		return nil, e1
	}
	return func() {
		procUnlockFile.Call(uintptr(h), 0, 0, 1, 0)
		f.Close()
	}, nil
}
