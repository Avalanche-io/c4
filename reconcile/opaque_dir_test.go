package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
)

// A target directory with no children listed under it is emptied only when
// its C4 ID is the empty listing's. A one-level record (each subdirectory
// one line carrying its own ID, as c4 1.1 stores a tree) used to empty every
// such subdirectory: `c4 patch record.c4m dir` removed their files with
// exit 0. Collapsed (another ID) and unread (no ID) directories are left as
// they are.
func TestPlanLeavesCollapsedAndUnreadDirectories(t *testing.T) {
	root := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("root.txt", "root")
	write("collapsed/child.txt", "child")
	write("collapsed/deep/leaf.txt", "leaf")
	write("unread/kept.txt", "kept")
	write("emptied/stray.txt", "stray")

	full, err := scan.NewGenerator().GenerateFromPath(root)
	if err != nil {
		t.Fatal(err)
	}
	// The one-level record: the root's own lines, each subdirectory as one
	// line with its own C4 ID. Then `unread/` loses its ID (nobody read it),
	// and `emptied/` is stated empty: the empty listing's ID, size 0.
	target := c4m.NewManifest()
	for _, e := range full.Entries {
		if e.Depth != 0 {
			continue
		}
		cp := *e
		switch e.Name {
		case "unread/":
			cp.C4ID, cp.Size = c4.ID{}, -1
		case "emptied/":
			cp.C4ID, cp.Size = emptyListingID, 0
		}
		target.AddEntry(&cp)
	}

	plan, err := New().Plan(target, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New().Apply(plan, root); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"root.txt", "collapsed/child.txt", "collapsed/deep/leaf.txt", "unread/kept.txt"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s was removed: a directory the target does not list must be left as it is", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "emptied", "stray.txt")); !os.IsNotExist(err) {
		t.Errorf("emptied/stray.txt survived: a directory stated empty (the empty listing's ID) must be emptied")
	}
}
