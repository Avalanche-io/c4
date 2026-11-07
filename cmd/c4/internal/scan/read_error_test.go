package scan

import (
	"runtime"
	"testing"
)

// A file whose read fails (a directory opened as a file, EISDIR; a
// non-local file on a mount, EIO) gets no ID from the scanner's hashing
// step and the error is returned, so the entry is recorded with a null
// ID and the scan is partial. Never the ID of the bytes read so far.
func TestComputeFileC4IDReturnsTheReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("opening a directory as a file fails at open on Windows")
	}
	g := NewGeneratorWithOptions(WithMode(ModeFull))
	id, err := g.computeFileC4ID(t.TempDir())
	if err == nil {
		t.Fatalf("a failed read returned no error (id %s)", id)
	}
	if !id.IsNil() {
		t.Fatalf("a failed read returned an ID: %s", id)
	}
}
