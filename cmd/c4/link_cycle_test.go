package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `c4 id` on a real tree holding a symbolic link to an ancestor (`up -> ..`)
// or to its own directory (`here -> .`) crashed master with a stack
// overflow: release/1.0.x had the scanner's cycle guard (9092fb1) and master
// never took it. Each such link is recorded with its target and a nil ID,
// and the scan exits 0.
func TestIDOfTreeWithALinkCycle(t *testing.T) {
	bin := buildC4(t)
	for _, c := range []struct{ link, target string }{
		{"sub/up", ".."},
		{"here", "."},
	} {
		t.Run(c.target, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "sub", "f.txt"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(c.target, filepath.Join(root, c.link)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			out, errOut, code := runC4(t, bin, "id", root)
			if code != 0 {
				t.Fatalf("c4 id exited %d\nstderr: %.400s", code, errOut)
			}
			name := filepath.Base(c.link)
			want := name + " -> " + c.target + " -"
			if !strings.Contains(out, want) {
				t.Errorf("c4 id output lacks %q:\n%s", want, out)
			}
		})
	}
}
