package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A symlink to a directory carries the directory's C4 ID (the ID of its
// record), and the record is stored by the directory's own entry. -s must
// not try to store the link's "content" by reading the link path as a
// file: through 1.0.21 that printed "Warning: failed to store l: copy:
// read d/l: is a directory" on every scan. (Reported from a c4m consumer,
// 2026-11-03.)
func TestStoreSkipsDirectorySymlinkContent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on windows")
	}
	bin := buildC4(t)
	root := t.TempDir()
	dir := filepath.Join(root, "d")
	if err := os.MkdirAll(filepath.Join(dir, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "f"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("x", filepath.Join(dir, "l")); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": filepath.Join(root, "home"), "C4_STORE": filepath.Join(root, "store")}
	os.MkdirAll(env["HOME"], 0o755)
	stdout, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", dir)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if strings.Contains(stderr, "Warning") {
		t.Fatalf("a warning on a tree with a directory symlink:\n%s", stderr)
	}
	if !strings.Contains(stdout, "l -> x") {
		t.Fatalf("the link is not listed:\n%s", stdout)
	}
	// The link's ID is the directory's, and `c4 cat` of it is the record.
	var linkID, dirID string
	for _, line := range strings.Split(stdout, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		switch {
		case f[3] == "l":
			linkID = f[len(f)-1]
		case f[3] == "x/" && len(f[len(f)-1]) == 90:
			dirID = f[len(f)-1]
		}
	}
	if linkID == "" || linkID != dirID {
		t.Fatalf("link ID %q, directory ID %q:\n%s", linkID, dirID, stdout)
	}
	rec, stderr, code := runC4WithEnv(t, bin, env, "cat", linkID)
	if code != 0 || !strings.Contains(rec, " f ") {
		t.Fatalf("cat of the link's ID is not the directory record (exit %d):\n%s%s", code, rec, stderr)
	}
}
