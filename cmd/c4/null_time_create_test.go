package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A tree whose files and directories sit at the Unix epoch scans as a
// null-dated description. Patching that description into an empty folder
// must create the files and folders at the epoch, so the folder scans back
// as the same description: the materialisation round-trips. The
// description is made by c4 id, never written by hand. (Master's rule,
// dfd6e12; its zero-mode half is not on this line.)
func TestPatchCreatesNullTimestampsAtTheEpoch(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{
		"a.txt":         "alpha",
		"sub/b.txt":     "beta",
		"sub/deep/c.go": "gamma",
	})
	epoch := time.Unix(0, 0).UTC()
	for _, p := range []string{"a.txt", "sub/b.txt", "sub/deep/c.go", "sub/deep", "sub", "."} {
		if err := os.Chtimes(filepath.Join(src, filepath.FromSlash(p)), epoch, epoch); err != nil {
			t.Fatal(err)
		}
	}

	want, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", src)
	if code != 0 {
		t.Fatalf("c4 id -s: exit %d: %s", code, stderr)
	}
	if !strings.Contains(want, " - ") || strings.Contains(want, "1970-") {
		t.Fatalf("the epoch should scan as a null timestamp:\n%s", want)
	}
	desc := filepath.Join(dir, "n.c4m")
	if err := os.WriteFile(desc, []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code = runC4WithEnv(t, bin, env, "patch", desc, dest); code != 0 {
		t.Fatalf("c4 patch: exit %d: %s", code, stderr)
	}
	for _, p := range []string{"a.txt", "sub/b.txt", "sub/deep/c.go", "sub/deep", "sub"} {
		info, err := os.Stat(filepath.Join(dest, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().UTC().Equal(epoch) {
			t.Errorf("%s created from a null timestamp has mtime %s, want the epoch", p, info.ModTime().UTC())
		}
	}
	got, _, code := runC4WithEnv(t, bin, env, "id", dest)
	if code != 0 {
		t.Fatal("c4 id of the patched folder failed")
	}
	if got != want {
		t.Errorf("patched folder scans as\n%s\nwant the description\n%s", got, want)
	}
}

// A null timestamp never overwrites the time of a path that already
// exists: only a path the apply creates gets the epoch.
func TestPatchNullTimestampKeepsAnExistingPathsTime(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"keep.txt": "same", "sub/new.txt": "new"})
	epoch := time.Unix(0, 0).UTC()
	for _, p := range []string{"keep.txt", "sub/new.txt", "sub", "."} {
		if err := os.Chtimes(filepath.Join(src, filepath.FromSlash(p)), epoch, epoch); err != nil {
			t.Fatal(err)
		}
	}
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", src)
	if code != 0 {
		t.Fatalf("c4 id -s: exit %d: %s", code, stderr)
	}
	desc := filepath.Join(dir, "n.c4m")
	if err := os.WriteFile(desc, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	writeTree(t, dest, map[string]string{"keep.txt": "same"})
	if err := os.MkdirAll(filepath.Join(dest, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	stated := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	for _, p := range []string{"keep.txt", "sub"} {
		if err := os.Chtimes(filepath.Join(dest, p), stated, stated); err != nil {
			t.Fatal(err)
		}
	}
	if _, stderr, code = runC4WithEnv(t, bin, env, "patch", desc, dest); code != 0 {
		t.Fatalf("c4 patch: exit %d: %s", code, stderr)
	}
	info, err := os.Stat(filepath.Join(dest, "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Equal(stated) {
		t.Errorf("existing keep.txt: mtime %s, want its own %s kept", info.ModTime().UTC(), stated)
	}
	// sub/ existed and gained a file. The description's null states no
	// time to set back, so writing the child moves sub/'s time as any
	// write does; the epoch is only for a folder the patch creates.
	info, err = os.Stat(filepath.Join(dest, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().UTC().Equal(epoch) {
		t.Errorf("existing sub/ was set to the epoch; a null never overwrites an existing path")
	}
	info, err = os.Stat(filepath.Join(dest, "sub", "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Equal(epoch) {
		t.Errorf("created sub/new.txt: mtime %s, want the epoch", info.ModTime().UTC())
	}
}

// Content the destination already holds under another name is moved, not
// created: it existed before, so a null timestamp keeps its own time.
func TestPatchNullTimestampKeepsAMovedFilesTime(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": storeDir, "HOME": filepath.Join(dir, "home")}

	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"moved.txt": "the same bytes"})
	epoch := time.Unix(0, 0).UTC()
	for _, p := range []string{"moved.txt", "."} {
		if err := os.Chtimes(filepath.Join(src, filepath.FromSlash(p)), epoch, epoch); err != nil {
			t.Fatal(err)
		}
	}
	out, stderr, code := runC4WithEnv(t, bin, env, "id", src)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	desc := filepath.Join(dir, "n.c4m")
	if err := os.WriteFile(desc, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "dest")
	writeTree(t, dest, map[string]string{"elsewhere.txt": "the same bytes"})
	stated := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(dest, "elsewhere.txt"), stated, stated); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code = runC4WithEnv(t, bin, env, "patch", desc, dest); code != 0 {
		t.Fatalf("c4 patch: exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dest, "elsewhere.txt")); !os.IsNotExist(err) {
		t.Fatalf("elsewhere.txt should have been moved to moved.txt (stat err %v)", err)
	}
	info, err := os.Stat(filepath.Join(dest, "moved.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().UTC().Equal(stated) {
		t.Errorf("moved.txt: mtime %s, want the moved file's own %s kept", info.ModTime().UTC(), stated)
	}
}
