package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// buildC4Hooked builds c4 with the test hooks (testhooks.go), so a test can
// act between the scan and the store pass of `c4 id -s` through
// C4_TEST_STORE_PASS.
func buildC4Hooked(t *testing.T) string {
	t.Helper()
	name := "c4"
	if runtime.GOOS == "windows" {
		name = "c4.exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-tags", "c4testhooks", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// storePassEnv is a fresh home and store for one run.
func storePassEnv(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	st := filepath.Join(dir, "store")
	for _, d := range []string{home, st} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return map[string]string{"HOME": home, "USERPROFILE": home, "C4_STORE": st}
}

var entryIDRe = regexp.MustCompile(`c4[1-9A-HJ-NP-Za-km-z]{88}`)

// entryIDs returns every C4 ID on an entry line of a c4m: bare boundary
// lines are left out (the chain form's boundary is 1.1 triage, not this
// test's question).
func entryIDs(text string) []string {
	var ids []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(strings.TrimSpace(line), " ") {
			continue
		}
		ids = append(ids, entryIDRe.FindAllString(line, -1)...)
	}
	return ids
}

// storedRoot returns the ID `stored:` narrates on stderr, or "".
func storedRoot(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "stored: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "stored: "))
		}
	}
	return ""
}

// TestIDStoreFailureIsFatal: a store that cannot take an object ends the
// run before anything is claimed. Through 2026-11-16 each store-pass Put
// failure printed a warning, the root record's failure left nothing to
// journal, and the run exited 0 with a printed description naming C4 IDs
// the store did not hold.
func TestIDStoreFailureIsFatal(t *testing.T) {
	bin := buildC4Hooked(t)
	tree := filepath.Join(t.TempDir(), "p")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "beta"})
	env := storePassEnv(t)
	env["C4_TEST_STORE_PASS"] = "breakstore"
	_, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 1 {
		t.Fatalf("c4 id -s with a store that takes nothing: exit %d, want 1\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "nothing claimed") {
		t.Errorf("stderr does not say nothing was claimed:\n%s", stderr)
	}
	if root := storedRoot(stderr); root != "" {
		t.Errorf("stored: %s reported after a failed store", root)
	}
}

// TestIDStoreEmptyDirectory: a tree with an empty directory and no empty
// file. The empty directory's line names the empty-content ID, its
// listing, so the store must hold that object (approved 2026-11-16, option
// A). Through 2026-11-16 it did not, unless an empty file in the same tree
// happened to store it.
func TestIDStoreEmptyDirectory(t *testing.T) {
	bin := buildC4(t)
	tree := filepath.Join(t.TempDir(), "p")
	writeTree(t, tree, map[string]string{"sub/f.txt": "x"})
	if err := os.MkdirAll(filepath.Join(tree, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := storePassEnv(t)
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
	if code != 0 {
		t.Fatalf("c4 id -s: exit %d: %s", code, stderr)
	}
	empty := c4.Identify(strings.NewReader("")).String()
	ids := append(entryIDs(out), storedRoot(stderr))
	sawEmpty := false
	for _, id := range ids {
		if id == empty {
			sawEmpty = true
		}
		if _, _, code := runC4WithEnv(t, bin, env, "cat", id); code != 0 {
			t.Errorf("c4 id -s printed %s, and c4 cat cannot return it", id)
		}
	}
	if !sawEmpty {
		t.Fatalf("fixture: the description names no empty-content ID:\n%s", out)
	}
}

// assertStoredAsPrinted checks the rule a `c4 id -s` run must keep: every
// C4 ID on an entry line of what it printed, and the description it reports
// as stored, returns through c4 cat, and what it printed is what it stored
// (on this line stored: names the encoded description, the bytes printed).
func assertStoredAsPrinted(t *testing.T, bin string, env map[string]string, out, stderr string) {
	t.Helper()
	root := storedRoot(stderr)
	if root == "" {
		t.Fatalf("no stored: line\nstderr:\n%s", stderr)
	}
	for _, id := range append(entryIDs(out), root) {
		if _, _, code := runC4WithEnv(t, bin, env, "cat", id); code != 0 {
			t.Errorf("c4 id -s printed %s, and c4 cat cannot return it\nstdout:\n%s", id, out)
		}
	}
	if got := c4.Identify(strings.NewReader(out)).String(); got != root {
		t.Errorf("the printed description is %s; stored: %s", got, root)
	}
}

// lineFor returns the line of a c4m naming name, last one wins (a chain's
// refinement restates entries).
func lineFor(text, name string) string {
	var found string
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) >= 5 && f[3] == name {
			found = line
		}
	}
	return found
}

// TestIDStorePassGivesUpAFile: a file that goes, turns unreadable or
// changes between the scan and the store pass is recorded as a scan that
// could not read it records it: a null C4 ID, its directory re-derived
// from the listing that holds it, partial: on stderr and exit 2. Through
// 2026-11-16 the store pass skipped such a file silently, or took a
// changed file's new ID unseen, and the run exited 0 naming content the
// store did not hold.
func TestIDStorePassGivesUpAFile(t *testing.T) {
	bin := buildC4Hooked(t)
	cases := []struct {
		name, hook, why string
		unix            bool
	}{
		{"gone", "rm sub/b.txt", "gone at the store pass", false},
		{"changed", "write sub/b.txt bytes the scan never saw", "changed since the scan", false},
		{"unreadable", "unreadable sub/b.txt", "unreadable at the store pass", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.unix && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
				t.Skip("chmod 000 does not make a file unreadable here")
			}
			tree := filepath.Join(t.TempDir(), "p")
			writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "beta", "sub/c.txt": "gamma"})
			env := storePassEnv(t)
			env["C4_TEST_STORE_PASS"] = c.hook
			out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", tree)
			if code != 2 {
				t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr)
			}
			if want := "partial: sub/b.txt recorded with nulls (" + c.why + ")"; !strings.Contains(stderr, want) {
				t.Errorf("stderr lacks %q:\n%s", want, stderr)
			}
			if line := lineFor(out, "b.txt"); !strings.HasSuffix(strings.TrimSpace(line), " -") {
				t.Errorf("b.txt is printed with a C4 ID: %q", line)
			}
			assertStoredAsPrinted(t, bin, env, out, stderr)
		})
	}
}

// TestIDStoreGuideTrustsAnUnreadableFile: with -c GUIDE a file made
// unreadable after the guide scan must not leave its guide C4 ID in a
// description the store cannot serve. On this line the scan already reads
// it and records it with nulls (measured 2026-11-16); master's trusted the
// guide and exited 0 naming an ID the store lacked. This guards the line.
func TestIDStoreGuideTrustsAnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not make a file unreadable here")
	}
	bin := buildC4(t)
	dir := t.TempDir()
	tree := filepath.Join(dir, "p")
	writeTree(t, tree, map[string]string{"a.txt": "alpha", "sub/b.txt": "beta"})
	old := time.Date(2026, 1, 2, 9, 4, 5, 0, time.UTC)
	for _, p := range []string{"a.txt", "sub/b.txt", "sub", ""} {
		if err := os.Chtimes(filepath.Join(tree, p), old, old); err != nil {
			t.Fatal(err)
		}
	}
	env := storePassEnv(t)
	guideText, stderr, code := runC4WithEnv(t, bin, map[string]string{"HOME": env["HOME"], "USERPROFILE": env["USERPROFILE"]}, "id", tree)
	if code != 0 {
		t.Fatalf("guide scan: exit %d: %s", code, stderr)
	}
	guide := filepath.Join(dir, "guide.c4m")
	if err := os.WriteFile(guide, []byte(guideText), 0o644); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(tree, "sub", "b.txt")
	if err := os.Chmod(b, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(b, 0o644)
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-s", "-c", guide, tree)
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "partial: sub/b.txt recorded with nulls (") {
		t.Errorf("stderr does not name sub/b.txt as partial:\n%s", stderr)
	}
	assertStoredAsPrinted(t, bin, env, out, stderr)
}

// TestIDStorePassUnfoldsARange: a folded range one of whose members the
// store pass cannot take is unfolded into its members, that member null,
// as a scan that could not read it records them: a member with no C4 ID
// blocks the fold, and frame IDs are never dropped (ruled 2026-11-16).
// Through 2026-11-16 the range line stayed, naming an ID list the store
// did not hold, and the run exited 0.
func TestIDStorePassUnfoldsARange(t *testing.T) {
	bin := buildC4Hooked(t)
	tree := filepath.Join(t.TempDir(), "p")
	writeTree(t, tree, map[string]string{
		"x.txt":          "x",
		"seq/s.0001.exr": "frame one",
		"seq/s.0002.exr": "frame two",
		"seq/s.0003.exr": "frame three",
		"seq/s.0004.exr": "frame four",
	})
	env := storePassEnv(t)
	plain, stderr, code := runC4WithEnv(t, bin, env, "id", "-S", tree)
	if code != 0 || !strings.Contains(plain, "s.[0001-0004].exr") {
		t.Fatalf("fixture: the scan does not fold the frames (exit %d):\n%s%s", code, plain, stderr)
	}
	env["C4_TEST_STORE_PASS"] = "rm seq/s.0002.exr"
	out, stderr, code := runC4WithEnv(t, bin, env, "id", "-S", "-s", tree)
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr)
	}
	if strings.Contains(out, "s.[") {
		t.Errorf("the range is still folded:\n%s", out)
	}
	for _, name := range []string{"s.0001.exr", "s.0003.exr", "s.0004.exr"} {
		if line := lineFor(out, name); entryIDRe.FindString(line) == "" {
			t.Errorf("%s is not printed with its C4 ID: %q", name, line)
		}
	}
	if line := lineFor(out, "s.0002.exr"); line == "" || entryIDRe.FindString(line) != "" {
		t.Errorf("s.0002.exr is not printed with a null C4 ID: %q", line)
	}
	if !strings.Contains(stderr, "partial: seq/s.0002.exr recorded with nulls (gone at the store pass)") {
		t.Errorf("stderr does not name seq/s.0002.exr as partial:\n%s", stderr)
	}
	assertStoredAsPrinted(t, bin, env, out, stderr)
}
