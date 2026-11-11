//go:build darwin
// +build darwin

package reconcile

import (
	"syscall"
	"time"
)

// linkTimeSupported reports whether setLinkTime can set a symlink's own
// time on this platform.
const linkTimeSupported = true

// setLinkTime sets a symlink's own modification (and access) time without
// following it. darwin's syscall package exports no utimensat, and a raw
// syscall number is not a supported interface there (Go calls libSystem),
// so the link itself is opened with O_SYMLINK and its times set through
// the descriptor.
func setLinkTime(path string, t time.Time) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_SYMLINK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	tv := syscall.NsecToTimeval(t.UnixNano())
	return syscall.Futimes(fd, []syscall.Timeval{tv, tv})
}
