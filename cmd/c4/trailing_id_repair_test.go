package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4/c4m"
	"github.com/Avalanche-io/c4/scan"
)

// trailingIDRepair is the repair README.md and CHANGELOG.md give for a chain
// grown by `c4 diff … >> FILE.c4m` on 1.0.26 or earlier, which may end with
// a bare C4 ID that c4 refuses ("a bare C4 ID ends the file"). The test runs
// this exact text, and checks that both documents carry it unchanged.
const trailingIDRepair = `if tail -n 1 FILE.c4m | grep -qE '^c4[1-9A-HJ-NP-Za-km-z]{88}$'; then
  sed '$d' FILE.c4m > FILE.c4m.tmp && mv FILE.c4m.tmp FILE.c4m
fi
`

var bareIDLine = regexp.MustCompile(`^c4[1-9A-HJ-NP-Za-km-z]{88}$`)

// repairInGo is the same repair as trailingIDRepair: delete the last line
// only when it is exactly a bare C4 ID.
func repairInGo(data []byte) []byte {
	text := strings.TrimSuffix(string(data), "\n")
	i := strings.LastIndex(text, "\n")
	if !bareIDLine.MatchString(text[i+1:]) {
		return data
	}
	return []byte(text[:i+1])
}

// TestTrailingIDRepair: the documented repair removes a trailing bare C4 ID
// and nothing else. A decoder reads a line of exactly 90 characters as a
// bare C4 ID and a longer one as a range's ID list (c4m/decoder.go
// isInlineIDList); the shortest list, two members, is 180 characters. Five
// files: the 1.0.24 chain (a real old output) is repaired and every verb then
// reads it; a `c4 id` output, a chain grown by this line's `c4 diff >>`, and
// files ending in a three-member and a two-member ID list are left byte for
// byte. `c4 id FILE.c4m` flattens the chain without the repair.
func TestTrailingIDRepair(t *testing.T) {
	for _, doc := range []string{"README.md", "CHANGELOG.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", doc))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), trailingIDRepair) {
			t.Errorf("%s does not carry the tested repair text", doc)
		}
	}

	bin := buildC4(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": filepath.Join(dir, "store")}
	run := func(args ...string) (string, string, int) {
		t.Helper()
		return runC4WithEnv(t, bin, env, args...)
	}
	must := func(args ...string) string {
		t.Helper()
		out, stderr, code := run(args...)
		if code != 0 {
			t.Fatalf("c4 %v: exit %d: %s", args, code, stderr)
		}
		return out
	}

	// The five files.
	files := map[string][]byte{}
	legacy, err := os.ReadFile(filepath.Join("..", "..", "c4m", "testdata", "legacy", "diff-chain-1.0.24.c4m"))
	if err != nil {
		t.Fatal(err)
	}
	files["legacy chain"] = legacy

	tree := filepath.Join(dir, "p")
	writeTree(t, tree, map[string]string{"a.txt": "one", "sub/b.txt": "two"})
	old := time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)
	for _, p := range []string{"a.txt", "sub/b.txt", "sub", ""} {
		if err := os.Chtimes(filepath.Join(tree, filepath.FromSlash(p)), old, old); err != nil {
			t.Fatal(err)
		}
	}
	files["c4 id output"] = []byte(must("id", tree))

	chainFile := filepath.Join(dir, "grown.c4m")
	if err := os.WriteFile(chainFile, files["c4 id output"], 0o644); err != nil {
		t.Fatal(err)
	}
	writeTree(t, tree, map[string]string{"sub/b.txt": "two, changed"})
	diff, stderr, code := run("diff", chainFile, tree)
	if code > 1 {
		t.Fatalf("c4 diff: exit %d: %s", code, stderr)
	}
	files["c4 diff >> chain"] = append(append([]byte{}, files["c4 id output"]...), diff...)

	frames := func(n int) string {
		d := filepath.Join(dir, "frames", string(rune('0'+n)))
		fs := map[string]string{}
		for i := 1; i <= n; i++ {
			fs["f.000"+string(rune('0'+i))+".exr"] = "frame " + string(rune('0'+i))
		}
		writeTree(t, d, fs)
		return d
	}
	files["3-member ID list"] = []byte(must("id", "-S", frames(3)))
	// The CLI folds three members or more; the library folds two, the
	// shortest list a description can end with.
	m, err := scan.NewGeneratorWithOptions(scan.WithMode(scan.ModeFull)).GenerateFromPath(frames(2))
	if err != nil {
		t.Fatal(err)
	}
	var two bytes.Buffer
	if err := c4m.NewEncoder(&two).Encode(c4m.NewSequenceDetector(2).DetectSequences(m)); err != nil {
		t.Fatal(err)
	}
	files["2-member ID list"] = two.Bytes()

	for name, data := range files {
		last := strings.TrimSuffix(string(data), "\n")
		last = last[strings.LastIndex(last, "\n")+1:]
		switch name {
		case "legacy chain":
			if !bareIDLine.MatchString(last) {
				t.Fatalf("fixture: the legacy chain does not end with a bare C4 ID: %q", last)
			}
		case "3-member ID list", "2-member ID list":
			want := 270
			if name == "2-member ID list" {
				want = 180
			}
			if len(last) != want {
				t.Fatalf("fixture: %s ends with a %d-character line, want its %d-character list", name, len(last), want)
			}
		}
	}

	// Each file repaired by the exact text (where a shell is available)
	// and by the Go equivalent; the two must agree.
	sh, shErr := exec.LookPath("sh")
	for name, data := range files {
		work := filepath.Join(dir, "repair", strings.ReplaceAll(name, " ", "-"))
		if err := os.MkdirAll(work, 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(work, "FILE.c4m")
		if err := os.WriteFile(file, data, 0o644); err != nil {
			t.Fatal(err)
		}
		want := repairInGo(data)
		if shErr == nil {
			cmd := exec.Command(sh, "-c", trailingIDRepair)
			cmd.Dir = work
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: the repair text failed: %v\n%s", name, err, out)
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s: the shell text and the Go repair disagree", name)
			}
		} else {
			t.Logf("no sh on PATH: the repair text itself is not run here")
			if err := os.WriteFile(file, want, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		repaired, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if name != "legacy chain" {
			if !bytes.Equal(repaired, data) {
				t.Errorf("%s: the repair changed a healthy file", name)
			}
			continue
		}
		if bytes.Equal(repaired, data) || !bytes.HasPrefix(data, repaired) || bytes.Count(data, []byte("\n"))-bytes.Count(repaired, []byte("\n")) != 1 {
			t.Fatalf("legacy chain: the repair did not remove exactly its last line")
		}
		// Before the repair the readers refuse it; after, every verb reads it.
		if _, stderr, code := run("paths", legacyCopy(t, dir, data)); code == 0 || !strings.Contains(stderr, "a bare C4 ID ends the file") {
			t.Errorf("c4 paths on the unrepaired chain: exit %d, %q; want it refused", code, stderr)
		}
		for _, args := range [][]string{{"id", file}, {"diff", file, tree}, {"merge", file, file}, {"paths", file}, {"log", file}, {"patch", file}} {
			if _, stderr, code := run(args...); code > 1 || (code == 1 && args[0] != "diff") || strings.Contains(stderr, "Error") {
				t.Errorf("c4 %s on the repaired chain: exit %d: %s", args[0], code, stderr)
			}
		}
		if log := must("log", file); strings.Count(strings.TrimSpace(log), "\n")+1 != 2 {
			t.Errorf("c4 log of the repaired chain shows %q; want 2 layers", log)
		}
		// c4 id flattens the chain without the repair: one plain c4m of
		// the final state, which every verb reads, equal to the repaired
		// chain's.
		flat := must("id", legacyCopy(t, dir, data))
		if flat != must("patch", file) {
			t.Errorf("c4 id of the unrepaired chain differs from the repaired chain's final state")
		}
		flatFile := filepath.Join(dir, "fixed.c4m")
		if err := os.WriteFile(flatFile, []byte(flat), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"id", flatFile}, {"merge", flatFile, flatFile}, {"paths", flatFile}, {"log", flatFile}} {
			if _, stderr, code := run(args...); code != 0 {
				t.Errorf("c4 %s on c4 id's output: exit %d: %s", args[0], code, stderr)
			}
		}
		if log := must("log", flatFile); strings.Count(strings.TrimSpace(log), "\n") != 0 {
			t.Errorf("c4 id's output is not one plain c4m: c4 log shows %q", log)
		}
	}
}

// legacyCopy writes the unrepaired chain to a file of its own.
func legacyCopy(t *testing.T, dir string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, "legacy-unrepaired.c4m")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
