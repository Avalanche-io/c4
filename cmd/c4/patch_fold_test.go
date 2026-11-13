package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A shape-B chain over a one-level base, through the CLI: block 1 adds
// d/z.txt, block 2 (naming block 1's byte ID) removes d/x.txt. The stored
// base lists d/ as one line; its children live in d/'s own listing.
// Through 2026-11-13 c4 patch folded the file's own sections without
// reading that listing, so `c4 patch block1.c4m` printed d/ holding z.txt
// alone under a line whose ID names x, y and z (exit 0); c4 #140 had fixed
// only the library fold. It also never checked a checkpoint, and dropped a
// first-line base that a checkpoint directly follows.
func TestPatchFoldsOneLevelBaseAndVerifiesCheckpoints(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": home, "USERPROFILE": home}
	if err := os.MkdirAll(env["C4_STORE"], 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, args...)
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}
	storeID := func(tree string) string {
		t.Helper()
		return strings.TrimSpace(run("id", "-s", "-q", tree))
	}
	write := func(name, text string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	resolve := func(file string) (string, string, int) {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, "patch", file)
		if code != 0 {
			return "", stderr, code
		}
		id, _, _ := runC4WithStdinEnv(t, bin, env, out, "id", "-q", "-")
		return strings.TrimSpace(id), stderr, code
	}
	afterFirstLine := func(s string) string {
		return s[strings.Index(s, "\n")+1:]
	}

	tree := filepath.Join(dir, "t")
	writeTree(t, tree, map[string]string{"d/x.txt": "x", "d/y.txt": "y"})
	v0 := storeID(tree)
	base0Path := write("base0.c4m", run("id", tree))

	writeTree(t, tree, map[string]string{"d/z.txt": "z"})
	v1 := storeID(tree)
	block1 := run("diff", v0, tree)
	b1, _, _ := runC4WithStdinEnv(t, bin, env, block1, "id", "-s")
	b1 = strings.TrimSpace(b1)
	block1Path := write("block1.c4m", block1)

	if err := os.Remove(filepath.Join(tree, "d", "x.txt")); err != nil {
		t.Fatal(err)
	}
	v2 := storeID(tree)
	tail := afterFirstLine(run("diff", v1, tree))
	block2Path := write("block2.c4m", b1+"\n"+tail)
	checkpointed := write("checkpointed.c4m", b1+"\n"+v1+"\n"+tail)
	wrong := v1[:len(v1)-1] + "X"
	if wrong == v1 {
		wrong = v1[:len(v1)-1] + "Y"
	}
	badCheckpoint := write("bad.c4m", b1+"\n"+wrong+"\n"+tail)

	for _, c := range []struct {
		name, file, want string
	}{
		{"block 1 deepens the base's collapsed d/", block1Path, v1},
		{"block 2 on block 1", block2Path, v2},
		{"a checkpoint directly after the first-line base", checkpointed, v2},
	} {
		got, stderr, code := resolve(c.file)
		if code != 0 || got != c.want {
			t.Errorf("%s: c4 patch resolves to %q (exit %d: %s), want %s", c.name, got, code, stderr, c.want)
		}
	}

	if _, stderr, code := resolve(badCheckpoint); code == 0 || !strings.Contains(stderr, "does not match") {
		t.Errorf("a wrong checkpoint after the base must be refused: exit %d: %s", code, stderr)
	}
	if _, stderr, code := resolve("../../c4m/testdata/chain-vector/bad-checkpoint.c4m"); code == 0 || !strings.Contains(stderr, "does not match") {
		t.Errorf("chain-vector/bad-checkpoint.c4m must be refused: exit %d: %s", code, stderr)
	}

	// A later file's first line is a boundary. It may name the state the
	// files before it resolve to (the checkpoint form c4 diff writes) or
	// the byte ID of the file before it (the block form); anything else is
	// refused, naming both (ruled 2026-11-13).
	concat := func(files ...string) (string, string, int) {
		t.Helper()
		out, stderr, code := runC4WithEnv(t, bin, env, append([]string{"patch"}, files...)...)
		if code != 0 {
			return "", stderr, code
		}
		id, _, _ := runC4WithStdinEnv(t, bin, env, out, "id", "-q", "-")
		return strings.TrimSpace(id), stderr, code
	}
	if got, stderr, code := concat(base0Path, block1Path); code != 0 || got != v1 {
		t.Errorf("checkpoint form (a scan, then a diff naming its state): %q exit %d: %s, want %s", got, code, stderr, v1)
	}
	if got, stderr, code := concat(block1Path, block2Path); code != 0 || got != v2 {
		t.Errorf("block form (block 1, then block 2 naming block 1's bytes): %q exit %d: %s, want %s", got, code, stderr, v2)
	}
	stray := write("stray.c4m", wrong+"\n"+tail)
	if _, stderr, code := concat(block1Path, stray); code == 0 || !strings.Contains(stderr, "neither the state") || !strings.Contains(stderr, b1) || !strings.Contains(stderr, v1) {
		t.Errorf("a boundary naming neither must be refused, naming both candidates: exit %d: %s", code, stderr)
	}

	// c4 log counts changes against the deepened layer: block 1 adds one
	// file, block 2 removes one.
	for _, c := range []struct{ file, want string }{
		{block1Path, "+1 -0 ~"},
		{block2Path, "+0 -1 ~"},
	} {
		out := run("log", c.file)
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 || !strings.Contains(lines[1], c.want) {
			t.Errorf("c4 log %s: want layer 1 to read %q:\n%s", filepath.Base(c.file), c.want, out)
		}
	}
}
