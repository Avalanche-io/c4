package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// With no content store configured, every verb that needs one refuses,
// exit 1, naming how to configure one, prints nothing on stdout, and
// creates no store as a side effect. A composable verb never prompts
// (decided 2026-11-13 by the release lead; reversible). Through
// 2026-11-13 a terminal was asked whether to create ~/.c4/store, and
// `... | c4 -s` with no store printed the ID of stdin as if it were stored.
func TestNoStoreRefusesWithoutPromptingOrCreating(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"C4_STORE": "", "HOME": home, "USERPROFILE": home}
	tree := filepath.Join(dir, "t")
	writeTree(t, tree, map[string]string{"a.txt": "a"})
	desc := filepath.Join(dir, "t.c4m")
	described, stderr, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 0 {
		t.Fatalf("c4 id: exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(desc, []byte(described), 0o644); err != nil {
		t.Fatal(err)
	}

	type run struct {
		name  string
		stdin string
		args  []string
	}
	for _, r := range []run{
		{"c4 id -s DIR", "", []string{"id", "-s", tree}},
		{"c4 -s DIR", "", []string{"-s", tree}},
		{"stdin | c4 -s", "some bytes", []string{"-s"}},
		{"c4 restore", "", []string{"restore", desc, filepath.Join(dir, "dst")}},
	} {
		var out string
		if r.stdin != "" {
			out, stderr, code = runC4WithStdinEnv(t, bin, env, r.stdin, r.args...)
		} else {
			out, stderr, code = runC4WithEnv(t, bin, env, r.args...)
		}
		if code != 1 {
			t.Errorf("%s: want exit 1, got %d: %s", r.name, code, stderr)
		}
		if out != "" {
			t.Errorf("%s: nothing may print on stdout, got %q", r.name, out)
		}
		if !strings.Contains(stderr, "no content store is configured") || !strings.Contains(stderr, "C4_STORE=") {
			t.Errorf("%s: the refusal must say how to configure a store: %s", r.name, stderr)
		}
		if strings.Contains(stderr, "[Y/n]") {
			t.Errorf("%s: a verb never prompts: %s", r.name, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".c4")); !os.IsNotExist(err) {
		t.Errorf("no store or config may be created as a side effect: %v", err)
	}
}
