package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// idListLines returns the inline ID-list lines of a c4m text: longer than
// one C4 ID, a multiple of 90 characters, starting c4, with no space.
func idListLines(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if len(l) > 90 && len(l)%90 == 0 && strings.HasPrefix(l, "c4") && !strings.Contains(l, " ") {
			out = append(out, l)
		}
	}
	return out
}

// A folded range's ID list is linked by ID from its line, as a
// subdirectory's listing is from its directory line, and it is stored
// beside the listings wherever they are stored (ruled 2026-11-13 by the
// maintainer: "Everything that you would expect of the creation and
// retention of that subdirectory c4m file you would also expect of the
// frame id list."). Through 2026-11-13 `c4 id -s FILE.c4m` stored the
// listings and not the lists, and `c4 id -S DIR` without -s wrote its
// lists nowhere.
func TestRangeListsStoredBesideListings(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	envFor := func(name string) (map[string]string, string) {
		home := filepath.Join(dir, "home-"+name)
		st := filepath.Join(dir, "store-"+name)
		for _, d := range []string{home, st} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": st}, st
	}
	mustRun := func(env map[string]string, args ...string) string {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, args...)
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}
	stored := func(what string, env map[string]string, lists []string) {
		t.Helper()
		if len(lists) == 0 {
			t.Fatalf("%s: no ID lists to look for", what)
		}
		for _, l := range lists {
			id := c4.Identify(strings.NewReader(l)).String()
			if got, _, code := runC4WithEnv(t, bin, env, "cat", id); code != 0 || got != l {
				t.Errorf("%s: the ID list %s is not in the store (cat exit %d)", what, id, code)
			}
		}
	}

	folded := filepath.Join("..", "..", "c4m", "testdata", "range-data-vector", "folded.c4m")
	data, err := os.ReadFile(folded)
	if err != nil {
		t.Fatal(err)
	}
	lists := idListLines(string(data))

	// c4 id -s FILE.c4m, and the same description on stdin.
	env, _ := envFor("file")
	out := mustRun(env, "id", "-s", folded)
	stored("c4 id -s folded.c4m", env, lists)
	if got := idListLines(out); len(got) != len(lists) {
		t.Errorf("c4 id -s folded.c4m writes %d inline lists, want %d", len(got), len(lists))
	}
	env, _ = envFor("stdin")
	if _, stderr, code := runC4WithStdinEnv(t, bin, env, string(data), "id", "-s", "-"); code != 0 {
		t.Fatalf("c4 id -s -: exit %d: %s", code, stderr)
	}
	stored("c4 id -s - < folded.c4m", env, lists)

	// A scan: the lists inline with or without -s, as nested listings
	// are, and stored with -s.
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"readme.txt": "readme", "sub/f.001.exr": "frame 1", "sub/f.002.exr": "frame 2", "sub/f.003.exr": "frame 3"})
	env, _ = envFor("scan")
	plain := idListLines(mustRun(env, "id", "-S", tree))
	if len(plain) != 1 {
		t.Fatalf("c4 id -S DIR writes %d inline lists, want 1", len(plain))
	}
	if got := idListLines(mustRun(env, "id", "-S", "-e", tree)); len(got) != 1 {
		t.Errorf("c4 id -S -e DIR writes %d inline lists, want 1", len(got))
	}
	env, _ = envFor("snap")
	if got := idListLines(mustRun(env, "id", "-S", "-s", tree)); len(got) != 1 || got[0] != plain[0] {
		t.Errorf("c4 id -S -s DIR writes inline lists %v, want %v", got, plain)
	}
	stored("c4 id -S -s DIR", env, plain)
}
