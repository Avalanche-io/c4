package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// escapeFixture builds a destination holding a stale symlink-to-directory at
// "sub", plus a real content source able to satisfy a create under it. The
// content source matters: without it a create fails for lack of bytes and a
// test can pass while proving nothing.
func escapeFixture(t *testing.T) (dest, outside string, src ContentSource, id c4.ID) {
	t.Helper()
	root := t.TempDir()
	dest = filepath.Join(root, "dest")
	outside = filepath.Join(root, "outside")
	srcDir := filepath.Join(root, "src")
	for _, d := range []string{dest, outside, srcDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	cid := writeFile(t, srcDir, "payload.txt", "ESCAPED")
	man := buildManifest(t, []testEntry{
		{name: "payload.txt", content: "ESCAPED", id: cid, mode: 0644},
	})
	if err := os.Symlink(outside, filepath.Join(dest, "sub")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return dest, outside, NewDirSource(man, srcDir), cid
}

// A symbolic link in the destination must never become a path a restore
// writes through. If the target records a directory at a name the
// destination holds as a symlink-to-directory, the reconciler must
// materialise a real directory there — never follow the link and write
// outside the tree it was told to reconcile.
//
// This is the archive-extraction traversal (zip-slip) in reconciler form,
// and it needs no special setup by an attacker: two ordinary snapshots
// restored in sequence into one directory produce it, so the whole path is
// reachable by supplying descriptions.
func TestApplyDoesNotWriteThroughSymlink(t *testing.T) {
	dest, outside, src, cid := escapeFixture(t)

	plan := &Plan{Operations: []Operation{
		{
			Type:  OpMkdir,
			Path:  filepath.Join(dest, "sub"),
			Entry: &c4m.Entry{Name: "sub", Mode: os.ModeDir | 0755},
		},
		{
			Type:      OpCreate,
			Path:      filepath.Join(dest, "sub", "payload.txt"),
			ContentID: cid,
			Entry:     &c4m.Entry{Name: "payload.txt", Mode: 0644, Size: 7},
		},
	}}

	_, _ = New(WithSource(src)).Apply(plan, dest)

	if _, err := os.Lstat(filepath.Join(outside, "payload.txt")); err == nil {
		t.Fatal("restore wrote through a symlink: payload.txt landed outside " +
			"the reconciled directory")
	}

	// The stale symlink must have become a real directory, or the reconcile
	// can never converge and restore will fail forever.
	fi, err := os.Lstat(filepath.Join(dest, "sub"))
	if err != nil {
		t.Fatalf("dest/sub missing after apply: %v", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dest/sub is still a symlink; the target records a directory")
	}
	if !fi.IsDir() {
		t.Fatal("dest/sub is not a directory")
	}
	if _, err := os.Lstat(filepath.Join(dest, "sub", "payload.txt")); err != nil {
		t.Fatalf("payload.txt was not written inside the destination: %v", err)
	}
}

// Defence in depth: with no mkdir op to clear the link first, a create under
// a symlinked parent must be refused outright rather than followed.
func TestApplyRefusesSymlinkedParent(t *testing.T) {
	dest, outside, src, cid := escapeFixture(t)

	plan := &Plan{Operations: []Operation{{
		Type:      OpCreate,
		Path:      filepath.Join(dest, "sub", "payload.txt"),
		ContentID: cid,
		Entry:     &c4m.Entry{Name: "payload.txt", Mode: 0644, Size: 7},
	}}}

	res, _ := New(WithSource(src)).Apply(plan, dest)

	if _, err := os.Lstat(filepath.Join(outside, "payload.txt")); err == nil {
		t.Fatal("a create traversed a symlinked parent and escaped the tree")
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected an error refusing to traverse the symlink, got none")
	}
}

// A restore that corrects a type mismatch must converge. The name was
// planned as both a create and a remove because the target keys a directory
// with a trailing slash while the destination keys a symlink without one;
// the remove then failed against the directory the same run had just built,
// and restore reported itself incomplete forever.
func TestPlanDoesNotBothCreateAndRemoveTheSameName(t *testing.T) {
	dest, _, _, cid := escapeFixture(t)

	target := buildManifest(t, []testEntry{
		{name: "sub/", mode: os.ModeDir | 0755},
		{name: "sub/payload.txt", content: "ESCAPED", id: cid, mode: 0644},
	})

	plan, err := New().Plan(target, dest)
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range plan.Operations {
		if (op.Type == OpRemove || op.Type == OpRmdir) &&
			filepath.Base(op.Path) == "sub" {
			t.Fatalf("planned %v on %s, a name the target records as a directory",
				op.Type, op.Path)
		}
	}
}
