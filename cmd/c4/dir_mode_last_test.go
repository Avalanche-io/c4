package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A read-only delivery folder (0555) holding a file and a subfolder round
// trips: scan it, restore the description into an empty folder, scan
// again, the same ID. Through 2026-11-20 the folder was made read-only
// before its children were written, and the restore failed: "permission
// denied". The tree's times are set before the scan, so nothing sits in
// the scan's racy window.
func TestRestoreRoundTripsAReadOnlyFolder(t *testing.T) {
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
	dest := filepath.Join(dir, "dest")
	if _, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", descFile, dest); code != 0 {
		t.Fatalf("restore: exit %d: %s", code, stderr)
	}
	want, _, _ := runC4WithEnv(t, bin, env, "id", "-q", descFile)
	got, _, _ := runC4WithEnv(t, bin, env, "id", "-q", dest)
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Errorf("restored folder scans as %s, want the description's %s", got, want)
	}
	info, err := os.Stat(filepath.Join(dest, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o555 {
		t.Errorf("ro: mode %v, want 0555", info.Mode().Perm())
	}
}
