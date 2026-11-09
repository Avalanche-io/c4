package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// A directory the scanner cannot open is recorded with what the walk
// observed: its mode (d---------, known), its time, and null for size and
// ID. Until 2026-11-09 PropagateMetadata filled the size with 0, which
// under the size rule is a known empty: a fact nobody had. A file at
// chmod 000 records its known zero mode and its stat size with a null ID.
func TestUnreadDirectoryKeepsNullSize(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix permissions and a non-root user")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"f": "hi\n", "d/inner": "in\n"})
	if err := os.MkdirAll(filepath.Join(tree, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(tree, "f"), filepath.Join(tree, "d")} {
		if err := os.Chmod(p, 0); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		os.Chmod(filepath.Join(tree, "d"), 0o755)
		os.Chmod(filepath.Join(tree, "f"), 0o644)
	})

	out, stderr, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 2 {
		t.Fatalf("a scan with unreadable entries is partial, exit 2; got %d: %s", code, stderr)
	}
	m, err := c4m.Unmarshal([]byte(out))
	if err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	byName := map[string]*c4m.Entry{}
	for _, e := range m.Entries {
		byName[e.Name] = e
	}
	d := byName["d/"]
	if d == nil || !d.IsDir() || d.Mode.Perm() != 0 || d.Mode == 0 {
		t.Fatalf("d/ must record a known d--------- mode:\n%s", out)
	}
	if d.Size >= 0 || !d.C4ID.IsNil() {
		t.Fatalf("an unread directory has null size and null ID, got size %d:\n%s", d.Size, out)
	}
	f := byName["f"]
	if f == nil || f.Mode != c4m.ZeroMode || f.Size != 3 || !f.C4ID.IsNil() {
		t.Fatalf("f must be the known zero mode, size 3, null ID:\n%s", out)
	}
	e := byName["emptydir/"]
	if e == nil || e.Size != 0 || e.C4ID.IsNil() {
		t.Fatalf("an empty directory is size 0 with the empty-content ID:\n%s", out)
	}
	if !strings.Contains(stderr, "partial: d/") || !strings.Contains(stderr, "partial: f") {
		t.Fatalf("both unreadable entries must be named on stderr: %s", stderr)
	}
}
