package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// TestEmptyDirectoryListingsThroughTheCLI runs a tree holding an empty
// directory at two depths and a folded range beside the deeper one through
// every CLI path that prints a directory's C4 ID, with -S and without: the
// streamed chain (its refinement layer restates each directory), its
// flattened form, the normaliser, and the store. Every directory's printed
// ID must be its own listing's, the normaliser must find nothing to
// correct, and every listing a -s run names must come back from the store.
// Through 2026-11-16, under -S, the directory above the deeper empty
// directory and everything above it named IDs that no listing had.
func TestEmptyDirectoryListingsThroughTheCLI(t *testing.T) {
	bin := buildC4(t)
	tree := filepath.Join(t.TempDir(), "p")
	writeTree(t, tree, map[string]string{
		"a/b/two.txt":    "two",
		"a/b/f.0001.exr": "frame 1",
		"a/b/f.0002.exr": "frame 2",
		"a/b/f.0003.exr": "frame 3",
		"a/c/x.txt":      "x",
	})
	for _, d := range []string{"a/b/empty", "top-empty"} {
		if err := os.MkdirAll(filepath.Join(tree, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	empty := c4.Identify(strings.NewReader(""))
	for _, flags := range [][]string{nil, {"-S"}} {
		dir := t.TempDir()
		home := filepath.Join(dir, "home")
		st := filepath.Join(dir, "store")
		for _, d := range []string{home, st} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		env := map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": st}
		run := func(args ...string) (string, int) {
			t.Helper()
			out, stderr, code := runC4WithEnv(t, bin, env, args...)
			if code != 0 && code != 1 {
				t.Fatalf("flags %v: c4 %v: exit %d: %s", flags, args, code, stderr)
			}
			return out, code
		}
		chain, code := run(append(append([]string{"id", "-s"}, flags...), tree)...)
		if code != 0 {
			t.Fatalf("flags %v: c4 id -s exit %d", flags, code)
		}
		printed := filepath.Join(dir, "printed.c4m")
		if err := os.WriteFile(printed, []byte(chain), 0o644); err != nil {
			t.Fatal(err)
		}
		flatText, code := run("patch", printed)
		if code != 0 {
			t.Fatalf("flags %v: c4 patch exit %d", flags, code)
		}
		flat, err := c4m.Unmarshal([]byte(flatText))
		if err != nil {
			t.Fatal(err)
		}
		ranges, dirs := 0, 0
		for path, e := range c4m.EntryPaths(flat.Entries) {
			if e.IsSequence {
				ranges++
			}
			if !e.IsDir() {
				continue
			}
			dirs++
			children := flat.Children(e)
			if _, listing := c4m.ListingRecord(children); e.C4ID != listing {
				t.Errorf("flags %v: %s is printed as %s, its listing is %s", flags, path, e.C4ID, listing)
			}
			if len(children) == 0 {
				if e.C4ID != empty || e.Size != 0 {
					t.Errorf("flags %v: the empty directory %s is size %d, ID %s", flags, path, e.Size, e.C4ID)
				}
				continue // storing the empty listing is the store pass's, not the scanner's
			}
			if _, code := run("cat", e.C4ID.String()); code != 0 {
				t.Errorf("flags %v: %s is printed as %s, and the store has no listing by that ID", flags, path, e.C4ID)
			}
		}
		if dirs != 5 || (len(flags) > 0) != (ranges > 0) {
			t.Fatalf("fixture: flags %v gave %d directories and %d ranges", flags, dirs, ranges)
		}
		flatFile := filepath.Join(dir, "flat.c4m")
		if err := os.WriteFile(flatFile, []byte(flatText), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, code := run("id", "-l", flatFile); code != 0 {
			t.Errorf("flags %v: the normaliser would correct the printed description: %s", flags, out)
		}
	}
}
