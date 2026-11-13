package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4/c4m"
)

// In content mode every mode is null except a symlink's, written
// lrwxrwxrwx (decided 2026-11-13 by the release lead; reversible). A link
// whose target has the shape of a flow target, host:path, under a null mode
// reads back as a flow link: content mode used to write exactly that, so a
// tree holding such a link did not round-trip.
func TestContentModeKeepsASymlinksType(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": "", "HOME": home, "USERPROFILE": home}
	tree := filepath.Join(dir, "t")
	writeTree(t, tree, map[string]string{"a.txt": "a"})
	if err := os.Symlink("host:path", filepath.Join(tree, "remote")); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}

	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-m", "c", tree)
	if code != 0 {
		t.Fatalf("c4 id -m c: exit %d: %s", code, stderr)
	}
	resolved, stderr, code := runC4WithStdinEnv(t, bin, env, out, "patch", "-")
	if code != 0 {
		t.Fatalf("c4 patch -: exit %d: %s", code, stderr)
	}
	m, err := c4m.Unmarshal([]byte(resolved))
	if err != nil {
		t.Fatal(err)
	}
	var link, file *c4m.Entry
	for _, e := range m.Entries {
		switch e.Name {
		case "remote":
			link = e
		case "a.txt":
			file = e
		}
	}
	if link == nil || file == nil {
		t.Fatalf("both entries must be present:\n%s", resolved)
	}
	if link.Mode&os.ModeSymlink == 0 || link.Mode.Perm() != 0o777 {
		t.Errorf("the link's mode must read back as lrwxrwxrwx, got %v:\n%s", link.Mode, resolved)
	}
	if link.Target != "host:path" || link.FlowDirection != c4m.FlowNone {
		t.Errorf("the link must read back as a symlink to host:path, not a flow link: target %q, flow %v", link.Target, link.FlowDirection)
	}
	if file.Mode != 0 {
		t.Errorf("a file's mode stays null in content mode, got %v", file.Mode)
	}
	if !strings.Contains(resolved, "lrwxrwxrwx ") {
		t.Errorf("content mode must spell the link's mode lrwxrwxrwx:\n%s", resolved)
	}
}

// A content-level target holding a symlink restores and verifies: the
// link's lrwxrwxrwx is the content projection's mode, so the target is
// still read as content-level and verified at that level.
func TestRestoreOfAContentTargetWithASymlinkVerifies(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	tree := filepath.Join(dir, "t")
	writeTree(t, tree, map[string]string{"a.txt": "alpha"})
	if err := os.Symlink("a.txt", filepath.Join(tree, "link")); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	if _, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree); code != 0 {
		t.Fatalf("storing: exit %d: %s", code, stderr)
	}
	content, stderr, code := runC4WithEnv(t, bin, env, "id", "-m", "c", tree)
	if code != 0 {
		t.Fatalf("c4 id -m c: exit %d: %s", code, stderr)
	}
	desc := filepath.Join(dir, "content.c4m")
	if err := os.WriteFile(desc, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")
	if _, stderr, code := runC4WithEnv(t, bin, env, "restore", "--force", desc, dst); code != 0 {
		t.Fatalf("restore of a content target with a link: exit %d: %s", code, stderr)
	}
	if got, err := os.Readlink(filepath.Join(dst, "link")); err != nil || got != "a.txt" {
		t.Errorf("link: %q, %v", got, err)
	}
}
