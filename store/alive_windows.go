//go:build windows
// +build windows

package store

import "syscall"

// stillActive is the exit code GetExitCodeProcess reports for a process
// that has not exited (STILL_ACTIVE).
const stillActive = 259

// processAlive reports whether a process with this pid exists. A handle
// we may not open (access denied) belongs to a process that exists; any
// other failure means there is none. When in doubt, alive: the cost of
// keeping a temp file is nothing, the cost of removing a live one is a
// failed ingest.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return err == syscall.ERROR_ACCESS_DENIED
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}
