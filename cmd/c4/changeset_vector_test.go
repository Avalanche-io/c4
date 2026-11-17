package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestChangesetVectorCLI runs the changeset vector's rows through the CLI:
// `c4 diff base.c4m v2.c4m` writes changeset.c4m byte for byte, and `c4 patch`
// resolves the chain to v2.c4m (c4m/testdata/changeset-vector/README.md).
// On this line `c4 patch <target> <dest>` makes dest match target, so
// master's pair row (the changeset over base) has no counterpart here.
func TestChangesetVectorCLI(t *testing.T) {
	bin := buildC4(t)
	vec := filepath.Join("..", "..", "c4m", "testdata", "changeset-vector")
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
		{[]string{"diff", file("base.c4m"), file("v2.c4m")}, "changeset.c4m"},
		{[]string{"patch", file("chain.c4m")}, "v2.c4m"},
		{[]string{"diff", file("v2.c4m"), file("v3.c4m")}, "changeset-v3.c4m"},
		{[]string{"patch", file("chain-v3.c4m")}, "v3.c4m"},
	}
	for _, c := range cases {
		out, stderr, code := runC4WithEnv(t, bin, env, c.args...)
		if code != 0 {
			t.Errorf("c4 %v: exit %d: %s", c.args, code, stderr)
			continue
		}
		if out != want(c.want) {
			t.Errorf("c4 %s %s writes\n%s\nwant %s:\n%s", c.args[0], filepath.Base(c.args[len(c.args)-1]), out, c.want, want(c.want))
		}
	}
}
