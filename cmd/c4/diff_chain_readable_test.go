package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A chain grown with this line's own `c4 diff FILE.c4m DIR >> FILE.c4m`
// reads back. Through 1.0.26 c4 diff ended its output with the new state's
// bare C4 ID, which this line's decoder refuses ("empty patch section"), so
// 1.0.x could not read the chains its own diff wrote. A c4m file never ends
// with a bare C4 ID (a format error, ruled 2026-11-08).
func TestDiffAppendedChainReadsBack(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": "", "HOME": home, "USERPROFILE": home}

	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "a\n"})
	chain := filepath.Join(dir, "chain.c4m")
	base, stderr, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 0 {
		t.Fatalf("c4 id tree: exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(chain, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	appendDiff := func() {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, "diff", chain, tree)
		if code != 0 {
			t.Fatalf("c4 diff chain.c4m tree: exit %d: %s", code, stderr)
		}
		f, err := os.OpenFile(chain, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(out)
		f.Close()
	}

	writeTree(t, tree, map[string]string{"b.txt": "b\n"})
	appendDiff()
	writeTree(t, tree, map[string]string{"c.txt": "c\n"})
	appendDiff()

	resolved, stderr, code := runC4WithEnv(t, bin, env, "patch", chain)
	if code != 0 {
		raw, _ := os.ReadFile(chain)
		t.Fatalf("c4 patch of a chain grown by c4 diff >>: exit %d: %s\n%s", code, stderr, raw)
	}
	want, _, _ := runC4WithEnv(t, bin, env, "id", tree)
	if resolved != want {
		t.Errorf("the chain resolves to\n%s\nwant the tree's own description\n%s", resolved, want)
	}
	raw, _ := os.ReadFile(chain)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if last := lines[len(lines)-1]; strings.HasPrefix(last, "c4") && len(last) == 90 {
		t.Errorf("the chain ends with a bare C4 ID:\n%s", raw)
	}
}
