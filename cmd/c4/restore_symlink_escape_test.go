package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `c4 restore --force` into a destination holding a symbolic link to a
// directory outside it (`dest/sub -> ../outside`) wrote the snapshot's
// `sub/payload.txt` through the link into `outside/`, then failed its own
// verification with exit 3. Release 1.0.x has refused that since 56c4416;
// master never took it. The link is stale (the snapshot records a directory
// at that name), so it is replaced by the directory, and nothing is written
// outside the destination.
func TestRestoreDoesNotWriteThroughADestinationSymlink(t *testing.T) {
	bin := buildC4(t)
	work := t.TempDir()
	src := filepath.Join(work, "src")
	dest := filepath.Join(work, "dest")
	outside := filepath.Join(work, "outside")
	for _, d := range []string{filepath.Join(src, "sub"), dest, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "payload.txt"), []byte("ESCAPED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../outside", filepath.Join(dest, "sub")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	env := map[string]string{"HOME": filepath.Join(work, "home"), "C4_STORE": filepath.Join(work, "store")}
	out, errOut, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", src)
	if code != 0 {
		t.Fatalf("c4 id -s exited %d: %s", code, errOut)
	}
	snap := strings.TrimSpace(out)

	_, errOut, code = runC4WithEnv(t, bin, env, "restore", "--force", snap, dest)
	if code != 0 {
		t.Errorf("c4 restore --force exited %d: %.400s", code, errOut)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("restore wrote outside the destination: %v", entries)
	}
	fi, err := os.Lstat(filepath.Join(dest, "sub"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("dest/sub should be a real directory now: %v %v", fi, err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "sub", "payload.txt")); err != nil || string(b) != "ESCAPED\n" {
		t.Errorf("dest/sub/payload.txt = %q, %v", b, err)
	}
}
