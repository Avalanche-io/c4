//go:build !windows
// +build !windows

package store

import "errors"

// errSharingViolation never comes from the filesystem here: a rename does
// not hold the file against an open. Tests inject it through the open seam
// (see moving).
var errSharingViolation = errors.New("sharing violation (simulated; Windows only)")
