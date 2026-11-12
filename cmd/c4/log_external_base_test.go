package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// c4 log on a c4m whose first line names an external base lists that base as
// layer 0, resolved from the store, and the file's own sections from 1 as
// patches against it. Before, it presented the block's own entries as the
// base state ("1  <id>  (base)  1 files") with the base ignored.
func TestLogFollowsExternalBase(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}

	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"first.txt": "first", "sub/inner": "in"})
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 {
		t.Fatalf("storing the base: exit %d: %s", code, stderr)
	}
	baseID := strings.TrimSpace(out)

	secondID, _, code := runC4WithStdinEnv(t, bin, env, "second", "id")
	if code != 0 {
		t.Fatalf("c4 id < second: exit %d", code)
	}
	block := baseID + "\n" +
		"-rw-r--r-- 2026-01-01T00:00:00Z 6 second.txt " + strings.TrimSpace(secondID) + "\n"
	blockPath := filepath.Join(dir, "block.c4m")
	if err := os.WriteFile(blockPath, []byte(block), 0o644); err != nil {
		t.Fatal(err)
	}

	logOut, stderr, code := runC4WithEnv(t, bin, env, "log", blockPath)
	if code != 0 {
		t.Fatalf("c4 log: exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(logOut), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two layers (the external base and the block's section):\n%s", logOut)
	}
	if !strings.HasPrefix(lines[0], "0  ") || !strings.Contains(lines[0], "(external base "+baseID+")") {
		t.Errorf("layer 0 must be the external base, named:\n%s", lines[0])
	}
	if strings.Contains(lines[1], "(base)") || !strings.HasPrefix(lines[1], "1  ") || !strings.Contains(lines[1], "+1 -0 ~0") {
		t.Errorf("the block's section is a patch on the base, not a base state:\n%s", lines[1])
	}

	// Without the base in the store, log refuses, naming the base.
	other := map[string]string{"C4_STORE": filepath.Join(dir, "empty-store"), "HOME": home, "USERPROFILE": home}
	if err := os.MkdirAll(other["C4_STORE"], 0o755); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runC4WithEnv(t, bin, other, "log", blockPath)
	if code == 0 || !strings.Contains(stderr, baseID) {
		t.Errorf("an unreadable base must fail, naming %s: exit %d: %s", baseID, code, stderr)
	}
}
