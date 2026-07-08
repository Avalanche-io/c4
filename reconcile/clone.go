package reconcile

import "errors"

// errNoClone reports that the platform or filesystem cannot clone files.
var errNoClone = errors.New("clone unsupported")

// cloneFile attempts a copy-on-write clone of src to dst, returning
// errNoClone when unavailable so the caller falls back to a byte copy.
// An implementation must produce dst atomically: clone to a temp name in
// dst's directory, then rename. No stdlib clone primitive exists on
// darwin; adopting golang.org/x/sys (unix.Clonefile) would enable it —
// see design/reconcile-performance.md. On Linux, byte copies already use
// copy_file_range via DurableWriter.ReadFrom, which reflinks on CoW
// filesystems.
func cloneFile(src, dst string) error {
	return errNoClone
}
