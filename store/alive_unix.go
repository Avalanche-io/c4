//go:build !windows
// +build !windows

package store

import "syscall"

// processAlive reports whether a process with this pid exists. Signal 0
// delivers nothing and only checks; EPERM means the process exists but
// belongs to someone else, which is still alive.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
