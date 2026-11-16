package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// A stored description resolves from the store alone: every subdirectory's
// listing and every folded range's ID list, at any depth, comes back by its
// C4 ID. The maintainer: "it would be an error to create a c4m tree and
// drop the subdirectories". Through 1.0.26 `c4 id -s FILE.c4m` stored the
// file's text and its root record only, so no nested listing could be
// fetched by its ID; `c4 id -s DIR` stored them all.
func TestStoredTreeResolvesFromStoreAlone(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{
		"top.txt":           "top",
		"a/one.txt":         "one",
		"a/b/two.txt":       "two",
		"a/b/c/three.txt":   "three",
		"a/b/c/f.001.exr":   "frame 1",
		"a/b/c/f.002.exr":   "frame 2",
		"a/b/c/f.003.exr":   "frame 3",
		"d/four.txt":        "four",
		"seq/s.0001.exr":    "shot 1",
		"seq/s.0002.exr":    "shot 2",
		"seq/s.0003.exr":    "shot 3",
		"seq/s.0004.exr":    "shot 4",
		"seq/notes/n.txt":   "notes",
		"empty-sibling.txt": "",
	})
	envFor := func(name string) map[string]string {
		home := filepath.Join(dir, "home-"+name)
		st := filepath.Join(dir, "store-"+name)
		for _, d := range []string{home, st} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": st}
	}
	run := func(env map[string]string, args ...string) string {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, args...)
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}

	// The description, made without a store.
	text := run(envFor("gen"), "id", "-S", tree)
	desc, err := c4m.Unmarshal([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "tree.c4m")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		args []string
	}{
		{"c4 id -s FILE.c4m", []string{"id", "-s", file}},
		{"c4 id -S -s DIR", []string{"id", "-S", "-s", tree}},
	} {
		env := envFor(strings.NewReplacer(" ", "-", ".", "").Replace(c.name))
		run(env, c.args...)

		// Resolve the tree from the store alone, from the root's ID down.
		fetch := func(id c4.ID) (string, bool) {
			out, _, code := runC4WithEnv(t, bin, env, "cat", id.String())
			if code != 0 || c4.Identify(strings.NewReader(out)) != id {
				return "", false
			}
			return out, true
		}
		got := map[string]c4.ID{}
		lists := map[c4.ID]bool{}
		var walk func(prefix string, id c4.ID)
		walk = func(prefix string, id c4.ID) {
			listing, ok := fetch(id)
			if !ok {
				t.Errorf("%s: the listing of %q (%s) is not in the store", c.name, prefix, id.String()[:12])
				return
			}
			m, err := c4m.Unmarshal([]byte(listing))
			if err != nil {
				t.Fatalf("%s: listing of %q: %v", c.name, prefix, err)
			}
			for _, e := range m.Entries {
				p := prefix + e.Name
				got[p] = e.C4ID
				switch {
				case e.IsDir():
					walk(p, e.C4ID)
				case e.IsSequence:
					list, ok := fetch(e.C4ID)
					if !ok || len(list)%90 != 0 || len(list) == 0 {
						t.Errorf("%s: the ID list of %s is not in the store", c.name, p)
						continue
					}
					lists[e.C4ID] = true
				}
			}
		}
		walk("", desc.ComputeC4ID())

		want := c4m.EntryPaths(desc.Entries)
		for p, e := range want {
			if got[p] != e.C4ID {
				t.Errorf("%s: %s does not resolve from the store", c.name, p)
			}
		}
		if len(got) != len(want) {
			t.Errorf("%s: the store resolves %d entries, the description holds %d", c.name, len(got), len(want))
		}
		for id := range desc.RangeData {
			if !lists[id] {
				t.Errorf("%s: range list %s not reached from the store", c.name, id.String()[:12])
			}
		}
		if len(desc.RangeData) != 2 {
			t.Fatalf("fixture: %d range lists, want 2", len(desc.RangeData))
		}
	}
}
