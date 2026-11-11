package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// A directory the scan listed and found no children in is empty by
// observation, in metadata mode as in full mode: size 0 and the empty
// listing's C4 ID, a known constant. Only a directory the scan could not
// read is unread (null size, null ID). Until 2026-11-11 a metadata scan
// left an observed empty directory with no ID, PropagateMetadata read that
// as unread, and the null size spread to every directory above it.
func TestMetadataScanObservedEmptyDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions and a non-root user")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"f": "hi\n", "locked/inner": "in\n", "a/z": "z\n"})
	for _, p := range []string{filepath.Join(tree, "empty"), filepath.Join(tree, "a", "b", "empty")} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(tree, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(tree, "locked"), 0o755) })

	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-m", "m", tree)
	if code != 0 && code != 2 {
		t.Fatalf("c4 id -m m exited %d: %s", code, stderr)
	}
	flat, stderr, code := runC4WithStdinEnv(t, bin, env, out, "patch", "-")
	if code != 0 {
		t.Fatalf("c4 patch - exited %d: %s", code, stderr)
	}
	m, err := c4m.Unmarshal([]byte(flat))
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, flat)
	}
	emptyID := c4.Identify(bytes.NewReader(nil))
	paths := c4m.EntryPaths(m.Entries)
	for _, want := range []string{"empty/", "a/b/empty/", "locked/", "a/", "a/b/"} {
		if paths[want] == nil {
			t.Fatalf("%s missing from the scan:\n%s", want, flat)
		}
	}
	for path, e := range paths {
		switch path {
		case "empty/", "a/b/empty/":
			if e.Size != 0 || e.C4ID != emptyID {
				t.Errorf("%s was listed with no children: want size 0 and the empty listing's ID, got size %d ID %s\n%s", path, e.Size, e.C4ID, flat)
			}
		case "locked/":
			if e.Size >= 0 || !e.C4ID.IsNil() {
				t.Errorf("locked/ could not be read: want null size and null ID, got size %d ID %s\n%s", e.Size, e.C4ID, flat)
			}
		case "a/", "a/b/":
			if e.Size < 0 {
				t.Errorf("%s holds only observed entries: its size must be known, got null\n%s", path, flat)
			}
		}
	}
}
