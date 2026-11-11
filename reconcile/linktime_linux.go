//go:build linux
// +build linux

package reconcile

import (
	"syscall"
	"time"
	"unsafe"
)

// linkTimeSupported reports whether setLinkTime can set a symlink's own
// time on this platform.
const linkTimeSupported = true

// Linux kernel ABI values, the same on every architecture; the syscall
// package keeps them unexported.
const (
	atFDCWD           = -100
	atSymlinkNoFollow = 0x100
)

// setLinkTime sets a symlink's own modification (and access) time without
// following it: utimensat(AT_FDCWD, path, times, AT_SYMLINK_NOFOLLOW).
func setLinkTime(path string, t time.Time) error {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return err
	}
	ts := [2]syscall.Timespec{syscall.NsecToTimespec(t.UnixNano()), syscall.NsecToTimespec(t.UnixNano())}
	dirfd := atFDCWD
	_, _, errno := syscall.Syscall6(syscall.SYS_UTIMENSAT, uintptr(dirfd),
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&ts[0])), atSymlinkNoFollow, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
