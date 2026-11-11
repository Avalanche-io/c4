package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Avalanche-io/c4"
	"github.com/Avalanche-io/c4/c4m"
)

// No file contains its own ID. For every form the CLI writes to a file,
// storing the file's bytes and asking for them back by that ID returns
// exactly the file; and no form ends with a bare ID of its own state.
func TestEveryWrittenFormIsTheFileItsIDNames(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}

	refined := filepath.Join(dir, "refined")
	writeTree(t, refined, map[string]string{"a.txt": "alpha", "d/b.txt": "beta"})
	flat := filepath.Join(dir, "flat")
	writeTree(t, flat, map[string]string{"a.txt": "alpha", "b.txt": "beta"})

	write := func(name string, args ...string) string {
		stdout, stderr, code := runC4WithEnv(t, bin, env, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, stderr)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(stdout), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	chain := write("refined.c4m", "id", refined)
	flatFile := write("flat.c4m", "id", flat)
	pretty := write("refined-e.c4m", "id", "-e", refined)
	normalised := write("normalised.c4m", "id", chain)

	// stdin parsed as a description, written again
	stdin, stderr, code := runC4StdinWithEnv(t, bin, env, mustRead(t, chain), "id", "-")
	if code != 0 {
		t.Fatalf("c4 id -: exit %d: %s", code, stderr)
	}
	fromStdin := filepath.Join(dir, "from-stdin.c4m")
	if err := os.WriteFile(fromStdin, []byte(stdin), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{chain, flatFile, pretty, normalised, fromStdin} {
		data := mustRead(t, p)
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if isBareID(lines[len(lines)-1]) {
			t.Errorf("%s ends with a bare ID: %q", filepath.Base(p), lines[len(lines)-1])
		}
		// store the bytes, get them back by their ID
		stdout, stderr, code := runC4StdinWithEnv(t, bin, env, data, "id", "-s")
		if code != 0 {
			t.Fatalf("c4 id -s < %s: exit %d: %s", p, code, stderr)
		}
		id := strings.TrimSpace(stdout)
		if want := c4.Identify(strings.NewReader(string(data))).String(); id != want {
			t.Errorf("%s: c4 id -s < file printed %s, the bytes' ID is %s", filepath.Base(p), id, want)
		}
		got, stderr, code := runC4WithEnv(t, bin, env, "cat", id)
		if code != 0 {
			t.Fatalf("c4 cat %s: exit %d: %s", id, code, stderr)
		}
		if got != string(data) {
			t.Errorf("%s: c4 cat of its ID returned %d bytes, the file is %d", filepath.Base(p), len(got), len(data))
		}
	}

	// bare IDs: none in a flat tree's stream, exactly one, before the
	// patch, in a refined tree's
	count := func(p string) (n int, last bool) {
		lines := strings.Split(strings.TrimRight(string(mustRead(t, p)), "\n"), "\n")
		for i, l := range lines {
			if isBareID(l) {
				n++
				last = i == len(lines)-1
			}
		}
		return
	}
	if n, _ := count(flatFile); n != 0 {
		t.Errorf("a flat tree's stream has %d bare IDs, want 0", n)
	}
	if n, last := count(chain); n != 1 || last {
		t.Errorf("a refined tree's stream has %d bare IDs (last line: %v), want exactly 1 before the patch", n, last)
	}
}

// A c4m file inside a scanned tree is stored as its bytes and comes back
// byte for byte, from the store and from restore: aligned and chained,
// with and without a .c4m name. On master before this change the entry
// was stored re-rendered and restore wrote a different file (c4-fleet,
// 2026-11-03).
func TestC4mFilesInsideATreeKeepTheirBytes(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}

	// something to describe, then the descriptions as files in a project
	src := filepath.Join(dir, "src")
	writeTree(t, src, map[string]string{"x.txt": "xx", "sub/y.txt": "yyy"})
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-e", src)
	if code != 0 {
		t.Fatal("c4 id -e failed")
	}
	aligned := stdout
	stdout, _, code = runC4WithEnv(t, bin, env, "id", src)
	if code != 0 {
		t.Fatal("c4 id failed")
	}
	chain := stdout

	proj := filepath.Join(dir, "proj")
	files := map[string]string{
		"notes.c4m": aligned,
		"notes.txt": aligned,
		"chain.c4m": chain,
		"chain.txt": chain,
		"readme.md": "a project",
	}
	writeTree(t, proj, files)

	_, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-q", proj)
	if code != 0 {
		t.Fatalf("c4 id -s -q: exit %d: %s", code, stderr)
	}
	stdout, _, code = runC4WithEnv(t, bin, env, "id", proj)
	if code != 0 {
		t.Fatal("c4 id proj failed")
	}
	desc := filepath.Join(dir, "proj.c4m")
	if err := os.WriteFile(desc, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := c4m.Unmarshal([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	paths := c4m.EntryPaths(m.Entries)
	for name, body := range files {
		e := paths[name]
		if e == nil {
			t.Fatalf("no entry for %s", name)
		}
		if want := c4.Identify(strings.NewReader(body)); e.C4ID != want {
			t.Errorf("%s: entry ID %s is not the bytes' ID %s", name, e.C4ID, want)
		}
		got, stderr, code := runC4WithEnv(t, bin, env, "cat", e.C4ID.String())
		if code != 0 {
			t.Errorf("%s: c4 cat: exit %d: %s", name, code, stderr)
		} else if got != body {
			t.Errorf("%s: c4 cat returned %d bytes, the file is %d", name, len(got), len(body))
		}
	}

	out := filepath.Join(dir, "out")
	if _, stderr, code = runC4WithEnv(t, bin, env, "restore", "--force", desc, out); code != 0 {
		t.Fatalf("restore: exit %d: %s", code, stderr)
	}
	for name, body := range files {
		got := mustRead(t, filepath.Join(out, name))
		if string(got) != body {
			t.Errorf("%s restored as %d bytes, the file is %d", name, len(got), len(body))
		}
	}
}

// The two pipes, and which ID each returns: `c4 id DIR | c4 id -q -` is
// the description's ID (the same as `c4 id -q DIR`); `c4 id DIR | c4` is
// the ID of exactly the bytes piped, which summons that exact text.
func TestPipedDescriptionIDAndPipedBytesID(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "d/b.txt": "beta"})

	stream, _, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 0 {
		t.Fatal("c4 id failed")
	}
	stdout, _, code := runC4WithEnv(t, bin, env, "id", "-q", tree)
	if code != 0 {
		t.Fatal("c4 id -q failed")
	}
	descID := strings.TrimSpace(stdout)

	stdout, stderr, code := runC4StdinWithEnv(t, bin, env, []byte(stream), "id", "-q", "-")
	if code != 0 {
		t.Fatalf("| c4 id -q -: exit %d: %s", code, stderr)
	}
	if got := strings.TrimSpace(stdout); got != descID {
		t.Errorf("| c4 id -q - gave %s, the description's ID is %s", got, descID)
	}

	stdout, stderr, code = runC4StdinWithEnv(t, bin, env, []byte(stream))
	if code != 0 {
		t.Fatalf("| c4: exit %d: %s", code, stderr)
	}
	bytesID := c4.Identify(strings.NewReader(stream)).String()
	if got := strings.TrimSpace(stdout); got != bytesID {
		t.Errorf("| c4 gave %s, the bytes' ID is %s", got, bytesID)
	}
	if bytesID == descID {
		t.Errorf("the stream's bytes and the description it resolves to must have different IDs")
	}
}

// `c4 patch -` reads the chain from stdin, so `c4 id DIR | c4 patch - | c4`
// composes: the resolved flat text, then its file ID.
func TestPatchReadsStdin(t *testing.T) {
	bin := buildC4(t)
	dir := t.TempDir()
	env := map[string]string{"C4_STORE": filepath.Join(dir, "store"), "HOME": filepath.Join(dir, "home")}
	tree := filepath.Join(dir, "tree")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "d/b.txt": "beta"})
	stream, _, code := runC4WithEnv(t, bin, env, "id", tree)
	if code != 0 {
		t.Fatal("c4 id failed")
	}
	fromStdin, stderr, code := runC4StdinWithEnv(t, bin, env, []byte(stream), "patch", "-")
	if code != 0 {
		t.Fatalf("c4 patch -: exit %d: %s", code, stderr)
	}
	chain := filepath.Join(dir, "chain.c4m")
	if err := os.WriteFile(chain, []byte(stream), 0o644); err != nil {
		t.Fatal(err)
	}
	fromFile, _, code := runC4WithEnv(t, bin, env, "patch", chain)
	if code != 0 || fromFile != fromStdin {
		t.Errorf("patch - differs from patch FILE (exit %d):\n%s\n---\n%s", code, fromStdin, fromFile)
	}
	if isBareID(strings.TrimSpace(fromStdin[strings.LastIndex(strings.TrimRight(fromStdin, "\n"), "\n")+1:])) {
		t.Errorf("patch output ends with a bare ID: %q", fromStdin)
	}
}

// runC4StdinWithEnv runs the binary with stdin and extra environment.
func runC4StdinWithEnv(t *testing.T, bin string, env map[string]string, stdin []byte, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = childEnv(env)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return stdout.String(), stderr.String(), code
}

func isBareID(line string) bool {
	line = strings.TrimSpace(line)
	if len(line) != 90 || !strings.HasPrefix(line, "c4") {
		return false
	}
	_, err := c4.Parse(line)
	return err == nil
}
