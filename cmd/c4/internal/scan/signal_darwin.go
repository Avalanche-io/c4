// +build darwin freebsd openbsd

package scan

import "syscall"

const (
	// SIGINFO is available on BSD-based systems (macOS, FreeBSD, OpenBSD)
	// It's triggered by Ctrl+T and is specifically meant for status information
	SIGINFO = syscall.Signal(29)
)