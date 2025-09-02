// +build !darwin,!freebsd,!openbsd

package c4m

import "syscall"

const (
	// SIGINFO doesn't exist on Linux and other non-BSD systems
	// We use SIGUSR1 as a fallback
	SIGINFO = syscall.SIGUSR1
)