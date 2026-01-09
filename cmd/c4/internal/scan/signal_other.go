// +build !darwin,!freebsd,!openbsd,!windows

package scan

import "syscall"

const (
	// SIGINFO doesn't exist on Linux and other non-BSD systems
	// We use SIGUSR1 as a fallback (signal 10)
	SIGINFO = syscall.Signal(10) // SIGUSR1
)