package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
)

// A read-only delivery folder (0555) holding a file and a subfolder round
// trips: scan it, c4 patch the description into an empty folder, scan
// again, the same description ID. Through 2026-11-20 the folder was made
// read-only before its children were written, and the patch failed:
// "permission denied". The tree's times are set before the scan, so nothing sits in
// the scan's racy window.
func TestPatchRoundTripsAReadOnlyFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a Windows folder has no owner-write bit to withhold")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	t.Cleanup(func() {
		filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				os.Chmod(p, 0o755)
			}
			return nil
		})
	})
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	if err := os.MkdirAll(env["C4_STORE"], 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"top.txt": "top\n", "ro/f.txt": "f\n", "ro/sub/g.txt": "g\n"})
	ts := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	for _, p := range []string{"top.txt", "ro/f.txt", "ro/sub/g.txt", "ro/sub", "ro", "."} {
		if err := os.Chtimes(filepath.Join(src, filepath.FromSlash(p)), ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(src, "ro"), 0o555); err != nil {
		t.Fatal(err)
	}

	desc, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", src)
	if code != 0 {
		t.Fatalf("c4 id -s: exit %d: %s", code, stderr)
	}
	if !strings.Contains(desc, "dr-xr-xr-x ") {
		t.Fatalf("the folder should scan as dr-xr-xr-x:\n%s", desc)
	}
	descFile := filepath.Join(dir, "desc.c4m")
	if err := os.WriteFile(descFile, []byte(desc), 0o644); err != nil {
		t.Fatal(err)
	}
	// A new target: it does not exist yet. The trailing separator makes
	// c4 patch read it as a folder (without one, a path that does not
	// exist is a c4m file to write).
	dest := filepath.Join(dir, "dest")
	if _, stderr, code = runC4WithEnv(t, bin, env, "patch", descFile, dest+string(filepath.Separator)); code != 0 {
		t.Fatalf("patch: exit %d: %s", code, stderr)
	}
	back, stderr, code := runC4WithEnv(t, bin, env, "id", dest)
	if code != 0 {
		t.Fatalf("c4 id of the patched folder: exit %d: %s", code, stderr)
	}
	// On this line c4 id -q prints nothing: description IDs from the library.
	id := func(text string) string {
		m, err := c4m.Unmarshal([]byte(text))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return m.ComputeC4ID().String()
	}
	if got, want := id(back), id(desc); got != want {
		t.Errorf("patched folder scans as %s, want the description's %s\n%s", got, want, back)
	}
	info, err := os.Stat(filepath.Join(dest, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Errorf("ro: mode %v, want 0555", info.Mode().Perm())
	}
}
