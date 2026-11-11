package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guard in TestMain must hold: every c4 the suite starts sees a home
// that is not the runner's own, on Unix (HOME) and on Windows (USERPROFILE),
// and a storing command run with the inherited environment leaves the
// runner's real store as it was. Without the guard a CLI test stored into
// the real ~/.c4/store and, on Windows, set up a store in the real profile
// that a later no-store test then found.
func TestSuiteNeverTouchesTheRealHome(t *testing.T) {
	if realHome == "" {
		t.Skip("no home directory on this runner")
	}
	if home, _ := os.UserHomeDir(); home == realHome {
		t.Fatalf("the suite's home is the runner's real home %s: TestMain's guard is not in effect", realHome)
	}
	for _, k := range []string{"HOME", "USERPROFILE"} {
		if v := os.Getenv(k); v == realHome {
			t.Fatalf("%s is the runner's real home %s", k, realHome)
		}
	}

	count := func() int {
		n := 0
		filepath.Walk(filepath.Join(realHome, ".c4", "store"), func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				n++
			}
			return nil
		})
		return n
	}
	before := count()

	bin := buildC4(t)
	tree := t.TempDir()
	if err := os.WriteFile(filepath.Join(tree, "guard.txt"), []byte("the suite must not store this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Storing commands, run with the inherited (guarded) environment only.
	runC4(t, bin, "id", "-s", tree)
	runC4(t, bin, tree)

	if after := count(); after != before {
		t.Fatalf("the runner's real store went from %d to %d objects: a c4 started by the suite stored into it", before, after)
	}
}

// A test's own home is its home on every platform: childEnv carries HOME
// into USERPROFILE (what os.UserHomeDir reads on Windows) unless the test
// set USERPROFILE itself.
func TestChildEnvGivesAHomeToBothPlatforms(t *testing.T) {
	last := func(env []string, k string) string {
		v := ""
		for _, kv := range env {
			if strings.HasPrefix(kv, k+"=") {
				v = strings.TrimPrefix(kv, k+"=")
			}
		}
		return v
	}
	env := childEnv(map[string]string{"HOME": "/tmp/own-home"})
	if got := last(env, "USERPROFILE"); got != "/tmp/own-home" {
		t.Errorf("USERPROFILE = %q, want the test's own HOME", got)
	}
	env = childEnv(map[string]string{"HOME": "/tmp/h", "USERPROFILE": "/tmp/p"})
	if got := last(env, "USERPROFILE"); got != "/tmp/p" {
		t.Errorf("USERPROFILE = %q, want the test's own USERPROFILE", got)
	}
}
