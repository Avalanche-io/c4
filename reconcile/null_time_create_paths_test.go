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
// path, not only the local-copy one the c4 patch and restore tests reach.
// Each test checks its premise first, so a planner change cannot quietly
// move it onto another path.

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
	wantEpoch(t, filepath.Join(dest, "f.txt"))
}

// The create copied from a move's destination: one C4 ID wanted at two
// paths, where the folder held it once. The move keeps the file's own
// time; the copy is a new file, so its null timestamp becomes the epoch.
func TestApplyCreateCopiedFromAMoveFromANullTimestampGetsTheEpoch(t *testing.T) {
	dest := t.TempDir()
	id := writeFile(t, dest, "old.exr", "plate bytes")
	was := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dest, "old.exr"), was, was); err != nil {
		t.Fatal(err)
	}
	target := c4m.NewManifest()
	for _, n := range []string{"a.exr", "b.exr"} {
		target.AddEntry(&c4m.Entry{Name: n, Mode: 0o644, Timestamp: c4m.NullTimestamp(), Size: 11, C4ID: id, Depth: 0})
	}
	r := New()
	plan, err := r.Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	var moved, copied string
	for _, op := range plan.Operations {
		switch {
		case op.Type == OpMove:
			moved = op.Path
		case op.Type == OpCreate && op.SrcPath != "":
			copied = op.Path
		}
	}
	if moved == "" || copied == "" {
		t.Fatalf("premise: want one move and one create copied from it, planned:\n%v", plan.Operations)
	}
	if res, err := r.Apply(plan, dest); err != nil || len(res.Errors) > 0 {
		t.Fatalf("apply: %v %v", err, res.Errors)
	}
	wantEpoch(t, copied)
	info, err := os.Stat(moved)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Equal(was) {
		t.Errorf("%s moved under a null timestamp has mtime %v, want its own %v", moved, info.ModTime().UTC(), was)
	}
}

func wantEpoch(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if epoch := time.Unix(0, 0).UTC(); !info.ModTime().UTC().Equal(epoch) {
		t.Errorf("%s created from a null timestamp has mtime %v, want the epoch", filepath.Base(path), info.ModTime().UTC())
	}
}
