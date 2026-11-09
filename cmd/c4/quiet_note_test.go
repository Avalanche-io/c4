package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// On 1.0.x `-q` suppresses output; on 1.1 it prints the ID. A script
// moving between the lines got silence with exit 0 here (absfs,
// 2026-11-09). The silence now narrates itself once on stderr, and only
// when -s is absent, since -s already narrates the stored: line.
func TestQuietWithoutStoreNarratesOnStderr(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "alpha"})

	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-q", tree)
	if code != 0 || out != "" {
		t.Fatalf("-q must still print nothing and exit 0: exit %d, out %q", code, out)
	}
	if !strings.Contains(stderr, "prints nothing by design") || !strings.Contains(stderr, "stored: line") {
		t.Fatalf("-q without -s must say on stderr that it printed nothing, got %q", stderr)
	}

	out, stderr, code = runC4WithEnv(t, bin, env, "id", "-s", "-q", tree)
	if code != 0 || out != "" {
		t.Fatalf("-s -q: exit %d, out %q", code, out)
	}
	if strings.Contains(stderr, "prints nothing by design") || !strings.Contains(stderr, "stored: ") {
		t.Fatalf("-s -q narrates the stored: line and nothing about silence, got %q", stderr)
	}
}
