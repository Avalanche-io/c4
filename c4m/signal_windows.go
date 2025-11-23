// +build windows

package c4m

import "syscall"

const (
	// Windows doesn't have SIGUSR1 or SIGINFO
	// We define a dummy signal that won't be used
	// Progressive scanner will just use SIGINT on Windows
	SIGINFO = syscall.Signal(0)
)
