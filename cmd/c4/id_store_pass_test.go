package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

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
