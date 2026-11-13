package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
)

// TestRangeDataVectorCLI runs c4 merge, diff and patch over
// c4m/testdata/range-data-vector and checks each output against the file
// its README names. Through 2026-11-13 every one of them dropped the
// inputs' inline ID lists (design/range-data-through-operations.md).
func TestRangeDataVectorCLI(t *testing.T) {
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "range-data-vector")
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	file := func(name string) string { return filepath.Join(vec, name) }
	want := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(file(name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"merge", file("folded.c4m"), file("plain.c4m")}, "merged.c4m"},
		{[]string{"merge", file("plain.c4m"), file("folded.c4m")}, "merged.c4m"},
		{[]string{"merge", file("folded.c4m"), file("folded.c4m")}, "folded.c4m"},
		{[]string{"diff", file("plain.c4m"), file("folded.c4m")}, "add.c4m"},
		{[]string{"diff", file("folded.c4m"), file("no-plate.c4m")}, "remove.c4m"},
		{[]string{"patch", file("add-chain.c4m")}, "folded.c4m"},
		{[]string{"patch", file("plain.c4m"), file("add.c4m")}, "folded.c4m"},
		{[]string{"patch", file("remove-chain.c4m")}, "no-plate.c4m"},
		{[]string{"patch", file("folded.c4m")}, "folded.c4m"},
	}
	for _, c := range cases {
		out, stderr, code := runC4WithEnv(t, bin, env, c.args...)
		if code != 0 {
			t.Errorf("c4 %v: exit %d: %s", c.args[0], code, stderr)
			continue
		}
		if out != want(c.want) {
			t.Errorf("c4 %s %s %s writes\n%s\nwant %s:\n%s", c.args[0], filepath.Base(c.args[1]), filepath.Base(c.args[len(c.args)-1]), out, c.want, want(c.want))
		}
	}

	// split keeps each section's lists: the part before the split is
	// plain.c4m, and the part after is add.c4m, its lists included.
	before, after := filepath.Join(dir, "before.c4m"), filepath.Join(dir, "after.c4m")
	if _, stderr, code := runC4WithEnv(t, bin, env, "split", file("add-chain.c4m"), "1", before, after); code != 0 {
		t.Fatalf("c4 split: exit %d: %s", code, stderr)
	}
	for path, name := range map[string]string{before: "plain.c4m", after: "add.c4m"} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want(name) {
			t.Errorf("c4 split add-chain.c4m 1: %s is\n%s\nwant %s", filepath.Base(path), got, name)
		}
	}

	// A first-line base that names a stored block: its lists come from the
	// block's own bytes. folded.c4m is stored as itself (a .c4m inside a
	// scanned tree keeps its bytes), and remove.c4m's patch is applied to
	// it by that block's byte ID.
	tree := filepath.Join(dir, "tree")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	foldedBytes := []byte(want("folded.c4m"))
	if err := os.WriteFile(filepath.Join(tree, "folded.c4m"), foldedBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", tree); code != 0 {
		t.Fatalf("c4 id -s: exit %d: %s", code, stderr)
	}
	remove := want("remove.c4m")
	block := c4.Identify(bytes.NewReader(foldedBytes)).String() + "\n" + remove[strings.Index(remove, "\n")+1:]
	blockPath := filepath.Join(dir, "block.c4m")
	if err := os.WriteFile(blockPath, []byte(block), 0o644); err != nil {
		t.Fatal(err)
	}
	out, stderr, code := runC4WithEnv(t, bin, env, "patch", blockPath)
	if code != 0 {
		t.Fatalf("c4 patch block.c4m: exit %d: %s", code, stderr)
	}
	if out != want("no-plate.c4m") {
		t.Errorf("c4 patch on a stored folded.c4m block writes\n%s\nwant no-plate.c4m:\n%s", out, want("no-plate.c4m"))
	}
}
