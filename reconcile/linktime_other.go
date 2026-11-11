//go:build !darwin && !linux
// +build !darwin,!linux

package reconcile

import (
	"errors"
	"time"
)

// linkTimeSupported reports whether setLinkTime can set a symlink's own
// time on this platform. Elsewhere (Windows included) the standard library
// offers no way to set a link's own time without following it: os.Chtimes
// follows the link and would change its target's time instead.
const linkTimeSupported = false

// ErrLinkTimeUnsupported reports that a symlink's own time cannot be set
// on this platform; the link keeps the time it was created with.
var errLinkTimeUnsupported = errors.New("this platform cannot set a symlink's own time")

func setLinkTime(path string, t time.Time) error { return errLinkTimeUnsupported }
