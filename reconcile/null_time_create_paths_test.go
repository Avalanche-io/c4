package reconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/store"
)

// A file created from a null timestamp gets the epoch on every create
// path, not only the local-copy one the c4 patch tests reach. The test
// checks its premise first, so a change cannot quietly move it onto
// another path.

// The streamed path: content from a source with no local file (a RAM
// store, as an S3 store is), written through the durable writer.
func TestApplyStreamedCreateFromANullTimestampGetsTheEpoch(t *testing.T) {
	src := store.NewRAM()
	id, err := src.Put(strings.NewReader("streamed\n"))
	if err != nil {
		t.Fatal(err)
	}
	target := c4m.NewManifest()
	target.AddEntry(&c4m.Entry{Name: "f.txt", Mode: 0o644, Timestamp: c4m.NullTimestamp(), Size: 9, C4ID: id, Depth: 0})
	dest := t.TempDir()
	r := New(WithSource(src))
	if _, ok := r.localPath(id); ok {
		t.Fatal("premise: the RAM store offers a local file, so Apply would not stream")
	}
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := r.Apply(plan, dest); err != nil || len(res.Errors) > 0 {
		t.Fatalf("apply: %v %v", err, res.Errors)
	}
	info, err := os.Stat(filepath.Join(dest, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if epoch := time.Unix(0, 0).UTC(); !info.ModTime().UTC().Equal(epoch) {
		t.Errorf("f.txt created from a null timestamp has mtime %v, want the epoch", info.ModTime().UTC())
	}
}
