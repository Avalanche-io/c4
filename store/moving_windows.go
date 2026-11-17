//go:build windows
// +build windows

package store

import "syscall"

// errSharingViolation is ERROR_SHARING_VIOLATION. An open meets it while
// another process's rename holds the file: MoveFileEx opens the file with
// delete access, and os.Open does not share delete (see moving).
var errSharingViolation error = syscall.Errno(32)
